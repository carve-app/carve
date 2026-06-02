package storage

import (
	"context"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

type local struct {
	dir string
}

func NewLocal(dir string) (Storage, error) {
	for _, sub := range []string{"screenshots", "audio"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	return &local{dir: dir}, nil
}

func (l *local) Put(_ context.Context, prefix, key, _ string, body io.Reader, _ int64) (string, error) {
	path := filepath.Join(l.dir, prefix, key)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, body); err != nil {
		return "", err
	}
	return "/" + prefix + "/" + key, nil
}

func (l *local) Open(_ context.Context, prefix, key string) (io.ReadCloser, string, error) {
	// Defense in depth — handlers already reject "..".
	if strings.Contains(key, "..") || strings.Contains(key, "/") {
		return nil, "", os.ErrNotExist
	}
	path := filepath.Join(l.dir, prefix, key)
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	ct := mime.TypeByExtension(filepath.Ext(key))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return f, ct, nil
}
