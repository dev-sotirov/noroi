package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// RequestID is a middleware that ensures every request carries an X-Request-Id
// header. If the incoming request already has one it is reused; otherwise a
// random 16-character hex ID is generated.
//
// The ID is propagated by:
//   - Setting it on a cloned *http.Request so downstream handlers can read it
//     via r.Header.Get("X-Request-Id").
//   - Setting it on the response writer so it appears in the response headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			var b [8]byte
			if _, err := rand.Read(b[:]); err != nil {
				// Extremely unlikely; fall back to a zero-value ID rather than
				// failing the request.
				id = "0000000000000000"
			} else {
				id = hex.EncodeToString(b[:])
			}
		}

		// Clone the request to avoid mutating the caller's header map.
		r2 := r.Clone(r.Context())
		r2.Header.Set("X-Request-Id", id)

		// Expose on the response so clients can correlate requests.
		w.Header().Set("X-Request-Id", id)

		next.ServeHTTP(w, r2)
	})
}
