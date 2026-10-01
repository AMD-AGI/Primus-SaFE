/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"gotest.tools/assert"
	k8sexec "k8s.io/client-go/util/exec"
)

func TestIdleTrackerCancelsAfterTimeout(t *testing.T) {
	idle := newIdleTracker(40 * time.Millisecond)
	ctx, cancel := idle.watch(context.Background())
	defer cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("idle watcher did not cancel")
	}
}

func TestIdleTrackerTouchExtendsDeadline(t *testing.T) {
	idle := newIdleTracker(80 * time.Millisecond)
	ctx, cancel := idle.watch(context.Background())
	defer cancel()
	deadline := time.Now().Add(50 * time.Millisecond)
	for time.Now().Before(deadline) {
		idle.Touch()
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case <-ctx.Done():
		t.Fatal("idle watcher cancelled while activity continued")
	default:
	}
}

func TestStreamExitCode(t *testing.T) {
	assert.Equal(t, uint32(0), streamExitCode(nil))
	assert.Equal(t, uint32(1), streamExitCode(errors.New("transport")))
	assert.Equal(t, uint32(42), streamExitCode(k8sexec.CodeExitError{Err: errors.New("x"), Code: 42}))
}

func TestSSHConnReadEOFClosesImmediately(t *testing.T) {
	fs := &fakeSession{fakeChannel: &fakeChannel{readErr: io.EOF}, rawCmd: "cat > /tmp/x"}
	conn := newSSHConn(fs)
	start := time.Now()
	_, err := conn.Read(make([]byte, 1))
	assert.Equal(t, io.EOF, err)
	assert.Assert(t, time.Since(start) < time.Second, "EOF must not sleep")
	assert.Equal(t, "client closed stdin", conn.ExitReason())
}
