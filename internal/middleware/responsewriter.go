package middleware

import "net/http"

// ResponseWriter wraps http.ResponseWriter to capture the status code and
// number of bytes written. It is safe to use with http.Flusher.
type ResponseWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int64
	wroteHeader  bool
}

// WrapResponseWriter returns a new ResponseWriter wrapping w.
func WrapResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{ResponseWriter: w}
}

func (rw *ResponseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.status = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *ResponseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// Flush implements http.Flusher if the underlying writer supports it.
func (rw *ResponseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Status returns the HTTP status code written, or 200 if WriteHeader was never called.
func (rw *ResponseWriter) Status() int {
	if rw.status == 0 {
		return http.StatusOK
	}
	return rw.status
}

// BytesWritten returns the total bytes written to the response body.
func (rw *ResponseWriter) BytesWritten() int64 { return rw.bytesWritten }
