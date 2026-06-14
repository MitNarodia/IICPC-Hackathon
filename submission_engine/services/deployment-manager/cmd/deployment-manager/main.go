package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/iicpc/track1/submission-engine/pkg/bootstrap"
	"github.com/iicpc/track1/submission-engine/pkg/events"
	kafkabus "github.com/iicpc/track1/submission-engine/pkg/events/kafka"
	"github.com/iicpc/track1/submission-engine/pkg/models"
	dockerorch "github.com/iicpc/track1/submission-engine/pkg/orchestrator/docker"
	"github.com/iicpc/track1/submission-engine/pkg/security"
	pgstore "github.com/iicpc/track1/submission-engine/pkg/store/postgres"
	"github.com/iicpc/track1/submission-engine/pkg/store/redisreg"
	"github.com/iicpc/track1/submission-engine/pkg/statemachine"
	dmsvc "github.com/iicpc/track1/submission-engine/services/deployment-manager/internal/service"
	"github.com/iicpc/track1/submission-engine/services/deployment-manager/internal/config"
)

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

	reg, err := redisreg.New(ctx, cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer reg.Close()

	brokers := strings.Split(cfg.KafkaBrokers, ",")
	producer := kafkabus.NewProducer(brokers)
	defer producer.Close()

	// NEW: Wire docker orchestrator
	orch, err := bootstrap.WireOrchestrator(ctx)
	if err != nil {
		log.Fatalf("orchestrator: %v", err)
	}
	defer orch.(*dockerorch.Orchestrator).Close()

	// Compose a repo that satisfies dmsvc.Repository (DeploymentRepository + Registry).
	repo := &combinedRepo{store: pg, registry: reg}

	mgr := dmsvc.Manager{
		Repo:          repo,
		Publisher:     producer,
		Namespace:     cfg.K8SSandboxNamespace,
		RuntimeClass:  cfg.RuntimeClass,
		DefaultCPU:    cfg.DefaultCPUCores,
		DefaultMemory: cfg.DefaultMemMB,
	}

	consumer := kafkabus.NewConsumer(kafkabus.ConsumerConfig{
		Brokers: brokers,
		GroupID: "deployment-manager",
		Topics:  []string{string(events.BuildSucceeded), string(events.TeardownRequested)},
	})
	defer consumer.Close()

	consumer.Handle(events.BuildSucceeded, func(ctx context.Context, env events.Envelope) error {
		log.Printf("deployment-manager: processing %s for %s", env.Type, env.SubmissionID)

		var data events.BuildSucceededData
		_ = env.DecodeData(&data)

		sub, err := pg.GetSubmission(ctx, env.SubmissionID)
		if err != nil {
			log.Printf("deployment-manager: get submission: %v", err)
			return nil
		}

		// Transition: SCANNED → DEPLOYING.
		_, err = pg.TransitionSubmission(ctx, sub.ID, statemachine.EventDeploymentStarted, "deployment-manager", nil)
		if err != nil {
			log.Printf("deployment-manager: transition error: %v", err)
			return nil
		}

		plan, err := mgr.Plan(dmsvc.DeployRequest{
			SubmissionID: sub.ID,
			ImageRef:     data.ImageDigest,
			Entrypoint:   sub.Entrypoint,
			DeclaredPort: sub.DeclaredPort,
		})
		if err != nil {
			log.Printf("deployment-manager: plan error: %v", err)
			return nil
		}

		// NEW: Call orchestrator to actually start the container sandbox
		spec := security.SandboxPodRequest{
			SubmissionID: sub.ID,
			ImageRef:     data.ImageDigest,
			Entrypoint:   sub.Entrypoint,
			DeclaredPort: sub.DeclaredPort,
			CPUCores:     plan.Deployment.CPUCores,
			MemoryMB:     plan.Deployment.MemoryMB,
			Namespace:    plan.Deployment.Namespace,
		}
		endpointInfo, err := orch.StartSandbox(ctx, sub.ID, data.ImageDigest, sub.DeclaredPort, spec)
		if err != nil {
			log.Printf("deployment-manager: StartSandbox error: %v", err)
			// Could transition to DEPLOYMENT_FAILED here
			return nil
		}

		// Update the plan endpoint with the real reachable address and container name
		plan.Deployment.PodName = endpointInfo.ContainerOrPodName
		plan.Endpoint.InternalURL = fmt.Sprintf("http://%s", endpointInfo.Address)
		plan.Endpoint.ServiceName = endpointInfo.ContainerOrPodName

		if err := mgr.RecordReady(ctx, plan); err != nil {
			log.Printf("deployment-manager: record ready error: %v", err)
			return nil
		}

		// Transition: DEPLOYING → HEALTH_CHECK → READY.
		_, _ = pg.TransitionSubmission(ctx, sub.ID, statemachine.EventDeploymentPodReady, "deployment-manager", nil)
		_, _ = pg.TransitionSubmission(ctx, sub.ID, statemachine.EventHealthReady, "deployment-manager", nil)
		return nil
	})

	consumer.Handle(events.TeardownRequested, func(ctx context.Context, env events.Envelope) error {
		log.Printf("deployment-manager: teardown for %s", env.SubmissionID)
		if err := mgr.Teardown(ctx, env.SubmissionID, "requested"); err != nil {
			log.Printf("deployment-manager: teardown error: %v", err)
		}
		_, _ = pg.TransitionSubmission(ctx, env.SubmissionID, statemachine.EventTeardownCompleted, "deployment-manager", nil)
		return nil
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	srv := &http.Server{Addr: ":" + cfg.GRPCPort, Handler: mux}
	go func() { _ = srv.ListenAndServe() }()

	log.Printf("deployment-manager consuming (group=deployment-manager)")
	if err := consumer.Run(ctx); err != nil {
		log.Printf("deployment-manager: consumer exited: %v", err)
	}
	srv.Shutdown(context.Background())
}

// combinedRepo satisfies dmsvc.Repository by combining Postgres + Redis.
type combinedRepo struct {
	store    *pgstore.Store
	registry *redisreg.Registry
}

func (r *combinedRepo) SaveDeployment(ctx context.Context, d *models.Deployment) error    { return r.store.SaveDeployment(ctx, d) }
func (r *combinedRepo) GetDeployment(ctx context.Context, subID string) (*models.Deployment, error) { return r.store.GetDeployment(ctx, subID) }
func (r *combinedRepo) SaveEndpoint(ctx context.Context, ep *models.Endpoint) error       { return r.store.SaveEndpoint(ctx, ep) }
func (r *combinedRepo) GetEndpoint(ctx context.Context, subID string) (*models.Endpoint, error) { return r.store.GetEndpoint(ctx, subID) }
func (r *combinedRepo) SetEndpointStatus(ctx context.Context, subID string, status models.EndpointStatus) error { return r.store.SetEndpointStatus(ctx, subID, status) }
func (r *combinedRepo) RegisterEndpoint(ctx context.Context, ep models.Endpoint) error    { return r.registry.RegisterEndpoint(ctx, ep) }
func (r *combinedRepo) DeregisterEndpoint(ctx context.Context, subID string) error        { return r.registry.DeregisterEndpoint(ctx, subID) }
func (r *combinedRepo) LookupEndpoint(ctx context.Context, subID string) (*models.Endpoint, error) { return r.registry.LookupEndpoint(ctx, subID) }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
