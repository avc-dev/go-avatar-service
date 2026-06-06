package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/avc-dev/go-avatar-service/internal/bootutil"
	"github.com/avc-dev/go-avatar-service/internal/broker"
	"github.com/avc-dev/go-avatar-service/internal/config"
	"github.com/avc-dev/go-avatar-service/internal/handlers"
	handleravatar "github.com/avc-dev/go-avatar-service/internal/handlers/avatar"
	"github.com/avc-dev/go-avatar-service/internal/logger"
	"github.com/avc-dev/go-avatar-service/internal/observability"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
	svcavatar "github.com/avc-dev/go-avatar-service/internal/services/avatar"
	"github.com/avc-dev/go-avatar-service/internal/storage"
)

// serviceName identifies this binary as a distinct node in the trace graph,
// separate from the worker (see observability.Init).
const serviceName = "gophprofile-server"

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

	// --- Observability ---

	shutdownTracing, err := observability.Init(ctx, cfg.Observability, serviceName, log)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() {
		if err := shutdownTracing(context.Background()); err != nil {
			log.Warn("tracing shutdown", "err", err)
		}
	}()

	// --- Infrastructure ---

	pool, err := bootutil.NewTracedPool(ctx, cfg.Postgres.DSN)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := bootutil.ProbeWithTimeout(ctx, bootutil.DefaultProbeTimeout, "postgres ping", pool.Ping); err != nil {
		return err
	}
	log.Info("postgres connected", "dsn", bootutil.RedactURL(cfg.Postgres.DSN))

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

	if err := bootutil.ProbeWithTimeout(ctx, bootutil.DefaultProbeTimeout, "ensure minio bucket", minioStore.EnsureBucket); err != nil {
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
	log.Info("rabbitmq connected", "url", bootutil.RedactURL(cfg.RabbitMQ.URL))

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
		Handler:           buildRouter(log, cfg.Security, healthH, avatarH, staticDir),
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
