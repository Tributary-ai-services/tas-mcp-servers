package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("minio-client")

// Client is the MinIO storage client
type Client struct {
	client   *minio.Client
	endpoint string
}

// NewClient creates a new MinIO client
func NewClient(endpoint, accessKey, secretKey string, useSSL bool) (*Client, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	return &Client{
		client:   client,
		endpoint: endpoint,
	}, nil
}

// EnsureBucket creates a bucket if it doesn't exist
func (c *Client) EnsureBucket(ctx context.Context, bucket string) error {
	ctx, span := tracer.Start(ctx, "minio_ensure_bucket")
	defer span.End()
	span.SetAttributes(attribute.String("minio.bucket", bucket))

	exists, err := c.client.BucketExists(ctx, bucket)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		if err := c.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			span.RecordError(err)
			return fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return nil
}

// Upload uploads data to MinIO
func (c *Client) Upload(ctx context.Context, bucket, key string, data []byte, contentType string) (string, error) {
	ctx, span := tracer.Start(ctx, "minio_upload")
	defer span.End()
	span.SetAttributes(
		attribute.String("minio.bucket", bucket),
		attribute.String("minio.key", key),
		attribute.Int("minio.size", len(data)),
	)

	if err := c.EnsureBucket(ctx, bucket); err != nil {
		return "", err
	}

	reader := bytes.NewReader(data)
	_, err := c.client.PutObject(ctx, bucket, key, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		span.RecordError(err)
		return "", fmt.Errorf("failed to upload to MinIO: %w", err)
	}

	protocol := "http"
	url := fmt.Sprintf("%s://%s/%s/%s", protocol, c.endpoint, bucket, key)
	return url, nil
}

// Download downloads data from MinIO
func (c *Client) Download(ctx context.Context, bucket, key string) ([]byte, error) {
	ctx, span := tracer.Start(ctx, "minio_download")
	defer span.End()
	span.SetAttributes(
		attribute.String("minio.bucket", bucket),
		attribute.String("minio.key", key),
	)

	obj, err := c.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get object from MinIO: %w", err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to read object data: %w", err)
	}

	return data, nil
}

// Delete deletes an object from MinIO
func (c *Client) Delete(ctx context.Context, bucket, key string) error {
	ctx, span := tracer.Start(ctx, "minio_delete")
	defer span.End()
	span.SetAttributes(
		attribute.String("minio.bucket", bucket),
		attribute.String("minio.key", key),
	)

	err := c.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to delete object from MinIO: %w", err)
	}

	return nil
}
