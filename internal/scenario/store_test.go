package scenario

import (
	"fmt"
	"sync"
	"testing"
)

func TestStore_GetEmptyStore(t *testing.T) {
	t.Parallel()

	s := New()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Fatal("Get on empty store returned true; want false")
	}
}

func TestStore_SetThenGet(t *testing.T) {
	t.Parallel()

	s := New()
	sc := Scenario{
		ID:          "test-1",
		Description: "a test scenario",
		Params:      map[string]string{"status": "503", "delay": "100ms"},
	}
	s.Set(sc)

	got, ok := s.Get("test-1")
	if !ok {
		t.Fatal("Get returned false after Set")
	}
	if got.ID != sc.ID {
		t.Fatalf("ID = %q; want %q", got.ID, sc.ID)
	}
	if got.Description != sc.Description {
		t.Fatalf("Description = %q; want %q", got.Description, sc.Description)
	}
	if len(got.Params) != len(sc.Params) {
		t.Fatalf("len(Params) = %d; want %d", len(got.Params), len(sc.Params))
	}
	for k, v := range sc.Params {
		if got.Params[k] != v {
			t.Fatalf("Params[%q] = %q; want %q", k, got.Params[k], v)
		}
	}
}

func TestStore_SetOverwrites(t *testing.T) {
	t.Parallel()

	s := New()
	s.Set(Scenario{ID: "x", Description: "first"})
	s.Set(Scenario{ID: "x", Description: "second"})

	got, ok := s.Get("x")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.Description != "second" {
		t.Fatalf("Description = %q; want %q", got.Description, "second")
	}
}

func TestStore_GetMissing(t *testing.T) {
	t.Parallel()

	s := New()
	s.Set(Scenario{ID: "present"})

	_, ok := s.Get("absent")
	if ok {
		t.Fatal("Get for absent key returned true")
	}
}

func TestStore_Params(t *testing.T) {
	t.Parallel()

	s := New()
	sc := Scenario{
		ID: "p1",
		Params: map[string]string{
			"status": "429",
			"size":   "1kb",
		},
	}
	s.Set(sc)

	vals, ok := s.Params("p1")
	if !ok {
		t.Fatal("Params returned false for existing scenario")
	}
	for k, v := range sc.Params {
		got := vals.Get(k)
		if got != v {
			t.Fatalf("Params()[%q] = %q; want %q", k, got, v)
		}
		// Each value must be a single-element slice.
		if len(vals[k]) != 1 {
			t.Fatalf("Params()[%q] slice len = %d; want 1", k, len(vals[k]))
		}
	}
}

func TestStore_ParamsMissing(t *testing.T) {
	t.Parallel()

	s := New()
	_, ok := s.Params("nope")
	if ok {
		t.Fatal("Params returned true for missing scenario")
	}
}

func TestStore_ConcurrentSetGet(t *testing.T) {
	t.Parallel()

	s := New()
	const goroutines = 20
	const ops = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				id := fmt.Sprintf("sc-%d", i%5) // shared IDs to increase contention
				s.Set(Scenario{ID: id, Description: fmt.Sprintf("goroutine %d op %d", i, j)})
				s.Get(id)
				s.Params(id)
			}
		}()
	}

	wg.Wait()
}

func TestStore_GetDoesNotAliasParams(t *testing.T) {
	t.Parallel()

	s := New()
	sc := Scenario{
		ID:     "test-alias",
		Params: map[string]string{"key": "original"},
	}
	s.Set(sc)

	// Get the scenario and modify its params
	got, _ := s.Get("test-alias")
	got.Params["key"] = "modified"
	got.Params["newkey"] = "newvalue"

	// Get it again and verify the stored scenario wasn't modified
	got2, _ := s.Get("test-alias")
	if got2.Params["key"] != "original" {
		t.Fatalf("Stored scenario was modified! Params[key] = %q; want %q", got2.Params["key"], "original")
	}
	if _, exists := got2.Params["newkey"]; exists {
		t.Fatal("Stored scenario now contains newkey that should not exist")
	}
}

func TestStore_SetDoesNotAliasParams(t *testing.T) {
	t.Parallel()

	s := New()
	sc := Scenario{
		ID:     "test-set-alias",
		Params: map[string]string{"key": "original"},
	}
	s.Set(sc)

	// Modify the original scenario's params
	sc.Params["key"] = "modified"
	sc.Params["newkey"] = "newvalue"

	// Get the stored scenario and verify it wasn't modified
	got, _ := s.Get("test-set-alias")
	if got.Params["key"] != "original" {
		t.Fatalf("Stored scenario was modified! Params[key] = %q; want %q", got.Params["key"], "original")
	}
	if _, exists := got.Params["newkey"]; exists {
		t.Fatal("Stored scenario now contains newkey that should not exist")
	}
}
