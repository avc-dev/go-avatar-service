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
	"golang.org/x/sync/errgroup"

	"github.com/avc-dev/go-avatar-service/internal/adminhttp"
	"github.com/avc-dev/go-avatar-service/internal/bootutil"
	"github.com/avc-dev/go-avatar-service/internal/broker"
	"github.com/avc-dev/go-avatar-service/internal/config"
	"github.com/avc-dev/go-avatar-service/internal/handlers"
	handleravatar "github.com/avc-dev/go-avatar-service/internal/handlers/avatar"
	"github.com/avc-dev/go-avatar-service/internal/logger"
	"github.com/avc-dev/go-avatar-service/internal/metrics"
	"github.com/avc-dev/go-avatar-service/internal/observability"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
	"github.com/avc-dev/go-avatar-service/internal/resilience"
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
	// Put circuit breakers in front of storage and the broker so a sick backend
	// fails fast instead of stalling requests. The health handler below keeps the
	// raw clients on purpose — a probe needs to reach the real dependency.
	breakerSettings := cfg.Resilience.BreakerSettings()
	guardedStorage := resilience.NewStorage(minioStore, breakerSettings, log)
	guardedPublisher := resilience.NewPublisher(rmq, breakerSettings, log)
	avatarSvc := svcavatar.New(avatarRepo, guardedStorage, guardedPublisher, log)
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

	// --- HTTP servers (public API + admin) + graceful shutdown ---

	// Public listener: API and SPA only. /health and /metrics live on the admin
	// listener instead, so they stay off the Ingress and out of the rate limiter.
	publicSrv := &http.Server{
		Addr:              ":" + cfg.HTTP.Port,
		Handler:           buildRouter(log, cfg.Security, avatarH, httpMetrics, staticDir),
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}
	adminSrv := &http.Server{
		Addr:              ":" + cfg.HTTP.AdminPort,
		Handler:           adminhttp.Router(healthH, metricsHandler),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
	}

	// errgroup.WithContext buys us two things: the first error cancels gctx (so
	// the other goroutines unblock), and g.Wait holds until they've all exited.
	g, gctx := errgroup.WithContext(ctx)
	serve := func(name string, srv *http.Server) {
		g.Go(func() error {
			log.Info("listening", "server", name, "addr", srv.Addr)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("%s listen: %w", name, err)
			}
			return nil
		})
	}
	serve("public", publicSrv)
	serve("admin", adminSrv)

	// On signal (or one server dying) drain both within a single deadline.
	g.Go(func() error {
		<-gctx.Done()
		log.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		var errs error
		if err := publicSrv.Shutdown(shutdownCtx); err != nil {
			errs = errors.Join(errs, fmt.Errorf("public server shutdown: %w", err))
		}
		if err := adminSrv.Shutdown(shutdownCtx); err != nil {
			errs = errors.Join(errs, fmt.Errorf("admin server shutdown: %w", err))
		}
		return errs
	})

	if err := g.Wait(); err != nil {
		return err
	}
	log.Info("server stopped")
	return nil
}
