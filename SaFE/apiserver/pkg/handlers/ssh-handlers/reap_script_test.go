/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	if os.Getenv("SAFE_RFWD_REAP_HELPER") == "1" {
		os.Exit(reapScriptHelper())
	}
	os.Exit(m.Run())
}

// reapScriptHelper stands in for a pod-side mux so the reap shell can be run
// against a real listen socket. Tests copy this binary onto a path named mux.
func reapScriptHelper() int {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: mux listen <addr> <port>")
		return 2
	}
	ln, err := net.Listen("tcp4", net.JoinHostPort(os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println("ready")
	_ = os.Stdout.Sync()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig
	_ = ln.Close()
	return 0
}

func TestReapScriptSignalsOnlyOurListener(t *testing.T) {
	port := freeListenPort(t)
	ours := startReapHelper(t, "mux", "127.0.0.1", port)
	otherPort := freeListenPort(t)
	other := startReapHelper(t, "other", "127.0.0.1", otherPort)

	script, err := reapScript("127.0.0.1", uint32(port))
	testifyassert.NoError(t, err)
	cmd := exec.Command("/bin/sh", "-c", script)
	out, err := cmd.CombinedOutput()
	testifyassert.NoError(t, err, string(out))

	waitHelper(t, ours)
	testifyassert.NoError(t, other.Process.Signal(syscall.Signal(0)))
}

func TestReapScriptRejectsANonLiteralAddress(t *testing.T) {
	_, err := reapScript("localhost", 7890)
	testifyassert.Error(t, err)
	_, err = reapScript("127.0.0.1", 0)
	testifyassert.Error(t, err)
}

func freeListenPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	testifyassert.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	testifyassert.NoError(t, ln.Close())
	return port
}

func startReapHelper(t *testing.T, name, addr string, port int) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	testifyassert.NoError(t, err)
	bin := filepath.Join(t.TempDir(), name)
	src, err := os.Open(exe)
	testifyassert.NoError(t, err)
	defer src.Close()
	dst, err := os.OpenFile(bin, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	testifyassert.NoError(t, err)
	_, err = io.Copy(dst, src)
	testifyassert.NoError(t, dst.Close())
	testifyassert.NoError(t, err)

	cmd := exec.Command(bin, "listen", addr, strconv.Itoa(port))
	cmd.Env = append(os.Environ(), "SAFE_RFWD_REAP_HELPER=1")
	stdout, err := cmd.StdoutPipe()
	testifyassert.NoError(t, err)
	cmd.Stderr = os.Stderr
	testifyassert.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
			return
		}
		ready <- ""
	}()
	select {
	case got := <-ready:
		testifyassert.Equal(t, "ready", got)
	case <-time.After(30 * time.Second):
		t.Fatal("listener did not become ready")
	}
	return cmd
}

func waitHelper(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		testifyassert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("reap script did not stop the listener")
	}
}
