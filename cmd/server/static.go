package main

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// fileServer mounts an http.FileServer at the given URL prefix. Adapted from
// the chi documentation recipe — chi's router does not handle path-stripped
// FileServer registration out of the box.
//
// A request to `/web/` without a trailing slash is redirected to `/web/` (301)
// so http.FileServer's directory-index logic kicks in correctly.
func fileServer(r chi.Router, urlPath string, root http.FileSystem) {
	if strings.ContainsAny(urlPath, "{}*") {
		panic("fileServer does not permit URL parameters")
	}

	if urlPath != "/" && urlPath[len(urlPath)-1] != '/' {
		r.Get(urlPath, http.RedirectHandler(urlPath+"/", http.StatusMovedPermanently).ServeHTTP)
		urlPath += "/"
	}
	urlPath += "*"

	r.Get(urlPath, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}
