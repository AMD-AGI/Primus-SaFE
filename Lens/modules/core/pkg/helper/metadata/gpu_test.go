// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metadata

import (
	"testing"
)

func TestGetResourceName(t *testing.T) {
	tests := []struct {
		name     string
		vendor   GpuVendor
		expected string
	}{
		{
			name:     "AMD vendor should return amd.com/gpu",
			vendor:   GpuVendorAMD,
			expected: "amd.com/gpu",
		},
		{
			name:     "NVIDIA vendor should return nvidia.com/gpu",
			vendor:   GpuVendorNVIDIA,
			expected: "nvidia.com/gpu",
		},
		{
			name:     "Unknown vendor should return empty string",
			vendor:   GpuVendor("unknown"),
			expected: "",
		},
		{
			name:     "Empty vendor should return empty string",
			vendor:   GpuVendor(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetResourceName(tt.vendor)
			if result != tt.expected {
				t.Errorf("GetResourceName(%q) = %q, want %q", tt.vendor, result, tt.expected)
			}
		})
	}
}

func TestGetNodeFilter(t *testing.T) {
	tests := []struct {
		name     string
		vendor   GpuVendor
		expected string
	}{
		{
			name:     "AMD vendor should return amd.com/gpu.product-name",
			vendor:   GpuVendorAMD,
			expected: "amd.com/gpu.product-name",
		},
		{
			name:     "NVIDIA vendor should return nvidia.com/gpu.product-name",
			vendor:   GpuVendorNVIDIA,
			expected: "nvidia.com/gpu.product-name",
		},
		{
			name:     "Unknown vendor should return empty string",
			vendor:   GpuVendor("unknown"),
			expected: "",
		},
		{
			name:     "Empty vendor should return empty string",
			vendor:   GpuVendor(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetNodeFilter(tt.vendor)
			if result != tt.expected {
				t.Errorf("GetNodeFilter(%q) = %q, want %q", tt.vendor, result, tt.expected)
			}
		})
	}
}

func TestGetDeviceTagNames(t *testing.T) {
	tests := []struct {
		name     string
		vendor   GpuVendor
		expected string
	}{
		{
			name:     "AMD vendor should return amd.com/gpu.product-name",
			vendor:   GpuVendorAMD,
			expected: "amd.com/gpu.product-name",
		},
		{
			name:     "NVIDIA vendor should return nvidia.com/gpu.device-name",
			vendor:   GpuVendorNVIDIA,
			expected: "nvidia.com/gpu.device-name",
		},
		{
			name:     "Unknown vendor should return empty string",
			vendor:   GpuVendor("unknown"),
			expected: "",
		},
		{
			name:     "Empty vendor should return empty string",
			vendor:   GpuVendor(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDeviceTagNames(tt.vendor)
			if result != tt.expected {
				t.Errorf("GetDeviceTagNames(%q) = %q, want %q", tt.vendor, result, tt.expected)
			}
		})
	}
}

func TestGpuVendorConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant GpuVendor
		expected string
	}{
		{
			name:     "GpuVendorAMD constant value",
			constant: GpuVendorAMD,
			expected: "amd",
		},
		{
			name:     "GpuVendorNVIDIA constant value",
			constant: GpuVendorNVIDIA,
			expected: "NVIDIA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("Constant value = %q, want %q", tt.constant, tt.expected)
			}
		})
	}
}

func TestDefaultGpuVendor(t *testing.T) {
	if DefaultGpuVendor != GpuVendorAMD {
		t.Errorf("DefaultGpuVendor = %q, want %q", DefaultGpuVendor, GpuVendorAMD)
	}
}

func TestGpuResourceNamesMapIntegrity(t *testing.T) {
	expectedVendors := []GpuVendor{
		GpuVendorAMD,
		GpuVendorNVIDIA,
	}

	for _, vendor := range expectedVendors {
		t.Run("Vendor "+string(vendor)+" should have resource name mapping", func(t *testing.T) {
			resourceName := gpuResourceNames[vendor]
			if resourceName == "" {
				t.Errorf("Vendor %q does not have a resource name mapping", vendor)
			}
		})
	}
}

func TestGpuNodeFilterMapIntegrity(t *testing.T) {
	expectedVendors := []GpuVendor{
		GpuVendorAMD,
		GpuVendorNVIDIA,
	}

	for _, vendor := range expectedVendors {
		t.Run("Vendor "+string(vendor)+" should have node filter mapping", func(t *testing.T) {
			nodeFilter := gpuNodeFilter[vendor]
			if nodeFilter == "" {
				t.Errorf("Vendor %q does not have a node filter mapping", vendor)
			}
		})
	}
}

func TestGpuDeviceTagNamesMapIntegrity(t *testing.T) {
	expectedVendors := []GpuVendor{
		GpuVendorAMD,
		GpuVendorNVIDIA,
	}

	for _, vendor := range expectedVendors {
		t.Run("Vendor "+string(vendor)+" should have device tag name mapping", func(t *testing.T) {
			deviceTagName := gpuDeviceTagNames[vendor]
			if deviceTagName == "" {
				t.Errorf("Vendor %q does not have a device tag name mapping", vendor)
			}
		})
	}
}

func TestGpuMapsConsistency(t *testing.T) {
	if len(gpuResourceNames) != len(gpuNodeFilter) {
		t.Errorf("gpuResourceNames has %d entries, gpuNodeFilter has %d entries, they should be equal",
			len(gpuResourceNames), len(gpuNodeFilter))
	}

	if len(gpuResourceNames) != len(gpuDeviceTagNames) {
		t.Errorf("gpuResourceNames has %d entries, gpuDeviceTagNames has %d entries, they should be equal",
			len(gpuResourceNames), len(gpuDeviceTagNames))
	}

	for vendor := range gpuResourceNames {
		if _, exists := gpuNodeFilter[vendor]; !exists {
			t.Errorf("Vendor %q exists in gpuResourceNames but not in gpuNodeFilter", vendor)
		}
		if _, exists := gpuDeviceTagNames[vendor]; !exists {
			t.Errorf("Vendor %q exists in gpuResourceNames but not in gpuDeviceTagNames", vendor)
		}
	}
}

func TestGpuVendorStringConversion(t *testing.T) {
	tests := []struct {
		name     string
		vendor   GpuVendor
		expected string
	}{
		{
			name:     "AMD vendor string conversion",
			vendor:   GpuVendorAMD,
			expected: "amd",
		},
		{
			name:     "NVIDIA vendor string conversion",
			vendor:   GpuVendorNVIDIA,
			expected: "NVIDIA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(tt.vendor)
			if result != tt.expected {
				t.Errorf("string(%v) = %q, want %q", tt.vendor, result, tt.expected)
			}
		})
	}
}

