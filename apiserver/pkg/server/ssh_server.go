/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package apiserver

import (
	"context"
	"net"
	"sync/atomic"

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
				return nil
			default:
				klog.ErrorS(err, "failed to accept connection")
				continue
			}
		}
		if s.Handler != nil {
			go func() {
				s.Handler.HandleConnection(conn)
				conn.Close()
			}()
		}
	}
}

func (s *SshServer) Shutdown() error {
	s.inShutdown.Store(true)
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
