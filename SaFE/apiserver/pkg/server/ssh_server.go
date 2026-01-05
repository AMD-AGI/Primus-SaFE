/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package apiserver

import (
	"context"
	"net"
	"sync/atomic"
	"time"

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

// NewSshServer creates a new SSH server instance with the specified address and handler.
// It initializes the server with a default maximum connection limit of MaxSSHConnections.
// Returns a pointer to the newly created SshServer.
func NewSshServer(addr string, handler SshHandler) *SshServer {
	return &SshServer{
		Addr:    addr,
		Handler: handler,
	}
}

// Start starts the SSH server and begins listening for incoming connections.
// It creates a TCP listener on the configured address and accepts incoming connections,
// dispatching each connection to the handler in a separate goroutine.
// The server respects the context for cancellation and handles shutdown gracefully.
// Returns an error if the server fails to start or encounters an unrecoverable error.
func (s *SshServer) Start(ctx context.Context) error {
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
				return ctx.Err()
			default:
				klog.ErrorS(err, "failed to accept connection")
				time.Sleep(100 * time.Millisecond)
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

// Shutdown gracefully shuts down the SSH server by closing the listener.
// It sets the shutdown flag and closes the underlying network listener,
// preventing new connections from being accepted.
// Returns an error if closing the listener fails.
func (s *SshServer) Shutdown() error {
	s.inShutdown.Store(true)
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
