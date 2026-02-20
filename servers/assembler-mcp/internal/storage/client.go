package storage

import (
	"bytes"
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps MinIO for object storage
type Client struct {
	minio  *minio.Client
	bucket string
}

// NewClient creates a new MinIO storage client
func NewClient(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Client, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Ensure bucket exists
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return &Client{minio: client, bucket: bucket}, nil
}

// Upload stores content to MinIO and returns the object URL
func (c *Client) Upload(key string, content []byte, contentType string) (string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Map format to content type
	switch contentType {
	case "md", "markdown":
		contentType = "text/markdown"
	case "docx":
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "pdf":
		contentType = "application/pdf"
	case "json":
		contentType = "application/json"
	case "text":
		contentType = "text/plain"
	}

	reader := bytes.NewReader(content)
	_, err := c.minio.PutObject(
		context.Background(),
		c.bucket,
		key,
		reader,
		int64(len(content)),
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return "", fmt.Errorf("failed to upload object: %w", err)
	}

	// Return the object path (external URL depends on ingress config)
	return fmt.Sprintf("/%s/%s", c.bucket, key), nil
}
