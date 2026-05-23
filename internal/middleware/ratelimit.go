package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"

	"github.com/avc-dev/go-avatar-service/internal/httpx"
)

// rateLimitWindow is the bucket refill window for both limiters. Token-bucket
// limiters express their budget as "N requests per WINDOW"; we hard-code one
// minute so the requests-per-minute config values read naturally end-to-end
// (env, code, logs).
const rateLimitWindow = time.Minute

// IPRateLimit returns middleware that throttles by remote IP. It is intended
// for public read endpoints where there is no authenticated user to attribute
// requests to. requestsPerMinute<=0 disables the limiter (no-op middleware),
// useful for local dev or e2e tests where the throttle gets in the way.
func IPRateLimit(requestsPerMinute int) func(http.Handler) http.Handler {
	if requestsPerMinute <= 0 {
		return passthrough
	}
	return httprate.Limit(
		requestsPerMinute,
		rateLimitWindow,
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(limitExceededHandler),
	)
}

// UserIDRateLimit returns middleware that throttles by the X-User-ID header.
// It MUST be mounted after the UserID middleware so the header has already
// been validated as a UUID v7 — this keeper just reads the trusted string.
// Requests without the header fall back to KeyByIP so an unauthenticated
// caller can't bypass the limiter by omitting it.
func UserIDRateLimit(requestsPerMinute int) func(http.Handler) http.Handler {
	if requestsPerMinute <= 0 {
		return passthrough
	}
	return httprate.Limit(
		requestsPerMinute,
		rateLimitWindow,
		httprate.WithKeyFuncs(keyByUserIDOrIP),
		httprate.WithLimitHandler(limitExceededHandler),
	)
}

// keyByUserIDOrIP prefers the X-User-ID header (set by the UserID middleware)
// and falls back to the remote IP if the header is absent. Returning an error
// from the key func short-circuits httprate's chain, so we always return nil
// here — KeyByIP is itself robust to malformed RemoteAddr.
func keyByUserIDOrIP(r *http.Request) (string, error) {
	if uid := r.Header.Get("X-User-ID"); uid != "" {
		return "user:" + uid, nil
	}
	return httprate.KeyByIP(r)
}

// limitExceededHandler renders the 429 response in our project-wide JSON
// error shape rather than httprate's default plain-text body. Retry-After
// hints at the bucket refill window so well-behaved clients back off.
func limitExceededHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Retry-After", "60")
	httpx.WriteError(w, http.StatusTooManyRequests, "Rate limit exceeded")
}

// passthrough is the no-op middleware used when a limiter is disabled by
// config. Declared as a package var so the two constructors share one closure.
func passthrough(next http.Handler) http.Handler { return next }
