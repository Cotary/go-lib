package rabbitMQ

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDSN = "amqp://guest:guest@localhost:5672/"

func tryConnect(t *testing.T) *Connect {
	t.Helper()
	config := Config{
		DSN:        []string{testDSN},
		Heartbeat:  5,
		MaxChannel: 10,
	}
	conn, err := NewRabbitMQ(config)
	if err != nil {
		t.Skipf("RabbitMQ not available, skipping: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestNewRabbitMQ(t *testing.T) {
	conn := tryConnect(t)
	assert.NotNil(t, conn.conn)
	assert.False(t, conn.conn.IsClosed())
}

func TestNewRabbitMQ_BadDSN(t *testing.T) {
	config := Config{
		DSN:       []string{"amqp://bad:bad@127.0.0.1:9999/"},
		Heartbeat: 1,
	}
	conn, err := NewRabbitMQ(config)
	assert.Error(t, err)
	assert.Nil(t, conn)
}

func TestClose_Idempotent(t *testing.T) {
	conn := tryConnect(t)

	conn.Close()
	assert.True(t, conn.conn == nil || conn.conn.IsClosed())

	assert.NotPanics(t, func() {
		conn.Close()
	})
}

func TestGetCh_AfterConnect(t *testing.T) {
	conn := tryConnect(t)

	ch, err := conn.GetCh()
	require.NoError(t, err)
	require.NotNil(t, ch)
	assert.False(t, ch.IsClosed())
	_ = ch.Close()
}

func TestGetCh_AfterClose(t *testing.T) {
	conn := tryConnect(t)
	conn.Close()

	ch, err := conn.GetCh()
	assert.Error(t, err)
	assert.Nil(t, ch)
}

func TestPutCh_ReturnAndReuse(t *testing.T) {
	conn := tryConnect(t)

	ch, err := conn.GetCh()
	require.NoError(t, err)
	conn.PutCh(ch)

	ch2, err := conn.GetCh()
	require.NoError(t, err)
	assert.NotNil(t, ch2)
	_ = ch2.Close()
}

func TestPutCh_NilAndClosed(t *testing.T) {
	conn := tryConnect(t)

	assert.NotPanics(t, func() {
		conn.PutCh(nil)
	})

	ch, err := conn.GetCh()
	require.NoError(t, err)
	_ = ch.Close()
	assert.NotPanics(t, func() {
		conn.PutCh(ch)
	})
}

func TestGetCh_Concurrent(t *testing.T) {
	conn := tryConnect(t)

	var wg sync.WaitGroup
	errCh := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, err := conn.GetCh()
			if err != nil {
				errCh <- err
				return
			}
			time.Sleep(10 * time.Millisecond)
			conn.PutCh(ch)
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent GetCh error: %v", err)
	}
}

func TestBackoff(t *testing.T) {
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
		{6, 60 * time.Second},
		{10, 60 * time.Second},
	}
	for _, tt := range tests {
		got := backoff(tt.attempt)
		assert.Equal(t, tt.expected, got, "attempt=%d", tt.attempt)
	}
}
