package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog/log"

	"github.com/dev-sotirov/noroi/internal/metrics"
)

// MetricsJSON handles GET /metrics/json and returns a JSON snapshot of
// key metrics gathered from the Prometheus registry.
type MetricsJSON struct {
	Metrics *metrics.Registry
}

type metricsSnapshot struct {
	SnapshotMS     int64   `json:"snapshot_ms"`
	RequestsTotal  float64 `json:"requests_total"`
	ErrorsTotal    float64 `json:"errors_total"`
	InFlight       float64 `json:"in_flight"`
	RPS1m          float64 `json:"rps_1m"`
	LatencyP50MS   float64 `json:"latency_p50_ms"`
	LatencyP90MS   float64 `json:"latency_p90_ms"`
	LatencyP99MS   float64 `json:"latency_p99_ms"`
	BytesSentTotal float64 `json:"bytes_sent_total"`
}

func (h *MetricsJSON) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mfs, err := h.Metrics.Prometheus.Gather()
	if err != nil {
		WriteError(w, r, http.StatusInternalServerError, "gather metrics: "+err.Error())
		return
	}

	snap := metricsSnapshot{
		SnapshotMS:     time.Now().UnixMilli(),
		RequestsTotal:  sumCounter(mfs, "noroi_requests_total"),
		ErrorsTotal:    sumCounter(mfs, "noroi_errors_total"),
		InFlight:       getGauge(mfs, "noroi_in_flight_requests"),
		RPS1m:          h.Metrics.RPS.Rate(),
		LatencyP50MS:   percentileMs(mfs, "noroi_request_duration_seconds", 0.50),
		LatencyP90MS:   percentileMs(mfs, "noroi_request_duration_seconds", 0.90),
		LatencyP99MS:   percentileMs(mfs, "noroi_request_duration_seconds", 0.99),
		BytesSentTotal: sumCounter(mfs, "noroi_response_bytes_total"),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(snap); err != nil {
		log.Ctx(r.Context()).Warn().Err(err).Msg("failed to encode metrics response")
		return
	}
}

// sumCounter sums all Counter values across all label combinations in the
// named MetricFamily.
func sumCounter(mfs []*dto.MetricFamily, name string) float64 {
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		var total float64
		for _, m := range mf.GetMetric() {
			if c := m.GetCounter(); c != nil {
				total += c.GetValue()
			}
		}
		return total
	}
	return 0
}

// getGauge returns the Gauge value for the named MetricFamily (first metric).
func getGauge(mfs []*dto.MetricFamily, name string) float64 {
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if g := m.GetGauge(); g != nil {
				return g.GetValue()
			}
		}
	}
	return 0
}

// percentileMs sums histogram buckets across all label combinations for the
// named MetricFamily and returns the estimated Nth percentile in milliseconds.
func percentileMs(mfs []*dto.MetricFamily, name string, p float64) float64 {
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}

		// Aggregate cumulative counts across all label combinations by
		// UpperBound. We need a merged view so we can compute a global
		// percentile.
		type bucket struct {
			upperBound      float64
			cumulativeCount uint64
		}

		countByBound := make(map[float64]uint64)
		var totalCount uint64

		for _, m := range mf.GetMetric() {
			h := m.GetHistogram()
			if h == nil {
				continue
			}
			totalCount += h.GetSampleCount()
			for _, b := range h.GetBucket() {
				countByBound[b.GetUpperBound()] += b.GetCumulativeCount()
			}
		}

		if totalCount == 0 {
			return 0
		}

		// Build sorted bucket list.
		buckets := make([]bucket, 0, len(countByBound))
		for bound, count := range countByBound {
			buckets = append(buckets, bucket{upperBound: bound, cumulativeCount: count})
		}
		sort.Slice(buckets, func(i, j int) bool {
			return buckets[i].upperBound < buckets[j].upperBound
		})

		target := p * float64(totalCount)

		var prevBound float64
		var prevCount uint64
		for _, b := range buckets {
			if float64(b.cumulativeCount) >= target {
				// Linear interpolation between prevBound and b.upperBound.
				width := b.upperBound - prevBound
				countInBucket := float64(b.cumulativeCount - prevCount)
				if countInBucket == 0 {
					return b.upperBound * 1000
				}
				fraction := (target - float64(prevCount)) / countInBucket
				return (prevBound + fraction*width) * 1000
			}
			prevBound = b.upperBound
			prevCount = b.cumulativeCount
		}

		// Beyond the last bucket — return the last upper bound.
		if len(buckets) > 0 {
			return buckets[len(buckets)-1].upperBound * 1000
		}
		return 0
	}
	return 0
}
