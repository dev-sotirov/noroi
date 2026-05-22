package handler

import (
	"compress/gzip"
	"context"
	"io"
	"math/rand"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/dev-sotirov/noroi/internal/body"
	"github.com/dev-sotirov/noroi/internal/config"
	"github.com/dev-sotirov/noroi/internal/metrics"
)

// Respond is the main configurable load-test endpoint. It accepts query
// parameters that control status code, body size, artificial delay, CPU burn,
// chunked transfer, gzip compression, and simulated error rate.
type Respond struct {
	Defaults *config.DefaultsConfig
	Body     *body.Generator
	Metrics  *metrics.Registry
}

// ServeHTTP implements http.Handler.
func (h *Respond) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// -------------------------------------------------------------------------
	// Parse & validate query parameters
	// -------------------------------------------------------------------------

	// -- delay / delay_min / delay_max / jitter --
	baseDelay := h.Defaults.Delay
	if raw := q.Get("delay"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 || d > config.MaxDelay {
			WriteError(w, r, http.StatusBadRequest, "invalid delay: must be a duration between 0 and 1h")
			return
		}
		baseDelay = d
	}

	var delayMin, delayMax time.Duration
	hasRange := false

	if raw := q.Get("delay_min"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 || d > config.MaxDelay {
			WriteError(w, r, http.StatusBadRequest, "invalid delay_min: must be a duration between 0 and 1h")
			return
		}
		delayMin = d
		hasRange = true
	}

	if raw := q.Get("delay_max"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 || d > config.MaxDelay {
			WriteError(w, r, http.StatusBadRequest, "invalid delay_max: must be a duration between 0 and 1h")
			return
		}
		delayMax = d
		hasRange = true
	}

	jitter := h.Defaults.Jitter
	if raw := q.Get("jitter"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 || d > config.MaxDelay {
			WriteError(w, r, http.StatusBadRequest, "invalid jitter: must be a duration between 0 and 1h")
			return
		}
		jitter = d
	}

	// -- status --
	statusCode := h.Defaults.StatusCode
	if raw := q.Get("status"); raw != "" {
		s, err := strconv.Atoi(raw)
		if err != nil || s < 100 || s > 599 {
			WriteError(w, r, http.StatusBadRequest, "invalid status: must be an integer in 100–599")
			return
		}
		statusCode = s
	}

	// -- size --
	var bodySize int64
	if raw := q.Get("size"); raw != "" {
		s, err := body.ParseSize(raw)
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "invalid size: "+err.Error())
			return
		}
		bodySize = s
	} else {
		bodySize = int64(h.Defaults.BodySize)
	}

	// -- body_type --
	bodyType := body.BodyType(h.Defaults.BodyType)
	if raw := q.Get("body_type"); raw != "" {
		switch body.BodyType(raw) {
		case body.Text, body.JSON, body.Binary, body.Zeros:
			bodyType = body.BodyType(raw)
		default:
			WriteError(w, r, http.StatusBadRequest, "invalid body_type: must be one of text, json, binary, zeros")
			return
		}
	}

	// -- error_rate --
	errorRate := h.Defaults.ErrorRate
	if raw := q.Get("error_rate"); raw != "" {
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil || f < 0.0 || f > 1.0 {
			WriteError(w, r, http.StatusBadRequest, "invalid error_rate: must be a float in 0.0–1.0")
			return
		}
		errorRate = f
	}

	// -- cpu_ms --
	cpuMs := 0
	if raw := q.Get("cpu_ms"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > config.MaxCPUMs {
			WriteError(w, r, http.StatusBadRequest, "invalid cpu_ms: must be an integer between 0 and 10000")
			return
		}
		cpuMs = n
	}

	// -- chunked --
	chunked := false
	if raw := q.Get("chunked"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "invalid chunked: must be true or false")
			return
		}
		chunked = b
	}

	// -- chunk_delay --
	chunkDelay := time.Duration(0)
	if raw := q.Get("chunk_delay"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 || d > config.MaxChunkDelay {
			WriteError(w, r, http.StatusBadRequest, "invalid chunk_delay: must be a duration between 0 and 1h")
			return
		}
		chunkDelay = d
	}

	// -- chunks --
	chunks := 5
	if raw := q.Get("chunks"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > config.MaxChunks {
			WriteError(w, r, http.StatusBadRequest, "invalid chunks: must be an integer between 1 and 100000")
			return
		}
		chunks = n
	}

	// -- compress --
	compress := false
	if raw := q.Get("compress"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "invalid compress: must be true or false")
			return
		}
		compress = b
	}

	// -- close --
	forceClose := false
	if raw := q.Get("close"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "invalid close: must be true or false")
			return
		}
		forceClose = b
	}

	// -------------------------------------------------------------------------
	// 1. Error rate — evaluated first, before any other work.
	// -------------------------------------------------------------------------
	if errorRate > 0 && rand.Float64() < errorRate { //nolint:gosec
		WriteError(w, r, http.StatusInternalServerError, "simulated error")
		return
	}

	// -------------------------------------------------------------------------
	// 2. CPU burn — before the delay.
	// -------------------------------------------------------------------------
	if cpuMs > 0 {
		cpuStart := time.Now()
		deadline := cpuStart.Add(time.Duration(cpuMs) * time.Millisecond)
		for time.Now().Before(deadline) {
			runtime.Gosched()
		}
		h.Metrics.CPUBurnDuration.Observe(time.Since(cpuStart).Seconds())
	}

	// -------------------------------------------------------------------------
	// 3. Compute delay.
	// -------------------------------------------------------------------------
	var delay time.Duration
	if hasRange && delayMin >= 0 && delayMax > delayMin {
		// Random value in [delayMin, delayMax).
		spread := delayMax - delayMin
		delay = delayMin + time.Duration(rand.Int63n(int64(spread))) //nolint:gosec
	} else {
		// Base delay ± random jitter in [-jitter, +jitter].
		if jitter > 0 {
			// rand.Int63n(2*jitter+1) gives [0, 2*jitter+1); subtract jitter → [-jitter, +jitter].
			jitterOffset := time.Duration(rand.Int63n(int64(jitter*2)+1)) - jitter //nolint:gosec
			delay = baseDelay + jitterOffset
		} else {
			delay = baseDelay
		}
		if delay < 0 {
			delay = 0
		}
	}

	// -------------------------------------------------------------------------
	// 4. Sleep for the computed delay, interruptible on client disconnect.
	// -------------------------------------------------------------------------
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	h.Metrics.SimulatedDelay.Observe(delay.Seconds())

	// -------------------------------------------------------------------------
	// 5. Generate response body.
	// -------------------------------------------------------------------------
	responseBody := h.Body.Generate(bodySize, bodyType)

	// -------------------------------------------------------------------------
	// 6. Set response headers.
	// -------------------------------------------------------------------------
	hdr := w.Header()

	if forceClose {
		hdr.Set("Connection", "close")
	}
	if chunked {
		hdr.Set("Transfer-Encoding", "chunked")
	}

	hdr.Set("X-Request-Id", r.Header.Get("X-Request-Id"))
	hdr.Set("X-Simulated-Delay", delay.String())
	hdr.Set("X-Body-Size", strconv.FormatInt(bodySize, 10))
	hdr.Set("X-Timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	hdr.Set("Content-Type", h.Body.ContentType(bodyType))

	// Gzip: only if client accepts it.
	useGzip := compress && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
	if useGzip {
		hdr.Set("Content-Encoding", "gzip")
	}

	// -------------------------------------------------------------------------
	// 7. Write status code.
	// -------------------------------------------------------------------------
	w.WriteHeader(statusCode)

	// -------------------------------------------------------------------------
	// 8. Write body — chunked or plain, with optional gzip wrapper.
	// -------------------------------------------------------------------------
	if chunked {
		flusher, ok := w.(http.Flusher)
		if !ok {
			// Fallback: write everything at once.
			if err := writeBody(w, responseBody, useGzip); err != nil {
				log.Ctx(r.Context()).Warn().Err(err).Msg("failed to write response")
				return
			}
			return
		}
		if err := writeChunked(w, r, flusher, responseBody, chunks, chunkDelay, useGzip); err != nil {
			log.Ctx(r.Context()).Warn().Err(err).Msg("failed to write chunked response")
			return
		}
		return
	}

	if err := writeBody(w, responseBody, useGzip); err != nil {
		log.Ctx(r.Context()).Warn().Err(err).Msg("failed to write response")
		return
	}
}

// writeBody writes data to w, optionally gzip-compressed.
// Returns an error if the write or gzip operations fail.
func writeBody(w io.Writer, data []byte, useGzip bool) error {
	if !useGzip {
		_, err := w.Write(data)
		return err
	}

	gz := gzip.NewWriter(w)
	if _, err := gz.Write(data); err != nil {
		_ = gz.Close() // best-effort close
		return err
	}
	return gz.Close()
}

// writeChunked splits data into n equal chunks, writing and flushing each
// one, sleeping chunkDelay between chunks. It honours context cancellation
// between chunks. When useGzip is true each chunk is compressed independently
// (each chunk is a valid gzip stream).
// Returns an error if any write, flush, or compression operation fails.
func writeChunked(w http.ResponseWriter, r *http.Request, flusher http.Flusher, data []byte, n int, chunkDelay time.Duration, useGzip bool) error {
	total := len(data)
	chunkSize := total / n
	if chunkSize < 1 {
		chunkSize = 1
	}

	for i := 0; i < n; i++ {
		start := i * chunkSize
		if start >= total {
			break
		}
		end := start + chunkSize
		if i == n-1 || end > total {
			end = total
		}

		if err := writeBody(w, data[start:end], useGzip); err != nil {
			return err
		}

		if err := flushWithContext(flusher, r.Context()); err != nil {
			return err
		}

		if i < n-1 && chunkDelay > 0 {
			select {
			case <-time.After(chunkDelay):
			case <-r.Context().Done():
				return r.Context().Err()
			}
		}
	}
	return nil
}

// flushWithContext flushes the http.Flusher, returning an error if flush fails.
// If the context is cancelled during the flush, returns the context error.
func flushWithContext(flusher http.Flusher, ctx context.Context) error {
	// Note: http.Flusher.Flush() does not return an error in the standard library,
	// but we follow the signature for consistency and potential future extensions.
	// If a write fails during Flush, it will manifest as a broken pipe or similar
	// error on the next write attempt.
	flusher.Flush()
	return ctx.Err()
}
