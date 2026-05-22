package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"
)

const echoBodyLimit = 64 * 1024 // 64 KB

// Echo handles GET /echo — returns a JSON description of the incoming request.
type Echo struct{}

type echoResponse struct {
	Method        string            `json:"method"`
	Path          string            `json:"path"`
	Query         map[string]string `json:"query"`
	Headers       map[string]string `json:"headers"`
	RemoteAddr    string            `json:"remote_addr"`
	ContentLength int64             `json:"content_length"`
	Body          string            `json:"body"`
	TimestampMs   int64             `json:"timestamp_ms"`
	ServerID      string            `json:"server_id"`
}

func (h *Echo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// query — first value per key
	query := make(map[string]string, len(r.URL.Query()))
	for k, vals := range r.URL.Query() {
		if len(vals) > 0 {
			query[k] = vals[0]
		}
	}

	// headers — first value per key
	headers := make(map[string]string, len(r.Header))
	for k, vals := range r.Header {
		if len(vals) > 0 {
			headers[k] = vals[0]
		}
	}

	// body — up to 64 KB; truncate if larger
	var bodyStr string
	if r.Body != nil {
		limited := io.LimitReader(r.Body, echoBodyLimit+1)
		raw, _ := io.ReadAll(limited)
		if len(raw) > echoBodyLimit {
			bodyStr = string(raw[:echoBodyLimit]) + "...<truncated>"
		} else {
			bodyStr = string(raw)
		}
	}

	// server_id from hostname; fallback to "noroi"
	serverID, err := os.Hostname()
	if err != nil || serverID == "" {
		serverID = "noroi"
	}

	resp := echoResponse{
		Method:        r.Method,
		Path:          r.URL.Path,
		Query:         query,
		Headers:       headers,
		RemoteAddr:    r.RemoteAddr,
		ContentLength: r.ContentLength,
		Body:          bodyStr,
		TimestampMs:   time.Now().UnixMilli(),
		ServerID:      serverID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}
