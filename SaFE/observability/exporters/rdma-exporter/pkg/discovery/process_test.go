// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package discovery

import (
	"os"
	"testing"
)

func TestResolveSymbolSymtab(t *testing.T) {
	const libPath = "/usr/local/lib/libbnxt_re-rdmav34.so"
	if _, err := os.Stat(libPath); err != nil {
		t.Skip("bnxt_re library not available:", err)
	}
	off, err := ResolveSymbol(libPath, "bnxt_re_post_send")
	if err != nil {
		t.Fatalf("ResolveSymbol: %v", err)
	}
	const want = uint64(0x8210)
	if off != want {
		t.Errorf("bnxt_re_post_send offset: got %#x, want %#x", off, want)
	}
}

func TestResolveSymbolDynsymFallback(t *testing.T) {
	const libPath = "/usr/lib/x86_64-linux-gnu/libibverbs.so"
	if _, err := os.Stat(libPath); err != nil {
		t.Skip("libibverbs not available:", err)
	}
	// ibv_modify_qp is exported in .dynsym but typically not in .symtab (stripped)
	off, err := ResolveSymbol(libPath, "ibv_modify_qp")
	if err != nil {
		t.Fatalf("ResolveSymbol dynsym fallback: %v", err)
	}
	if off == 0 {
		t.Error("expected non-zero offset for ibv_modify_qp")
	}
}

func TestResolveSymbolNotFound(t *testing.T) {
	const libPath = "/usr/local/lib/libbnxt_re-rdmav34.so"
	if _, err := os.Stat(libPath); err != nil {
		t.Skip("bnxt_re library not available:", err)
	}
	_, err := ResolveSymbol(libPath, "nonexistent_function_xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent symbol")
	}
}

func TestMultiDiscovererCandidates(t *testing.T) {
	disc := NewMultiDiscoverer("", []string{"libbnxt_re", "libmlx5"}, []string{"bnxt_re_post_send", "mlx5_post_send"})
	if len(disc.targetLibs) != 2 {
		t.Fatalf("expected 2 target libs, got %d", len(disc.targetLibs))
	}
	if len(disc.targetFuncs) != 2 {
		t.Fatalf("expected 2 target funcs, got %d", len(disc.targetFuncs))
	}
}

func TestMultiDiscovererDefaults(t *testing.T) {
	disc := NewMultiDiscoverer("", nil, nil)
	if len(disc.targetLibs) != 1 || disc.targetLibs[0] != "libbnxt_re" {
		t.Fatalf("unexpected default libs: %v", disc.targetLibs)
	}
	if len(disc.targetFuncs) != 1 || disc.targetFuncs[0] != "bnxt_re_post_send" {
		t.Fatalf("unexpected default funcs: %v", disc.targetFuncs)
	}
}
