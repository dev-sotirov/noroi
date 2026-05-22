package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimit returns a middleware that enforces a token-bucket rate limit.
//
// It maintains one limiter per client IP so that a single aggressive client
// cannot starve others. A global limiter caps the total server-wide throughput.
// Per-IP limiters are evicted after their TTL to prevent unbounded memory growth.
//
//   - rps            — steady-state requests per second (token refill rate)
//   - burst          — maximum burst above the steady-state rate
//   - ttl            — how long to keep a limiter without activity
//   - trustForwarded — whether to trust X-Forwarded-For / X-Real-IP headers
//
// Requests that exceed the limit receive HTTP 429 Too Many Requests with a
// JSON body consistent with the rest of the API error envelope.
func RateLimit(rps float64, burst int, ttl time.Duration, trustForwarded bool) func(http.Handler) http.Handler {
	global := rate.NewLimiter(rate.Limit(rps), burst)
	perIP := &ipLimiterMap{
		limiters:       make(map[netip.Addr]ipLimiter),
		rps:            rate.Limit(rps),
		burst:          burst,
		ttl:            ttl,
		maxIPs:         10_000, // prevent unbounded growth
		trustForwarded: trustForwarded,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Global limit first — cheapest check.
			if !global.Allow() {
				writeTooManyRequests(w, r)
				return
			}

			// Per-IP limit — isolates misbehaving clients.
			addr := clientIP(r, trustForwarded)
			// Zero address indicates an unparseable IP; reject it.
			if addr == (netip.Addr{}) {
				writeTooManyRequests(w, r)
				return
			}
			if !perIP.allow(addr) {
				writeTooManyRequests(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ipLimiter wraps a rate limiter with a last-seen timestamp for TTL eviction.
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// ipLimiterMap holds one rate.Limiter per unique client IP with TTL-based eviction.
type ipLimiterMap struct {
	mu             sync.Mutex
	limiters       map[netip.Addr]ipLimiter
	rps            rate.Limit
	burst          int
	ttl            time.Duration
	maxIPs         int
	trustForwarded bool
}

// allow checks the rate limit for the given IP, evicting expired entries as needed.
// Returns false if the IP is rate-limited or if max IP count is reached with no evictable entries.
func (m *ipLimiterMap) allow(addr netip.Addr) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	il, ok := m.limiters[addr]
	if !ok {
		// Check if we've hit the max IP limit; if so, evict expired entries.
		if len(m.limiters) >= m.maxIPs {
			m.evictExpired(now)
		}
		// If still at max, reject the request.
		if len(m.limiters) >= m.maxIPs {
			return false
		}
		il = ipLimiter{
			limiter:  rate.NewLimiter(m.rps, m.burst),
			lastSeen: now,
		}
	} else {
		il.lastSeen = now
	}

	m.limiters[addr] = il
	return il.limiter.Allow()
}

// evictExpired removes limiters that haven't been used within the TTL.
// Must be called with mu held.
func (m *ipLimiterMap) evictExpired(now time.Time) {
	deadline := now.Add(-m.ttl)
	for addr, il := range m.limiters {
		if il.lastSeen.Before(deadline) {
			delete(m.limiters, addr)
		}
	}
}

// clientIP extracts the client IP from the request.
// If trustForwarded is true, it prefers X-Forwarded-For / X-Real-IP; otherwise, it always uses RemoteAddr.
// Returns a zero IP if the address cannot be parsed.
func clientIP(r *http.Request, trustForwarded bool) netip.Addr {
	var candidates []string

	if trustForwarded {
		// X-Forwarded-For is "ip1, ip2, ..." — take the first.
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			for i := range xff {
				if xff[i] == ',' {
					candidates = append(candidates, xff[:i])
					break
				}
			}
			if len(candidates) == 0 {
				candidates = append(candidates, xff)
			}
		}

		// X-Real-IP is a single IP.
		if xri := r.Header.Get("X-Real-Ip"); xri != "" {
			candidates = append(candidates, xri)
		}
	}

	// RemoteAddr is "host:port" — try parsing as AddrPort first, then as Addr.
	if addr, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		return addr.Addr()
	}
	if addr, err := netip.ParseAddr(r.RemoteAddr); err == nil {
		return addr
	}

	// Fallback: try trusted candidates if no valid RemoteAddr.
	for _, c := range candidates {
		if addr, err := netip.ParseAddrPort(c); err == nil {
			return addr.Addr()
		}
		if addr, err := netip.ParseAddr(c); err == nil {
			return addr
		}
	}

	// If RemoteAddr is a hostname, try SplitHostPort and parse the host part.
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if addr, err := netip.ParseAddr(host); err == nil {
			return addr
		}
	}

	// Return zero address to signal an unparseable IP.
	return netip.Addr{}
}

func writeTooManyRequests(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "1")
	w.WriteHeader(http.StatusTooManyRequests)
	requestID := r.Header.Get("X-Request-Id")
	body := `{"error":true,"code":429,"message":"rate limit exceeded","request_id":"` + requestID + `"}`
	w.Write([]byte(body)) //nolint:errcheck
}
