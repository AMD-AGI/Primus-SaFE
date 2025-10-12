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

func NewSshServer(addr string, handler SshHandler, maxConns int64) *SshServer {
	return &SshServer{
		Addr:           addr,
		Handler:        handler,
		maxConnections: maxConns,
	}
}

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

func (s *SshServer) Shutdown() error {
	s.inShutdown.Store(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
