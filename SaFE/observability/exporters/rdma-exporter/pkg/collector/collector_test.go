// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.

package collector

import (
	"testing"
)

func TestParseRDMAStatistics(t *testing.T) {
	input := `link mlx5_0/1 rx_write_requests 100 rx_read_requests 200 rx_atomic_requests 50
link mlx5_0/2 rx_write_requests 300 rx_read_requests 400 rx_atomic_requests 150
link mlx5_1/1 rx_write_requests 500 rx_read_requests 600 rx_atomic_requests 250`

	results, err := parseRDMAStatistics(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Check first entry
	if results[0].Device != "mlx5_0" {
		t.Errorf("entry[0].Device: got %q, want mlx5_0", results[0].Device)
	}
	if results[0].Port != "1" {
		t.Errorf("entry[0].Port: got %q, want 1", results[0].Port)
	}
	if results[0].Stats["rx_write_requests"] != 100 {
		t.Errorf("entry[0].Stats[rx_write_requests]: got %d, want 100", results[0].Stats["rx_write_requests"])
	}
	if results[0].Stats["rx_read_requests"] != 200 {
		t.Errorf("entry[0].Stats[rx_read_requests]: got %d, want 200", results[0].Stats["rx_read_requests"])
	}

	// Check second entry
	if results[1].Device != "mlx5_0" {
		t.Errorf("entry[1].Device: got %q, want mlx5_0", results[1].Device)
	}
	if results[1].Port != "2" {
		t.Errorf("entry[1].Port: got %q, want 2", results[1].Port)
	}

	// Check third entry
	if results[2].Device != "mlx5_1" {
		t.Errorf("entry[2].Device: got %q, want mlx5_1", results[2].Device)
	}
}

func TestParseRDMAStatisticsMultiline(t *testing.T) {
	input := `link mlx5_0/1 rx_write_requests 100 rx_read_requests 200
     tx_write_requests 300 tx_read_requests 400`

	results, err := parseRDMAStatistics(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Stats["rx_write_requests"] != 100 {
		t.Errorf("rx_write_requests: got %d, want 100", results[0].Stats["rx_write_requests"])
	}
	if results[0].Stats["tx_write_requests"] != 300 {
		t.Errorf("tx_write_requests: got %d, want 300", results[0].Stats["tx_write_requests"])
	}
}

func TestParseRDMAStatisticsEmpty(t *testing.T) {
	results, err := parseRDMAStatistics("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestParseRDMAStatisticsNoPort(t *testing.T) {
	input := `link mlx5_bond_0 rx_write_requests 100 rx_read_requests 200`

	results, err := parseRDMAStatistics(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Device != "mlx5_bond_0" {
		t.Errorf("Device: got %q, want mlx5_bond_0", results[0].Device)
	}
	if results[0].Port != "unknown" {
		t.Errorf("Port: got %q, want unknown", results[0].Port)
	}
}

func TestSanitizeMetricName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"rx_write_requests", "rx_write_requests"},
		{"Rx-Write-Requests", "rx_write_requests"},
		{"some.dotted.name", "some_dotted_name"},
		{"UPPERCASE", "uppercase"},
		{"mixed-Case.Name", "mixed_case_name"},
		{"spaces in name", "spaces_in_name"},
		{"already_clean", "already_clean"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeMetricName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeMetricName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

