package avatar_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleDeleted(t *testing.T) {
	ctx := context.Background()
	msgID := "msg-del"

	t.Run("happy: every key deleted", func(t *testing.T) {
		w, _, st, _ := newWorker(t)
		_, body := makeDeleteEvent(t,
			"originals/abc.jpg",
			"thumbnails/abc/100x100.jpg",
			"thumbnails/abc/300x300.jpg",
		)

		st.EXPECT().Delete(ctx, "originals/abc.jpg").Return(nil)
		st.EXPECT().Delete(ctx, "thumbnails/abc/100x100.jpg").Return(nil)
		st.EXPECT().Delete(ctx, "thumbnails/abc/300x300.jpg").Return(nil)

		require.NoError(t, w.HandleDeleted(ctx, body, msgID))
	})

	t.Run("malformed JSON: error, no Delete calls", func(t *testing.T) {
		w, _, _, _ := newWorker(t)
		err := w.HandleDeleted(ctx, []byte("nope"), msgID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unmarshal delete event")
	})

	t.Run("empty key list: ack without work", func(t *testing.T) {
		w, _, _, _ := newWorker(t)
		_, body := makeDeleteEvent(t)
		require.NoError(t, w.HandleDeleted(ctx, body, msgID))
	})

	t.Run("one key fails after retries: rest attempted, error returned", func(t *testing.T) {
		w, _, st, _ := newWorker(t)
		_, body := makeDeleteEvent(t,
			"originals/abc.jpg",
			"thumbnails/abc/100x100.jpg",
			"thumbnails/abc/300x300.jpg",
		)

		delErr := errors.New("s3 unavailable")
		// First key fails on every attempt → exhausts retries.
		st.EXPECT().Delete(ctx, "originals/abc.jpg").Return(delErr).Times(4)
		// Subsequent keys still attempted (best-effort cleanup).
		st.EXPECT().Delete(ctx, "thumbnails/abc/100x100.jpg").Return(nil)
		st.EXPECT().Delete(ctx, "thumbnails/abc/300x300.jpg").Return(nil)

		err := w.HandleDeleted(ctx, body, msgID)
		require.Error(t, err)
		require.ErrorIs(t, err, delErr)
		require.Contains(t, err.Error(), "1/3 keys")
	})
}
