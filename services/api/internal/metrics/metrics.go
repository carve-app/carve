// Package metrics is a tiny zero-dependency Prometheus exporter. It collects
// per-endpoint latency histograms + request/error counters + arbitrary named
// counters (used for FSRS throughput, queue depth, etc.) and renders them in
// the Prometheus text exposition format.
//
// Rationale for not pulling in prometheus/client_golang: the carve API only
// needs five metrics. The full client adds ~1 MB to the binary, a flag-handling
// dependency tree, and a registry abstraction we won't touch. Replace this
// package with the official client the day we want histograms-over-quantiles
// or push-gateway support.
package metrics

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ProtectedHandler requires a bearer token when METRICS_TOKEN is configured.
// Local development remains frictionless when the variable is intentionally
// empty, while production can no longer expose operational data by accident.
func ProtectedHandler(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("METRICS_TOKEN")
	if token != "" {
		const prefix = "Bearer "
		got := r.Header.Get("Authorization")
		if len(got) <= len(prefix) || got[:len(prefix)] != prefix ||
			subtle.ConstantTimeCompare([]byte(got[len(prefix):]), []byte(token)) != 1 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
	}
	Handler(w, r)
}

// Buckets in seconds — Prometheus' default web histogram.
var defaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type histogram struct {
	mu     sync.Mutex
	counts []uint64 // one per bucket; the last slot is +Inf overflow
	sum    float64
	total  uint64
}

func newHistogram() *histogram {
	return &histogram{counts: make([]uint64, len(defaultBuckets)+1)}
}

func (h *histogram) observe(seconds float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum += seconds
	h.total++
	for i, b := range defaultBuckets {
		if seconds <= b {
			h.counts[i]++
			return
		}
	}
	h.counts[len(defaultBuckets)]++ // +Inf
}

type labelKey struct {
	method string
	path   string
}

var (
	mu        sync.RWMutex
	reqTotal  = map[labelKey]*uint64{}
	errTotal  = map[labelKey]*uint64{}
	latencies = map[labelKey]*histogram{}
	counters  = map[string]*uint64{}
)

// IncRequest records a single completed HTTP request.
func IncRequest(method, path string, status int, dur time.Duration) {
	k := labelKey{method, path}
	mu.Lock()
	c, ok := reqTotal[k]
	if !ok {
		c = new(uint64)
		reqTotal[k] = c
	}
	h, ok := latencies[k]
	if !ok {
		h = newHistogram()
		latencies[k] = h
	}
	if status >= 500 {
		e, ok := errTotal[k]
		if !ok {
			e = new(uint64)
			errTotal[k] = e
		}
		atomic.AddUint64(e, 1)
	}
	mu.Unlock()
	atomic.AddUint64(c, 1)
	h.observe(dur.Seconds())
}

// IncCounter bumps a named counter (e.g. metrics.IncCounter("fsrs_event_total")).
func IncCounter(name string) {
	mu.Lock()
	c, ok := counters[name]
	if !ok {
		c = new(uint64)
		counters[name] = c
	}
	mu.Unlock()
	atomic.AddUint64(c, 1)
}

// Handler serves the metrics in Prometheus text format.
func Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	mu.RLock()
	defer mu.RUnlock()

	writeReqTotals(w)
	writeErrTotals(w)
	writeLatencies(w)
	writeCounters(w)
}

func writeReqTotals(w io.Writer) {
	fmt.Fprintln(w, "# HELP carve_http_requests_total Number of HTTP requests served.")
	fmt.Fprintln(w, "# TYPE carve_http_requests_total counter")
	for _, k := range sortedKeys(reqTotal) {
		fmt.Fprintf(w, "carve_http_requests_total{method=%q,path=%q} %d\n", k.method, k.path, atomic.LoadUint64(reqTotal[k]))
	}
}

func writeErrTotals(w io.Writer) {
	fmt.Fprintln(w, "# HELP carve_http_errors_total Number of HTTP 5xx responses.")
	fmt.Fprintln(w, "# TYPE carve_http_errors_total counter")
	for _, k := range sortedKeys(errTotal) {
		fmt.Fprintf(w, "carve_http_errors_total{method=%q,path=%q} %d\n", k.method, k.path, atomic.LoadUint64(errTotal[k]))
	}
}

func writeLatencies(w io.Writer) {
	fmt.Fprintln(w, "# HELP carve_http_request_duration_seconds HTTP request latency histogram.")
	fmt.Fprintln(w, "# TYPE carve_http_request_duration_seconds histogram")
	for _, k := range sortedHistKeys() {
		h := latencies[k]
		h.mu.Lock()
		var cumulative uint64
		for i, b := range defaultBuckets {
			cumulative += h.counts[i]
			fmt.Fprintf(w, "carve_http_request_duration_seconds_bucket{method=%q,path=%q,le=%q} %d\n",
				k.method, k.path, fmtFloat(b), cumulative)
		}
		cumulative += h.counts[len(defaultBuckets)]
		fmt.Fprintf(w, "carve_http_request_duration_seconds_bucket{method=%q,path=%q,le=\"+Inf\"} %d\n",
			k.method, k.path, cumulative)
		fmt.Fprintf(w, "carve_http_request_duration_seconds_sum{method=%q,path=%q} %g\n",
			k.method, k.path, h.sum)
		fmt.Fprintf(w, "carve_http_request_duration_seconds_count{method=%q,path=%q} %d\n",
			k.method, k.path, h.total)
		h.mu.Unlock()
	}
}

func writeCounters(w io.Writer) {
	if len(counters) == 0 {
		return
	}
	fmt.Fprintln(w, "# HELP carve_app_counter Application-defined counters (FSRS events, queue flushes, etc.).")
	fmt.Fprintln(w, "# TYPE carve_app_counter counter")
	names := make([]string, 0, len(counters))
	for n := range counters {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(w, "carve_app_counter{name=%q} %d\n", n, atomic.LoadUint64(counters[n]))
	}
}

func sortedKeys(m map[labelKey]*uint64) []labelKey {
	out := make([]labelKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].path != out[j].path {
			return out[i].path < out[j].path
		}
		return out[i].method < out[j].method
	})
	return out
}

func sortedHistKeys() []labelKey {
	out := make([]labelKey, 0, len(latencies))
	for k := range latencies {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].path != out[j].path {
			return out[i].path < out[j].path
		}
		return out[i].method < out[j].method
	})
	return out
}

func fmtFloat(f float64) string {
	return fmt.Sprintf("%g", f)
}
