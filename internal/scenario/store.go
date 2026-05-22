package scenario

import (
	"net/url"
	"sync"
)

// Scenario is a named set of /respond query parameters.
type Scenario struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Params      map[string]string `json:"params"`
}

// Store is a concurrency-safe in-memory map of scenarios.
type Store struct {
	mu    sync.RWMutex
	items map[string]Scenario
}

// New returns an initialised, empty Store.
func New() *Store {
	return &Store{
		items: make(map[string]Scenario),
	}
}

// cloneScenario creates a deep copy of a Scenario by cloning its Params map.
func cloneScenario(sc Scenario) Scenario {
	if sc.Params == nil {
		return sc
	}
	// Deep copy the Params map to prevent aliasing.
	clonedParams := make(map[string]string, len(sc.Params))
	for k, v := range sc.Params {
		clonedParams[k] = v
	}
	sc.Params = clonedParams
	return sc
}

// Set stores a scenario, overwriting any existing entry with the same ID.
// The Params map is cloned to prevent external modifications from affecting the stored scenario.
func (s *Store) Set(sc Scenario) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[sc.ID] = cloneScenario(sc)
}

// Get retrieves a scenario by ID. Returns false if not found.
// The Params map is cloned to prevent external modifications from affecting the stored scenario.
func (s *Store) Get(id string) (Scenario, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sc, ok := s.items[id]
	if !ok {
		return sc, ok
	}
	return cloneScenario(sc), ok
}

// Params returns the scenario's params as url.Values for merging into a
// request query. Each map value becomes a single-element slice.
// Returns false if the scenario is not found.
func (s *Store) Params(id string) (url.Values, bool) {
	sc, ok := s.Get(id)
	if !ok {
		return nil, false
	}
	vals := make(url.Values, len(sc.Params))
	for k, v := range sc.Params {
		vals[k] = []string{v}
	}
	return vals, true
}
