package util

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseIPv4(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "正常IPv4地址-localhost",
			input:       "0100007F", // 127.0.0.1 的小端序hex
			expected:    "127.0.0.1",
			expectError: false,
		},
		{
			name:        "正常IPv4地址-0.0.0.0",
			input:       "00000000",
			expected:    "0.0.0.0",
			expectError: false,
		},
		{
			name:        "正常IPv4地址-192.168.1.1",
			input:       "0101A8C0", // 192.168.1.1 的小端序
			expected:    "192.168.1.1",
			expectError: false,
		},
		{
			name:        "正常IPv4地址-10.0.0.1",
			input:       "0100000A", // 10.0.0.1 的小端序
			expected:    "10.0.0.1",
			expectError: false,
		},
		{
			name:        "正常IPv4地址-255.255.255.255",
			input:       "FFFFFFFF",
			expected:    "255.255.255.255",
			expectError: false,
		},
		{
			name:        "无效输入-非hex字符",
			input:       "ZZZZZZZZ",
			expected:    "",
			expectError: true,
		},
		{
			name:        "无效输入-长度不足但能解析",
			input:       "01000",
			expected:    "0.16.0.0", // strconv.ParseUint会解析成功
			expectError: false,
		},
		{
			name:        "无效输入-空字符串",
			input:       "",
			expected:    "",
			expectError: true,
		},
		{
			name:        "无效输入-长度超过8",
			input:       "0100007F00",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseIPv4(tt.input)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected, result.String())
				assert.Equal(t, net.IPv4len, len(result))
			}
		})
	}
}

func TestParseIPv6(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "IPv6-localhost",
			input:       "00000000000000000000000001000000", // ::1 的小端序
			expected:    "::1",
			expectError: false,
		},
		{
			name:        "IPv6-all zeros",
			input:       "00000000000000000000000000000000", // ::
			expected:    "::",
			expectError: false,
		},
		{
			name:        "IPv6-特定地址",
			input:       "00000000000000000000000000000001",
			expected:    "::100:0", // Go会使用最短的IPv6表示法
			expectError: false,
		},
		{
			name:        "IPv6-另一个地址",
			input:       "FFFFFFFF00000000000000000000FFFF",
			expected:    "ffff:ffff::ffff:0",
			expectError: false,
		},
		{
			name:        "无效输入-长度不足但能解析",
			input:       "0000000000000000",
			expected:    "::",
			expectError: false,
		},
		{
			name:        "无效输入-非hex字符",
			input:       "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",
			expected:    "",
			expectError: true,
		},
		{
			name:        "无效输入-空字符串返回零地址",
			input:       "",
			expected:    "::",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseIPv6(tt.input)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected, result.String())
				assert.Equal(t, net.IPv6len, len(result))
			}
		})
	}
}

func TestParseAddr(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedIP  string
		expectedPort uint16
		expectError bool
	}{
		{
			name:         "IPv4地址带端口-127.0.0.1:80",
			input:        "0100007F:0050", // 127.0.0.1:80
			expectedIP:   "127.0.0.1",
			expectedPort: 80,
			expectError:  false,
		},
		{
			name:         "IPv4地址带端口-0.0.0.0:8080",
			input:        "00000000:1F90", // 0.0.0.0:8080 (0x1F90 = 8080)
			expectedIP:   "0.0.0.0",
			expectedPort: 8080,
			expectError:  false,
		},
		{
			name:         "IPv4地址带端口-192.168.1.100:443",
			input:        "6401A8C0:01BB", // 192.168.1.100:443 (0x01BB = 443)
			expectedIP:   "192.168.1.100",
			expectedPort: 443,
			expectError:  false,
		},
		{
			name:         "IPv4地址带端口-端口为0",
			input:        "0100007F:0000",
			expectedIP:   "127.0.0.1",
			expectedPort: 0,
			expectError:  false,
		},
		{
			name:         "IPv4地址带端口-端口为65535",
			input:        "0100007F:FFFF",
			expectedIP:   "127.0.0.1",
			expectedPort: 65535,
			expectError:  false,
		},
		{
			name:         "IPv6地址带端口-[::1]:80",
			input:        "00000000000000000000000001000000:0050",
			expectedIP:   "::1",
			expectedPort: 80,
			expectError:  false,
		},
		{
			name:         "IPv6地址带端口-全零地址",
			input:        "00000000000000000000000000000000:1F90",
			expectedIP:   "::",
			expectedPort: 8080,
			expectError:  false,
		},
		{
			name:        "无效输入-缺少端口",
			input:       "0100007F",
			expectedIP:  "",
			expectError: true,
		},
		{
			name:        "无效输入-缺少冒号",
			input:       "0100007F0050",
			expectedIP:  "",
			expectError: true,
		},
		{
			name:        "无效输入-空字符串",
			input:       "",
			expectedIP:  "",
			expectError: true,
		},
		{
			name:        "无效输入-端口非hex",
			input:       "0100007F:ZZZZ",
			expectedIP:  "",
			expectError: true,
		},
		{
			name:        "无效输入-IP非hex",
			input:       "ZZZZZZZZ:0050",
			expectedIP:  "",
			expectError: true,
		},
		{
			name:        "无效输入-IP长度不对",
			input:       "01000:0050", // 不是8或32字符
			expectedIP:  "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseAddr(tt.input)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedIP, result.IP.String())
				assert.Equal(t, tt.expectedPort, result.Port)
			}
		})
	}
}

func TestGetProcName(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "正常进程名",
			input:    []byte("(sshd)"),
			expected: "sshd",
		},
		{
			name:     "正常进程名-包含空格",
			input:    []byte("(my process)"),
			expected: "my process",
		},
		{
			name:     "正常进程名-包含数字",
			input:    []byte("(nginx-1)"),
			expected: "nginx-1",
		},
		{
			name:     "正常进程名-包含特殊字符",
			input:    []byte("(my-app_v1.0)"),
			expected: "my-app_v1.0",
		},
		{
			name:     "进程名为空",
			input:    []byte("()"),
			expected: "",
		},
		{
			name:     "只有左括号",
			input:    []byte("(sshd"),
			expected: "",
		},
		{
			name:     "只有右括号",
			input:    []byte("sshd)"),
			expected: "",
		},
		{
			name:     "缺少括号",
			input:    []byte("sshd"),
			expected: "",
		},
		{
			name:     "空字节数组",
			input:    []byte(""),
			expected: "",
		},
		{
			name:     "括号顺序错误",
			input:    []byte(")sshd("),
			expected: "",
		},
		{
			name:     "多个括号对-取最外层",
			input:    []byte("(outer(inner))"),
			expected: "outer(inner)", // LastIndex会找到最后一个右括号
		},
		{
			name:     "包含完整stat格式",
			input:    []byte("1234 (process-name) S 1 1234 1234"),
			expected: "process-name",
		},
		{
			name:     "长进程名",
			input:    []byte("(very-long-process-name-with-many-characters)"),
			expected: "very-long-process-name-with-many-characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getProcName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSocket_String(t *testing.T) {
	t.Run("Socket基本功能", func(t *testing.T) {
		socket := &Socket{
			IP:   net.ParseIP("192.168.1.1"),
			Port: 8080,
		}
		
		assert.NotNil(t, socket)
		assert.Equal(t, "192.168.1.1", socket.IP.String())
		assert.Equal(t, uint16(8080), socket.Port)
	})

	t.Run("Socket-IPv6", func(t *testing.T) {
		socket := &Socket{
			IP:   net.ParseIP("::1"),
			Port: 443,
		}
		
		assert.NotNil(t, socket)
		assert.Equal(t, "::1", socket.IP.String())
		assert.Equal(t, uint16(443), socket.Port)
	})
}

