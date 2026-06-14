// ContestantDetail is the deep-dive panel shown when a row is selected. It pulls
// the REST detail (rolling stats, validation findings, window history) and
// refreshes on an interval so the charts track the live run. The headline entry
// comes from the live WS board so the numbers match the table exactly.

import { useEffect, useState } from 'react';
import { fetchContestant } from '../api/client';
import type { ContestantDetail as Detail, LeaderboardEntry } from '../types';
import { fmtFloat, fmtMicros, fmtPct, fmtTPS, scoreColor } from '../format';
import { MetricBadge } from './MetricBadge';
import { Sparkline } from './Sparkline';

interface Props {
  runID: string;
  entry: LeaderboardEntry;
  onClose: () => void;
}

export function ContestantDetail({ runID, entry, onClose }: Props) {
  const [detail, setDetail] = useState<Detail | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    const load = () => {
      fetchContestant(runID, entry.submission_id)
        .then((d) => active && (setDetail(d), setErr(null)))
        .catch((e) => active && setErr(String(e)));
    };
    load();
    const id = window.setInterval(load, 2000);
    return () => {
      active = false;
      window.clearInterval(id);
    };
  }, [runID, entry.submission_id]);

  const tpsSeries = detail?.history.map((w) => w.tps) ?? [];
  const p99Series = detail?.history.map((w) => w.p99_us) ?? [];
  const v = detail?.validation;

  return (
    <aside className="detail">
      <header className="detail-head">
        <div>
          <h2>{entry.display_name || entry.submission_id.slice(0, 8)}</h2>
          <span className="detail-sub">{entry.submission_id}</span>
        </div>
        <button className="close" onClick={onClose} aria-label="close">×</button>
      </header>

      <div className="detail-scores">
        <MetricBadge label="Composite" value={fmtFloat(entry.composite, 1)} color={scoreColor(entry.composite)} />
        <MetricBadge label="Latency" value={fmtFloat(entry.latency_score, 0)} />
        <MetricBadge label="Throughput" value={fmtFloat(entry.throughput_score, 0)} />
        <MetricBadge label="Correctness" value={fmtFloat(entry.correctness_score, 0)} />
        <MetricBadge label="Stability" value={fmtFloat(entry.stability_score, 0)} />
      </div>

      <section className="detail-chart">
        <h3>Throughput (TPS)</h3>
        <Sparkline values={tpsSeries} color="var(--accent)" />
        <div className="chart-foot">
          <span>now {fmtTPS(entry.tps)}</span>
          {detail?.rolling && <span>peak {fmtTPS(detail.rolling.peak_tps)}</span>}
          {detail?.rolling && <span>CoV {fmtFloat(detail.rolling.tps_cov, 3)}</span>}
        </div>
      </section>

      <section className="detail-chart">
        <h3>Tail latency (p99)</h3>
        <Sparkline values={p99Series} color="var(--warn)" />
        <div className="chart-foot">
          <span>p50 {fmtMicros(entry.p50_us)}</span>
          <span>p99 {fmtMicros(entry.p99_us)}</span>
          <span>err {fmtPct(entry.error_rate)}</span>
        </div>
      </section>

      <section className="detail-validation">
        <h3>
          Correctness
          {v && <span className={`verdict ${v.violations === 0 ? 'clean' : 'flagged'}`}>
            {v.violations === 0 ? 'clean' : `${v.violations} violations`}
          </span>}
        </h3>
        {v && v.orders_checked > 0 && (
          <p className="checked">
            {v.orders_checked.toLocaleString()} orders · {v.trades_checked.toLocaleString()} trades checked
          </p>
        )}
        {v && Object.keys(v.violations_by_rule || {}).length > 0 && (
          <ul className="rule-list">
            {Object.entries(v.violations_by_rule).map(([rule, count]) => (
              <li key={rule}>
                <code>{rule}</code>
                <span className="count">{count}</span>
              </li>
            ))}
          </ul>
        )}
        {v && v.recent_findings && v.recent_findings.length > 0 && (
          <ul className="finding-list">
            {v.recent_findings.slice(0, 6).map((f, i) => (
              <li key={i}>
                <span className="finding-rule">{f.rule}</span>
                <span className="finding-msg">{f.message}</span>
              </li>
            ))}
          </ul>
        )}
        {v && v.violations === 0 && <p className="all-good">No correctness violations detected.</p>}
      </section>

      {err && <p className="error">detail unavailable: {err}</p>}
    </aside>
  );
}
