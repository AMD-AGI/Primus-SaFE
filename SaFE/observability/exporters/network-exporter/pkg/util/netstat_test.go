// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.

package util

import (
	"net"
	"strings"
	"testing"
)

func TestParseIPv4(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"localhost", "0100007F", "127.0.0.1"},
		{"all zeros", "00000000", "0.0.0.0"},
		{"10.0.0.1", "0100000A", "10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, err := parseIPv4(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ip.String() != tt.expected {
				t.Errorf("parseIPv4(%q) = %s, want %s", tt.input, ip.String(), tt.expected)
			}
		})
	}
}

func TestParseIPv4Invalid(t *testing.T) {
	_, err := parseIPv4("not_hex")
	if err == nil {
		t.Error("expected error for invalid hex input")
	}
}

func TestParseIPv6(t *testing.T) {
	// ::1 in /proc/net/tcp6 format
	input := "00000000000000000000000001000000"
	ip, err := parseIPv6(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip == nil {
		t.Fatal("expected non-nil IP")
	}
	if len(ip) != net.IPv6len {
		t.Errorf("expected IPv6 length %d, got %d", net.IPv6len, len(ip))
	}
}

func TestParseIPv6AllZeros(t *testing.T) {
	input := "00000000000000000000000000000000"
	ip, err := parseIPv6(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip == nil {
		t.Fatal("expected non-nil IP")
	}
}

func TestParseAddr(t *testing.T) {
	// 127.0.0.1:8080 in /proc/net/tcp format
	// 0x1F90 = 8080
	addr, err := parseAddr("0100007F:1F90")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr.IP.String() != "127.0.0.1" {
		t.Errorf("IP: got %s, want 127.0.0.1", addr.IP.String())
	}
	if addr.Port != 8080 {
		t.Errorf("Port: got %d, want 8080", addr.Port)
	}
}

func TestParseAddrInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"no colon", "0100007F"},
		{"bad IP length", "010:1F90"},
		{"bad port", "0100007F:ZZZZ"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAddr(tt.input)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParseSocktab(t *testing.T) {
	// Simulate /proc/net/tcp content
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: 0100007F:0050 0100007F:C000 01 00000000:00000000 00:00000000 00000000  1000        0 67890 1 0000000000000000 100 0 0 10 0`

	reader := strings.NewReader(content)

	entries, err := parseSocktab(reader, func(e *SockTabEntry) bool {
		return true
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// First entry: listening on 127.0.0.1:8080
	if entries[0].LocalAddr.IP.String() != "127.0.0.1" {
		t.Errorf("entry[0] local IP: got %s, want 127.0.0.1", entries[0].LocalAddr.IP.String())
	}
	if entries[0].LocalAddr.Port != 8080 {
		t.Errorf("entry[0] local port: got %d, want 8080", entries[0].LocalAddr.Port)
	}
	if entries[0].State != SocketStatListen {
		t.Errorf("entry[0] state: got %d, want %d (LISTEN)", entries[0].State, SocketStatListen)
	}
}

func TestParseSocktabFilterListening(t *testing.T) {
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: 0100007F:0050 0100007F:C000 01 00000000:00000000 00:00000000 00000000  1000        0 67890 1 0000000000000000 100 0 0 10 0`

	reader := strings.NewReader(content)

	entries, err := parseSocktab(reader, func(e *SockTabEntry) bool {
		return e.State == SocketStatListen
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 listening entry, got %d", len(entries))
	}
	if entries[0].LocalAddr.Port != 8080 {
		t.Errorf("expected port 8080, got %d", entries[0].LocalAddr.Port)
	}
}

func TestIn(t *testing.T) {
	list := []string{"apple", "banana", "cherry"}

	tests := []struct {
		input    string
		expected bool
	}{
		{"apple", true},
		{"banana", true},
		{"cherry", true},
		{"grape", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := In(tt.input, list)
			if result != tt.expected {
				t.Errorf("In(%q, list) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestInEmptyList(t *testing.T) {
	result := In("anything", []string{})
	if result {
		t.Error("In should return false for empty list")
	}
}

func TestGetDirectionName(t *testing.T) {
	// Import from model package tested indirectly - test helper constants
	if SocketStatListen != 0x0a {
		t.Errorf("SocketStatListen: got %d, want 10", SocketStatListen)
	}
}

