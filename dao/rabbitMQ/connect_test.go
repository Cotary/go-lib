package rabbitMQ

import (
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

// Mock connection to replace actual RabbitMQ connection
type MockConnection struct {
	amqp.Connection
	closed bool
}

func (m *MockConnection) IsClosed() bool {
	return m.closed
}

func (m *MockConnection) Close() error {
	m.closed = true
	return nil
}

func TestNewRabbitMQ(t *testing.T) {
	config := Config{
		DSN:       []string{"amqp://guest:guest@localhost:5672/"},
		Heartbeat: 1, // Set low heartbeat for testing purposes
	}

	rabbitMQ, err := NewRabbitMQ(config)
	assert.NoError(t, err)
	assert.NotNil(t, rabbitMQ)

	// Ensure health check is running
	time.Sleep(2 * time.Second)
	assert.False(t, rabbitMQ.Conn.IsClosed())
}

func TestClose(t *testing.T) {
	config := Config{
		DSN:       []string{"amqp://guest:guest@localhost:5672/"},
		Heartbeat: 1, // Set low heartbeat for testing purposes
	}

	rabbitMQ, err := NewRabbitMQ(config)
	assert.NoError(t, err)
	assert.NotNil(t, rabbitMQ)

	rabbitMQ.Close()

	// Verify that the connection is closed
	assert.True(t, rabbitMQ.Conn.IsClosed())

	// Ensure health check is stopped
	time.Sleep(2 * time.Second)
	assert.True(t, rabbitMQ.Conn.IsClosed())
}
