package middleware

import (
	"net/http"

	"github.com/go-chi/cors"
)

// CORS returns middleware that applies a strict whitelist policy. An empty
// allowedOrigins list disables cross-origin entirely (same-origin /web/ still
// works because the browser doesn't send Origin for same-origin navigation).
//
// Important behavioural quirk of go-chi/cors: an empty AllowedOrigins slice
// combined with a nil AllowOriginFunc is normalised to "allow all" (*). We
// guard against that by installing a deny-all AllowOriginFunc whenever the
// whitelist is empty — safer default for a service whose write endpoints
// might otherwise become trivially CSRF-able from any origin.
//
// AllowCredentials is intentionally false: avatar reads are public and writes
// are authenticated via the X-User-ID header rather than a session cookie,
// so we never need the browser to attach credentials cross-origin.
//
// MaxAge caches the preflight result for 5 minutes — long enough to avoid
// chatter on a busy page, short enough to pick up policy changes without
// requiring users to clear their cache.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	opts := cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Content-Type", "X-User-ID"},
		ExposedHeaders:   []string{},
		AllowCredentials: false,
		MaxAge:           300,
	}
	if len(allowedOrigins) == 0 {
		opts.AllowOriginFunc = func(_ *http.Request, _ string) bool { return false }
	}
	return cors.Handler(opts)
}
