/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package apiserver

import (
	"context"
	"net"
	"sync/atomic"

	"k8s.io/klog/v2"
)

// SshHandler defines an interface to handle an accepted connection.
type SshHandler interface {
	HandleConnection(net.Conn)
}

// SshServer represents a simple TCP server for SSH-like connections.
// Addr optionally specifies the TCP address for the server to listen on,
// in the form "host:port". Handler is invoked for each accepted connection.
type SshServer struct {
	// Addr optionally specifies the TCP address for the server to listen on,
	// in the form "host:port".
	Addr string
	// Handler is the handler to invoke for each accepted connection.
	Handler SshHandler

	listener   net.Listener
	inShutdown atomic.Bool // true when server is in shutdown
}

// ListenAndServe starts listening on the configured address and serves connections.
// The method returns when the provided context is done, when Shutdown is called,
// or when a non-recoverable error occurs.
func (s *SshServer) ListenAndServe(ctx context.Context) error {
	cfg := net.ListenConfig{}
	var err error
	s.listener, err = cfg.Listen(ctx, "tcp", s.Addr)
	if err != nil {
		return err
	}
	s.inShutdown.Store(false)

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// If server is shutting down, return nil to indicate graceful stop.
			if s.inShutdown.Load() {
				return nil
			}
			// If context was canceled, return nil.
			select {
			case <-ctx.Done():
				return nil
			default:
				klog.ErrorS(err, "failed to accept connection")
				continue
			}
		}

		// If no handler is provided, close the connection immediately.
		if s.Handler == nil {
			_ = conn.Close()
			continue
		}

		// Capture the connection variable for the goroutine to avoid capturing
		// the loop variable (which would lead to incorrect behavior).
		c := conn
		go func() {
			// Ensure connection is closed when handler finishes.
			defer c.Close()
			s.Handler.HandleConnection(c)
		}()
	}
}

// Shutdown marks the server as shutting down and closes the listener.
// It returns the error from listener.Close() when applicable.
func (s *SshServer) Shutdown() error {
	s.inShutdown.Store(true)
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
