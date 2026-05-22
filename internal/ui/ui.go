package ui

import (
	_ "embed"
	"net/http"
)

//go:embed static/index.html
var indexHTML []byte

// Handler serves the embedded single-page dashboard at GET /ui.
type Handler struct{}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(indexHTML) //nolint:errcheck
}
