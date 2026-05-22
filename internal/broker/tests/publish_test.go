package broker_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/broker"
	"github.com/avc-dev/go-avatar-service/internal/domain"
)

// readOne pulls a single delivery from queueName via a basic.get poll loop.
// It avoids the streaming Consume API so the test stays linear; we don't
// need throughput here.
func readOne(t *testing.T, queueName string) amqp.Delivery {
	t.Helper()
	ch := rawChannel(t)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		msg, ok, err := ch.Get(queueName, false /*autoAck*/)
		require.NoError(t, err)
		if ok {
			require.NoError(t, msg.Ack(false))
			return msg
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no message arrived on %s within deadline", queueName)
	return amqp.Delivery{}
}

func TestPublishAvatarUploaded(t *testing.T) {
	purgeQueue(t, broker.QueueUploaded)

	event := domain.AvatarUploadEvent{
		AvatarID: uuid.New(),
		UserID:   uuid.New(),
		S3Key:    "avatars/" + uuid.NewString() + "/original.png",
	}

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	err := testRMQ.PublishAvatarUploaded(ctx, event)
	require.NoError(t, err)

	d := readOne(t, broker.QueueUploaded)
	require.Equal(t, "application/json", d.ContentType)
	require.Equal(t, broker.RoutingUploaded, d.RoutingKey)
	require.Equal(t, event.AvatarID.String(), d.MessageId)

	var got domain.AvatarUploadEvent
	require.NoError(t, json.Unmarshal(d.Body, &got))
	require.Equal(t, event, got)
}

func TestPublishAvatarDeleted(t *testing.T) {
	purgeQueue(t, broker.QueueDeleted)

	event := domain.AvatarDeleteEvent{
		AvatarID: uuid.New(),
		S3Keys: []string{
			"avatars/x/original.png",
			"avatars/x/thumb-128.png",
		},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	err := testRMQ.PublishAvatarDeleted(ctx, event)
	require.NoError(t, err)

	d := readOne(t, broker.QueueDeleted)
	require.Equal(t, "application/json", d.ContentType)
	require.Equal(t, broker.RoutingDeleted, d.RoutingKey)
	require.Equal(t, event.AvatarID.String(), d.MessageId)

	var got domain.AvatarDeleteEvent
	require.NoError(t, json.Unmarshal(d.Body, &got))
	require.Equal(t, event, got)
}
