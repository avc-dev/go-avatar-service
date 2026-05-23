package broker_test

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"

	"github.com/avc-dev/go-avatar-service/internal/broker"
)

// TestTopologyDeclared exercises the passive declare trick: passive declares
// fail (with a 404 channel exception) if the entity does not exist with
// matching parameters. Each successful passive call therefore proves the
// constructor declared the entity correctly.
//
// NB: a failed passive declare closes the channel, so each assertion opens
// its own channel.
func TestTopologyDeclared(t *testing.T) {
	t.Run("main exchange exists", func(t *testing.T) {
		ch := rawChannel(t)
		err := ch.ExchangeDeclarePassive(
			broker.ExchangeName, broker.ExchangeType,
			true, false, false, false,
			nil,
		)
		require.NoError(t, err)
	})

	t.Run("dlx exchange exists", func(t *testing.T) {
		ch := rawChannel(t)
		err := ch.ExchangeDeclarePassive(
			broker.DLXName, "fanout",
			true, false, false, false,
			nil,
		)
		require.NoError(t, err)
	})

	t.Run("uploaded queue exists", func(t *testing.T) {
		ch := rawChannel(t)
		_, err := ch.QueueDeclarePassive(
			broker.QueueUploaded,
			true, false, false, false,
			amqp.Table{"x-dead-letter-exchange": broker.DLXName},
		)
		require.NoError(t, err)
	})

	t.Run("deleted queue exists", func(t *testing.T) {
		ch := rawChannel(t)
		_, err := ch.QueueDeclarePassive(
			broker.QueueDeleted,
			true, false, false, false,
			amqp.Table{"x-dead-letter-exchange": broker.DLXName},
		)
		require.NoError(t, err)
	})

	t.Run("dead queue exists", func(t *testing.T) {
		ch := rawChannel(t)
		_, err := ch.QueueDeclarePassive(
			broker.QueueDead,
			true, false, false, false,
			nil,
		)
		require.NoError(t, err)
	})
}
