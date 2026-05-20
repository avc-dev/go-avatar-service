package broker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/broker"
	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// received aggregates what a Handler observed, so the test goroutine can
// inspect it after Consume returns.
type received struct {
	mu        sync.Mutex
	body      []byte
	messageID string
}

func (r *received) set(body []byte, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.body = append([]byte(nil), body...)
	r.messageID = id
}

func (r *received) snapshot() ([]byte, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body, r.messageID
}

func TestConsumeDeliversPublishedMessage(t *testing.T) {
	purgeQueue(t, broker.QueueUploaded)
	purgeQueue(t, broker.QueueDead)

	event := domain.AvatarUploadEvent{
		AvatarID: uuid.New(),
		UserID:   uuid.New(),
		S3Key:    "avatars/" + uuid.NewString() + "/original.png",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := &received{}
	done := make(chan struct{})

	go func() {
		_ = testRMQ.Consume(ctx, broker.QueueUploaded, func(_ context.Context, body []byte, id string) error {
			got.set(body, id)
			close(done)
			return nil
		})
	}()

	// Give Consume a moment to register before publishing — not strictly
	// required (amqp queues hold messages until a consumer arrives) but it
	// makes failures easier to diagnose.
	time.Sleep(100 * time.Millisecond)

	pubCtx, pubCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pubCancel()
	require.NoError(t, testRMQ.PublishAvatarUploaded(pubCtx, event))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never received message")
	}

	body, id := got.snapshot()
	require.NotEmpty(t, body)
	require.Equal(t, event.AvatarID.String(), id)
}

func TestConsumeHandlerErrorRoutesToDLX(t *testing.T) {
	purgeQueue(t, broker.QueueUploaded)
	purgeQueue(t, broker.QueueDead)

	event := domain.AvatarUploadEvent{
		AvatarID: uuid.New(),
		UserID:   uuid.New(),
		S3Key:    "avatars/" + uuid.NewString() + "/original.png",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handlerCalled := make(chan struct{}, 1)
	go func() {
		_ = testRMQ.Consume(ctx, broker.QueueUploaded, func(_ context.Context, _ []byte, _ string) error {
			select {
			case handlerCalled <- struct{}{}:
			default:
			}
			return errors.New("forced failure")
		})
	}()

	time.Sleep(100 * time.Millisecond)

	pubCtx, pubCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pubCancel()
	require.NoError(t, testRMQ.PublishAvatarUploaded(pubCtx, event))

	select {
	case <-handlerCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("handler was never invoked")
	}

	// Verify the message landed on QueueDead via a raw client.
	d := pollDLX(t, broker.QueueDead, event.AvatarID.String(), 5*time.Second)
	require.Equal(t, event.AvatarID.String(), d.MessageId)
	require.Equal(t, "application/json", d.ContentType)
}

// pollDLX waits up to timeout for a message with the given messageID to
// appear on queueName. Returns the delivery once found; fails the test
// otherwise. Messages with non-matching IDs are nack-requeued so concurrent
// tests stay isolated.
func pollDLX(t *testing.T, queueName, wantID string, timeout time.Duration) amqp.Delivery {
	t.Helper()
	ch := rawChannel(t)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg, ok, err := ch.Get(queueName, false /*autoAck*/)
		require.NoError(t, err)
		if !ok {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if msg.MessageId == wantID {
			require.NoError(t, msg.Ack(false))
			return msg
		}
		require.NoError(t, msg.Nack(false, true))
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no message with id %s arrived on %s within %s", wantID, queueName, timeout)
	return amqp.Delivery{}
}
