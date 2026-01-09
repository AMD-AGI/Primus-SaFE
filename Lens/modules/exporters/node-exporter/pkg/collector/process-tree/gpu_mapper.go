// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package processtree

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// GPUInfoProvider provides GPU information (used to avoid import cycle)
type GPUInfoProvider interface {
	GetDriCardInfoMapping() map[string]model.DriDevice
	GetGpuDeviceInfo() []model.GPUInfo
}

// Global GPU info provider (set by collector package during init)
var globalGPUInfoProvider GPUInfoProvider

// SetGPUInfoProvider sets the global GPU info provider
func SetGPUInfoProvider(provider GPUInfoProvider) {
	globalGPUInfoProvider = provider
}

// GPUMapper maps processes to GPU devices
type GPUMapper struct {
	// driRenderPattern matches /dev/dri/renderD128, /dev/dri/card0, etc.
	driRenderPattern *regexp.Regexp
	driCardPattern   *regexp.Regexp
}

// NewGPUMapper creates a new GPU mapper
func NewGPUMapper() *GPUMapper {
	return &GPUMapper{
		driRenderPattern: regexp.MustCompile(`^/dev/dri/renderD(\d+)$`),
		driCardPattern:   regexp.MustCompile(`^/dev/dri/card(\d+)$`),
	}
}

// GPUDeviceBinding represents GPU device binding for a process
type GPUDeviceBinding struct {
	DevicePath string `json:"device_path"`           // e.g., /dev/dri/renderD128
	CardIndex  int    `json:"card_index"`            // e.g., 0, 1, 2
	UUID       string `json:"uuid,omitempty"`        // GPU UUID
	MarketName string `json:"market_name,omitempty"` // e.g., "AMD Instinct MI300X"
	BDF        string `json:"bdf,omitempty"`         // e.g., "0000:03:00.0"
}

// GetProcessGPUDevices detects GPU devices accessed by a process
// by scanning /proc/[pid]/fd for symlinks to /dev/dri/* devices
func (m *GPUMapper) GetProcessGPUDevices(pid int) ([]GPUDeviceBinding, bool) {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		log.Debugf("Failed to read fd directory for PID %d: %v", pid, err)
		return nil, false
	}

	// Use a map to deduplicate GPU devices
	gpuDeviceMap := make(map[string]GPUDeviceBinding)

	for _, entry := range entries {
		fdPath := filepath.Join(fdDir, entry.Name())
		target, err := os.Readlink(fdPath)
		if err != nil {
			continue
		}

		// Check if the fd points to a DRI device
		if strings.HasPrefix(target, "/dev/dri/") {
			binding := m.createGPUBinding(target)
			if binding != nil {
				gpuDeviceMap[binding.DevicePath] = *binding
			}
		}
	}

	if len(gpuDeviceMap) == 0 {
		return nil, false
	}

	// Convert map to slice
	devices := make([]GPUDeviceBinding, 0, len(gpuDeviceMap))
	for _, binding := range gpuDeviceMap {
		devices = append(devices, binding)
	}

	return devices, true
}

// createGPUBinding creates a GPU binding from a device path
func (m *GPUMapper) createGPUBinding(devicePath string) *GPUDeviceBinding {
	binding := &GPUDeviceBinding{
		DevicePath: devicePath,
	}

	// Try to match renderD device (e.g., /dev/dri/renderD128)
	if matches := m.driRenderPattern.FindStringSubmatch(devicePath); len(matches) == 2 {
		renderNum, _ := strconv.Atoi(matches[1])
		// renderD128 -> card0, renderD129 -> card1, etc.
		// Usually renderDN = 128 + cardIndex
		binding.CardIndex = renderNum - 128
		if binding.CardIndex < 0 {
			binding.CardIndex = 0
		}
	} else if matches := m.driCardPattern.FindStringSubmatch(devicePath); len(matches) == 2 {
		// Direct card device (e.g., /dev/dri/card0)
		binding.CardIndex, _ = strconv.Atoi(matches[1])
	} else {
		// Unknown device format
		return nil
	}

	// Try to enrich with GPU info from node-exporter's global mapping
	m.enrichWithGPUInfo(binding)

	return binding
}

// enrichWithGPUInfo enriches the binding with GPU information from node-exporter
func (m *GPUMapper) enrichWithGPUInfo(binding *GPUDeviceBinding) {
	if globalGPUInfoProvider == nil {
		return
	}

	// Get the global GPU info mapping from provider
	driCardInfoMapping := globalGPUInfoProvider.GetDriCardInfoMapping()
	if driCardInfoMapping == nil {
		return
	}

	// Try to find by card path (e.g., /dev/dri/card0)
	cardPath := fmt.Sprintf("/dev/dri/card%d", binding.CardIndex)
	if driDevice, ok := driCardInfoMapping[cardPath]; ok {
		binding.BDF = driDevice.PCIAddr
		// Now try to get more info from GPU device info
		m.enrichFromGPUDeviceInfo(binding, driDevice.PCIAddr)
	}
}

// enrichFromGPUDeviceInfo gets additional GPU info using BDF address
func (m *GPUMapper) enrichFromGPUDeviceInfo(binding *GPUDeviceBinding, bdf string) {
	if globalGPUInfoProvider == nil {
		return
	}

	gpuDeviceInfo := globalGPUInfoProvider.GetGpuDeviceInfo()
	for _, gpu := range gpuDeviceInfo {
		if gpu.Bus.BDF == bdf {
			binding.UUID = gpu.Asic.AsicSerial
			binding.MarketName = gpu.Asic.MarketName
			return
		}
	}
}

// GetProcessGPUDevicesFromFDs detects GPU devices from a list of open file descriptors
// This is useful when fd info is already collected
func (m *GPUMapper) GetProcessGPUDevicesFromFDs(fds map[string]string) ([]GPUDeviceBinding, bool) {
	gpuDeviceMap := make(map[string]GPUDeviceBinding)

	for _, target := range fds {
		if strings.HasPrefix(target, "/dev/dri/") {
			binding := m.createGPUBinding(target)
			if binding != nil {
				gpuDeviceMap[binding.DevicePath] = *binding
			}
		}
	}

	if len(gpuDeviceMap) == 0 {
		return nil, false
	}

	devices := make([]GPUDeviceBinding, 0, len(gpuDeviceMap))
	for _, binding := range gpuDeviceMap {
		devices = append(devices, binding)
	}

	return devices, true
}

// ToModelGPUDeviceBindings converts internal GPUDeviceBinding to model.GPUDeviceBinding
func ToModelGPUDeviceBindings(bindings []GPUDeviceBinding) []model.GPUDeviceBinding {
	if len(bindings) == 0 {
		return nil
	}

	result := make([]model.GPUDeviceBinding, len(bindings))
	for i, b := range bindings {
		result[i] = model.GPUDeviceBinding{
			DevicePath: b.DevicePath,
			CardIndex:  b.CardIndex,
			UUID:       b.UUID,
			MarketName: b.MarketName,
			BDF:        b.BDF,
		}
	}
	return result
}

