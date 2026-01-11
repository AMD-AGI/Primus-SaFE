// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package processtree

import (
	"testing"
)

func TestNewGPUMapper(t *testing.T) {
	mapper := NewGPUMapper()
	if mapper == nil {
		t.Fatal("NewGPUMapper returned nil")
	}
	if mapper.driRenderPattern == nil {
		t.Error("driRenderPattern is nil")
	}
	if mapper.driCardPattern == nil {
		t.Error("driCardPattern is nil")
	}
}

func TestCreateGPUBinding_RenderDevice(t *testing.T) {
	mapper := NewGPUMapper()

	tests := []struct {
		devicePath       string
		expectedCardIdx  int
		expectNil        bool
	}{
		{"/dev/dri/renderD128", 0, false},
		{"/dev/dri/renderD129", 1, false},
		{"/dev/dri/renderD130", 2, false},
		{"/dev/dri/renderD135", 7, false},
	}

	for _, tc := range tests {
		binding := mapper.createGPUBinding(tc.devicePath)
		if tc.expectNil {
			if binding != nil {
				t.Errorf("Expected nil for %s, got binding", tc.devicePath)
			}
			continue
		}

		if binding == nil {
			t.Errorf("Expected binding for %s, got nil", tc.devicePath)
			continue
		}

		if binding.DevicePath != tc.devicePath {
			t.Errorf("Expected DevicePath %s, got %s", tc.devicePath, binding.DevicePath)
		}

		if binding.CardIndex != tc.expectedCardIdx {
			t.Errorf("Expected CardIndex %d for %s, got %d", tc.expectedCardIdx, tc.devicePath, binding.CardIndex)
		}
	}
}

func TestCreateGPUBinding_CardDevice(t *testing.T) {
	mapper := NewGPUMapper()

	tests := []struct {
		devicePath       string
		expectedCardIdx  int
		expectNil        bool
	}{
		{"/dev/dri/card0", 0, false},
		{"/dev/dri/card1", 1, false},
		{"/dev/dri/card2", 2, false},
		{"/dev/dri/card7", 7, false},
	}

	for _, tc := range tests {
		binding := mapper.createGPUBinding(tc.devicePath)
		if tc.expectNil {
			if binding != nil {
				t.Errorf("Expected nil for %s, got binding", tc.devicePath)
			}
			continue
		}

		if binding == nil {
			t.Errorf("Expected binding for %s, got nil", tc.devicePath)
			continue
		}

		if binding.DevicePath != tc.devicePath {
			t.Errorf("Expected DevicePath %s, got %s", tc.devicePath, binding.DevicePath)
		}

		if binding.CardIndex != tc.expectedCardIdx {
			t.Errorf("Expected CardIndex %d for %s, got %d", tc.expectedCardIdx, tc.devicePath, binding.CardIndex)
		}
	}
}

func TestCreateGPUBinding_InvalidDevice(t *testing.T) {
	mapper := NewGPUMapper()

	invalidPaths := []string{
		"/dev/null",
		"/dev/sda1",
		"/dev/dri/",
		"/dev/dri/unknown",
		"/tmp/test",
		"",
	}

	for _, path := range invalidPaths {
		binding := mapper.createGPUBinding(path)
		if binding != nil {
			t.Errorf("Expected nil for invalid path %s, got binding", path)
		}
	}
}

func TestGetProcessGPUDevicesFromFDs(t *testing.T) {
	mapper := NewGPUMapper()

	// Test with GPU FDs
	fds := map[string]string{
		"3": "/dev/dri/renderD128",
		"4": "/dev/null",
		"5": "/dev/dri/card0",
		"6": "/proc/1/maps",
	}

	devices, hasGPU := mapper.GetProcessGPUDevicesFromFDs(fds)
	if !hasGPU {
		t.Error("Expected hasGPU to be true")
	}

	if len(devices) != 2 {
		t.Errorf("Expected 2 GPU devices, got %d", len(devices))
	}

	// Test with no GPU FDs
	noGPUFDs := map[string]string{
		"3": "/dev/null",
		"4": "/tmp/file",
	}

	devices, hasGPU = mapper.GetProcessGPUDevicesFromFDs(noGPUFDs)
	if hasGPU {
		t.Error("Expected hasGPU to be false")
	}

	if len(devices) != 0 {
		t.Errorf("Expected 0 GPU devices, got %d", len(devices))
	}
}

func TestToModelGPUDeviceBindings(t *testing.T) {
	// Test with nil/empty input
	result := ToModelGPUDeviceBindings(nil)
	if result != nil {
		t.Error("Expected nil for nil input")
	}

	result = ToModelGPUDeviceBindings([]GPUDeviceBinding{})
	if result != nil {
		t.Error("Expected nil for empty input")
	}

	// Test with valid input
	bindings := []GPUDeviceBinding{
		{
			DevicePath: "/dev/dri/renderD128",
			CardIndex:  0,
			UUID:       "test-uuid",
			MarketName: "AMD Instinct MI300X",
			BDF:        "0000:03:00.0",
		},
	}

	result = ToModelGPUDeviceBindings(bindings)
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(result))
	}

	if result[0].DevicePath != "/dev/dri/renderD128" {
		t.Errorf("Expected DevicePath /dev/dri/renderD128, got %s", result[0].DevicePath)
	}

	if result[0].CardIndex != 0 {
		t.Errorf("Expected CardIndex 0, got %d", result[0].CardIndex)
	}

	if result[0].UUID != "test-uuid" {
		t.Errorf("Expected UUID test-uuid, got %s", result[0].UUID)
	}

	if result[0].MarketName != "AMD Instinct MI300X" {
		t.Errorf("Expected MarketName AMD Instinct MI300X, got %s", result[0].MarketName)
	}

	if result[0].BDF != "0000:03:00.0" {
		t.Errorf("Expected BDF 0000:03:00.0, got %s", result[0].BDF)
	}
}

