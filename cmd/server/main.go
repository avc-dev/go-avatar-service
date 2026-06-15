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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/avc-dev/go-avatar-service/internal/bootutil"
	"github.com/avc-dev/go-avatar-service/internal/broker"
	"github.com/avc-dev/go-avatar-service/internal/config"
	"github.com/avc-dev/go-avatar-service/internal/handlers"
	handleravatar "github.com/avc-dev/go-avatar-service/internal/handlers/avatar"
	"github.com/avc-dev/go-avatar-service/internal/logger"
	"github.com/avc-dev/go-avatar-service/internal/metrics"
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
		// The shutdown timeout is the caller's call: bound the flush here so a
		// dead trace backend cannot stall process exit.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			log.Error("tracing shutdown", "err", err)
		}
	}()

	// Metrics registry: this binary's own registry (no global default), with
	// the standard Go/process collectors for the resource-utilisation panels.
	reg := prometheus.NewRegistry()
	if err := metrics.RegisterRuntime(reg); err != nil {
		return fmt.Errorf("register runtime metrics: %w", err)
	}

	// --- Infrastructure ---

	pool, err := bootutil.NewTracedPool(ctx, cfg.Postgres.DSN)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := metrics.RegisterPool(reg, pool, "server"); err != nil {
		return fmt.Errorf("register pool metrics: %w", err)
	}

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

	// --- Metrics instruments ---

	httpMetrics, err := metrics.NewHTTP(reg)
	if err != nil {
		return fmt.Errorf("build http metrics: %w", err)
	}
	uploadMetrics, err := metrics.NewUpload(reg)
	if err != nil {
		return fmt.Errorf("build upload metrics: %w", err)
	}
	storageMetrics, err := metrics.NewStorage(reg)
	if err != nil {
		return fmt.Errorf("build storage metrics: %w", err)
	}
	metricsHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})

	// --- Application layer ---

	avatarRepo := repoavatar.NewPostgresRepository(pool)
	avatarSvc := svcavatar.New(avatarRepo, minioStore, rmq, log)
	avatarH := handleravatar.NewHandler(avatarSvc, log, cfg.HTTP.MaxUploadBytes, uploadMetrics)

	healthH := handlers.NewHealthHandler(
		map[string]handlers.HealthChecker{
			"postgres": handlers.HealthCheckerFunc(pool.Ping),
			"minio":    minioStore,
			"rabbitmq": rmq,
		},
		healthCheckTimeout,
	)

	// Storage-usage gauge: sample the DB sum periodically rather than at scrape
	// time, so a slow query can never stall a Prometheus scrape.
	go sampleStorageBytes(ctx, log, avatarRepo, storageMetrics)

	// --- HTTP server + graceful shutdown ---

	srv := &http.Server{
		Addr:              ":" + cfg.HTTP.Port,
		Handler:           buildRouter(log, cfg.Security, healthH, avatarH, httpMetrics, metricsHandler, staticDir),
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
