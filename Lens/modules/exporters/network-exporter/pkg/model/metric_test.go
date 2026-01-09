// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTcpEgressMetricValue_String(t *testing.T) {
	tests := []struct {
		name     string
		metric   TcpEgressMetricValue
		expected string
	}{
		{
			name: "normal Egress metric-outbound",
			metric: TcpEgressMetricValue{
				Raddr:     "192.168.1.1",
				Rport:     8080,
				Direction: DirectionOutbound,
				Type:      "bandwidth",
				Value:     1024.5,
			},
			expected: "192.168.1.1_" + string(rune(8080)) + "_outbound_bandwidth",
		},
		{
			name: "IPv6 address",
			metric: TcpEgressMetricValue{
				Raddr:     "2001:db8::1",
				Rport:     443,
				Direction: DirectionOutbound,
				Type:      "latency",
				Value:     50.25,
			},
			expected: "2001:db8::1_" + string(rune(443)) + "_outbound_latency",
		},
		{
			name: "port is 0",
			metric: TcpEgressMetricValue{
				Raddr:     "10.0.0.1",
				Rport:     0,
				Direction: DirectionOutbound,
				Type:      "connections",
				Value:     100,
			},
			expected: "10.0.0.1_" + string(rune(0)) + "_outbound_connections",
		},
		{
			name: "empty address",
			metric: TcpEgressMetricValue{
				Raddr:     "",
				Rport:     8080,
				Direction: DirectionOutbound,
				Type:      "throughput",
				Value:     0,
			},
			expected: "_" + string(rune(8080)) + "_outbound_throughput",
		},
		{
			name: "high port number",
			metric: TcpEgressMetricValue{
				Raddr:     "192.168.1.1",
				Rport:     65535,
				Direction: DirectionOutbound,
				Type:      "packets",
				Value:     10000,
			},
			expected: "192.168.1.1_" + string(rune(65535)) + "_outbound_packets",
		},
		{
			name: "Inbound direction",
			metric: TcpEgressMetricValue{
				Raddr:     "10.0.0.1",
				Rport:     9090,
				Direction: DirectionInbound,
				Type:      "bytes",
				Value:     2048.75,
			},
			expected: "10.0.0.1_" + string(rune(9090)) + "_inbound_bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metric.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTcpIngressMetricValue_String(t *testing.T) {
	tests := []struct {
		name     string
		metric   TcpIngressMetricValue
		expected string
	}{
		{
			name: "normal Ingress metric-inbound",
			metric: TcpIngressMetricValue{
				Lport:     8080,
				Raddr:     "192.168.1.100",
				Direction: DirectionInbound,
				Type:      "bandwidth",
				Value:     2048.5,
			},
			expected: string(rune(8080)) + "_192.168.1.100_inbound_bandwidth",
		},
		{
			name: "IPv6 remote address",
			metric: TcpIngressMetricValue{
				Lport:     443,
				Raddr:     "::1",
				Direction: DirectionInbound,
				Type:      "latency",
				Value:     25.5,
			},
			expected: string(rune(443)) + "_::1_inbound_latency",
		},
		{
			name: "local port is 0",
			metric: TcpIngressMetricValue{
				Lport:     0,
				Raddr:     "10.0.0.1",
				Direction: DirectionInbound,
				Type:      "connections",
				Value:     50,
			},
			expected: string(rune(0)) + "_10.0.0.1_inbound_connections",
		},
		{
			name: "empty remote address",
			metric: TcpIngressMetricValue{
				Lport:     8080,
				Raddr:     "",
				Direction: DirectionInbound,
				Type:      "throughput",
				Value:     0,
			},
			expected: string(rune(8080)) + "__inbound_throughput",
		},
		{
			name: "high port number",
			metric: TcpIngressMetricValue{
				Lport:     65535,
				Raddr:     "192.168.1.1",
				Direction: DirectionInbound,
				Type:      "packets",
				Value:     15000,
			},
			expected: string(rune(65535)) + "_192.168.1.1_inbound_packets",
		},
		{
			name: "Outbound direction",
			metric: TcpIngressMetricValue{
				Lport:     9090,
				Raddr:     "10.0.0.1",
				Direction: DirectionOutbound,
				Type:      "bytes",
				Value:     4096.25,
			},
			expected: string(rune(9090)) + "_10.0.0.1_outbound_bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metric.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMetricValue_Fields(t *testing.T) {
	t.Run("TcpEgressMetricValue all fields", func(t *testing.T) {
		metric := TcpEgressMetricValue{
			Raddr:     "192.168.1.1",
			Rport:     8080,
			Direction: "outbound",
			Type:      "bandwidth",
			Value:     1024.5,
		}
		
		assert.Equal(t, "192.168.1.1", metric.Raddr)
		assert.Equal(t, 8080, metric.Rport)
		assert.Equal(t, "outbound", metric.Direction)
		assert.Equal(t, "bandwidth", metric.Type)
		assert.Equal(t, 1024.5, metric.Value)
	})
	
	t.Run("TcpIngressMetricValue all fields", func(t *testing.T) {
		metric := TcpIngressMetricValue{
			Lport:     443,
			Raddr:     "10.0.0.1",
			Direction: "inbound",
			Type:      "latency",
			Value:     50.25,
		}
		
		assert.Equal(t, 443, metric.Lport)
		assert.Equal(t, "10.0.0.1", metric.Raddr)
		assert.Equal(t, "inbound", metric.Direction)
		assert.Equal(t, "latency", metric.Type)
		assert.Equal(t, 50.25, metric.Value)
	})
}

func TestMetricValue_EdgeCases(t *testing.T) {
	t.Run("TcpEgressMetricValue-zero value", func(t *testing.T) {
		metric := TcpEgressMetricValue{}
		result := metric.String()
		assert.Equal(t, "_"+string(rune(0))+"__", result)
	})
	
	t.Run("TcpIngressMetricValue-zero value", func(t *testing.T) {
		metric := TcpIngressMetricValue{}
		result := metric.String()
		assert.Equal(t, string(rune(0))+"___", result)
	})
	
	t.Run("TcpEgressMetricValue-special characters", func(t *testing.T) {
		metric := TcpEgressMetricValue{
			Raddr:     "192.168.1.1",
			Rport:     8080,
			Direction: "out-bound",
			Type:      "band_width",
			Value:     1024.5,
		}
		result := metric.String()
		assert.Contains(t, result, "out-bound")
		assert.Contains(t, result, "band_width")
	})
	
	t.Run("TcpIngressMetricValue-special characters", func(t *testing.T) {
		metric := TcpIngressMetricValue{
			Lport:     8080,
			Raddr:     "192.168.1.1",
			Direction: "in-bound",
			Type:      "lat_ency",
			Value:     50.25,
		}
		result := metric.String()
		assert.Contains(t, result, "in-bound")
		assert.Contains(t, result, "lat_ency")
	})
}

func TestMetricValue_CompareDifferentInstances(t *testing.T) {
	t.Run("different Egress metrics generate different Strings", func(t *testing.T) {
		metric1 := TcpEgressMetricValue{
			Raddr:     "192.168.1.1",
			Rport:     8080,
			Direction: "outbound",
			Type:      "bandwidth",
			Value:     1024.5,
		}
		
		metric2 := TcpEgressMetricValue{
			Raddr:     "192.168.1.1",
			Rport:     8081, // different port
			Direction: "outbound",
			Type:      "bandwidth",
			Value:     1024.5,
		}
		
		assert.NotEqual(t, metric1.String(), metric2.String())
	})
	
	t.Run("different Ingress metrics generate different Strings", func(t *testing.T) {
		metric1 := TcpIngressMetricValue{
			Lport:     8080,
			Raddr:     "192.168.1.1",
			Direction: "inbound",
			Type:      "bandwidth",
			Value:     2048.5,
		}
		
		metric2 := TcpIngressMetricValue{
			Lport:     8080,
			Raddr:     "192.168.1.2", // different address
			Direction: "inbound",
			Type:      "bandwidth",
			Value:     2048.5,
		}
		
		assert.NotEqual(t, metric1.String(), metric2.String())
	})
}

