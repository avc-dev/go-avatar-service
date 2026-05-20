package broker_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcrabbitmq "github.com/testcontainers/testcontainers-go/modules/rabbitmq"

	"github.com/avc-dev/go-avatar-service/internal/broker"
)

var (
	testContainer testcontainers.Container
	testRMQ       *broker.RabbitMQ
	testURL       string
)

// TestMain owns the lifecycle of the shared RabbitMQ container: one is spun
// up before any tests run, a *broker.RabbitMQ is built against it (which
// also declares the project topology), and everything is torn down after
// the suite finishes.
func TestMain(m *testing.M) {
	if err := mainSetup(); err != nil {
		log.Fatalf("broker test setup: %v", err)
	}
	code := m.Run()
	mainTeardown()
	os.Exit(code)
}

func mainSetup() error {
	ctx := context.Background()

	ctr, err := tcrabbitmq.Run(ctx, "rabbitmq:3.13-management-alpine")
	if err != nil {
		return fmt.Errorf("start rabbitmq: %w", err)
	}
	testContainer = ctr

	url, err := ctr.AmqpURL(ctx)
	if err != nil {
		return fmt.Errorf("rabbitmq url: %w", err)
	}
	testURL = url

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	r, err := broker.NewRabbitMQ(url, logger)
	if err != nil {
		return fmt.Errorf("broker.NewRabbitMQ: %w", err)
	}
	testRMQ = r

	return nil
}

func mainTeardown() {
	if testRMQ != nil {
		_ = testRMQ.Close()
	}
	if testContainer != nil {
		_ = testContainer.Terminate(context.Background())
	}
}

// rawConn opens a fresh amqp connection to the test container. Tests use it
// to publish/consume directly when they need to bypass the SUT.
func rawConn(t *testing.T) *amqp.Connection {
	t.Helper()
	conn, err := amqp.Dial(testURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// rawChannel opens a fresh channel from a fresh connection, suitable for one
// arrange/assert step. Cleanup is registered with t.
func rawChannel(t *testing.T) *amqp.Channel {
	t.Helper()
	ch, err := rawConn(t).Channel()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ch.Close() })
	return ch
}

// purgeQueue empties queueName so the test starts from a clean slate.
func purgeQueue(t *testing.T, queueName string) {
	t.Helper()
	ch := rawChannel(t)
	_, err := ch.QueuePurge(queueName, false)
	require.NoError(t, err)
}
