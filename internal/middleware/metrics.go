package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/dev-sotirov/noroi/internal/metrics"
)

// Metrics returns a middleware that instruments HTTP handlers with Prometheus metrics.
func Metrics(reg *metrics.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method := r.Method
			path := r.URL.Path

			reg.InFlightRequests.Inc()
			start := time.Now()
			rw := WrapResponseWriter(w)

			defer func() {
				elapsed := time.Since(start)
				status := rw.Status()
				statusStr := fmt.Sprintf("%d", status)

				reg.InFlightRequests.Dec()
				reg.RPS.Record()
				reg.RequestDuration.WithLabelValues(method, path).Observe(elapsed.Seconds())
				reg.RequestsTotal.WithLabelValues(method, path, statusStr).Inc()
				reg.ResponseBytesTotal.WithLabelValues(method, path).Add(float64(rw.BytesWritten()))

				if status >= 500 {
					reg.ErrorsTotal.WithLabelValues(method, path).Inc()
				}
			}()

			next.ServeHTTP(rw, r)
		})
	}
}
