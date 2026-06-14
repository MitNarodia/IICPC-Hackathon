// Package s3url implements store.UploadURLProvider using AWS SDK v2 S3 presigning,
// configured for MinIO (path-style, custom endpoint).
package s3url

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Config holds the settings needed to build a presigning S3 client.
type Config struct {
	Endpoint       string // e.g. "http://minio:9000"
	Region         string // e.g. "us-east-1"
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool
	BucketUploads  string // e.g. "raw-uploads"
	BucketLogs     string // e.g. "build-logs"
	URLTTLSeconds  int    // presign URL validity (default 900)
}

// Provider implements store.UploadURLProvider and provides S3 object helpers.
type Provider struct {
	client       *s3.Client
	presigner    *s3.PresignClient
	bucketUpload string
	bucketLogs   string
	urlTTL       time.Duration
}

// New builds a Provider from config.
func New(cfg Config) (*Provider, error) {
	if cfg.Endpoint == "" || cfg.AccessKey == "" || cfg.SecretKey == "" || cfg.BucketUploads == "" {
		return nil, fmt.Errorf("s3url: endpoint, access_key, secret_key, and bucket_uploads are required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	ttl := time.Duration(cfg.URLTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 900 * time.Second
	}

	client := s3.New(s3.Options{
		Region:       cfg.Region,
		BaseEndpoint: aws.String(cfg.Endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		UsePathStyle: cfg.ForcePathStyle,
	})
	presigner := s3.NewPresignClient(client)
	return &Provider{
		client:       client,
		presigner:    presigner,
		bucketUpload: cfg.BucketUploads,
		bucketLogs:   cfg.BucketLogs,
		urlTTL:       ttl,
	}, nil
}

// PresignUploadURL returns a time-limited PUT URL for the submission artifact.
func (p *Provider) PresignUploadURL(ctx context.Context, submissionID string) (string, error) {
	key := submissionID + "/artifact"
	req, err := p.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(p.bucketUpload),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(p.urlTTL))
	if err != nil {
		return "", fmt.Errorf("s3url: presign: %w", err)
	}
	return req.URL, nil
}

// PutObject uploads an object to the given bucket+key.
func (p *Provider) PutObject(ctx context.Context, bucket, key string, body io.Reader) error {
	_, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   body,
	})
	return err
}

// GetObject downloads an object.
func (p *Provider) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	out, err := p.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

// PresignDownloadURL returns a time-limited GET URL.
func (p *Provider) PresignDownloadURL(ctx context.Context, bucket, key string) (string, error) {
	req, err := p.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(p.urlTTL))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// BucketUploads returns the configured uploads bucket name.
func (p *Provider) BucketUploads() string { return p.bucketUpload }

// BucketLogs returns the configured build-logs bucket name.
func (p *Provider) BucketLogs() string { return p.bucketLogs }

// WaitForObject polls S3 until the object exists or the timeout elapses.
// It uses HeadObject to avoid downloading the full body on each probe.
func (p *Provider) WaitForObject(ctx context.Context, bucket, key string, pollInterval, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		_, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if err == nil {
			return nil // object exists
		}
		// Any error other than context cancellation: sleep and retry.
		// MinIO returns 404 / NoSuchKey when the object doesn't exist yet.
		select {
		case <-ctx.Done():
			return fmt.Errorf("s3url: wait for %s/%s: %w", bucket, key, ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// DownloadToFile downloads an S3 object to a temporary local file and returns
// the file path. The caller is responsible for removing the file when done.
func (p *Provider) DownloadToFile(ctx context.Context, bucket, key string) (string, error) {
	body, err := p.GetObject(ctx, bucket, key)
	if err != nil {
		return "", fmt.Errorf("s3url: download %s/%s: %w", bucket, key, err)
	}
	defer body.Close()

	tmp, err := os.CreateTemp("", "track1-artifact-*")
	if err != nil {
		return "", fmt.Errorf("s3url: create temp file: %w", err)
	}
	if _, err := io.Copy(tmp, body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("s3url: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}
