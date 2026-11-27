package http

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// verifiedSubjectKey is the request context key for the subject of a bearer
// token that AuthorizationMiddleware has validated.
type verifiedSubjectKey struct{}

// withVerifiedSubject returns ctx carrying subject. An empty subject leaves ctx unchanged.
func withVerifiedSubject(ctx context.Context, subject string) context.Context {
	if subject == "" {
		return ctx
	}
	return context.WithValue(ctx, verifiedSubjectKey{}, subject)
}

// verifiedSubject returns the subject stored by withVerifiedSubject, or "".
func verifiedSubject(ctx context.Context) string {
	subject, _ := ctx.Value(verifiedSubjectKey{}).(string)
	return subject
}

const (
	// clientLimiterIdleTTL is the minimum time an idle client's limiter is kept.
	clientLimiterIdleTTL = 10 * time.Minute
	// clientLimiterMaxIdleTTL caps how long an idle limiter is kept for very low limits.
	clientLimiterMaxIdleTTL = 24 * time.Hour
	// clientLimiterSweepInterval is how often idle limiters are removed.
	clientLimiterSweepInterval = time.Minute
)

// clientRateLimitKey identifies the client a request is counted against: the
// subject of a verified bearer token, otherwise the client IP address.
func clientRateLimitKey(r *http.Request, trustProxy bool) string {
	if subject := verifiedSubject(r.Context()); subject != "" {
		return "sub:" + subject
	}
	return "ip:" + getClientIP(r, trustProxy)
}

// ClientRateLimitMiddleware limits the MCP requests (POST to the MCP endpoint)
// each client can make, independently of how many MCP sessions it opens and
// also in stateless mode, where requests carry no session ID.
// Requests over the limit receive 429 Too Many Requests with Retry-After.
// http.client_rate_limit_rps and http.client_rate_limit_burst are read per
// request from cfgState so SIGHUP-reloaded values take effect immediately.
// It must run after AuthorizationMiddleware, which stores the verified subject.
func ClientRateLimitMiddleware(cfgState *config.ConfigState) func(http.Handler) http.Handler {
	limiters := newClientLimiters(time.Now)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != mcpEndpoint {
				next.ServeHTTP(w, r)
				return
			}
			cfg := cfgState.Load()
			limit := rate.Limit(cfg.HTTP.ClientRateLimitRPS.Get())
			if limit <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			burst := cfg.HTTP.ClientRateLimitBurst.Get()
			if burst == 0 {
				burst = config.DefaultRateLimitBurst
			}
			key := clientRateLimitKey(r, cfg.TrustProxyHeaders.Get())
			if wait, ok := limiters.allow(key, limit, burst); !ok {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(wait)))
				http.Error(w, "Too Many Requests: client rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// retryAfterSeconds rounds wait up to whole seconds, at least 1.
func retryAfterSeconds(wait time.Duration) int {
	return max(1, int(math.Ceil(wait.Seconds())))
}

// clientLimiters holds one token bucket per client key.
type clientLimiters struct {
	mu        sync.Mutex
	now       func() time.Time
	entries   map[string]*clientLimiterEntry
	limit     rate.Limit
	burst     int
	lastSweep time.Time
}

type clientLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newClientLimiters(now func() time.Time) *clientLimiters {
	return &clientLimiters{now: now, entries: make(map[string]*clientLimiterEntry)}
}

// allow takes one token from key's bucket. When none is available it returns
// false and how long until one is.
func (c *clientLimiters) allow(key string, limit rate.Limit, burst int) (time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	// New settings start every client with a full bucket at the new size.
	if limit != c.limit || burst != c.burst {
		clear(c.entries)
		c.limit, c.burst = limit, burst
	}
	if now.Sub(c.lastSweep) >= clientLimiterSweepInterval {
		c.sweep(now)
	}
	entry, ok := c.entries[key]
	if !ok {
		entry = &clientLimiterEntry{limiter: rate.NewLimiter(limit, burst)}
		c.entries[key] = entry
	}
	entry.lastSeen = now
	reservation := entry.limiter.ReserveN(now, 1)
	if !reservation.OK() {
		return time.Second, false
	}
	if wait := reservation.DelayFrom(now); wait > 0 {
		reservation.CancelAt(now)
		return wait, false
	}
	return 0, true
}

// sweep removes limiters that have been idle long enough to refill completely
// (capped at clientLimiterMaxIdleTTL), so dropping one gives the client nothing
// it would not have had anyway.
func (c *clientLimiters) sweep(now time.Time) {
	ttl := clientLimiterIdleTTL
	if refill := float64(c.burst) / float64(c.limit); refill > clientLimiterMaxIdleTTL.Seconds() {
		ttl = clientLimiterMaxIdleTTL
	} else if refill > ttl.Seconds() {
		ttl = time.Duration(refill * float64(time.Second))
	}
	for key, entry := range c.entries {
		if now.Sub(entry.lastSeen) > ttl {
			delete(c.entries, key)
		}
	}
	c.lastSweep = now
}
