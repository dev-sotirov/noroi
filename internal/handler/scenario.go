package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/dev-sotirov/noroi/internal/scenario"
)

// ScenarioSave handles POST /scenario — stores a named scenario.
type ScenarioSave struct {
	Store *scenario.Store
}

type scenarioSaveResponse struct {
	ID        string `json:"id"`
	ReplayURL string `json:"replay_url"`
}

// ServeHTTP decodes the request body, validates it, and persists the scenario.
func (h *ScenarioSave) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Use MaxBytesReader for explicit 413 response on oversized bodies
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer func() { _ = r.Body.Close() }()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var sc scenario.Scenario
	if err := dec.Decode(&sc); err != nil {
		WriteError(w, r, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Verify no trailing JSON after the first object
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		WriteError(w, r, http.StatusBadRequest, "request body must contain one JSON object")
		return
	}

	if sc.ID == "" {
		WriteError(w, r, http.StatusBadRequest, "id is required")
		return
	}

	h.Store.Set(sc)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(scenarioSaveResponse{
		ID:        sc.ID,
		ReplayURL: "/scenario/" + sc.ID,
	}); err != nil {
		log.Ctx(r.Context()).Warn().Err(err).Msg("failed to encode scenario response")
		return
	}
}

// ScenarioReplay handles GET /scenario/:id — replays a stored scenario by
// merging its params into the request query and delegating to Respond.
type ScenarioReplay struct {
	Store   *scenario.Store
	Respond *Respond
}

// ServeHTTP looks up the scenario, merges params with request query params
// (request params take priority), and delegates to the Respond handler.
func (h *ScenarioReplay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	storedParams, ok := h.Store.Params(id)
	if !ok {
		WriteError(w, r, http.StatusNotFound, "scenario not found: "+id)
		return
	}

	merged := make(url.Values, len(storedParams))
	for k, v := range storedParams {
		merged[k] = v
	}
	for k, v := range r.URL.Query() {
		merged[k] = v // request params override stored params
	}

	r2 := r.Clone(r.Context())
	r2.URL = r.URL.JoinPath() // shallow copy of the URL
	r2.URL.RawQuery = merged.Encode()

	h.Respond.ServeHTTP(w, r2)
}
