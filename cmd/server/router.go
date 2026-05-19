package main

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/avc-dev/go-avatar-service/internal/handlers"
	handleravatar "github.com/avc-dev/go-avatar-service/internal/handlers/avatar"
	mw "github.com/avc-dev/go-avatar-service/internal/middleware"
)

// buildRouter wires the HTTP routes. Middleware order is significant:
//   - RequestID first so downstream middlewares (and slog logs) can attach it.
//   - Logger second so it captures the (potentially panic-recovered) status.
//   - Recoverer last in this trio so panics from handlers are caught and a 500
//     is written into the wrapped writer the Logger sees.
//
// Write endpoints (POST/DELETE) are mounted with the UserID middleware applied
// per-route (via `.With(...)`), keeping public reads cheap and uncoupled from
// the X-User-ID parsing path.
func buildRouter(
	log *slog.Logger,
	healthH *handlers.HealthHandler,
	avatarH *handleravatar.Handler,
	staticPath string,
) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(mw.Logger(log))
	r.Use(chimw.Recoverer)

	r.Get("/health", healthH.Get)

	// Root redirects to the SPA so a stray browser hit on `/` lands on the UI
	// rather than a 404.
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/web/", http.StatusFound)
	})

	fileServer(r, "/web", http.Dir(staticPath))

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/avatars", func(r chi.Router) {
			r.With(mw.UserID).Post("/", avatarH.Upload)
			r.Get("/{avatar_id}", avatarH.Download)
			r.Get("/{avatar_id}/metadata", avatarH.Metadata)
			r.With(mw.UserID).Delete("/{avatar_id}", avatarH.Delete)
		})
		r.Route("/users/{user_id}", func(r chi.Router) {
			r.Get("/avatar", avatarH.GetUserAvatar)
			r.Get("/avatars", avatarH.ListUserAvatars)
			r.With(mw.UserID).Delete("/avatar", avatarH.DeleteUserAvatar)
		})
	})

	return r
}
