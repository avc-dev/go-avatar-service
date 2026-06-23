package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/avc-dev/go-avatar-service/api"
)

// mountSwagger serves the API docs: the embedded spec at /swagger/openapi.yaml
// and a Swagger UI page at /swagger that renders it. The UI loads from a CDN
// (same trick as Tailwind in the SPA) so there's no JS build step to maintain.
func mountSwagger(r chi.Router) {
	r.Get("/swagger/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(api.OpenAPISpec)
	})
	r.Get("/swagger", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(swaggerUIPage))
	})
}

// swaggerUIPage is a minimal Swagger UI host page pointing at our embedded spec.
const swaggerUIPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <title>GophProfile API — Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"/>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: "/swagger/openapi.yaml",
        dom_id: "#swagger-ui",
      });
    };
  </script>
</body>
</html>
`
