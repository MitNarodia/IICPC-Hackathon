// Command telemetry-simulator generates realistic Track 3 telemetry for demos
// and load tests. It spins up N synthetic "submissions", each with its own
// latency/throughput/error profile, and streams order events plus periodic bot
// and sandbox metrics to the ingestion service over HTTP. The result is a live,
// differentiated leaderboard with no real Track 1/Track 2 deployment needed.
//
//	go run ./cmd/telemetry-simulator -url http://localhost:8081 -subs 8 -eps 500 -duration 5m
//
// It is also a crude load generator: total event rate is subs × eps.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/iicpc/track3/telemetry-engine/pkg/events"
)

type profile struct {
	name    string
	baseUS  int64   // typical ack latency
	jitter  int64   // latency standard deviation
	errRate float64 // fraction of acks rejected
	burst   float64 // TPS variance factor (0 steady … 1 bursty)
}

func makeProfiles(n int) []profile {
	// A spread of archetypes so the board is interesting; cycled if n is large.
	base := []profile{
		{"falcon", 120, 40, 0.001, 0.05},   // fast + steady + correct → top
		{"hawk", 200, 80, 0.002, 0.10},
		{"eagle", 350, 120, 0.005, 0.15},
		{"owl", 600, 200, 0.010, 0.25},
		{"raven", 900, 400, 0.020, 0.40},    // slower, burstier
		{"sparrow", 1500, 700, 0.050, 0.60}, // struggling
	}
	out := make([]profile, n)
	for i := 0; i < n; i++ {
		out[i] = base[i%len(base)]
	}
	return out
}

func main() {
	url := flag.String("url", "http://localhost:8081", "ingestion service base URL")
	runID := flag.String("run", "run-demo", "benchmark run id")
	subs := flag.Int("subs", 6, "number of synthetic submissions")
	dur := flag.Duration("duration", 60*time.Second, "how long to run")
	eps := flag.Int("eps", 200, "order events per second per submission")
	flag.Parse()

	profiles := makeProfiles(*subs)
	ctx, cancel := context.WithTimeout(context.Background(), *dur)
	defer cancel()

	client := &http.Client{Timeout: 5 * time.Second}
	fmt.Printf("simulating run=%s subs=%d eps=%d total=%d ev/s for %s -> %s\n",
		*runID, *subs, *eps, (*subs)*(*eps), *dur, *url)

	var wg sync.WaitGroup
	for i := 0; i < *subs; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			subID := fmt.Sprintf("sub-%02d-%s", idx, profiles[idx].name)
			runSubmission(ctx, client, *url, *runID, subID, profiles[idx], *eps)
		}(i)
	}
	wg.Wait()
	fmt.Println("done")
}

// runSubmission emits one submission's stream until ctx ends.
func runSubmission(ctx context.Context, client *http.Client, url, runID, subID string, p profile, eps int) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano() ^ int64(len(subID))))
	const ticksPerSec = 5
	ticker := time.NewTicker(time.Second / ticksPerSec)
	defer ticker.Stop()

	var seq uint64
	var orderID uint64
	windowTxns, windowErrs := uint64(0), uint64(0)
	lastBotMetrics := time.Now()
	lastSandbox := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Bursty profiles vary the per-tick volume around the mean.
			factor := 1.0 + p.burst*(rng.Float64()*2-1)
			count := int(float64(eps) / ticksPerSec * factor)
			if count < 1 {
				count = 1
			}
			batch := make([]*events.Envelope, 0, count*2)
			now := time.Now().UnixNano()
			for i := 0; i < count; i++ {
				orderID++
				seq++
				side := events.SideBuy
				if rng.Intn(2) == 0 {
					side = events.SideSell
				}
				price := 100.0 + float64(rng.Intn(20)-10)*0.01
				submitted := events.OrderSubmitted{
					BotID: uint64(rng.Intn(64)), OrderID: orderID, Side: side,
					Kind: events.KindLimit, Price: price, Quantity: uint32(1 + rng.Intn(10)),
					SendTS: now,
				}
				if env, err := events.NewEnvelope(runID, subID, "simulator", seq, events.TypeOrderSubmitted, submitted); err == nil {
					batch = append(batch, env)
				}

				// Latency: gaussian around base, with a rare heavy tail.
				lat := p.baseUS + int64(rng.NormFloat64()*float64(p.jitter))
				if rng.Float64() < 0.01 {
					lat *= 8 // tail spike (coordinated-omission realism)
				}
				if lat < 1 {
					lat = 1
				}
				accepted := rng.Float64() >= p.errRate
				seq++
				ack := events.OrderAck{
					BotID: submitted.BotID, OrderID: orderID, Accepted: accepted,
					RecvTS: time.Now().UnixNano(), AckLatencyUS: lat,
				}
				if !accepted {
					ack.RejectReason = "synthetic_reject"
					windowErrs++
				} else {
					windowTxns++
				}
				if env, err := events.NewEnvelope(runID, subID, "simulator", seq, events.TypeOrderAck, ack); err == nil {
					batch = append(batch, env)
				}
			}
			postBatch(ctx, client, url, batch)

			// Periodic BotMetrics (Track 2 AggregateView shape) ~ every second.
			if time.Since(lastBotMetrics) >= time.Second {
				emitBotMetrics(ctx, client, url, runID, subID, &seq, p, windowTxns, windowErrs, lastBotMetrics)
				windowTxns, windowErrs = 0, 0
				lastBotMetrics = time.Now()
			}
			// Periodic SandboxMetrics (Track 1 shape) ~ every 2 seconds.
			if time.Since(lastSandbox) >= 2*time.Second {
				emitSandbox(ctx, client, url, runID, subID, &seq, p, rng)
				lastSandbox = time.Now()
			}
		}
	}
}

func emitBotMetrics(ctx context.Context, client *http.Client, url, runID, subID string, seq *uint64, p profile, txns, errs uint64, since time.Time) {
	*seq++
	secs := time.Since(since).Seconds()
	bm := events.BotMetrics{
		Transactions: txns, Errors: errs, Timeouts: 0,
		WindowSeconds: secs,
		P50US:         p.baseUS, P90US: p.baseUS + 2*p.jitter, P99US: p.baseUS + 5*p.jitter,
		MeanUS:        float64(p.baseUS),
		WindowStartTS: since.UnixNano(), WindowEndTS: time.Now().UnixNano(),
	}
	if env, err := events.NewEnvelope(runID, subID, "simulator", *seq, events.TypeBotMetrics, bm); err == nil {
		postBatch(ctx, client, url, []*events.Envelope{env})
	}
}

func emitSandbox(ctx context.Context, client *http.Client, url, runID, subID string, seq *uint64, p profile, rng *rand.Rand) {
	*seq++
	health := "READY"
	if p.errRate > 0.03 && rng.Float64() < 0.3 {
		health = "DEGRADED"
	}
	sm := events.SandboxMetrics{
		PodName: subID + "-pod", Namespace: "track1-sandbox",
		CPUMillicores: 200 + rng.Float64()*800, CPULimitMillicores: 2000,
		MemoryBytes: uint64(128<<20) + uint64(rng.Intn(256))<<20, MemoryLimitBytes: 512 << 20,
		OpenFDs: uint32(50 + rng.Intn(200)), ActiveConnections: uint32(rng.Intn(64)),
		Health: health, OOMKilled: false, RestartCount: 0,
		SampleTS: time.Now().UnixNano(),
	}
	if env, err := events.NewEnvelope(runID, subID, "simulator", *seq, events.TypeSandboxMetrics, sm); err == nil {
		postBatch(ctx, client, url, []*events.Envelope{env})
	}
}

func postBatch(ctx context.Context, client *http.Client, url string, batch []*events.Envelope) {
	if len(batch) == 0 {
		return
	}
	body, err := json.Marshal(batch)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/v1/events", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return // best-effort; the simulator tolerates transient ingest hiccups
	}
	_ = resp.Body.Close()
}
