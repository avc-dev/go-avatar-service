package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

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

	// consumeErrs is buffered for the number of consumers so a fatal error
	// from either can be reported without blocking the producer goroutine.
	const consumerCount = 2
	consumeErrs := make(chan error, consumerCount)
	var wg sync.WaitGroup

	startConsumer := func(queue string, handler broker.Handler) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Info("consumer starting", "queue", queue)
			err := rmq.Consume(ctx, queue, handler)
			// Graceful shutdown wraps context.Canceled — not a real error.
			if err != nil && !errors.Is(err, context.Canceled) {
				consumeErrs <- fmt.Errorf("consume %s: %w", queue, err)
			}
			log.Info("consumer stopped", "queue", queue)
		}()
	}

	startConsumer(broker.QueueUploaded, w.HandleUploaded)
	startConsumer(broker.QueueDeleted, w.HandleDeleted)

	log.Info("worker running", "queues", []string{broker.QueueUploaded, broker.QueueDeleted})

	// Wait for either:
	//   - SIGINT/SIGTERM (ctx.Done)
	//   - a consumer returning a fatal (non-Cancelled) error
	// On either path, ensure both consumers have fully drained before we
	// touch deferred Close() calls.
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-consumeErrs:
		log.Error("consumer failed; initiating shutdown", "err", err)
		stop() // cancels ctx → other consumer exits too
	}

	wg.Wait()
	log.Info("worker stopped")
	return nil
}
