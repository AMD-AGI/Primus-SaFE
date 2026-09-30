/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"
)

// runningMux is a real multiplexer started by the production run script, standing
// in for one forward's run exec.
type runningMux struct {
	cmd    *exec.Cmd
	token  string
	port   uint32
	exited chan error
}

// startRunningMux installs and runs the real multiplexer the way a forward does,
// with a stdin that stays open, which is what a run exec whose peer has gone
// quiet looks like from inside the pod.
func startRunningMux(t *testing.T) *runningMux {
	t.Helper()
	binary := hostMuxBinary(t)
	token := testToken(t)
	dir := filepath.Join(t.TempDir(), ".safe-rfwd-"+token)
	_, stderr, err := runScriptLocally(t, installScript(dir), bytes.NewReader(binary))
	testifyassert.NoError(t, err, stderr)

	port := freeTCPPort(t)
	cmd := exec.Command("/bin/sh", "-c", runScript(dir, "127.0.0.1", port))
	stdin, err := cmd.StdinPipe()
	testifyassert.NoError(t, err)
	cmd.Stdout = io.Discard
	stderrPipe, err := cmd.StderrPipe()
	testifyassert.NoError(t, err)
	testifyassert.NoError(t, cmd.Start())

	m := &runningMux{cmd: cmd, token: token, port: port, exited: make(chan error, 1)}
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == rfwdReadyMarker {
				ready <- true
			}
		}
		close(ready)
		m.exited <- cmd.Wait()
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
	})
	select {
	case ok := <-ready:
		testifyassert.True(t, ok, "the multiplexer exited before binding")
	case <-time.After(30 * time.Second):
		t.Fatal("the multiplexer never reported the port bound")
	}
	return m
}

func (m *runningMux) waitExited(t *testing.T) {
	t.Helper()
	select {
	case <-m.exited:
	case <-time.After(10 * time.Second):
		t.Fatal("the multiplexer is still running")
	}
	waitForPortFree(t, m.port)
}

// portHeld reports whether something is listening on the loopback port.
func portHeld(port uint32) bool {
	ln, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", itoa(port)))
	if err != nil {
		return true
	}
	_ = ln.Close()
	return false
}

func (m *runningMux) stillRunning(t *testing.T) {
	t.Helper()
	select {
	case err := <-m.exited:
		t.Fatalf("the multiplexer exited: %v", err)
	default:
	}
	testifyassert.True(t, portHeld(m.port), "the multiplexer let go of its port")
}

// TestRunScriptLetsASignalReachTheListener is why the run script execs: a shell
// in front that traps TERM would run the trap and keep waiting on its child, and
// the child would keep the port.
func TestRunScriptLetsASignalReachTheListener(t *testing.T) {
	m := startRunningMux(t)
	testifyassert.NoError(t, m.cmd.Process.Signal(syscall.SIGTERM))
	m.waitExited(t)
}

// TestReapScriptStopsOnlyItsOwnMultiplexer covers Close's fallback. Another
// session's forward on the same pod has its own token and must survive, even when
// it now holds the port the reaping forward used to.
func TestReapScriptStopsOnlyItsOwnMultiplexer(t *testing.T) {
	ours := startRunningMux(t)
	other := startRunningMux(t)

	_, stderr, err := runScriptLocally(t, reapScript(testToken(t)), nil)
	testifyassert.NoError(t, err, stderr)
	ours.stillRunning(t)
	other.stillRunning(t)

	_, stderr, err = runScriptLocally(t, reapScript(ours.token), nil)
	testifyassert.NoError(t, err, stderr)
	ours.waitExited(t)
	other.stillRunning(t)
}
