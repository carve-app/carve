package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// s3Backend writes to any S3-compatible bucket. For Cloudflare R2 the endpoint
// is `<account-id>.r2.cloudflarestorage.com` and the public read URL routes
// through the Cloudflare custom domain attached to the bucket.
type s3Backend struct {
	client     *minio.Client
	bucket     string
	publicBase string
}

func NewS3FromEnv() (Storage, error) {
	bucket := os.Getenv("R2_BUCKET")
	if bucket == "" {
		bucket = os.Getenv("S3_BUCKET")
	}
	accountID := os.Getenv("R2_ACCOUNT_ID")
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" && accountID != "" {
		endpoint = accountID + ".r2.cloudflarestorage.com"
	}
	if endpoint == "" || bucket == "" {
		return nil, errors.New("storage: need R2_BUCKET + R2_ACCOUNT_ID (or S3_BUCKET + S3_ENDPOINT)")
	}

	accessKey := firstNonEmpty("R2_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID", "S3_ACCESS_KEY_ID")
	secretKey := firstNonEmpty("R2_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY", "S3_SECRET_ACCESS_KEY")
	if accessKey == "" || secretKey == "" {
		return nil, errors.New("storage: missing access key / secret in env")
	}

	region := os.Getenv("S3_REGION")
	if region == "" {
		region = "auto" // R2 is region-less but minio-go wants a value
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: true,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}

	publicBase := os.Getenv("R2_PUBLIC_BASE")
	if publicBase == "" {
		publicBase = os.Getenv("S3_PUBLIC_BASE")
	}

	return &s3Backend{
		client:     client,
		bucket:     bucket,
		publicBase: publicBase,
	}, nil
}

func (s *s3Backend) Put(ctx context.Context, prefix, key, contentType string, body io.Reader, size int64) (string, error) {
	objectKey := prefix + "/" + key
	_, err := s.client.PutObject(ctx, s.bucket, objectKey, body, size, minio.PutObjectOptions{
		ContentType:  contentType,
		CacheControl: "public, max-age=31536000, immutable",
	})
	if err != nil {
		return "", err
	}
	if s.publicBase != "" {
		return s.publicBase + "/" + objectKey, nil
	}
	return "/" + objectKey, nil
}

func (s *s3Backend) Open(_ context.Context, _, _ string) (io.ReadCloser, string, error) {
	// Public reads go through Cloudflare custom domain on the R2 bucket; the
	// media service does not proxy bytes through itself.
	return nil, "", ErrUnsupported
}

func firstNonEmpty(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
