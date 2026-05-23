package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/avc-dev/go-avatar-service/internal/bootutil"
	"github.com/avc-dev/go-avatar-service/internal/broker"
	"github.com/avc-dev/go-avatar-service/internal/config"
	"github.com/avc-dev/go-avatar-service/internal/imageproc"
	"github.com/avc-dev/go-avatar-service/internal/logger"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
	"github.com/avc-dev/go-avatar-service/internal/storage"
	workeravatar "github.com/avc-dev/go-avatar-service/internal/worker/avatar"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker failed", "err", err)
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

	// --- Worker ---

	resizer := imageproc.New()
	repo := repoavatar.NewPostgresRepository(pool)
	w := workeravatar.New(repo, minioStore, resizer, log)

	// --- Consumer loop: two queues, two goroutines ---

	// errgroup.WithContext gives us two guarantees: the first non-nil return
	// cancels gctx (so the sibling consumer drops out of its blocking Consume),
	// and g.Wait blocks until both have actually exited.
	g, gctx := errgroup.WithContext(ctx)

	startConsumer := func(queue string, handler broker.Handler) {
		g.Go(func() error {
			log.Info("consumer starting", "queue", queue)
			defer log.Info("consumer stopped", "queue", queue)
			err := rmq.Consume(gctx, queue, handler)
			// Graceful shutdown wraps context.Canceled — not a real error.
			if err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("consume %s: %w", queue, err)
			}
			return nil
		})
	}

	startConsumer(broker.QueueUploaded, w.HandleUploaded)
	startConsumer(broker.QueueDeleted, w.HandleDeleted)

	log.Info("worker running", "queues", []string{broker.QueueUploaded, broker.QueueDeleted})

	err = g.Wait()
	if err != nil {
		log.Error("consumer failed; initiating shutdown", "err", err)
	} else {
		log.Info("shutdown signal received")
	}
	log.Info("worker stopped")
	return err
}
