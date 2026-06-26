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
	"time"

	"github.com/iicpc/track1/submission-engine/pkg/events"
	kafkabus "github.com/iicpc/track1/submission-engine/pkg/events/kafka"
	pgstore "github.com/iicpc/track1/submission-engine/pkg/store/postgres"
	"github.com/iicpc/track1/submission-engine/pkg/store/s3url"
	"github.com/iicpc/track1/submission-engine/pkg/statemachine"
	"github.com/iicpc/track1/submission-engine/services/upload-service/internal/config"
	uploadsvc "github.com/iicpc/track1/submission-engine/services/upload-service/internal/service"
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

	// Wire S3 provider for real artifact download + validation.
	s3p, err := s3url.New(s3url.Config{
		Endpoint:       cfg.S3Endpoint,
		Region:         envOr("S3_REGION", "us-east-1"),
		AccessKey:      os.Getenv("S3_ACCESS_KEY"),
		SecretKey:      os.Getenv("S3_SECRET_KEY"),
		ForcePathStyle: true,
		BucketUploads:  cfg.S3BucketUploads,
	})
	if err != nil {
		log.Fatalf("s3: %v", err)
	}

	consumer := kafkabus.NewConsumer(kafkabus.ConsumerConfig{
		Brokers: brokers,
		GroupID: "upload-service",
		Topics:  []string{string(events.SubmissionCreated)},
	})
	defer consumer.Close()

	consumer.Handle(events.SubmissionCreated, func(ctx context.Context, env events.Envelope) error {
		log.Printf("upload-service: waiting for artifact for %s", env.SubmissionID)

		// 1. Wait for the artifact to appear in S3 (user PUTs to the presigned URL).
		objectKey := env.SubmissionID + "/artifact"
		if err := s3p.WaitForObject(ctx, cfg.S3BucketUploads, objectKey,
			2*time.Second, 5*time.Minute); err != nil {
			log.Printf("upload-service: artifact not found for %s: %v", env.SubmissionID, err)
			_, _ = pg.TransitionSubmission(ctx, env.SubmissionID,
				statemachine.EventUploadFailed, "upload-service",
				map[string]interface{}{"error": err.Error()})
			return nil // don't block the partition
		}
		log.Printf("upload-service: artifact found for %s, downloading", env.SubmissionID)

		// 2. Download artifact to a temp file.
		localPath, err := s3p.DownloadToFile(ctx, cfg.S3BucketUploads, objectKey)
		if err != nil {
			log.Printf("upload-service: download failed for %s: %v", env.SubmissionID, err)
			_, _ = pg.TransitionSubmission(ctx, env.SubmissionID,
				statemachine.EventUploadFailed, "upload-service",
				map[string]interface{}{"error": err.Error()})
			return nil
		}
		defer os.Remove(localPath)

		// 3. Get submission metadata for validation.
		sub, err := pg.GetSubmission(ctx, env.SubmissionID)
		if err != nil {
			log.Printf("upload-service: get submission %s: %v", env.SubmissionID, err)
			return nil
		}

		// 4. Validate artifact using the real validator.
		result, err := uploadsvc.ValidateArtifact(localPath, uploadsvc.ArtifactMetadata{
			Language:       sub.Language,
			SubmissionType: sub.Type,
		}, uploadsvc.Limits{             // kuch to gadbad hai !!
			MaxUploadBytes:       cfg.MaxUploadBytes,
			MaxDecompressedBytes: cfg.MaxDecompressedBytes,
			MaxFiles:             cfg.MaxFiles,
		})
		if err != nil {
			log.Printf("upload-service: validation failed for %s: %v", env.SubmissionID, err)
			_, _ = pg.TransitionSubmission(ctx, env.SubmissionID,
				statemachine.EventUploadFailed, "upload-service",
				map[string]interface{}{"error": err.Error()})
			return nil
		}
		log.Printf("upload-service: artifact validated for %s (sha256=%s, files=%d)",
			env.SubmissionID, result.SHA256, result.Files)

		// 5. Update submission with artifact metadata.
		sub.ArtifactURI = fmt.Sprintf("s3://%s/%s", cfg.S3BucketUploads, objectKey)
		sub.ArtifactSHA256 = result.SHA256
		_ = pg.UpdateSubmission(ctx, sub)

		// 6. Transition: CREATED → UPLOADED.
		sub, err = pg.TransitionSubmission(ctx, env.SubmissionID,
			statemachine.EventUploadSucceeded, "upload-service", nil)
		if err != nil {
			log.Printf("upload-service: transition error for %s: %v", env.SubmissionID, err)
			return nil
		}

		// 7. Emit submission.uploaded so build-service picks it up.
		uploaded, err := events.NewEnvelope(events.SubmissionUploaded, sub.ID, "upload-service",
			events.SubmissionUploadedData{
				ArtifactURI:    sub.ArtifactURI,
				ArtifactSHA256: sub.ArtifactSHA256,
				DeclaredPort:   sub.DeclaredPort,
				Entrypoint:     sub.Entrypoint,
			})
		if err != nil {
			return err
		}
		return producer.Publish(ctx, uploaded)
	})

	// Health endpoint.
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	srv := &http.Server{Addr: ":" + envOr("HTTP_PORT", cfg.GRPCPort), Handler: mux}
	go func() { _ = srv.ListenAndServe() }()

	log.Printf("upload-service consuming (group=upload-service)")
	if err := consumer.Run(ctx); err != nil {
		log.Printf("upload-service: consumer exited: %v", err)
	}
	srv.Shutdown(context.Background())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
