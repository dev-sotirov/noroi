package handler

import (
	"encoding/json"
	"net/http"
)

// errorResponse is the JSON envelope returned for all handler errors.
type errorResponse struct {
	Error     bool   `json:"error"`
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteError writes a JSON error response with the given status code and message.
// It extracts the X-Request-ID header if present.
func WriteError(w http.ResponseWriter, r *http.Request, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	resp := errorResponse{
		Error:     true,
		Code:      code,
		Message:   msg,
		RequestID: r.Header.Get("X-Request-Id"),
	}
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}
