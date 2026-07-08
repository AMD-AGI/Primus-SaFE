// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.

package model

import (
	"testing"
)

func TestGetDirectionName(t *testing.T) {
	tests := []struct {
		name      string
		direction int
		expected  string
	}{
		{"ingress", FlowTypeIngress, FlowTypeNameIngress},
		{"egress", FlowTypeEgress, FlowTypeNameEgress},
		{"unknown positive", 99, FlowTypeNameEgress},
		{"negative", -1, FlowTypeNameEgress},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDirectionName(tt.direction)
			if result != tt.expected {
				t.Errorf("GetDirectionName(%d) = %q, want %q", tt.direction, result, tt.expected)
			}
		})
	}
}

func TestTcpFlowCacheKeyString(t *testing.T) {
	key := TcpFlowCacheKey{
		SAddr:  "10.0.0.1",
		Daddr:  "10.0.0.2",
		Sport:  8080,
		Dport:  443,
		Family: 2,
	}

	result := key.String()
	expected := "10.0.0.1-10.0.0.2-8080-443-2"
	if result != expected {
		t.Errorf("String() = %q, want %q", result, expected)
	}
}

func TestTcpFlowCacheKeyStringEmpty(t *testing.T) {
	key := TcpFlowCacheKey{}
	result := key.String()
	expected := "--0-0-0"
	if result != expected {
		t.Errorf("String() = %q, want %q", result, expected)
	}
}

func TestFlowTypeConstants(t *testing.T) {
	if FlowTypeIngress != 0 {
		t.Errorf("FlowTypeIngress: got %d, want 0", FlowTypeIngress)
	}
	if FlowTypeEgress != 1 {
		t.Errorf("FlowTypeEgress: got %d, want 1", FlowTypeEgress)
	}
	if FlowTypeNameIngress != "ingress" {
		t.Errorf("FlowTypeNameIngress: got %q, want ingress", FlowTypeNameIngress)
	}
	if FlowTypeNameEgress != "egress" {
		t.Errorf("FlowTypeNameEgress: got %q, want egress", FlowTypeNameEgress)
	}
	if DirectionInbound != "inbound" {
		t.Errorf("DirectionInbound: got %q, want inbound", DirectionInbound)
	}
	if DirectionOutbound != "outbound" {
		t.Errorf("DirectionOutbound: got %q, want outbound", DirectionOutbound)
	}
}

func TestTcpFlowDataValue(t *testing.T) {
	v := TcpFlowDataValue{
		RttTotal:  1000,
		PktCount:  50,
		FlowData:  2048,
		ConnCount: 3,
	}

	if v.RttTotal != 1000 {
		t.Errorf("RttTotal: got %d, want 1000", v.RttTotal)
	}
	if v.PktCount != 50 {
		t.Errorf("PktCount: got %d, want 50", v.PktCount)
	}
	if v.FlowData != 2048 {
		t.Errorf("FlowData: got %d, want 2048", v.FlowData)
	}
	if v.ConnCount != 3 {
		t.Errorf("ConnCount: got %d, want 3", v.ConnCount)
	}
}

