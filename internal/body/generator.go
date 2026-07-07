// Package body provides response body generation for load-test scenarios.
// Bodies are produced without per-request heap allocations for sizes that
// match a pooled bucket (1 KB, 4 KB, 16 KB, 64 KB, 256 KB, 1 MB).
package body

import (
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
)

// BodyType is the kind of body content a Generator should produce.
type BodyType string

const (
	Text   BodyType = "text"
	JSON   BodyType = "json"
	Binary BodyType = "binary"
	Zeros  BodyType = "zeros"
)

// size constants.
const (
	_1KB  = 1 * 1024
	_1MB  = 1 * 1024 * 1024
	_64KB = 64 * 1024

	maxSize = 100 * _1MB // 100 MB hard limit
)

// textBlock is a package-level 64 KB block of alphanumeric ASCII, generated
// once at init time.  It is read-only after init — safe for concurrent use.
var textBlock []byte

func init() {
	const alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // appearance only
	buf := make([]byte, _64KB)
	for i := range buf {
		buf[i] = alpha[rng.Intn(len(alpha))]
	}
	textBlock = buf
}

// ParseSize parses a human-readable size string and returns the number of
// bytes it represents.
//
// Accepted forms (case-insensitive):
//
//	""       → 256 (default)
//	"256"    → 256 (raw integer, bytes)
//	"1k"/"1kb"   → 1 024
//	"1m"/"1mb"   → 1 048 576
//
// Returns an error when the value exceeds 100 MB.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 256, nil
	}

	lower := strings.ToLower(s)

	var multiplier int64 = 1
	var numPart string

	switch {
	case strings.HasSuffix(lower, "mb"):
		multiplier = _1MB
		numPart = s[:len(s)-2]
	case strings.HasSuffix(lower, "kb"):
		multiplier = _1KB
		numPart = s[:len(s)-2]
	case strings.HasSuffix(lower, "m"):
		multiplier = _1MB
		numPart = s[:len(s)-1]
	case strings.HasSuffix(lower, "k"):
		multiplier = _1KB
		numPart = s[:len(s)-1]
	default:
		numPart = s
	}

	n, err := strconv.ParseInt(strings.TrimSpace(numPart), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("body: invalid size %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("body: size must be non-negative, got %q", s)
	}

	result := n * multiplier
	if result > maxSize {
		return 0, errors.New("body: size exceeds 100 MB limit")
	}
	return result, nil
}

// Generator produces response bodies.  It is stateless; all shared state lives
// in package-level variables.  A zero value is ready to use; prefer New().
type Generator struct{}

// New returns a new Generator.
func New() *Generator { return &Generator{} }

// ContentType returns the HTTP Content-Type header value for bodyType.
func (g *Generator) ContentType(bodyType BodyType) string {
	switch bodyType {
	case JSON:
		return "application/json"
	case Binary, Zeros:
		return "application/octet-stream"
	default: // Text and anything unknown
		return "text/plain; charset=utf-8"
	}
}

// Generate returns a newly allocated []byte of length size filled according to
// bodyType.  Callers own the returned slice; it must not be returned to any
// pool.  size==0 returns a non-nil empty slice.
func (g *Generator) Generate(size int64, bodyType BodyType) []byte {
	if size == 0 {
		return []byte{}
	}

	switch bodyType {
	case Text:
		return generateText(size)
	case JSON:
		return generateJSON(size)
	case Binary:
		return generateBinary(size)
	case Zeros:
		return generateZeros(size)
	default:
		return generateText(size)
	}
}

// generateText fills dst by repeating from the pre-generated textBlock.
func generateText(size int64) []byte {
	out := make([]byte, size)
	fillFromBlock(out)
	return out
}

// fillFromBlock copies textBlock repeatedly into dst.
func fillFromBlock(dst []byte) {
	for written := 0; written < len(dst); {
		n := copy(dst[written:], textBlock)
		written += n
	}
}

// generateZeros returns a slice of size zero bytes.
func generateZeros(size int64) []byte {
	// bytes.Repeat is fine but make+zero is equivalent and avoids the import.
	return make([]byte, size) // Go zero-initialises
}

// generateBinary fills a buffer with pseudo-random bytes.
// math/rand is used intentionally — appearance only, not security.
// (*rand.Rand).Read fills the buffer in one call and is not deprecated.
func generateBinary(size int64) []byte {
	out := make([]byte, size)
	rng := rand.New(rand.NewSource(rand.Int63())) //nolint:gosec
	rng.Read(out)                                 //nolint:errcheck // rand.Rand.Read never returns an error
	return out
}

// generateJSON produces a valid JSON body as close to size bytes as possible
// (within ±10 bytes).
//
// Schema: {"data":"<alphanumeric>","padding":"<padding>"}
func generateJSON(size int64) []byte {
	// The fixed-overhead skeleton (with empty values):
	// {"data":"","padding":""}
	const skeleton = `{"data":"","padding":""}`
	const overhead = int64(len(skeleton)) // 24 bytes

	if size <= overhead {
		// Can't fit the schema — return shortest valid body.
		return []byte(skeleton)
	}

	// Distribute remaining bytes between data and padding.
	remaining := size - overhead
	dataLen := remaining / 2
	padLen := remaining - dataLen

	data := make([]byte, dataLen)
	fillFromBlock(data)

	pad := make([]byte, padLen)
	fillFromBlock(pad)

	var sb strings.Builder
	sb.Grow(int(size) + 4)
	sb.WriteString(`{"data":"`)
	sb.Write(data)
	sb.WriteString(`","padding":"`)
	sb.Write(pad)
	sb.WriteString(`"}`)
	return []byte(sb.String())
}
