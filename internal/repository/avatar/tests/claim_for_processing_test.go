package avatar_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/domain"
	"github.com/avc-dev/go-avatar-service/internal/repository/avatar"
)

func TestClaimForProcessing(t *testing.T) {
	repo := newRepo()
	ctx := t.Context()

	t.Run("pending → processing returns the claimed row", func(t *testing.T) {
		truncateAvatars(t)
		a := newAvatar(t, uuid.Must(uuid.NewV7()))
		require.NoError(t, repo.Create(ctx, a))

		claimed, err := repo.ClaimForProcessing(ctx, a.ID)
		require.NoError(t, err)
		require.NotNil(t, claimed)
		require.Equal(t, domain.ProcessingStatusProcessing, claimed.ProcessingStatus,
			"claim must return the row with status already moved to processing")
		require.Equal(t, a.S3Key, claimed.S3Key, "claim must echo the row's S3 key for the worker to use")

		// Confirm the persisted state too — not just the RETURNING projection.
		fetched, err := repo.GetByID(ctx, a.ID)
		require.NoError(t, err)
		require.Equal(t, domain.ProcessingStatusProcessing, fetched.ProcessingStatus)
	})

	t.Run("already-processing → ErrNotFound (claim is non-actionable)", func(t *testing.T) {
		truncateAvatars(t)
		a := newAvatar(t, uuid.Must(uuid.NewV7()))
		require.NoError(t, repo.Create(ctx, a))

		// First claim wins.
		_, err := repo.ClaimForProcessing(ctx, a.ID)
		require.NoError(t, err)

		// Second claim sees status='processing' → falls through to ErrNotFound.
		_, err = repo.ClaimForProcessing(ctx, a.ID)
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})

	t.Run("completed → ErrNotFound", func(t *testing.T) {
		truncateAvatars(t)
		a := newAvatar(t, uuid.Must(uuid.NewV7()))
		require.NoError(t, repo.Create(ctx, a))
		require.NoError(t, repo.UpdateProcessingStatus(ctx, a.ID, domain.ProcessingStatusCompleted, map[string]string{"100x100": "k"}))

		_, err := repo.ClaimForProcessing(ctx, a.ID)
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})

	t.Run("missing id → ErrNotFound", func(t *testing.T) {
		truncateAvatars(t)
		_, err := repo.ClaimForProcessing(ctx, uuid.Must(uuid.NewV7()))
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})

	t.Run("soft-deleted → ErrNotFound", func(t *testing.T) {
		truncateAvatars(t)
		a := newAvatar(t, uuid.Must(uuid.NewV7()))
		require.NoError(t, repo.Create(ctx, a))
		require.NoError(t, repo.SoftDelete(ctx, a.ID))

		_, err := repo.ClaimForProcessing(ctx, a.ID)
		require.ErrorIs(t, err, avatar.ErrNotFound)
	})

	t.Run("concurrent claims: exactly one winner", func(t *testing.T) {
		// The whole point of the conditional UPDATE: under N goroutines racing,
		// the DB serialises them and only one observes the row as 'pending'.
		// This is the worker's defence against at-least-once redelivery.
		truncateAvatars(t)
		a := newAvatar(t, uuid.Must(uuid.NewV7()))
		require.NoError(t, repo.Create(ctx, a))

		const racers = 8
		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			successes int
			notFounds int
		)
		wg.Add(racers)
		for i := 0; i < racers; i++ {
			go func() {
				defer wg.Done()
				_, err := repo.ClaimForProcessing(ctx, a.ID)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					successes++
				case errors.Is(err, avatar.ErrNotFound):
					notFounds++
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}()
		}
		wg.Wait()

		require.Equal(t, 1, successes, "exactly one racer must observe the row as pending")
		require.Equal(t, racers-1, notFounds, "all other racers must see ErrNotFound")
	})
}
