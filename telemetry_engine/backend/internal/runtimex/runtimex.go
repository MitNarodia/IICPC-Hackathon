// Package runtimex holds the tiny bits of process plumbing every Track 3
// service repeats: a context that cancels on SIGINT/SIGTERM, and a Prometheus
// metrics/health endpoint. Keeping it here means each cmd/*/main.go stays a
// short, readable wiring file.
package runtimex

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// SignalContext returns a context cancelled on the first SIGINT/SIGTERM so the
// service can drain and exit cleanly (the orchestrator sends SIGTERM on stop).
func SignalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		cancel()
	}()
	return ctx, cancel
}

// ServeMetrics starts a background HTTP server exposing Prometheus /metrics and
// a /healthz liveness probe. Returns the server so the caller can shut it down.
func ServeMetrics(addr string, log *logpkg.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Info("metrics listening", logpkg.F("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("metrics server", logpkg.F("err", err.Error()))
		}
	}()
	return srv
}

// Shutdown gracefully stops an HTTP server with a bounded deadline.
func Shutdown(srv *http.Server) {
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
