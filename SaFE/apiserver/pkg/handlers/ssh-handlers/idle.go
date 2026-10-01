/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// sshIdleTimeout closes a connection after this long with no session or forward data.
// Keepalive probes do not count as activity.
const sshIdleTimeout = 2 * time.Hour

// sshKeepAlivePeriod is both the TCP keepalive interval and the SSH keepalive probe interval.
const sshKeepAlivePeriod = 30 * time.Second

// sshIdleCheckPeriod is how often the idle watcher compares last activity to the timeout.
const sshIdleCheckPeriod = 30 * time.Second

type idleCtxKey struct{}

// idleTracker records the last time real bytes moved on a connection.
type idleTracker struct {
	mu      sync.Mutex
	last    time.Time
	timeout time.Duration
}

func newIdleTracker(timeout time.Duration) *idleTracker {
	if timeout <= 0 {
		timeout = sshIdleTimeout
	}
	return &idleTracker{last: time.Now(), timeout: timeout}
}

// Touch records activity. Keepalive probes must not call it.
func (t *idleTracker) Touch() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.last = time.Now()
	t.mu.Unlock()
}

// watch cancels the returned context once the connection has been idle for the timeout.
func (t *idleTracker) watch(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	if t == nil {
		return ctx, cancel
	}
	go func() {
		check := sshIdleCheckPeriod
		if half := t.timeout / 2; half > 0 && half < check {
			check = half
		}
		if check < time.Millisecond {
			check = time.Millisecond
		}
		ticker := time.NewTicker(check)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.mu.Lock()
				idle := time.Since(t.last)
				timeout := t.timeout
				t.mu.Unlock()
				if idle >= timeout {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, cancel
}

func withIdleTracker(ctx context.Context, idle *idleTracker) context.Context {
	return context.WithValue(ctx, idleCtxKey{}, idle)
}

func idleFromContext(ctx context.Context) *idleTracker {
	idle, _ := ctx.Value(idleCtxKey{}).(*idleTracker)
	return idle
}

// runSSHKeepAlive sends SSH keepalive requests until the context ends or the peer is gone.
func runSSHKeepAlive(ctx context.Context, conn ssh.Conn) {
	ticker := time.NewTicker(sshKeepAlivePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, _, err := conn.SendRequest("keepalive@openssh.com", false, nil); err != nil {
				return
			}
		}
	}
}

type touchingReader struct {
	r    io.Reader
	idle *idleTracker
}

func (r touchingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		r.idle.Touch()
	}
	return n, err
}

type touchingWriter struct {
	w    io.Writer
	idle *idleTracker
}

func (w touchingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 {
		w.idle.Touch()
	}
	return n, err
}
