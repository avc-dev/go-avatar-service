// Package httpx contains thin HTTP-layer utilities shared across handler and
// middleware packages: a unified JSON error format and a small WriteJSON
// helper. Keep it free of business types so it can be imported by any layer
// that ends up writing to an http.ResponseWriter.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErrorResponse is the canonical shape of every error body returned by this
// service. Details is optional and omitted from the wire when empty.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// WriteError writes a JSON ErrorResponse with the given status. details is
// variadic so callers can omit it for the common case.
//
// Encoding cannot fail for the simple types we use, so a failure here would
// indicate the connection is gone; we discard the error and avoid masking
// the original 4xx/5xx the caller wanted to report.
func WriteError(w http.ResponseWriter, status int, message string, details ...string) {
	body := ErrorResponse{Error: message}
	if len(details) > 0 {
		body.Details = details[0]
	}
	writeJSON(w, status, body)
}

// WriteJSON writes an arbitrary value as JSON with the given status. Use it
// for error shapes that don't fit ErrorResponse (e.g. 413 must include
// "max_size" per the spec).
func WriteJSON(w http.ResponseWriter, status int, body any) {
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The response is already partially written; nothing actionable
		// remains except a debug record for whoever is reading the logs.
		slog.Default().Debug("httpx: response encode failed", "err", err)
	}
}
