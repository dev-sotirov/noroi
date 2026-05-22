package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

func TestHealth_AlwaysOK(t *testing.T) {
	t.Parallel()

	h := &Health{}
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/health", nil)
		h.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("iteration %d: status = %d; want 200", i, w.Code)
		}

		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("iteration %d: invalid JSON: %v", i, err)
		}
		if body["status"] != "ok" {
			t.Fatalf("iteration %d: status field = %q; want %q", i, body["status"], "ok")
		}
	}
}

func TestHealth_ContentTypeJSON(t *testing.T) {
	t.Parallel()

	h := &Health{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	h.ServeHTTP(w, r)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("Content-Type = %q; want %q", ct, "application/json")
	}
}

// ---------------------------------------------------------------------------
// Ready
// ---------------------------------------------------------------------------

func TestReady_Before503(t *testing.T) {
	t.Parallel()

	rd := &Ready{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rd.ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("before MarkReady: status = %d; want 503", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "starting" {
		t.Fatalf("status field = %q; want %q", body["status"], "starting")
	}
}

func TestReady_After200(t *testing.T) {
	t.Parallel()

	rd := &Ready{}
	rd.MarkReady()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rd.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("after MarkReady: status = %d; want 200", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "ready" {
		t.Fatalf("status field = %q; want %q", body["status"], "ready")
	}
}

func TestReady_TransitionFromStartingToReady(t *testing.T) {
	t.Parallel()

	rd := &Ready{}

	// First call: should be 503.
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rd.ServeHTTP(w1, r1)
	if w1.Code != http.StatusServiceUnavailable {
		t.Fatalf("before MarkReady: status = %d; want 503", w1.Code)
	}

	rd.MarkReady()

	// Second call: should be 200.
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rd.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("after MarkReady: status = %d; want 200", w2.Code)
	}
}
