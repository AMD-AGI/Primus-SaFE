// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package discovery

import (
	"bufio"
	"debug/elf"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultProcPath = "/proc"

// RDMAProcess represents a discovered process using RDMA.
type RDMAProcess struct {
	PID         int
	LibPath     string
	PostSendSym string
}

// Discoverer finds processes that have loaded the bnxt_re RDMA provider library
// and resolves the uprobe target symbol within it.
type Discoverer struct {
	procPath   string
	targetLib  string
	targetFunc string
}

func NewDiscoverer(procPath, targetLib, targetFunc string) *Discoverer {
	if procPath == "" {
		procPath = defaultProcPath
	}
	if targetLib == "" {
		targetLib = "libbnxt_re"
	}
	if targetFunc == "" {
		targetFunc = "bnxt_re_post_send"
	}
	return &Discoverer{
		procPath:   procPath,
		targetLib:  targetLib,
		targetFunc: targetFunc,
	}
}

// FindRDMAProcesses scans /proc/*/maps for processes that have loaded the target
// RDMA provider library. Returns a list of discovered processes.
func (d *Discoverer) FindRDMAProcesses(pidsToCheck []int) []RDMAProcess {
	var result []RDMAProcess
	for _, pid := range pidsToCheck {
		libPath := d.findLibInMaps(pid)
		if libPath == "" {
			continue
		}
		if !d.verifySymbol(libPath) {
			slog.Warn("target symbol not found in library",
				"pid", pid, "lib", libPath, "symbol", d.targetFunc)
			continue
		}
		result = append(result, RDMAProcess{
			PID:         pid,
			LibPath:     libPath,
			PostSendSym: d.targetFunc,
		})
	}
	return result
}

// ScanAllRDMAProcesses scans all /proc entries for RDMA processes.
func (d *Discoverer) ScanAllRDMAProcesses() []RDMAProcess {
	entries, err := os.ReadDir(d.procPath)
	if err != nil {
		slog.Error("scan proc", "error", err)
		return nil
	}

	var pids []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return d.FindRDMAProcesses(pids)
}

func (d *Discoverer) findLibInMaps(pid int) string {
	mapsPath := filepath.Join(d.procPath, strconv.Itoa(pid), "maps")
	f, err := os.Open(mapsPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	seen := make(map[string]bool)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		path := fields[len(fields)-1]
		if seen[path] {
			continue
		}
		seen[path] = true
		if strings.Contains(filepath.Base(path), d.targetLib) {
			return path
		}
	}
	return ""
}

func (d *Discoverer) verifySymbol(libPath string) bool {
	_, err := ResolveSymbol(libPath, d.targetFunc)
	return err == nil
}

// ResolveSymbol finds a function symbol in an ELF file, trying .symtab first then .dynsym.
func ResolveSymbol(path, name string) (uint64, error) {
	f, err := elf.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open elf: %w", err)
	}
	defer f.Close()

	if addr, err := findSymbolIn(f.Symbols, name); err == nil {
		return addr, nil
	}
	if addr, err := findSymbolIn(f.DynamicSymbols, name); err == nil {
		return addr, nil
	}
	return 0, fmt.Errorf("symbol %q not found in .symtab or .dynsym", name)
}

func findSymbolIn(getter func() ([]elf.Symbol, error), name string) (uint64, error) {
	symbols, err := getter()
	if err != nil {
		return 0, err
	}
	for _, sym := range symbols {
		if sym.Name == name && elf.ST_TYPE(sym.Info) == elf.STT_FUNC {
			return sym.Value, nil
		}
	}
	return 0, fmt.Errorf("not found")
}

// MultiDiscoverer supports multiple provider library and function name candidates.
type MultiDiscoverer struct {
	procPath    string
	targetLibs  []string
	targetFuncs []string
}

func NewMultiDiscoverer(procPath string, targetLibs, targetFuncs []string) *MultiDiscoverer {
	if procPath == "" {
		procPath = defaultProcPath
	}
	if len(targetLibs) == 0 {
		targetLibs = []string{"libbnxt_re"}
	}
	if len(targetFuncs) == 0 {
		targetFuncs = []string{"bnxt_re_post_send"}
	}
	return &MultiDiscoverer{
		procPath:    procPath,
		targetLibs:  targetLibs,
		targetFuncs: targetFuncs,
	}
}

// FindRDMAProcesses tries all provider lib/func candidates per PID and returns the first match.
func (d *MultiDiscoverer) FindRDMAProcesses(pidsToCheck []int) []RDMAProcess {
	var result []RDMAProcess
	for _, pid := range pidsToCheck {
		proc := d.tryAttachCandidates(pid)
		if proc != nil {
			result = append(result, *proc)
		}
	}
	return result
}

func (d *MultiDiscoverer) tryAttachCandidates(pid int) *RDMAProcess {
	mapsPath := filepath.Join(d.procPath, strconv.Itoa(pid), "maps")
	f, err := os.Open(mapsPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	seen := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 6 {
			continue
		}
		path := fields[len(fields)-1]
		if seen[path] {
			continue
		}
		seen[path] = true
		base := filepath.Base(path)
		for _, lib := range d.targetLibs {
			if !strings.Contains(base, lib) {
				continue
			}
			for _, fn := range d.targetFuncs {
				if _, err := ResolveSymbol(path, fn); err == nil {
					return &RDMAProcess{PID: pid, LibPath: path, PostSendSym: fn}
				}
				slog.Debug("candidate miss", "pid", pid, "lib", path, "func", fn)
			}
		}
	}
	return nil
}
