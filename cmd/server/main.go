package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/avc-dev/go-avatar-service/internal/broker"
	"github.com/avc-dev/go-avatar-service/internal/config"
	"github.com/avc-dev/go-avatar-service/internal/handlers"
	handleravatar "github.com/avc-dev/go-avatar-service/internal/handlers/avatar"
	"github.com/avc-dev/go-avatar-service/internal/logger"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
	svcavatar "github.com/avc-dev/go-avatar-service/internal/services/avatar"
	"github.com/avc-dev/go-avatar-service/internal/storage"
)

// startupProbeTimeout caps both the Postgres Ping and the MinIO EnsureBucket
// calls performed once at boot. The intent is fail-fast: if the dependency
// is unreachable, crash on startup with a clear error rather than letting
// the first inbound request fail mysteriously.
const startupProbeTimeout = 5 * time.Second

// healthCheckTimeout bounds the total /health probe (across all components).
const healthCheckTimeout = 5 * time.Second

// staticDir is the on-disk location of the SPA assets served under /web/.
// Resolved relative to the process working directory; expected to be the repo
// root for local dev (`make run-server`) and /app inside the Docker image
// (pinned via WORKDIR).
const staticDir = "web/static"

func main() {
	if err := run(); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.Log)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Infrastructure ---

	pool, err := pgxpool.New(ctx, cfg.Postgres.DSN)
	if err != nil {
		return fmt.Errorf("create pg pool: %w", err)
	}
	defer pool.Close()

	if err := probeWithTimeout(ctx, "postgres ping", pool.Ping); err != nil {
		return err
	}
	log.Info("postgres connected", "dsn", redactedDSN(cfg.Postgres.DSN))

	minioStore, err := storage.NewMinIO(storage.Config{
		Endpoint:  cfg.MinIO.Endpoint,
		AccessKey: cfg.MinIO.AccessKey,
		SecretKey: cfg.MinIO.SecretKey,
		Bucket:    cfg.MinIO.Bucket,
		UseSSL:    cfg.MinIO.UseSSL,
	}, log)
	if err != nil {
		return fmt.Errorf("create minio client: %w", err)
	}

	if err := probeWithTimeout(ctx, "ensure minio bucket", minioStore.EnsureBucket); err != nil {
		return err
	}
	log.Info("minio bucket ready", "bucket", cfg.MinIO.Bucket)

	rmq, err := broker.NewRabbitMQ(cfg.RabbitMQ.URL, log)
	if err != nil {
		return fmt.Errorf("create rabbitmq client: %w", err)
	}
	defer func() {
		if cerr := rmq.Close(); cerr != nil {
			log.Warn("rabbitmq close", "err", cerr)
		}
	}()
	log.Info("rabbitmq connected", "url", redactedAMQPURL(cfg.RabbitMQ.URL))

	// --- Application layer ---

	avatarRepo := repoavatar.NewPostgresRepository(pool)
	avatarSvc := svcavatar.New(avatarRepo, minioStore, rmq, log)
	avatarH := handleravatar.NewHandler(avatarSvc, log, cfg.HTTP.MaxUploadBytes)

	healthH := handlers.NewHealthHandler(
		map[string]handlers.HealthChecker{
			"postgres": handlers.HealthCheckerFunc(pool.Ping),
			"minio":    minioStore,
			"rabbitmq": rmq,
		},
		healthCheckTimeout,
	)

	// --- HTTP server + graceful shutdown ---

	srv := &http.Server{
		Addr:              ":" + cfg.HTTP.Port,
		Handler:           buildRouter(log, healthH, avatarH, staticDir),
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server listen: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	log.Info("server stopped")
	return nil
}

// probeWithTimeout runs a startup probe under a bounded context. label is
// used only to wrap the resulting error so the failure message names the
// failing step. parent is the long-lived application context — cancelling it
// (SIGTERM during boot) also cancels the probe.
func probeWithTimeout(parent context.Context, label string, probe func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, startupProbeTimeout)
	defer cancel()
	if err := probe(ctx); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

// redactedDSN strips credentials from a Postgres DSN for safe logging. Only
// the userinfo segment is replaced; scheme, host, port, db, and query params
// are preserved.
func redactedDSN(dsn string) string {
	return redactScheme(dsn)
}

// redactedAMQPURL strips userinfo from an amqp:// URL for safe logging.
func redactedAMQPURL(url string) string {
	return redactScheme(url)
}

func redactScheme(u string) string {
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
