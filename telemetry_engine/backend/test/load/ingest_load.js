// k6 load test for the Track 3 ingestion service.
//
//   k6 run backend/test/load/ingest_load.js
//   INGEST_URL=http://localhost:8081 SUBS=12 BATCH=100 k6 run backend/test/load/ingest_load.js
//
// It ramps virtual users that each POST batches of order_ack envelopes (the
// hot path that drives latency/throughput scoring). Thresholds fail the run if
// ingestion p99 latency or the error rate regress — wire this into CI as a perf
// gate.
import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const INGEST = __ENV.INGEST_URL || 'http://localhost:8081';
const RUN = __ENV.RUN_ID || 'run-load';
const SUBS = parseInt(__ENV.SUBS || '8', 10);
const BATCH = parseInt(__ENV.BATCH || '50', 10);

export const options = {
  scenarios: {
    ramp: {
      executor: 'ramping-vus',
      startVUs: 5,
      stages: [
        { duration: '30s', target: 50 },
        { duration: '1m', target: 100 },
        { duration: '30s', target: 0 },
      ],
    },
  },
  thresholds: {
    // Ingestion is a thin publish-to-Redpanda hop; it should stay fast.
    http_req_duration: ['p(95)<150', 'p(99)<300'],
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
  },
};

const eventsAccepted = new Counter('track3_events_accepted');

export default function () {
  const sub = `sub-${(__VU % SUBS).toString().padStart(2, '0')}`;
  const nowNs = Date.now() * 1e6;
  const batch = [];
  for (let i = 0; i < BATCH; i++) {
    const lat = Math.max(1, Math.floor(80 + Math.random() * 400));
    batch.push({
      run_id: RUN,
      submission_id: sub,
      seq: i,
      emit_ts: nowNs,
      source: 'k6',
      type: 'order_ack',
      payload: {
        bot_id: __VU,
        order_id: i,
        accepted: Math.random() > 0.01,
        recv_ts: nowNs,
        ack_latency_us: lat,
      },
    });
  }

  const res = http.post(`${INGEST}/v1/events`, JSON.stringify(batch), {
    headers: { 'Content-Type': 'application/json' },
  });

  const ok = check(res, {
    'status is 202': (r) => r.status === 202,
    'body reports accepted': (r) => {
      try {
        return JSON.parse(r.body).accepted >= 0;
      } catch (_) {
        return false;
      }
    },
  });
  if (ok) eventsAccepted.add(BATCH);
}
