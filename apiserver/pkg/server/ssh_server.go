/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package apiserver

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"
)

const (
	MaxSSHConnections = 10000
)

type SshHandler interface {
	HandleConnection(net.Conn)
}

type SshServer struct {
	// Addr optionally specifies the TCP address for the server to listen on,
	// in the form "host:port"
	Addr    string
	Handler SshHandler //  handler to invoke

	listener   net.Listener
	inShutdown atomic.Bool // true when server is in shutdown
	mu         sync.Mutex

	// current active number of connections
	connections int64
	// maxConnections is the limit for the maximum number of connections, where 0 means no limit.
	maxConnections int64
}

// NewSshServer: creates a new SSH server instance with the specified address and handler.
// It initializes the server with a default maximum connection limit of MaxSSHConnections.
// Returns a pointer to the newly created SshServer.
func NewSshServer(addr string, handler SshHandler) *SshServer {
	return &SshServer{
		Addr:           addr,
		Handler:        handler,
		maxConnections: MaxSSHConnections,
	}
}

// Start: starts the SSH server and begins listening for incoming connections.
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
			if s.inShutdown.Load() {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				klog.ErrorS(err, "failed to accept connection")
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}
		if s.Handler != nil {
			if s.maxConnections > 0 && atomic.LoadInt64(&s.connections) >= s.maxConnections {
				conn.Close()
				continue
			}
			atomic.AddInt64(&s.connections, 1)
			go func() {
				defer atomic.AddInt64(&s.connections, -1)
				s.Handler.HandleConnection(conn)
				conn.Close()
			}()
		}
	}
}

// Shutdown: gracefully shuts down the SSH server by closing the listener.
// It sets the shutdown flag and closes the underlying network listener,
// preventing new connections from being accepted.
// Returns an error if closing the listener fails.
func (s *SshServer) Shutdown() error {
	s.inShutdown.Store(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
