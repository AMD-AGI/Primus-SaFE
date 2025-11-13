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

// mockConn 是一个模拟的网络连接
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

// mockTimeoutError 模拟超时错误
type mockTimeoutError struct{}

func (e *mockTimeoutError) Error() string   { return "timeout" }
func (e *mockTimeoutError) Timeout() bool   { return true }
func (e *mockTimeoutError) Temporary() bool { return true }

func TestStatConnRead(t *testing.T) {
	t.Run("成功读取数据", func(t *testing.T) {
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

	t.Run("读取错误", func(t *testing.T) {
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

	t.Run("读取EOF不算错误", func(t *testing.T) {
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
		// 第一次读取成功
		n, err := sc.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 4, n)

		// 第二次读取返回EOF
		_, err = sc.Read(buf)
		assert.Equal(t, io.EOF, err)
	})
}

func TestStatConnWrite(t *testing.T) {
	t.Run("成功写入数据", func(t *testing.T) {
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

	t.Run("写入错误", func(t *testing.T) {
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

	t.Run("写入超时", func(t *testing.T) {
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
	t.Run("成功关闭连接", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")
		cm.conns.Inc() // 模拟连接计数增加

		mock := &mockConn{}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		err := sc.Close()

		assert.NoError(t, err)
		assert.True(t, mock.closeCalled)
	})

	t.Run("关闭连接失败", func(t *testing.T) {
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

	t.Run("多次关闭连接", func(t *testing.T) {
		ms := metrics.NewSet()
		cm := &connMetrics{}
		cm.init(ms, "test", "test_conn", "localhost:8080")
		cm.conns.Inc()

		mock := &mockConn{}
		sc := &statConn{
			Conn: mock,
			cm:   cm,
		}

		// 第一次关闭
		err := sc.Close()
		assert.NoError(t, err)
		assert.True(t, mock.closeCalled)

		// 重置标志
		mock.closeCalled = false

		// 第二次关闭不应该真正关闭底层连接
		err = sc.Close()
		assert.NoError(t, err)
		assert.False(t, mock.closeCalled)
	})
}

func TestConnMetricsInit(t *testing.T) {
	ms := metrics.NewSet()
	cm := &connMetrics{}

	cm.init(ms, "test_group", "test_name", "test_addr")

	// 验证所有指标都已初始化
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
	t.Run("读取操作增加指标", func(t *testing.T) {
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

		// 验证指标是否正确增加
		assert.Equal(t, uint64(1), cm.readCalls.Get())
		assert.Equal(t, uint64(9), cm.readBytes.Get())
	})

	t.Run("写入操作增加指标", func(t *testing.T) {
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

		// 验证指标是否正确增加
		assert.Equal(t, uint64(1), cm.writeCalls.Get())
		assert.Equal(t, uint64(9), cm.writtenBytes.Get())
	})
}

