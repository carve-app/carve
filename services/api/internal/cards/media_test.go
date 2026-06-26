package cards

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mediaDoerFunc func(*http.Request) (*http.Response, error)

func (f mediaDoerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func TestHTTPMediaUploaderQualifiesRelativeURLAndAuthenticates(t *testing.T) {
	uploader := &httpMediaUploader{
		baseURL: "http://media.test",
		token:   "internal-token",
		client: mediaDoerFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://media.test/audio" {
				t.Fatalf("unexpected URL: %s", req.URL)
			}
			if req.Header.Get("Authorization") != "Bearer internal-token" {
				t.Fatal("missing media authorization")
			}
			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(strings.NewReader(`{"url":"/media/audio.mp3"}`)),
			}, nil
		}),
	}
	got, err := uploader.Upload(context.Background(), "/audio", strings.NewReader("mp3"), "audio/mpeg")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://media.test/media/audio.mp3" {
		t.Fatalf("unexpected qualified URL: %s", got)
	}
}

func TestHTTPMediaUploaderRejectsMalformedAndInterruptedResponses(t *testing.T) {
	tests := []struct {
		name   string
		client mediaDoerFunc
	}{
		{
			name: "interrupted",
			client: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection reset")
			},
		},
		{
			name: "malformed",
			client: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader("not-json"))}, nil
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uploader := &httpMediaUploader{baseURL: "http://media.test", client: tc.client}
			if _, err := uploader.Upload(context.Background(), "/audio", strings.NewReader("mp3"), "audio/mpeg"); err == nil {
				t.Fatal("expected upload failure")
			}
		})
	}
}
