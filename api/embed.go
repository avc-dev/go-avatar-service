// Package api embeds the OpenAPI spec so the server can serve it from the binary
// without shipping a separate file. Edit openapi.yaml in this directory — that's
// the source of truth.
package api

import _ "embed"

// OpenAPISpec is the raw OpenAPI document for the public HTTP API.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
