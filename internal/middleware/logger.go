package middleware

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// Logger returns a chi-compatible middleware that emits a single structured
// log line per request using the provided zerolog.Logger.  It is -race clean
// and carries no global state.
func Logger(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := WrapResponseWriter(w)

			next.ServeHTTP(rw, r)

			status := rw.Status()
			durationMS := float64(time.Since(start)) / float64(time.Millisecond)

			level := zerolog.InfoLevel
			if status >= 400 {
				level = zerolog.WarnLevel
			}

			log.WithLevel(level).
				Str("request_id", r.Header.Get("X-Request-Id")).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", status).
				Float64("duration_ms", durationMS).
				Int64("bytes", rw.BytesWritten()).
				Str("remote_addr", r.RemoteAddr).
				Msg("")
		})
	}
}
