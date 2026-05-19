package avatar_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	"github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

const (
	pgDB   = "avatars"
	pgUser = "avatars"
	pgPass = "avatars"
)

var (
	testPool      *pgxpool.Pool
	testContainer testcontainers.Container
)

// TestMain owns the lifecycle of the shared Postgres container: it spins one
// up before any tests run, applies migrations, exposes a pgxpool via testPool,
// and tears everything down after the suite finishes. Each individual test
// resets state via truncateAvatars instead of restarting the container.
func TestMain(m *testing.M) {
	if err := mainSetup(); err != nil {
		log.Fatalf("test setup: %v", err)
	}
	code := m.Run()
	mainTeardown()
	os.Exit(code)
}

func mainSetup() error {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase(pgDB),
		tcpostgres.WithUsername(pgUser),
		tcpostgres.WithPassword(pgPass),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return fmt.Errorf("start postgres: %w", err)
	}
	testContainer = container

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return fmt.Errorf("get DSN: %w", err)
	}

	if err := applyMigrations(dsn); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("pgxpool.New: %w", err)
	}
	testPool = pool
	return nil
}

func applyMigrations(dsn string) error {
	// internal/repository/avatar/tests/ -> repo root is four levels up.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("runtime.Caller failed")
	}
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "migrations")
	migrationsURL := "file://" + filepath.ToSlash(migrationsDir)

	m, err := migrate.New(migrationsURL, dsn)
	if err != nil {
		return fmt.Errorf("migrate.New: %w", err)
	}
	if err := m.Up(); err != nil {
		return fmt.Errorf("migrate.Up: %w", err)
	}
	if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
		return fmt.Errorf("migrate.Close: src=%v db=%v", srcErr, dbErr)
	}
	return nil
}

func mainTeardown() {
	if testPool != nil {
		testPool.Close()
	}
	if testContainer != nil {
		_ = testContainer.Terminate(context.Background())
	}
}

// newRepo builds a fresh repository against the shared pool.
func newRepo() *avatar.PostgresRepository {
	return avatar.NewPostgresRepository(testPool)
}

// truncateAvatars wipes the avatars table; call at the start of each test
// (or via t.Cleanup) when isolation matters.
func truncateAvatars(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), "TRUNCATE TABLE avatars")
	require.NoError(t, err)
}

// newAvatar builds a minimally-valid Avatar with a freshly-generated UUID v7.
func newAvatar(t *testing.T, userID uuid.UUID) *domain.Avatar {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	return &domain.Avatar{
		ID:               id,
		UserID:           userID,
		FileName:         "avatar.jpg",
		MIMEType:         "image/jpeg",
		SizeBytes:        12345,
		S3Key:            "originals/" + id.String() + ".jpg",
		UploadStatus:     domain.UploadStatusUploading,
		ProcessingStatus: domain.ProcessingStatusPending,
	}
}
