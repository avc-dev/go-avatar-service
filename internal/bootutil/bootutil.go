// Package bootutil holds tiny helpers shared by the cmd/server and cmd/worker
// process entrypoints: bounded dependency probes at startup, and userinfo
// redaction for DSN-style URLs before they hit the logs.
//
// The package is deliberately narrow. Anything specific to a single binary
// (signal handling, route wiring, consumer fan-out) stays in that binary's
// main.go so cmd-level code remains readable end-to-end.
package bootutil

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultProbeTimeout is the timeout applied to each ProbeWithTimeout call
// unless the caller overrides it. Five seconds gives slow CI hosts room to
// answer while still failing fast on a wedged dependency.
const DefaultProbeTimeout = 5 * time.Second

// ProbeWithTimeout runs probe under a bounded child of parent. The label is
// only used to wrap the resulting error so the failure message names the
// failing step (e.g. "postgres ping: dial tcp: ..."). Cancelling parent —
// for instance via SIGTERM during boot — also cancels the probe.
func ProbeWithTimeout(parent context.Context, timeout time.Duration, label string, probe func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if err := probe(ctx); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

// NewTracedPool builds a pgx connection pool from dsn with the otelpgx tracer
// attached, so every query issued through it emits a span under the active
// trace. Both binaries construct their pool this way; the tracer reads the
// global TracerProvider installed by observability.Init, so it is a no-op until
// (and unless) tracing is enabled.
func NewTracedPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pg dsn: %w", err)
	}
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pg pool: %w", err)
	}
	return pool, nil
}

// RedactURL strips the userinfo segment from a URL of the form
// `<scheme>://[user[:password]@]host[...]` and replaces it with `***@` for
// safe logging. Works for Postgres DSNs (`postgres://user:pass@host/db`),
// AMQP URLs (`amqp://guest:guest@host:5672/`) and any other scheme that
// follows the same userinfo convention.
//
// If the input does not contain `://` or has no `@` in the host portion,
// it is returned unchanged — there is nothing to redact.
func RedactURL(u string) string {
	i := strings.Index(u, "://")
	if i < 0 {
		return u
	}
	rest := u[i+3:]
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return u
	}
	return u[:i+3] + "***@" + rest[at+1:]
}
