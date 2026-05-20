package bootutil_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/bootutil"
)

func TestRedactURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "postgres dsn with user:pass",
			in:   "postgres://avatars:secretpass@localhost:5432/avatars?sslmode=disable",
			want: "postgres://***@localhost:5432/avatars?sslmode=disable",
		},
		{
			name: "amqp url with guest:guest",
			in:   "amqp://guest:guest@rabbitmq:5672/",
			want: "amqp://***@rabbitmq:5672/",
		},
		{
			name: "url without userinfo passes through",
			in:   "http://localhost:9000/",
			want: "http://localhost:9000/",
		},
		{
			name: "string without scheme separator passes through",
			in:   "not a url",
			want: "not a url",
		},
		{
			name: "user only, no password",
			in:   "postgres://alice@host:5432/db",
			want: "postgres://***@host:5432/db",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, bootutil.RedactURL(tc.in))
		})
	}
}

func TestProbeWithTimeout(t *testing.T) {
	t.Run("probe succeeds: no error", func(t *testing.T) {
		err := bootutil.ProbeWithTimeout(context.Background(), 100*time.Millisecond, "ok probe",
			func(context.Context) error { return nil })
		require.NoError(t, err)
	})

	t.Run("probe returns error: wrapped with label", func(t *testing.T) {
		sentinel := errors.New("boom")
		err := bootutil.ProbeWithTimeout(context.Background(), 100*time.Millisecond, "broken probe",
			func(context.Context) error { return sentinel })
		require.Error(t, err)
		require.ErrorIs(t, err, sentinel)
		require.Contains(t, err.Error(), "broken probe")
	})

	t.Run("probe respects timeout: context cancelled propagates", func(t *testing.T) {
		err := bootutil.ProbeWithTimeout(context.Background(), 10*time.Millisecond, "slow probe",
			func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			})
		require.Error(t, err)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}
