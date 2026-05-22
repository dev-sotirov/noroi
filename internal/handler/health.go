package handler

import (
	"net/http"
	"sync/atomic"
)

var (
	healthBody   = []byte(`{"status":"ok"}` + "\n")
	readyBody    = []byte(`{"status":"ready"}` + "\n")
	startingBody = []byte(`{"status":"starting"}` + "\n")
)

// Health handles GET /health — always returns 200 OK.
type Health struct{}

func (h *Health) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(healthBody) //nolint:errcheck
}

// Ready handles GET /ready — returns 503 until MarkReady is called.
type Ready struct {
	ready atomic.Bool
}

func (rd *Ready) MarkReady() { rd.ready.Store(true) }

func (rd *Ready) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !rd.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write(startingBody) //nolint:errcheck
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(readyBody) //nolint:errcheck
}
