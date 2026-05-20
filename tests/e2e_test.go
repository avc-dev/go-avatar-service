//go:build integration

// Package tests holds end-to-end integration tests that exercise every layer
// of the application against real dependencies (Postgres, MinIO, RabbitMQ
// spun up via testcontainers). Run with `make test-integration` or
// `go test -tags=integration ./tests/...`.
package tests_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcrmq "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/avc-dev/go-avatar-service/internal/broker"
	handleravatar "github.com/avc-dev/go-avatar-service/internal/handlers/avatar"
	"github.com/avc-dev/go-avatar-service/internal/imageproc"
	mw "github.com/avc-dev/go-avatar-service/internal/middleware"
	repoavatar "github.com/avc-dev/go-avatar-service/internal/repository/avatar"
	svcavatar "github.com/avc-dev/go-avatar-service/internal/services/avatar"
	"github.com/avc-dev/go-avatar-service/internal/storage"
	workeravatar "github.com/avc-dev/go-avatar-service/internal/worker/avatar"
)

// TestEndToEnd_AvatarLifecycle drives the full happy path: upload → worker
// processes → metadata shows thumbnails → download thumb → delete → worker
// cleans S3. Failures here mean some layer is broken at the seam, not in its
// own unit tests.
func TestEndToEnd_AvatarLifecycle(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// --- Dependencies (testcontainers) ---
	pgDSN := startPostgres(t, ctx)
	minioCfg := startMinIO(t, ctx)
	rmqURL := startRabbitMQ(t, ctx)

	require.NoError(t, applyMigrations(t, pgDSN))

	pool, err := pgxpool.New(ctx, pgDSN)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	st, err := storage.NewMinIO(minioCfg, log)
	require.NoError(t, err)
	require.NoError(t, st.EnsureBucket(ctx))

	rmq, err := broker.NewRabbitMQ(rmqURL, log)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rmq.Close() })

	// --- Application graph ---
	repo := repoavatar.NewPostgresRepository(pool)
	svc := svcavatar.New(repo, st, rmq, log)
	handler := handleravatar.NewHandler(svc, log, 10*1024*1024)

	resizer := imageproc.New()
	worker := workeravatar.New(repo, st, resizer, log)

	// --- Start worker consumers in the background ---
	//
	// We capture consumer errors on a buffered channel and surface them in
	// t.Cleanup. Without this, a broker dying mid-test would silently swallow
	// the real cause and the test would just fail with "thumbnails never
	// appeared" — making the failure much harder to debug in CI.
	workerCtx, stopWorker := context.WithCancel(context.Background())
	var consumerWG sync.WaitGroup
	consumerErrs := make(chan error, 2)

	startConsumer := func(queue string, handler broker.Handler) {
		consumerWG.Add(1)
		go func() {
			defer consumerWG.Done()
			if err := rmq.Consume(workerCtx, queue, handler); err != nil && !errors.Is(err, context.Canceled) {
				consumerErrs <- fmt.Errorf("consumer %s: %w", queue, err)
			}
		}()
	}
	startConsumer(broker.QueueUploaded, worker.HandleUploaded)
	startConsumer(broker.QueueDeleted, worker.HandleDeleted)

	t.Cleanup(func() {
		stopWorker()
		consumerWG.Wait()
		close(consumerErrs)
		for err := range consumerErrs {
			t.Errorf("worker consumer returned unexpected error: %v", err)
		}
	})

	// --- HTTP test server (router mirrors cmd/server.buildRouter) ---
	r := buildTestRouter(log, handler)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// --- Upload ---
	userID := uuid.Must(uuid.NewV7())
	imgBytes := makePNG(t, 200, 200)

	uploadResp := uploadAvatar(t, srv.URL, userID, "selfie.png", "image/png", imgBytes)
	require.Equal(t, userID.String(), uploadResp.UserID)
	require.Equal(t, "/api/v1/avatars/"+uploadResp.ID, uploadResp.URL)

	// --- Wait for worker to populate thumbnails (poll metadata) ---
	var meta handleravatar.MetadataResponse
	require.Eventually(t, func() bool {
		meta = fetchMetadata(t, srv.URL, uploadResp.ID)
		return len(meta.Thumbnails) == 2
	}, 10*time.Second, 100*time.Millisecond, "worker did not generate thumbnails in time")

	// --- Download original ---
	body, ct, sz := downloadAvatar(t, srv.URL, uploadResp.ID, "")
	require.Equal(t, "image/png", ct)
	require.Equal(t, int64(len(imgBytes)), sz)
	require.Equal(t, imgBytes, body)

	// --- Download both thumbnails via ?size=… ---
	for _, size := range []string{"100x100", "300x300"} {
		body, ct, sz := downloadAvatar(t, srv.URL, uploadResp.ID, size)
		require.Equal(t, "image/jpeg", ct, "thumbnail %s should be JPEG", size)
		require.Positive(t, sz)
		// JPEG SOI marker is 0xFFD8.
		require.GreaterOrEqual(t, len(body), 2)
		require.Equal(t, byte(0xFF), body[0])
		require.Equal(t, byte(0xD8), body[1])
	}

	// --- List user avatars: one entry ---
	list := fetchUserAvatars(t, srv.URL, userID)
	require.Len(t, list.Avatars, 1)
	require.Equal(t, uploadResp.ID, list.Avatars[0].ID.String())

	// --- Delete by owner ---
	deleteAvatar(t, srv.URL, userID, uploadResp.ID, http.StatusNoContent)

	// --- Verify metadata gone immediately (DB soft-delete is sync) ---
	requireMetadataStatus(t, srv.URL, uploadResp.ID, http.StatusNotFound)

	// --- Wait for worker to remove the S3 objects ---
	require.Eventually(t, func() bool {
		exists, _ := st.Exists(ctx, "originals/"+uploadResp.ID+".png")
		if exists {
			return false
		}
		for _, size := range []string{"100x100", "300x300"} {
			exists, _ := st.Exists(ctx, "thumbnails/"+uploadResp.ID+"/"+size+".jpg")
			if exists {
				return false
			}
		}
		return true
	}, 10*time.Second, 100*time.Millisecond, "worker did not delete S3 objects in time")
}

// ---------------------------------------------------------------------------
// Container setup helpers — testcontainers wrappers.
// ---------------------------------------------------------------------------

func startPostgres(t *testing.T, ctx context.Context) string {
	t.Helper()
	c, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("avatars"),
		tcpostgres.WithUsername("avatars"),
		tcpostgres.WithPassword("avatars"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return dsn
}

func startMinIO(t *testing.T, ctx context.Context) storage.Config {
	t.Helper()
	c, err := tcminio.Run(ctx, "minio/minio:latest",
		tcminio.WithUsername("minioadmin"),
		tcminio.WithPassword("minioadmin"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	endpoint, err := c.ConnectionString(ctx)
	require.NoError(t, err)

	return storage.Config{
		Endpoint:  endpoint,
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "avatars",
		UseSSL:    false,
	}
}

func startRabbitMQ(t *testing.T, ctx context.Context) string {
	t.Helper()
	c, err := tcrmq.Run(ctx,
		"rabbitmq:3.13-management-alpine",
		tcrmq.WithAdminUsername("guest"),
		tcrmq.WithAdminPassword("guest"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	url, err := c.AmqpURL(ctx)
	require.NoError(t, err)
	return url
}

func applyMigrations(t *testing.T, dsn string) error {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "migrations")
	url := "file://" + filepath.ToSlash(migrationsDir)

	m, err := migrate.New(url, dsn)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil {
		return err
	}
	srcErr, dbErr := m.Close()
	if srcErr != nil {
		return srcErr
	}
	return dbErr
}

// ---------------------------------------------------------------------------
// HTTP router (mini-replica of cmd/server.buildRouter — kept inline so the
// test doesn't depend on internal-of-internal package).
// ---------------------------------------------------------------------------

func buildTestRouter(log *slog.Logger, h *handleravatar.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(mw.Logger(log))
	r.Use(chimw.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/avatars", func(r chi.Router) {
			r.With(mw.UserID).Post("/", h.Upload)
			r.Get("/{avatar_id}", h.Download)
			r.Get("/{avatar_id}/metadata", h.Metadata)
			r.With(mw.UserID).Delete("/{avatar_id}", h.Delete)
		})
		r.Route("/users/{user_id}", func(r chi.Router) {
			r.Get("/avatar", h.GetUserAvatar)
			r.Get("/avatars", h.ListUserAvatars)
			r.With(mw.UserID).Delete("/avatar", h.DeleteUserAvatar)
		})
	})
	return r
}

// ---------------------------------------------------------------------------
// HTTP client helpers (avoid repetition in the test body above).
// ---------------------------------------------------------------------------

type uploadResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	URL       string `json:"url"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func uploadAvatar(t *testing.T, baseURL string, userID uuid.UUID, filename, contentType string, body []byte) uploadResponse {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	h.Set("Content-Type", contentType)
	part, err := mw.CreatePart(h)
	require.NoError(t, err)
	_, err = part.Write(body)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/avatars", &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-User-ID", userID.String())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "unexpected status from upload")

	var out uploadResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func fetchMetadata(t *testing.T, baseURL, id string) handleravatar.MetadataResponse {
	t.Helper()
	resp, err := http.Get(baseURL + "/api/v1/avatars/" + id + "/metadata")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out handleravatar.MetadataResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func downloadAvatar(t *testing.T, baseURL, id, size string) ([]byte, string, int64) {
	t.Helper()
	url := baseURL + "/api/v1/avatars/" + id
	if size != "" {
		url += "?size=" + size
	}
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return body, resp.Header.Get("Content-Type"), resp.ContentLength
}

func fetchUserAvatars(t *testing.T, baseURL string, userID uuid.UUID) handleravatar.ListResponse {
	t.Helper()
	resp, err := http.Get(baseURL + "/api/v1/users/" + userID.String() + "/avatars")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out handleravatar.ListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func deleteAvatar(t *testing.T, baseURL string, userID uuid.UUID, id string, wantStatus int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/api/v1/avatars/"+id, nil)
	require.NoError(t, err)
	req.Header.Set("X-User-ID", userID.String())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, wantStatus, resp.StatusCode)
}

func requireMetadataStatus(t *testing.T, baseURL, id string, want int) {
	t.Helper()
	resp, err := http.Get(baseURL + "/api/v1/avatars/" + id + "/metadata")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, want, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Image fixture: deterministic PNG of the requested size with a gradient.
// Generating in-process keeps the test self-contained — no binary fixtures.
// ---------------------------------------------------------------------------

func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x * 255 / w),
				G: uint8(y * 255 / h),
				B: 128,
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}
