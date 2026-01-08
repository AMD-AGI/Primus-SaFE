// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metadata

type GpuVendor string

const (
	GpuVendorAMD    = GpuVendor("amd")
	GpuVendorNVIDIA = GpuVendor("NVIDIA")
)

var (
	gpuResourceNames = map[GpuVendor]string{
		GpuVendorAMD:    "amd.com/gpu",
		GpuVendorNVIDIA: "nvidia.com/gpu",
	}
	gpuNodeFilter = map[GpuVendor]string{
		GpuVendorNVIDIA: "nvidia.com/gpu.product-name",
		GpuVendorAMD:    "amd.com/gpu.product-name",
	}
	gpuDeviceTagNames = map[GpuVendor]string{
		GpuVendorNVIDIA: "nvidia.com/gpu.device-name",
		GpuVendorAMD:    "amd.com/gpu.product-name",
	}
)

const DefaultGpuVendor = GpuVendorAMD

func GetResourceName(vendor GpuVendor) string {
	return gpuResourceNames[vendor]
}

func GetNodeFilter(vendor GpuVendor) string {
	return gpuNodeFilter[vendor]
}

func GetDeviceTagNames(vendor GpuVendor) string {
	return gpuDeviceTagNames[vendor]
}
