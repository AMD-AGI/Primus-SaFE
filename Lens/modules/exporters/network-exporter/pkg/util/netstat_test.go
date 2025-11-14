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
			name:        "normal IPv4 address - localhost",
			input:       "0100007F", // 127.0.0.1 in little-endian hex
			expected:    "127.0.0.1",
			expectError: false,
		},
		{
			name:        "normal IPv4 address - 0.0.0.0",
			input:       "00000000",
			expected:    "0.0.0.0",
			expectError: false,
		},
		{
			name:        "normal IPv4 address - 192.168.1.1",
			input:       "0101A8C0", // 192.168.1.1 in little-endian
			expected:    "192.168.1.1",
			expectError: false,
		},
		{
			name:        "normal IPv4 address - 10.0.0.1",
			input:       "0100000A", // 10.0.0.1 in little-endian
			expected:    "10.0.0.1",
			expectError: false,
		},
		{
			name:        "normal IPv4 address - 255.255.255.255",
			input:       "FFFFFFFF",
			expected:    "255.255.255.255",
			expectError: false,
		},
		{
			name:        "invalid input - non-hex characters",
			input:       "ZZZZZZZZ",
			expected:    "",
			expectError: true,
		},
		{
			name:        "invalid input - insufficient length but parseable",
			input:       "01000",
			expected:    "0.16.0.0", // strconv.ParseUint will parse successfully
			expectError: false,
		},
		{
			name:        "invalid input - empty string",
			input:       "",
			expected:    "",
			expectError: true,
		},
		{
			name:        "invalid input - length exceeds 8",
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
			input:       "00000000000000000000000001000000", // ::1 in little-endian
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
			name:        "IPv6 - specific address",
			input:       "00000000000000000000000000000001",
			expected:    "::100:0", // Go will use shortest IPv6 notation
			expectError: false,
		},
		{
			name:        "IPv6 - another address",
			input:       "FFFFFFFF00000000000000000000FFFF",
			expected:    "ffff:ffff::ffff:0",
			expectError: false,
		},
		{
			name:        "invalid input - insufficient length but parseable",
			input:       "0000000000000000",
			expected:    "::",
			expectError: false,
		},
		{
			name:        "invalid input - non-hex characters",
			input:       "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",
			expected:    "",
			expectError: true,
		},
		{
			name:        "invalid input - empty string returns zero address",
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
		name         string
		input        string
		expectedIP   string
		expectedPort uint16
		expectError  bool
	}{
		{
			name:         "IPv4 address with port - 127.0.0.1:80",
			input:        "0100007F:0050", // 127.0.0.1:80
			expectedIP:   "127.0.0.1",
			expectedPort: 80,
			expectError:  false,
		},
		{
			name:         "IPv4 address with port - 0.0.0.0:8080",
			input:        "00000000:1F90", // 0.0.0.0:8080 (0x1F90 = 8080)
			expectedIP:   "0.0.0.0",
			expectedPort: 8080,
			expectError:  false,
		},
		{
			name:         "IPv4 address with port - 192.168.1.100:443",
			input:        "6401A8C0:01BB", // 192.168.1.100:443 (0x01BB = 443)
			expectedIP:   "192.168.1.100",
			expectedPort: 443,
			expectError:  false,
		},
		{
			name:         "IPv4 address with port - port is 0",
			input:        "0100007F:0000",
			expectedIP:   "127.0.0.1",
			expectedPort: 0,
			expectError:  false,
		},
		{
			name:         "IPv4 address with port - port is 65535",
			input:        "0100007F:FFFF",
			expectedIP:   "127.0.0.1",
			expectedPort: 65535,
			expectError:  false,
		},
		{
			name:         "IPv6 address with port - [::1]:80",
			input:        "00000000000000000000000001000000:0050",
			expectedIP:   "::1",
			expectedPort: 80,
			expectError:  false,
		},
		{
			name:         "IPv6 address with port - all-zero address",
			input:        "00000000000000000000000000000000:1F90",
			expectedIP:   "::",
			expectedPort: 8080,
			expectError:  false,
		},
		{
			name:        "invalid input - missing port",
			input:       "0100007F",
			expectedIP:  "",
			expectError: true,
		},
		{
			name:        "invalid input - missing colon",
			input:       "0100007F0050",
			expectedIP:  "",
			expectError: true,
		},
		{
			name:        "invalid input - empty string",
			input:       "",
			expectedIP:  "",
			expectError: true,
		},
		{
			name:        "invalid input - port not hex",
			input:       "0100007F:ZZZZ",
			expectedIP:  "",
			expectError: true,
		},
		{
			name:        "invalid input - IP not hex",
			input:       "ZZZZZZZZ:0050",
			expectedIP:  "",
			expectError: true,
		},
		{
			name:        "invalid input - incorrect IP length",
			input:       "01000:0050", // not 8 or 32 characters
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
			name:     "normal process name",
			input:    []byte("(sshd)"),
			expected: "sshd",
		},
		{
			name:     "normal process name - with spaces",
			input:    []byte("(my process)"),
			expected: "my process",
		},
		{
			name:     "normal process name - with numbers",
			input:    []byte("(nginx-1)"),
			expected: "nginx-1",
		},
		{
			name:     "normal process name - with special characters",
			input:    []byte("(my-app_v1.0)"),
			expected: "my-app_v1.0",
		},
		{
			name:     "empty process name",
			input:    []byte("()"),
			expected: "",
		},
		{
			name:     "only left parenthesis",
			input:    []byte("(sshd"),
			expected: "",
		},
		{
			name:     "only right parenthesis",
			input:    []byte("sshd)"),
			expected: "",
		},
		{
			name:     "missing parentheses",
			input:    []byte("sshd"),
			expected: "",
		},
		{
			name:     "empty byte array",
			input:    []byte(""),
			expected: "",
		},
		{
			name:     "incorrect parenthesis order",
			input:    []byte(")sshd("),
			expected: "",
		},
		{
			name:     "multiple parenthesis pairs - take outermost",
			input:    []byte("(outer(inner))"),
			expected: "outer(inner)", // LastIndex will find the last right parenthesis
		},
		{
			name:     "contains complete stat format",
			input:    []byte("1234 (process-name) S 1 1234 1234"),
			expected: "process-name",
		},
		{
			name:     "long process name",
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
	t.Run("Socket basic functionality", func(t *testing.T) {
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
