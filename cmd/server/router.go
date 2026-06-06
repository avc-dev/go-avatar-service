package main

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/avc-dev/go-avatar-service/internal/config"
	"github.com/avc-dev/go-avatar-service/internal/handlers"
	handleravatar "github.com/avc-dev/go-avatar-service/internal/handlers/avatar"
	mw "github.com/avc-dev/go-avatar-service/internal/middleware"
)

// buildRouter wires the HTTP routes. Middleware order is significant:
//   - RequestID first so downstream middlewares (and slog logs) can attach it.
//   - Logger second so it captures the (potentially panic-recovered) status.
//   - Recoverer next so panics from handlers are caught and a 500 is written
//     into the wrapped writer the Logger sees.
//   - CORS last in the global chain so preflight responses still go through
//     RequestID + Logger and show up correlated in logs.
//
// Per-API middlewares (rate limit, UserID) are mounted on /api/v1 so /health
// and the static /web/ assets stay throttle-free.
//
// The two limiters are layered, not exclusive: writes hit the IP limiter and
// the user limiter on top — whichever budget runs out first wins. That keeps
// a single misbehaving account from saturating the per-IP read budget while
// still capping anonymous traffic from a shared NAT.
func buildRouter(
	log *slog.Logger,
	cfg config.SecurityConfig,
	healthH *handlers.HealthHandler,
	avatarH *handleravatar.Handler,
	staticPath string,
) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	// SpanName runs early so its deferred rename observes the matched chi route
	// pattern once the inner chain (routing + handler) has returned.
	r.Use(mw.SpanName)
	r.Use(mw.Logger(log))
	r.Use(chimw.Recoverer)
	r.Use(mw.CORS(cfg.CORSAllowedOrigins))

	r.Get("/health", healthH.Get)

	// Root redirects to the SPA so a stray browser hit on `/` lands on the UI
	// rather than a 404.
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/web/", http.StatusFound)
	})

	fileServer(r, "/web", http.Dir(staticPath))

	ipLimit := mw.IPRateLimit(cfg.RateLimitReadPerMin)
	userLimit := mw.UserIDRateLimit(cfg.RateLimitWritePerMin)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(ipLimit)
		r.Route("/avatars", func(r chi.Router) {
			r.With(mw.UserID, userLimit).Post("/", avatarH.Upload)
			r.Get("/{avatar_id}", avatarH.Download)
			r.Get("/{avatar_id}/metadata", avatarH.Metadata)
			r.With(mw.UserID, userLimit).Delete("/{avatar_id}", avatarH.Delete)
		})
		r.Route("/users/{user_id}", func(r chi.Router) {
			r.Get("/avatar", avatarH.GetUserAvatar)
			r.Get("/avatars", avatarH.ListUserAvatars)
			r.With(mw.UserID, userLimit).Delete("/avatar", avatarH.DeleteUserAvatar)
		})
	})

	// otelhttp wraps the whole router so every request gets a server span with
	// W3C trace-context extracted from inbound headers. The filter keeps
	// operational/static traffic (health probes, SPA assets) out of the trace
	// stream — they would otherwise dominate it with no diagnostic value.
	return otelhttp.NewHandler(r, "http.server", otelhttp.WithFilter(traceableRequest))
}

// traceableRequest reports whether a request should produce a server span. We
// trace the API surface and skip health checks and static SPA assets.
func traceableRequest(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case p == "/health", p == "/metrics", p == "/":
		return false
	case strings.HasPrefix(p, "/web"):
		return false
	default:
		return true
	}
}
