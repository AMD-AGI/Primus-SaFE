// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/config"
)

// StorageMetrics contains the metrics for a storage mount
type StorageMetrics struct {
	Name           string
	MountPath      string
	StorageType    string
	FilesystemName string

	// Capacity metrics in bytes
	TotalBytes     uint64
	UsedBytes      uint64
	AvailableBytes uint64

	// Inode metrics
	TotalInodes uint64
	UsedInodes  uint64
	FreeInodes  uint64

	// Usage percentage (0-100)
	UsagePercent float64

	// Collection status
	Error error
}

// Collector collects storage metrics from mounted filesystems
type Collector struct {
	mounts []config.MountConfig
}

// NewCollector creates a new storage collector
func NewCollector(mounts []config.MountConfig) *Collector {
	return &Collector{
		mounts: mounts,
	}
}

// Collect collects metrics from all configured mounts
func (c *Collector) Collect(ctx context.Context) []StorageMetrics {
	results := make([]StorageMetrics, 0, len(c.mounts))

	for _, mount := range c.mounts {
		metrics := c.collectMount(ctx, mount)
		results = append(results, metrics)
	}

	return results
}

// collectMount collects metrics from a single mount point
func (c *Collector) collectMount(ctx context.Context, mount config.MountConfig) StorageMetrics {
	metrics := StorageMetrics{
		Name:           mount.Name,
		MountPath:      mount.MountPath,
		StorageType:    mount.StorageType,
		FilesystemName: mount.FilesystemName,
	}

	// Try syscall first (more accurate and faster)
	if err := c.collectUsingSyscall(&metrics); err != nil {
		log.Warnf("Syscall failed for %s, trying df command: %v", mount.MountPath, err)
		// Fallback to df command
		if err := c.collectUsingDf(ctx, &metrics); err != nil {
			metrics.Error = err
			log.Errorf("Failed to collect metrics for %s: %v", mount.MountPath, err)
		}
	}

	return metrics
}

// collectUsingSyscall uses syscall.Statfs to get filesystem stats
func (c *Collector) collectUsingSyscall(metrics *StorageMetrics) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(metrics.MountPath, &stat); err != nil {
		return err
	}

	// Calculate bytes
	metrics.TotalBytes = stat.Blocks * uint64(stat.Bsize)
	metrics.AvailableBytes = stat.Bavail * uint64(stat.Bsize)
	metrics.UsedBytes = metrics.TotalBytes - (stat.Bfree * uint64(stat.Bsize))

	// Inode info
	metrics.TotalInodes = stat.Files
	metrics.FreeInodes = stat.Ffree
	metrics.UsedInodes = metrics.TotalInodes - metrics.FreeInodes

	// Usage percentage
	if metrics.TotalBytes > 0 {
		metrics.UsagePercent = float64(metrics.UsedBytes) / float64(metrics.TotalBytes) * 100
	}

	return nil
}

// collectUsingDf uses the df command as a fallback
func (c *Collector) collectUsingDf(ctx context.Context, metrics *StorageMetrics) error {
	// Run df command with 1K blocks for accuracy
	cmd := exec.CommandContext(ctx, "df", "-k", metrics.MountPath)
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	// Parse output (skip header line)
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return nil
	}

	// Find the data line (may be split across lines for long filesystem names)
	var dataLine string
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			dataLine = line
			break
		}
	}

	fields := strings.Fields(dataLine)
	if len(fields) < 4 {
		return nil
	}

	// Parse values (df -k gives values in 1K blocks)
	// Fields: Filesystem, 1K-blocks, Used, Available, Use%, Mounted on
	totalKB, _ := strconv.ParseUint(fields[1], 10, 64)
	usedKB, _ := strconv.ParseUint(fields[2], 10, 64)
	availKB, _ := strconv.ParseUint(fields[3], 10, 64)

	metrics.TotalBytes = totalKB * 1024
	metrics.UsedBytes = usedKB * 1024
	metrics.AvailableBytes = availKB * 1024

	// Parse usage percentage
	if len(fields) >= 5 {
		useStr := strings.TrimSuffix(fields[4], "%")
		usePct, _ := strconv.ParseFloat(useStr, 64)
		metrics.UsagePercent = usePct
	}

	// Get inode info using df -i
	cmdInode := exec.CommandContext(ctx, "df", "-i", metrics.MountPath)
	outputInode, err := cmdInode.Output()
	if err == nil {
		linesInode := strings.Split(string(outputInode), "\n")
		if len(linesInode) >= 2 {
			var dataLineInode string
			for i := 1; i < len(linesInode); i++ {
				line := strings.TrimSpace(linesInode[i])
				if line != "" {
					dataLineInode = line
					break
				}
			}
			fieldsInode := strings.Fields(dataLineInode)
			if len(fieldsInode) >= 4 {
				metrics.TotalInodes, _ = strconv.ParseUint(fieldsInode[1], 10, 64)
				metrics.UsedInodes, _ = strconv.ParseUint(fieldsInode[2], 10, 64)
				metrics.FreeInodes, _ = strconv.ParseUint(fieldsInode[3], 10, 64)
			}
		}
	}

	return nil
}

// GetMounts returns the configured mounts
func (c *Collector) GetMounts() []config.MountConfig {
	return c.mounts
}
