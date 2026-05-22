package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dev-sotirov/noroi/internal/body"
)

// Stream handles the /stream endpoint. It sends a response in N chunks with a
// configurable delay between each chunk.
type Stream struct {
	Body *body.Generator
}

// ServeHTTP implements http.Handler.
func (h *Stream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// --- chunks ---
	chunks := 10
	if raw := q.Get("chunks"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			WriteError(w, r, http.StatusBadRequest,
				fmt.Sprintf("invalid chunks %q: must be an integer >= 1", raw))
			return
		}
		chunks = n
	}

	// --- chunk_size ---
	chunkSizeStr := q.Get("chunk_size")
	if chunkSizeStr == "" {
		chunkSizeStr = "256"
	}
	chunkSize, err := body.ParseSize(chunkSizeStr)
	if err != nil {
		WriteError(w, r, http.StatusBadRequest,
			fmt.Sprintf("invalid chunk_size: %s", err))
		return
	}

	// --- chunk_delay ---
	chunkDelay := 100 * time.Millisecond
	if raw := q.Get("chunk_delay"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			WriteError(w, r, http.StatusBadRequest,
				fmt.Sprintf("invalid chunk_delay %q: %s", raw, err))
			return
		}
		chunkDelay = d
	}

	// --- status ---
	status := http.StatusOK
	if raw := q.Get("status"); raw != "" {
		code, err := strconv.Atoi(raw)
		if err != nil || code < 100 || code > 599 {
			WriteError(w, r, http.StatusBadRequest,
				fmt.Sprintf("invalid status %q: must be an integer between 100 and 599", raw))
			return
		}
		status = code
	}

	// --- response ---
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)

	flusher, canFlush := w.(http.Flusher)
	ctx := r.Context()

	for i := 0; i < chunks; i++ {
		chunk := h.Body.Generate(chunkSize, body.Text)
		w.Write(chunk) //nolint:errcheck // client disconnect handled via context

		if canFlush {
			flusher.Flush()
		}

		// Sleep between chunks, but wake early on context cancellation.
		// Skip the sleep after the last chunk.
		if i < chunks-1 {
			select {
			case <-time.After(chunkDelay):
			case <-ctx.Done():
				return
			}
		}
	}
}
