package metrics

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// RPSWindow tracks requests-per-second over a rolling 60-second window.
// It is goroutine-safe.
type RPSWindow struct {
	mu      sync.Mutex
	buckets [60]int64 // one bucket per second, circular
	current int       // index of the active bucket
	second  int64     // unix second of the active bucket
}

// Record increments the counter for the current second.
func (w *RPSWindow) Record() {
	now := time.Now().Unix()
	w.mu.Lock()
	defer w.mu.Unlock()

	if now != w.second {
		diff := now - w.second
		if diff > 60 {
			diff = 60
		}
		for i := int64(0); i < diff; i++ {
			w.current = (w.current + 1) % 60
			w.buckets[w.current] = 0
		}
		w.second = now
	}
	w.buckets[w.current]++
}

// Rate returns the average RPS over the last 60 seconds.
func (w *RPSWindow) Rate() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	var sum int64
	for _, v := range w.buckets {
		sum += v
	}
	return float64(sum) / 60.0
}

// latencyBuckets are the shared histogram buckets used across all duration metrics.
var latencyBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Registry holds a non-default Prometheus registry and all instrumentation
// metrics for the noroi load-test server. Use New() to construct it.
type Registry struct {
	// Prometheus is the raw registry; pass it to promhttp.HandlerFor.
	Prometheus *prometheus.Registry

	// RequestsTotal counts every completed request.
	// Labels: method, path, status.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration measures end-to-end handler latency.
	// Labels: method, path.
	RequestDuration *prometheus.HistogramVec

	// SimulatedDelay measures the artificial delay injected per request.
	// No labels.
	SimulatedDelay prometheus.Histogram

	// ResponseBytesTotal counts bytes written to clients.
	// Labels: method, path.
	ResponseBytesTotal *prometheus.CounterVec

	// InFlightRequests tracks the number of requests currently being handled.
	// No labels.
	InFlightRequests prometheus.Gauge

	// ErrorsTotal counts requests that resulted in an application-level error.
	// Labels: method, path.
	ErrorsTotal *prometheus.CounterVec

	// CPUBurnDuration measures the CPU-burn duration performed per request.
	// No labels.
	CPUBurnDuration prometheus.Histogram

	// RPS tracks requests-per-second over a rolling 60-second window.
	RPS *RPSWindow
}

// New creates a Registry with a fresh, isolated prometheus.Registry and
// registers all metrics on it. It returns an error instead of panicking if
// any registration fails.
func New() (*Registry, error) {
	reg := prometheus.NewRegistry()

	r := &Registry{
		Prometheus: reg,
		RPS:        &RPSWindow{second: time.Now().Unix()},

		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "noroi_requests_total",
				Help: "Total number of HTTP requests processed, partitioned by method, path, and status code.",
			},
			[]string{"method", "path", "status"},
		),

		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "noroi_request_duration_seconds",
				Help:    "End-to-end HTTP request latency in seconds.",
				Buckets: latencyBuckets,
			},
			[]string{"method", "path"},
		),

		SimulatedDelay: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "noroi_simulated_delay_seconds",
				Help:    "Artificial delay injected per request, in seconds.",
				Buckets: latencyBuckets,
			},
		),

		ResponseBytesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "noroi_response_bytes_total",
				Help: "Total bytes written in HTTP response bodies, partitioned by method and path.",
			},
			[]string{"method", "path"},
		),

		InFlightRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "noroi_in_flight_requests",
				Help: "Current number of HTTP requests being handled.",
			},
		),

		ErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "noroi_errors_total",
				Help: "Total number of requests that resulted in an application-level error.",
			},
			[]string{"method", "path"},
		),

		CPUBurnDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "noroi_cpu_burn_duration_seconds",
				Help:    "Duration of the CPU-burn work performed per request, in seconds.",
				Buckets: latencyBuckets,
			},
		),
	}

	collectors := []prometheus.Collector{
		r.RequestsTotal,
		r.RequestDuration,
		r.SimulatedDelay,
		r.ResponseBytesTotal,
		r.InFlightRequests,
		r.ErrorsTotal,
		r.CPUBurnDuration,
	}

	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("metrics: failed to register collector: %w", err)
		}
	}

	return r, nil
}
