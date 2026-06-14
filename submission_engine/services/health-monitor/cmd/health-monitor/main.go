package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/iicpc/track1/submission-engine/pkg/events"
	kafkabus "github.com/iicpc/track1/submission-engine/pkg/events/kafka"
	"github.com/iicpc/track1/submission-engine/pkg/models"
	"github.com/iicpc/track1/submission-engine/pkg/statemachine"
	pgstore "github.com/iicpc/track1/submission-engine/pkg/store/postgres"
	track3reporter "github.com/iicpc/track1/submission-engine/pkg/telemetry/track3"
	"github.com/iicpc/track1/submission-engine/services/health-monitor/internal/config"
	"github.com/iicpc/track1/submission-engine/services/health-monitor/internal/service"
)

var activeMonitors sync.Map

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	pg, err := pgstore.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pg.Close()

	brokers := strings.Split(cfg.KafkaBrokers, ",")
	producer := kafkabus.NewProducer(brokers)
	defer producer.Close()

	consumer := kafkabus.NewConsumer(kafkabus.ConsumerConfig{
		Brokers: brokers,
		GroupID: "health-monitor",
		Topics:  []string{string(events.DeploymentReady), string(events.TeardownRequested)},
	})
	defer consumer.Close()

	consumer.Handle(events.DeploymentReady, func(ctx context.Context, env events.Envelope) error {
		log.Printf("health-monitor: deployment ready for %s — monitoring started", env.SubmissionID)

		deployment, err := pg.GetDeployment(ctx, env.SubmissionID)
		if err != nil || deployment == nil {
			log.Printf("health-monitor: missing deployment for %s", env.SubmissionID)
			return nil
		}

		endpoint, err := pg.GetEndpoint(ctx, env.SubmissionID)
		if err != nil || endpoint == nil {
			log.Printf("health-monitor: missing endpoint for %s", env.SubmissionID)
			return nil
		}

		if cancelIntf, ok := activeMonitors.Load(env.SubmissionID); ok {
			cancelIntf.(context.CancelFunc)()
		}

		monCtx, monCancel := context.WithCancel(context.Background())
		activeMonitors.Store(env.SubmissionID, monCancel)

		// Report sandbox start to Track 3 telemetry engine.
		reporter := track3reporter.NewReporter()
		_ = reporter.ReportSandbox(ctx, env.SubmissionID, track3reporter.SandboxSample{
			SampleTS:     time.Now().UnixNano(),
			Health:       "starting",
			RestartCount: 0,
		})

		go runMonitor(monCtx, env.SubmissionID, deployment.ID, endpoint, pg, producer)

		return nil
	})

	consumer.Handle(events.TeardownRequested, func(ctx context.Context, env events.Envelope) error {
		if cancelIntf, ok := activeMonitors.Load(env.SubmissionID); ok {
			log.Printf("health-monitor: teardown requested for %s - stopping monitor", env.SubmissionID)
			cancelIntf.(context.CancelFunc)()
			activeMonitors.Delete(env.SubmissionID)
		}
		return nil
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	srv := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: mux}
	go func() { _ = srv.ListenAndServe() }()

	log.Printf("health-monitor consuming (group=health-monitor)")
	if err := consumer.Run(ctx); err != nil {
		log.Printf("health-monitor: consumer exited: %v", err)
	}
	srv.Shutdown(context.Background())
}

func runMonitor(ctx context.Context, submissionID, deploymentID string, endpoint *models.Endpoint, pg *pgstore.Store, producer *kafkabus.Producer) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	prober := service.Prober{Timeout: 2 * time.Second}
	classifier := service.NewClassifier(3, 2, 500)

	log.Printf("health-monitor: monitor loop started for %s", submissionID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("health-monitor: monitor loop stopped for %s", submissionID)
			return
		case <-ticker.C:
			var result service.ProbeResult
			if endpoint.Protocol == models.ProtocolHTTP || endpoint.Protocol == models.ProtocolWS {
				url := endpoint.InternalURL
				if strings.HasPrefix(url, "ws://") {
					url = "http://" + strings.TrimPrefix(url, "ws://")
				}
				result = prober.ProbeHTTP(ctx, url)
			} else {
				addr := strings.TrimPrefix(endpoint.InternalURL, "tcp://")
				addr = strings.TrimPrefix(addr, "grpc://")
				result = prober.ProbeTCP(ctx, addr)
			}

			sample := result.ToSample(submissionID, deploymentID)
			if err := pg.AddHealthSample(ctx, sample); err != nil {
				log.Printf("health-monitor: failed to save health sample for %s: %v", submissionID, err)
			} else {
				log.Printf("health-monitor: sample persisted for %s (healthy=%v, latency=%vms)", submissionID, sample.Healthy, sample.LatencyMS)
			}

			newState := classifier.Observe(result)

			if newState == service.HealthReady {
				sub, _ := pg.GetSubmission(ctx, submissionID)
				var transitionEvent statemachine.Event
				if sub != nil && statemachine.CanTransition(sub.Status, statemachine.EventHealthReady) {
					transitionEvent = statemachine.EventHealthReady
				} else if sub != nil && statemachine.CanTransition(sub.Status, statemachine.EventHealthRecovered) {
					transitionEvent = statemachine.EventHealthRecovered
				}
				
				if transitionEvent != "" {
					_, err := pg.TransitionSubmission(ctx, submissionID, transitionEvent, "health-monitor", nil)
					if err == nil {
						log.Printf("health-monitor: state transition %s for %s", transitionEvent, submissionID)
						evt, _ := events.NewEnvelope(events.HealthReady, submissionID, "health-monitor", events.HealthReadyData{
							InternalURL: endpoint.InternalURL,
						})
						_ = producer.Publish(ctx, evt)
					}
				}
			} else if newState == service.HealthDegraded {
				sub, _ := pg.GetSubmission(ctx, submissionID)
				if sub != nil && statemachine.CanTransition(sub.Status, statemachine.EventHealthDegraded) {
					_, err := pg.TransitionSubmission(ctx, submissionID, statemachine.EventHealthDegraded, "health-monitor", nil)
					if err == nil {
						log.Printf("health-monitor: state transition to DEGRADED for %s", submissionID)
						evt, _ := events.NewEnvelope(events.HealthDegraded, submissionID, "health-monitor", events.HealthDegradedData{
							Reason:        result.Error,
							LastLatencyMS: result.LatencyMS,
						})
						_ = producer.Publish(ctx, evt)
					}
				}
			}
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
