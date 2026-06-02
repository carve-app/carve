// Package storage abstracts media persistence behind a small interface so the
// service can run against either local disk (dev) or S3-compatible object
// storage (prod — Cloudflare R2 via the S3 API).
package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
)

type Storage interface {
	// Put stores body under (prefix, key) with the given content type.
	// Returns the public URL the object will be served from.
	Put(ctx context.Context, prefix, key, contentType string, body io.Reader, size int64) (string, error)

	// Open returns a reader for an object. Only used by the local backend;
	// the R2 backend serves via Cloudflare custom domain and Open returns
	// ErrUnsupported.
	Open(ctx context.Context, prefix, key string) (io.ReadCloser, string, error)
}

var ErrUnsupported = errors.New("operation unsupported by this backend")

// New picks a backend based on STORAGE_BACKEND env var:
//
//	"local" (default): writes under MEDIA_STORAGE_DIR
//	"r2":              writes to a Cloudflare R2 bucket via the S3 API
func New() (Storage, error) {
	backend := strings.ToLower(os.Getenv("STORAGE_BACKEND"))
	if backend == "" {
		backend = "local"
	}
	switch backend {
	case "local":
		dir := os.Getenv("MEDIA_STORAGE_DIR")
		if dir == "" {
			dir = "/tmp/carve-media"
		}
		return NewLocal(dir)
	case "r2", "s3":
		return NewS3FromEnv()
	default:
		return nil, errors.New("unknown STORAGE_BACKEND: " + backend)
	}
}
