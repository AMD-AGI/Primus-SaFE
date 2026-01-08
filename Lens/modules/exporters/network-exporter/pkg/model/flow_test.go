// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDirectionName(t *testing.T) {
	tests := []struct {
		name      string
		direction int
		expected  string
	}{
		{
			name:      "FlowTypeIngress-0",
			direction: FlowTypeIngress,
			expected:  FlowTypeNameIngress,
		},
		{
			name:      "FlowTypeIngress-explicit 0",
			direction: 0,
			expected:  "ingress",
		},
		{
			name:      "FlowTypeEgress-1",
			direction: FlowTypeEgress,
			expected:  FlowTypeNameEgress,
		},
		{
			name:      "FlowTypeEgress-explicit 1",
			direction: 1,
			expected:  "egress",
		},
		{
			name:      "other value - 2",
			direction: 2,
			expected:  FlowTypeNameEgress, // default returns egress
		},
		{
			name:      "other value - negative number",
			direction: -1,
			expected:  FlowTypeNameEgress,
		},
		{
			name:      "other value - large number",
			direction: 100,
			expected:  FlowTypeNameEgress,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDirectionName(tt.direction)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTcpConnUpstream_String(t *testing.T) {
	tests := []struct {
		name     string
		upstream TcpConnUpstream
		expected string
	}{
		{
			name: "normal IPv4 address",
			upstream: TcpConnUpstream{
				Addr:       "192.168.1.1",
				Port:       8080,
				Family:     2, // AF_INET
				ConnCount:  10,
				CloseCount: 5,
			},
			expected: "192.168.1.1-" + string(rune(8080)) + "-2", // string(int32) converts to Unicode character
		},
		{
			name: "IPv6 address",
			upstream: TcpConnUpstream{
				Addr:       "::1",
				Port:       443,
				Family:     10, // AF_INET6
				ConnCount:  20,
				CloseCount: 10,
			},
			expected: "::1-" + string(rune(443)) + "-10",
		},
		{
			name: "port is 0",
			upstream: TcpConnUpstream{
				Addr:       "10.0.0.1",
				Port:       0,
				Family:     2,
				ConnCount:  1,
				CloseCount: 0,
			},
			expected: "10.0.0.1-" + string(rune(0)) + "-2",
		},
		{
			name: "empty address",
			upstream: TcpConnUpstream{
				Addr:       "",
				Port:       80,
				Family:     2,
				ConnCount:  0,
				CloseCount: 0,
			},
			expected: "-" + string(rune(80)) + "-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.upstream.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTcpConnDownstream_String(t *testing.T) {
	tests := []struct {
		name       string
		downstream TcpConnDownstream
		expected   string
	}{
		{
			name: "normal IPv4 address",
			downstream: TcpConnDownstream{
				LocalPort:  8080,
				RemoteAddr: "192.168.1.100",
				Family:     2,
				ConnCount:  15,
				CloseCount: 8,
			},
			expected: string(rune(8080)) + "-192.168.1.100-2",
		},
		{
			name: "IPv6 address",
			downstream: TcpConnDownstream{
				LocalPort:  443,
				RemoteAddr: "2001:db8::1",
				Family:     10,
				ConnCount:  25,
				CloseCount: 12,
			},
			expected: string(rune(443)) + "-2001:db8::1-10",
		},
		{
			name: "port is 0",
			downstream: TcpConnDownstream{
				LocalPort:  0,
				RemoteAddr: "10.0.0.1",
				Family:     2,
				ConnCount:  1,
				CloseCount: 0,
			},
			expected: string(rune(0)) + "-10.0.0.1-2",
		},
		{
			name: "empty remote address",
			downstream: TcpConnDownstream{
				LocalPort:  80,
				RemoteAddr: "",
				Family:     2,
				ConnCount:  0,
				CloseCount: 0,
			},
			expected: string(rune(80)) + "--2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.downstream.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTcpFlowCacheKey_String(t *testing.T) {
	tests := []struct {
		name     string
		key      TcpFlowCacheKey
		expected string
	}{
		{
			name: "normal IPv4 flow",
			key: TcpFlowCacheKey{
				SAddr:  "192.168.1.1",
				Daddr:  "10.0.0.1",
				Sport:  12345,
				Dport:  80,
				Family: 2,
			},
			expected: "192.168.1.1-10.0.0.1-12345-80-2",
		},
		{
			name: "IPv6 flow",
			key: TcpFlowCacheKey{
				SAddr:  "::1",
				Daddr:  "2001:db8::1",
				Sport:  54321,
				Dport:  443,
				Family: 10,
			},
			expected: "::1-2001:db8::1-54321-443-10",
		},
		{
			name: "port is 0",
			key: TcpFlowCacheKey{
				SAddr:  "192.168.1.1",
				Daddr:  "192.168.1.2",
				Sport:  0,
				Dport:  0,
				Family: 2,
			},
			expected: "192.168.1.1-192.168.1.2-0-0-2",
		},
		{
			name: "empty address",
			key: TcpFlowCacheKey{
				SAddr:  "",
				Daddr:  "",
				Sport:  8080,
				Dport:  9090,
				Family: 2,
			},
			expected: "--8080-9090-2",
		},
		{
			name: "high port numbers",
			key: TcpFlowCacheKey{
				SAddr:  "192.168.1.1",
				Daddr:  "192.168.1.2",
				Sport:  65535,
				Dport:  65534,
				Family: 2,
			},
			expected: "192.168.1.1-192.168.1.2-65535-65534-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.key.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTcpFlowEvent(t *testing.T) {
	t.Run("TcpFlowEvent struct creation", func(t *testing.T) {
		event := TcpFlowEvent{
			TcpFlowCacheKey: TcpFlowCacheKey{
				SAddr:  "192.168.1.1",
				Daddr:  "10.0.0.1",
				Sport:  12345,
				Dport:  80,
				Family: 2,
			},
			DataLen: 1024,
		}
		
		assert.Equal(t, "192.168.1.1", event.SAddr)
		assert.Equal(t, "10.0.0.1", event.Daddr)
		assert.Equal(t, 12345, event.Sport)
		assert.Equal(t, 80, event.Dport)
		assert.Equal(t, 2, event.Family)
		assert.Equal(t, uint64(1024), event.DataLen)
	})
}

func TestTcpFlowDataValue(t *testing.T) {
	t.Run("TcpFlowDataValue struct creation", func(t *testing.T) {
		value := TcpFlowDataValue{
			RttTotal:  1000000,
			PktCount:  500,
			FlowData:  1048576,
			ConnCount: 10,
		}
		
		assert.Equal(t, uint64(1000000), value.RttTotal)
		assert.Equal(t, uint64(500), value.PktCount)
		assert.Equal(t, uint64(1048576), value.FlowData)
		assert.Equal(t, uint64(10), value.ConnCount)
	})
	
	t.Run("calculate average RTT", func(t *testing.T) {
		value := TcpFlowDataValue{
			RttTotal:  10000,
			PktCount:  100,
			FlowData:  0,
			ConnCount: 0,
		}
		
		// average RTT = RttTotal / PktCount
		avgRtt := float64(value.RttTotal) / float64(value.PktCount)
		assert.Equal(t, 100.0, avgRtt)
	})
}

func TestTcpConnReport(t *testing.T) {
	t.Run("TcpConnReport-ingress direction", func(t *testing.T) {
		report := TcpConnReport{
			Direction: FlowTypeIngress,
			Node:      "node-1",
			Ingress: &TcpConnDownstream{
				LocalPort:  8080,
				RemoteAddr: "192.168.1.100",
				Family:     2,
				ConnCount:  10,
				CloseCount: 5,
			},
			Egress:   nil,
			Duration: 60,
		}
		
		assert.Equal(t, uint8(FlowTypeIngress), report.Direction)
		assert.Equal(t, "node-1", report.Node)
		assert.NotNil(t, report.Ingress)
		assert.Nil(t, report.Egress)
		assert.Equal(t, int32(60), report.Duration)
	})
	
	t.Run("TcpConnReport-egress direction", func(t *testing.T) {
		report := TcpConnReport{
			Direction: FlowTypeEgress,
			Node:      "node-2",
			Ingress:   nil,
			Egress: &TcpConnUpstream{
				Addr:       "10.0.0.1",
				Port:       443,
				Family:     2,
				ConnCount:  20,
				CloseCount: 10,
			},
			Duration: 120,
		}
		
		assert.Equal(t, uint8(FlowTypeEgress), report.Direction)
		assert.Equal(t, "node-2", report.Node)
		assert.Nil(t, report.Ingress)
		assert.NotNil(t, report.Egress)
		assert.Equal(t, int32(120), report.Duration)
	})
}

