package body

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// ParseSize
// ---------------------------------------------------------------------------

func TestParseSize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		// default
		{"", 256, false},
		// raw integer
		{"0", 0, false},
		{"1024", 1024, false},
		// kb suffix
		{"1kb", 1024, false},
		{"1KB", 1024, false},
		{"1k", 1024, false},
		{"1K", 1024, false},
		// mb suffix
		{"2mb", 2 * 1024 * 1024, false},
		{"2MB", 2 * 1024 * 1024, false},
		{"2m", 2 * 1024 * 1024, false},
		// whitespace trimming
		{"  256  ", 256, false},
		// errors
		{"200mb", 0, true}, // exceeds 100 MB
		{"abc", 0, true},   // unparseable
		{"-1", 0, true},    // negative
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := ParseSize(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseSize(%q) = %d, nil; want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSize(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ParseSize(%q) = %d; want %d", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Generate – size correctness
// ---------------------------------------------------------------------------

func TestGenerate_SizeCorrectness(t *testing.T) {
	t.Parallel()

	g := New()

	for _, bt := range []BodyType{Text, Binary, Zeros} {
		bt := bt
		t.Run(string(bt), func(t *testing.T) {
			t.Parallel()
			for _, size := range []int64{0, 1, 256, 1024, 4 * 1024} {
				size := size
				t.Run("", func(t *testing.T) {
					t.Parallel()
					got := g.Generate(size, bt)
					if size == 0 {
						if len(got) != 0 {
							t.Fatalf("Generate(0, %s) returned %d bytes; want 0", bt, len(got))
						}
						return
					}
					if int64(len(got)) != size {
						t.Fatalf("Generate(%d, %s) returned %d bytes; want %d", size, bt, len(got), size)
					}
				})
			}
		})
	}
}

func TestGenerate_JSONSizeApproximate(t *testing.T) {
	t.Parallel()

	g := New()
	sizes := []int64{256, 1024, 10 * 1024, 64 * 1024}

	for _, size := range sizes {
		size := size
		t.Run("", func(t *testing.T) {
			t.Parallel()
			got := g.Generate(size, JSON)
			delta := int64(len(got)) - size
			if delta < -10 || delta > 10 {
				t.Fatalf("Generate(%d, json) returned %d bytes; delta %d exceeds ±10", size, len(got), delta)
			}
			// Must be valid JSON
			var v map[string]interface{}
			if err := json.Unmarshal(got, &v); err != nil {
				t.Fatalf("Generate(%d, json) returned invalid JSON: %v", size, err)
			}
		})
	}
}

func TestGenerate_ZeroSize(t *testing.T) {
	t.Parallel()

	g := New()
	for _, bt := range []BodyType{Text, JSON, Binary, Zeros} {
		bt := bt
		t.Run(string(bt), func(t *testing.T) {
			t.Parallel()
			// Must not panic and must return empty-like slice.
			got := g.Generate(0, bt)
			if len(got) != 0 {
				t.Fatalf("Generate(0, %s) returned %d bytes; want 0", bt, len(got))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Generate – Zeros body type content check
// ---------------------------------------------------------------------------

func TestGenerate_ZerosAllZero(t *testing.T) {
	t.Parallel()

	g := New()
	got := g.Generate(512, Zeros)
	for i, b := range got {
		if b != 0 {
			t.Fatalf("byte at index %d is %d; want 0", i, b)
		}
	}
}

// ---------------------------------------------------------------------------
// ContentType
// ---------------------------------------------------------------------------

func TestContentType(t *testing.T) {
	t.Parallel()

	g := New()
	cases := []struct {
		bt   BodyType
		want string
	}{
		{Text, "text/plain; charset=utf-8"},
		{JSON, "application/json"},
		{Binary, "application/octet-stream"},
		{Zeros, "application/octet-stream"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.bt), func(t *testing.T) {
			t.Parallel()
			got := g.ContentType(tc.bt)
			if got != tc.want {
				t.Fatalf("ContentType(%s) = %q; want %q", tc.bt, got, tc.want)
			}
		})
	}
}
