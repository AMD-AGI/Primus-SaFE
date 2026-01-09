// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package netutil

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConn is a mock network connection
type mockConn struct {
	readData    []byte
	readPos     int
	writeData   []byte
	readErr     error
	writeErr    error
	closeErr    error
	readTimeout bool
	closeCalled bool
}

func (m *mockConn) Read(p []byte) (n int, err error) {
	if m.readTimeout {
		time.Sleep(2 * time.Second)
		return 0, &mockTimeoutError{}
	}
	if m.readErr != nil {
		return 0, m.readErr
	}
	if m.readPos >= len(m.readData) {
		return 0, io.EOF
	}
	n = copy(p, m.readData[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *mockConn) Write(p []byte) (n int, err error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.writeData = append(m.writeData, p...)
	return len(p), nil
}

func (m *mockConn) Close() error {
	m.closeCalled = true
	return m.closeErr
}

func (m *mockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
}

func (m *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678}
}

func (m *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// mockTimeoutError simulates timeout error
type mockTimeoutError struct{}

func (e *mockTimeoutError) Error() string   { return "timeout" }
func (e *mockTimeoutError) Timeout() bool   { return true }
func (e *mockTimeoutError) Temporary() bool { return true }

func TestStatConnRead(t *testing.T) {
	t.Run("successfully read data", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")

		mock := &mockConn{
			readData: []byte("hello world"),
		}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		buf := make([]byte, 100)
		n, err := sc.Read(buf)

		assert.NoError(t, err)
		assert.Equal(t, 11, n)
		assert.Equal(t, "hello world", string(buf[:n]))
	})

	t.Run("read error", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")

		mock := &mockConn{
			readErr: assert.AnError,
		}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		buf := make([]byte, 100)
		_, err := sc.Read(buf)

		assert.Error(t, err)
	})

	t.Run("reading EOF is not an error", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")

		mock := &mockConn{
			readData: []byte("data"),
		}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		buf := make([]byte, 100)
		// first read succeeds
		n, err := sc.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 4, n)

		// second read returns EOF
		_, err = sc.Read(buf)
		assert.Equal(t, io.EOF, err)
	})
}

func TestStatConnWrite(t *testing.T) {
	t.Run("successfully write data", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")

		mock := &mockConn{}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		data := []byte("hello world")
		n, err := sc.Write(data)

		assert.NoError(t, err)
		assert.Equal(t, 11, n)
		assert.Equal(t, "hello world", string(mock.writeData))
	})

	t.Run("write error", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")

		mock := &mockConn{
			writeErr: assert.AnError,
		}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		data := []byte("hello")
		_, err := sc.Write(data)

		assert.Error(t, err)
	})

	t.Run("write timeout", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")

		mock := &mockConn{
			writeErr: &mockTimeoutError{},
		}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		data := []byte("hello")
		_, err := sc.Write(data)

		assert.Error(t, err)
		var ne net.Error
		assert.ErrorAs(t, err, &ne)
		assert.True(t, ne.Timeout())
	})
}

func TestStatConnClose(t *testing.T) {
	t.Run("successfully close connection", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")
		cm.conns.Inc() // simulate connection count increment

		mock := &mockConn{}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		err := sc.Close()

		assert.NoError(t, err)
		assert.True(t, mock.closeCalled)
	})

	t.Run("failed to close connection", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")
		cm.conns.Inc()

		mock := &mockConn{
			closeErr: assert.AnError,
		}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		err := sc.Close()

		assert.Error(t, err)
		assert.True(t, mock.closeCalled)
	})

	t.Run("close connection multiple times", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")
		cm.conns.Inc()

		mock := &mockConn{}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		// first close
		err := sc.Close()
		assert.NoError(t, err)
		assert.True(t, mock.closeCalled)

		// reset flag
		mock.closeCalled = false

		// second close should not actually close underlying connection
		err = sc.Close()
		assert.NoError(t, err)
		assert.False(t, mock.closeCalled)
	})
}

func TestConnMetricsInit(t *testing.T) {
	ms := metrics.NewSet()
	cm := &connMetrics{}

	cm.init(ms, "test_group", "test_name", "test_addr")

	// verify all metrics are initialized
	assert.NotNil(t, cm.readCalls)
	assert.NotNil(t, cm.readBytes)
	assert.NotNil(t, cm.readErrors)
	assert.NotNil(t, cm.readTimeouts)
	assert.NotNil(t, cm.writeCalls)
	assert.NotNil(t, cm.writtenBytes)
	assert.NotNil(t, cm.writeErrors)
	assert.NotNil(t, cm.writeTimeouts)
	assert.NotNil(t, cm.closeErrors)
	assert.NotNil(t, cm.conns)
}

func TestStatConnMetricsIncrement(t *testing.T) {
	t.Run("read operation increments metrics", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")

		mock := &mockConn{
			readData: []byte("test data"),
		}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		buf := make([]byte, 100)
		_, err := sc.Read(buf)
		require.NoError(t, err)

		// verify metrics are correctly incremented
		assert.Equal(t, uint64(1), cm.readCalls.Get())
		assert.Equal(t, uint64(9), cm.readBytes.Get())
	})

	t.Run("write operation increments metrics", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")

		mock := &mockConn{}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		data := []byte("test data")
		_, err := sc.Write(data)
		require.NoError(t, err)

		// verify metrics are correctly incremented
		assert.Equal(t, uint64(1), cm.writeCalls.Get())
		assert.Equal(t, uint64(9), cm.writtenBytes.Get())
	})
}

