package storage_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"

	"github.com/avc-dev/go-avatar-service/internal/storage"
)

const testBucket = "test-avatars"

var (
	testContainer testcontainers.Container
	testStorage   *storage.MinIO
	testAdmin     *minio.Client
)

// TestMain owns the lifecycle of the shared MinIO container: it spins one up
// before any tests run, builds a *storage.MinIO bound to a shared bucket,
// and tears everything down after the suite finishes. Individual tests use
// cleanBucket to isolate state between runs.
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

	ctr, err := tcminio.Run(ctx, "minio/minio:RELEASE.2024-01-16T16-07-38Z")
	if err != nil {
		return fmt.Errorf("start minio: %w", err)
	}
	testContainer = ctr

	endpoint, err := ctr.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("minio endpoint: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	s, err := storage.NewMinIO(storage.Config{
		Endpoint:  endpoint,
		AccessKey: ctr.Username,
		SecretKey: ctr.Password,
		Bucket:    testBucket,
		UseSSL:    false,
	}, logger)
	if err != nil {
		return fmt.Errorf("storage.NewMinIO: %w", err)
	}
	testStorage = s

	if err := s.EnsureBucket(ctx); err != nil {
		return fmt.Errorf("ensure bucket: %w", err)
	}

	admin, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(ctr.Username, ctr.Password, ""),
		Secure: false,
	})
	if err != nil {
		return fmt.Errorf("admin client: %w", err)
	}
	testAdmin = admin

	return nil
}

func mainTeardown() {
	if testContainer != nil {
		_ = testContainer.Terminate(context.Background())
	}
}

// newMinIO returns the shared *storage.MinIO. A function is exposed (rather
// than direct package-var access) so future per-test isolation tweaks have a
// single chokepoint.
func newMinIO(t *testing.T) *storage.MinIO {
	t.Helper()
	require.NotNil(t, testStorage, "test storage not initialised")
	return testStorage
}

// cleanBucket removes every object in testBucket; call at the start of each
// test that needs a clean slate.
func cleanBucket(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	objCh := testAdmin.ListObjects(ctx, testBucket, minio.ListObjectsOptions{Recursive: true})
	for obj := range objCh {
		if obj.Err != nil {
			// Bucket may not exist yet (e.g. EnsureBucket tests). Ignore.
			return
		}
		err := testAdmin.RemoveObject(ctx, testBucket, obj.Key, minio.RemoveObjectOptions{})
		require.NoError(t, err)
	}
}

// putRaw uploads payload directly via the admin client, bypassing the SUT.
// Useful for arranging state in tests that exercise read/delete paths.
func putRaw(t *testing.T, key, contentType string, payload []byte) {
	t.Helper()
	_, err := testAdmin.PutObject(
		context.Background(),
		testBucket,
		key,
		bytes.NewReader(payload),
		int64(len(payload)),
		minio.PutObjectOptions{ContentType: contentType},
	)
	require.NoError(t, err)
}
