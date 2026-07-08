// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package process

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"syscall"
)

// Info holds cached process metadata.
type Info struct {
	Comm  string
	Pid   uint32
	NsPid uint32
}

// Cache provides a thread-safe PID → Info lookup with /proc-based resolution.
// It caches results so that repeated lookups for the same PID avoid filesystem access.
type Cache struct {
	mu       sync.RWMutex
	items    map[uint32]*Info
	procRoot string // "/host-proc" or "/proc"
}

// NewCache creates a process info cache.
// It tries /host-proc first (container with host mount), then falls back to /proc.
func NewCache() *Cache {
	procRoot := "/host-proc"
	if _, err := os.Stat(procRoot); os.IsNotExist(err) {
		procRoot = "/proc"
	}
	return &Cache{
		items:    make(map[uint32]*Info),
		procRoot: procRoot,
	}
}

// Lookup returns the process info for the given host PID.
// It caches the result on first lookup.
func (c *Cache) Lookup(pid uint32) *Info {
	if pid == 0 {
		return &Info{Comm: "unknown", Pid: 0, NsPid: 0}
	}

	c.mu.RLock()
	info, ok := c.items[pid]
	c.mu.RUnlock()
	if ok {
		return info
	}

	info = c.resolve(pid)

	c.mu.Lock()
	c.items[pid] = info
	c.mu.Unlock()

	return info
}

// resolve reads /proc/<pid>/comm and /proc/<pid>/status to get comm and NsPid.
func (c *Cache) resolve(pid uint32) *Info {
	info := &Info{Pid: pid, NsPid: pid, Comm: "unknown"}

	// Read comm
	commPath := fmt.Sprintf("%s/%d/comm", c.procRoot, pid)
	if data, err := os.ReadFile(commPath); err == nil {
		info.Comm = strings.TrimSpace(string(data))
	} else {
		slog.Debug("failed to read process comm", "pid", pid, "error", err)
	}

	// Read NsPid from /proc/<pid>/status
	statusPath := fmt.Sprintf("%s/%d/status", c.procRoot, pid)
	nspid, err := parseNsPid(statusPath)
	if err != nil {
		slog.Debug("failed to read NsPid", "pid", pid, "error", err)
	} else {
		info.NsPid = nspid
	}

	return info
}

// parseNsPid parses the NSpid line from /proc/<pid>/status.
// The NSpid line looks like: "NSpid:\t1234\t5678"
// where the last value is the PID in the innermost namespace.
func parseNsPid(statusPath string) (uint32, error) {
	f, err := os.Open(statusPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "NSpid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("malformed NSpid line: %s", line)
		}
		// The last field is the PID in the innermost (container) namespace
		last := fields[len(fields)-1]
		var nspid uint32
		_, err := fmt.Sscanf(last, "%d", &nspid)
		return nspid, err
	}
	return 0, fmt.Errorf("NSpid not found in %s", statusPath)
}

// Cleanup removes entries for PIDs that no longer exist.
func (c *Cache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for pid := range c.items {
		// Check if process still exists via kill(pid, 0)
		if err := syscall.Kill(int(pid), 0); err != nil {
			delete(c.items, pid)
		}
	}
}
