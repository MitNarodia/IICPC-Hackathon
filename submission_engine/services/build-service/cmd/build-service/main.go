package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/iicpc/track1/submission-engine/pkg/bootstrap"
	"github.com/iicpc/track1/submission-engine/pkg/events"
	kafkabus "github.com/iicpc/track1/submission-engine/pkg/events/kafka"
	"github.com/iicpc/track1/submission-engine/pkg/models"
	dockerorch "github.com/iicpc/track1/submission-engine/pkg/orchestrator/docker"
	pgstore "github.com/iicpc/track1/submission-engine/pkg/store/postgres"
	"github.com/iicpc/track1/submission-engine/pkg/store/s3url"
	"github.com/iicpc/track1/submission-engine/pkg/statemachine"
	"github.com/iicpc/track1/submission-engine/services/build-service/internal/config"
	"github.com/iicpc/track1/submission-engine/services/build-service/internal/profiles"
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

	brokers := strings.Split(cfg.KafkaBrokers, ",")
	producer := kafkabus.NewProducer(brokers)
	defer producer.Close()

	s3p, err := s3url.New(s3url.Config{
		Endpoint:       cfg.S3Endpoint,
		AccessKey:      cfg.S3AccessKey,
		SecretKey:      cfg.S3SecretKey,
		ForcePathStyle: true,
		BucketUploads:  cfg.S3BucketUploads,
		Region:         cfg.S3Region,
	})
	if err != nil {
		log.Fatalf("s3: %v", err)
	}

	orch, err := bootstrap.WireOrchestrator(ctx)
	if err != nil {
		log.Fatalf("orchestrator: %v", err)
	}
	defer orch.(*dockerorch.Orchestrator).Close()

	consumer := kafkabus.NewConsumer(kafkabus.ConsumerConfig{
		Brokers: brokers,
		GroupID: "build-service",
		Topics:  []string{string(events.SubmissionUploaded)},
	})
	defer consumer.Close()

	consumer.Handle(events.SubmissionUploaded, func(ctx context.Context, env events.Envelope) error {
		log.Printf("build-service: processing %s for %s", env.Type, env.SubmissionID)

		// Transition UPLOADED → VALIDATING → VALIDATED → BUILDING.
		// ye dono faltu hai !!
		_, _ = pg.TransitionSubmission(ctx, env.SubmissionID, statemachine.EventValidationStarted, "build-service", nil)
		_, _ = pg.TransitionSubmission(ctx, env.SubmissionID, statemachine.EventValidationPassed, "build-service", nil)
		sub, err := pg.TransitionSubmission(ctx, env.SubmissionID, statemachine.EventBuildStarted, "build-service", nil)
		if err != nil {
			log.Printf("build-service: transition error: %v", err)
			return nil
		}

		// Record build start.
		buildID, _ := models.NewUUIDv7()
		now := time.Now().UTC()
		build := &models.Build{
			ID:           buildID,
			SubmissionID: sub.ID,
			Status:       models.BuildRunning,
			StartedAt:    &now,
		}
		_ = pg.SaveBuild(ctx, build)

		// Render the Dockerfile.
		df, err := profiles.RenderDockerfile(profiles.RenderRequest{
			Language:       sub.Language,
			SubmissionType: sub.Type,
			Entrypoint:     sub.Entrypoint,
			DeclaredPort:   sub.DeclaredPort,
		})
		if err != nil {
			failBuild(ctx, pg, producer, build, sub.ID, err.Error())
			return nil
		}

		// NEW: Download artifact from S3
		objectKey := sub.ID + "/artifact"
		localPath, err := s3p.DownloadToFile(ctx, cfg.S3BucketUploads, objectKey)
		if err != nil {
			failBuild(ctx, pg, producer, build, sub.ID, "artifact download: "+err.Error())
			return nil
		}
		defer os.Remove(localPath)

		// NEW: Convert downloaded artifact (tar.gz or ELF binary) into an uncompressed tar stream
		tarPath, err := createBuildContext(localPath, sub.Type)
		if err != nil {
			failBuild(ctx, pg, producer, build, sub.ID, "build context: "+err.Error())
			return nil
		}
		defer os.Remove(tarPath)

		contextFile, err := os.Open(tarPath)
		if err != nil {
			failBuild(ctx, pg, producer, build, sub.ID, err.Error())
			return nil
		}
		defer contextFile.Close()

		// NEW: Real Docker build via orchestrator
		outcome, err := orch.RunBuild(ctx, sub.ID, df, contextFile)
		if err != nil {
			failBuild(ctx, pg, producer, build, sub.ID, err.Error())
			return nil
		}

		fin := time.Now().UTC()
		build.Status = models.BuildSuccess
		build.ImageRef = outcome.ImageRef
		build.ImageDigest = outcome.ImageDigest
		build.LogsURI = outcome.LogsURI
		build.FinishedAt = &fin
		_ = pg.SaveBuild(ctx, build)

		// Transition BUILDING → BUILT.
		_, _ = pg.TransitionSubmission(ctx, sub.ID, statemachine.EventBuildSucceeded, "build-service", nil)

		// Skip scan for now: BUILT → SCANNING → SCANNED.
		_, _ = pg.TransitionSubmission(ctx, sub.ID, statemachine.EventScanStarted, "build-service", nil)
		_, _ = pg.TransitionSubmission(ctx, sub.ID, statemachine.EventScanPassed, "build-service", nil)

		// Emit build.succeeded.
		evt, _ := events.NewEnvelope(events.BuildSucceeded, sub.ID, "build-service", events.BuildSucceededData{
			ImageRef:    outcome.ImageRef,
			ImageDigest: outcome.ImageDigest,
		})
		return producer.Publish(ctx, evt)
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	srv := &http.Server{Addr: ":" + cfg.GRPCPort, Handler: mux}
	go func() { _ = srv.ListenAndServe() }()

	log.Printf("build-service consuming (group=build-service)")
	if err := consumer.Run(ctx); err != nil {
		log.Printf("build-service: consumer exited: %v", err)
	}
	srv.Shutdown(context.Background())
}

func failBuild(ctx context.Context, pg *pgstore.Store, producer *kafkabus.Producer, build *models.Build, subID, reason string) {
	now := time.Now().UTC()
	build.Status = models.BuildFailed
	build.Error = reason
	build.FinishedAt = &now
	_ = pg.SaveBuild(ctx, build)
	_, _ = pg.TransitionSubmission(ctx, subID, statemachine.EventBuildFailed, "build-service", map[string]interface{}{"error": reason})
	evt, _ := events.NewEnvelope(events.BuildFailed, subID, "build-service", events.BuildFailedData{Error: reason})
	_ = producer.Publish(ctx, evt)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func createBuildContext(artifactPath string, subType models.SubmissionType) (string, error) {
	out, err := os.CreateTemp("", "track1-build-ctx-*.tar")
	if err != nil {
		return "", err
	}
	defer out.Close()

	tw := tar.NewWriter(out)
	defer tw.Close()

	in, err := os.Open(artifactPath)
	if err != nil {
		return "", err
	}
	defer in.Close()

	if subType == models.SubmissionTypeBinary {
		// Just put the binary as 'bot' in the tar
		stat, err := in.Stat()
		if err != nil {
			return "", err
		}
		if err := tw.WriteHeader(&tar.Header{
			Name: "bot",
			Mode: 0755,
			Size: stat.Size(),
		}); err != nil {
			return "", err
		}
		if _, err := io.Copy(tw, in); err != nil {
			return "", err
		}
		return out.Name(), nil
	}
    // kya hum isko skip kar skte hai ? !!
	// Source: copy all files from tar.gz into the new tar
	gz, err := gzip.NewReader(in)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return "", err
		}
		if hdr.Size > 0 {
			if _, err := io.Copy(tw, tr); err != nil {
				return "", err
			}
		}
	}
	return out.Name(), nil
}
