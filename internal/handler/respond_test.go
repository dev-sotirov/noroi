package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dev-sotirov/noroi/internal/body"
	"github.com/dev-sotirov/noroi/internal/config"
	"github.com/dev-sotirov/noroi/internal/metrics"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newRespondHandler(t *testing.T) *Respond {
	t.Helper()
	reg, err := metrics.New()
	if err != nil {
		t.Fatalf("metrics.New: %v", err)
	}
	return &Respond{
		Defaults: &config.DefaultsConfig{
			StatusCode: 200,
			BodySize:   256,
			BodyType:   "text",
		},
		Body:    body.New(),
		Metrics: reg,
	}
}

func doRequest(t *testing.T, h *Respond, url string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, url, nil)
	h.ServeHTTP(w, r)
	return w
}

// ---------------------------------------------------------------------------
// Status code tests
// ---------------------------------------------------------------------------

func TestRespond_DefaultStatus200(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
}

func TestRespond_DefaultBodyLen(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond")
	if w.Body.Len() != 256 {
		t.Fatalf("body len = %d; want 256", w.Body.Len())
	}
}

func TestRespond_StatusOverride(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond?status=503")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", w.Code)
	}
}

func TestRespond_StatusInvalid(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond?status=abc")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
	assertJSONError(t, w)
}

// ---------------------------------------------------------------------------
// Body size tests
// ---------------------------------------------------------------------------

func TestRespond_SizeOverride1KB(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond?size=1kb")
	if w.Body.Len() != 1024 {
		t.Fatalf("body len = %d; want 1024", w.Body.Len())
	}
}

func TestRespond_SizeInvalidTooLarge(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond?size=200mb")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
	assertJSONError(t, w)
}

// ---------------------------------------------------------------------------
// Body type tests
// ---------------------------------------------------------------------------

func TestRespond_BodyTypeJSON(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond?size=10kb&body_type=json")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}

	got := w.Body.Bytes()
	var v map[string]interface{}
	if err := json.Unmarshal(got, &v); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}

	const target = 10 * 1024
	delta := len(got) - target
	if delta < -10 || delta > 10 {
		t.Fatalf("body len = %d; delta %d from %d exceeds ±10", len(got), delta, target)
	}
}

func TestRespond_BodyTypeZeros(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond?body_type=zeros&size=512")

	got := w.Body.Bytes()
	if len(got) != 512 {
		t.Fatalf("body len = %d; want 512", len(got))
	}
	for i, b := range got {
		if b != 0 {
			t.Fatalf("byte[%d] = %d; want 0", i, b)
		}
	}
}

func TestRespond_BodyTypeBinary(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond?body_type=binary&size=1kb")
	if w.Body.Len() != 1024 {
		t.Fatalf("body len = %d; want 1024", w.Body.Len())
	}
}

func TestRespond_BodyTypeInvalid(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond?body_type=nope")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
	assertJSONError(t, w)
}

// ---------------------------------------------------------------------------
// Error rate tests
// ---------------------------------------------------------------------------

func TestRespond_ErrorRateAlways(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	for i := 0; i < 10; i++ {
		w := doRequest(t, h, "/respond?error_rate=1.0")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("iteration %d: status = %d; want 500", i, w.Code)
		}
	}
}

func TestRespond_ErrorRateNever(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	for i := 0; i < 10; i++ {
		w := doRequest(t, h, "/respond?error_rate=0.0")
		if w.Code == http.StatusInternalServerError {
			t.Fatalf("iteration %d: unexpected 500 with error_rate=0.0", i)
		}
	}
}

func TestRespond_ErrorRateInvalid(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond?error_rate=2.0")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
	assertJSONError(t, w)
}

// ---------------------------------------------------------------------------
// Delay tests
// ---------------------------------------------------------------------------

func TestRespond_DelayFixed(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	start := time.Now()
	doRequest(t, h, "/respond?delay=100ms")
	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Fatalf("elapsed = %v; want >= 100ms", elapsed)
	}
}

func TestRespond_DelayRange(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	start := time.Now()
	doRequest(t, h, "/respond?delay_min=50ms&delay_max=150ms")
	elapsed := time.Since(start)

	// Lower bound: must have slept at least 50ms.
	// Upper bound: generous for CI (delay_max=150ms + scheduling slack).
	if elapsed < 50*time.Millisecond {
		t.Fatalf("elapsed = %v; want >= 50ms", elapsed)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("elapsed = %v; want <= 1s", elapsed)
	}
}

func TestRespond_DelayInvalid(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := doRequest(t, h, "/respond?delay=notaduration")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
	assertJSONError(t, w)
}

// ---------------------------------------------------------------------------
// Response headers
// ---------------------------------------------------------------------------

func TestRespond_ResponseHeaders(t *testing.T) {
	t.Parallel()

	h := newRespondHandler(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/respond", nil)
	// Inject a request ID so it gets echoed back.
	r.Header.Set("X-Request-Id", "test-req-123")
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}

	if v := w.Header().Get("X-Request-Id"); v == "" {
		t.Error("X-Request-Id header missing")
	}
	if v := w.Header().Get("X-Simulated-Delay"); v == "" {
		t.Error("X-Simulated-Delay header missing")
	}
	if v := w.Header().Get("X-Body-Size"); v == "" {
		t.Error("X-Body-Size header missing")
	}
	if v := w.Header().Get("X-Timestamp"); v == "" {
		t.Error("X-Timestamp header missing")
	}
}

// ---------------------------------------------------------------------------
// assertJSONError verifies that the response is a JSON error envelope.
// ---------------------------------------------------------------------------

func assertJSONError(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var resp struct {
		Error   bool   `json:"error"`
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v — body: %s", err, w.Body.String())
	}
	if !resp.Error {
		t.Fatalf("JSON error field = false; want true")
	}
	if resp.Code == 0 {
		t.Fatal("JSON code field is 0; want non-zero HTTP status")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkRespondDefault(b *testing.B) {
	reg, _ := metrics.New()
	h := &Respond{
		Defaults: &config.DefaultsConfig{StatusCode: 200, BodySize: 256, BodyType: "text"},
		Body:     body.New(),
		Metrics:  reg,
	}
	r := httptest.NewRequest(http.MethodGet, "/respond", nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
		}
	})
}

func BenchmarkRespond1KB(b *testing.B) {
	reg, _ := metrics.New()
	h := &Respond{
		Defaults: &config.DefaultsConfig{StatusCode: 200, BodySize: 256, BodyType: "text"},
		Body:     body.New(),
		Metrics:  reg,
	}
	r := httptest.NewRequest(http.MethodGet, "/respond?size=1kb", nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
		}
	})
}

func BenchmarkRespond10KB(b *testing.B) {
	reg, _ := metrics.New()
	h := &Respond{
		Defaults: &config.DefaultsConfig{StatusCode: 200, BodySize: 256, BodyType: "text"},
		Body:     body.New(),
		Metrics:  reg,
	}
	r := httptest.NewRequest(http.MethodGet, "/respond?size=10kb", nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
		}
	})
}

func BenchmarkRespondJSON10KB(b *testing.B) {
	reg, _ := metrics.New()
	h := &Respond{
		Defaults: &config.DefaultsConfig{StatusCode: 200, BodySize: 256, BodyType: "text"},
		Body:     body.New(),
		Metrics:  reg,
	}
	r := httptest.NewRequest(http.MethodGet, "/respond?size=10kb&body_type=json", nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
		}
	})
}

func BenchmarkRespondBinary10KB(b *testing.B) {
	reg, _ := metrics.New()
	h := &Respond{
		Defaults: &config.DefaultsConfig{StatusCode: 200, BodySize: 256, BodyType: "text"},
		Body:     body.New(),
		Metrics:  reg,
	}
	r := httptest.NewRequest(http.MethodGet, "/respond?size=10kb&body_type=binary", nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
		}
	})
}
