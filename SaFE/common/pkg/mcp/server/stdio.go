// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"

	"k8s.io/klog/v2"
)

// STDIOTransport implements the MCP STDIO transport protocol.
// Preferred transport for IDE integrations like Cursor.
type STDIOTransport struct {
	server *Server
	reader io.Reader
	writer io.Writer
	done   chan struct{}
	wg     sync.WaitGroup
}

func NewSTDIOTransport(server *Server) *STDIOTransport {
	return &STDIOTransport{
		server: server,
		reader: os.Stdin,
		writer: os.Stdout,
		done:   make(chan struct{}),
	}
}

func NewSTDIOTransportWithIO(server *Server, reader io.Reader, writer io.Writer) *STDIOTransport {
	return &STDIOTransport{
		server: server,
		reader: reader,
		writer: writer,
		done:   make(chan struct{}),
	}
}

func (t *STDIOTransport) Start(ctx context.Context) error {
	klog.Info("MCP STDIO: Starting transport")

	scanner := bufio.NewScanner(t.reader)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		<-ctx.Done()
		close(t.done)
	}()

	for {
		select {
		case <-t.done:
			klog.Info("MCP STDIO: Shutting down")
			return nil
		default:
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				klog.Errorf("MCP STDIO: Read error: %v", err)
				return err
			}
			klog.Info("MCP STDIO: EOF reached")
			return nil
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		klog.V(4).Infof("MCP STDIO: Received: %s", string(line))

		response, err := t.server.HandleMessage(ctx, line)
		if err != nil {
			klog.Errorf("MCP STDIO: Handle error: %v", err)
			errResp := NewErrorResponse(nil, ErrorCodeInternalError, err.Error(), nil)
			response, _ = json.Marshal(errResp)
		}

		if response != nil {
			klog.V(4).Infof("MCP STDIO: Sending: %s", string(response))
			if err := t.writeResponse(response); err != nil {
				klog.Errorf("MCP STDIO: Write error: %v", err)
				return err
			}
		}
	}
}

func (t *STDIOTransport) writeResponse(data []byte) error {
	if _, err := t.writer.Write(data); err != nil {
		return err
	}
	if _, err := t.writer.Write([]byte("\n")); err != nil {
		return err
	}
	if f, ok := t.writer.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	if f, ok := t.writer.(*os.File); ok {
		return f.Sync()
	}
	return nil
}

func (t *STDIOTransport) Stop() {
	select {
	case <-t.done:
	default:
		close(t.done)
	}
	t.wg.Wait()
}

// RunStdio starts an MCP server with STDIO transport.
func RunStdio(ctx context.Context, server *Server) error {
	transport := NewSTDIOTransport(server)
	return transport.Start(ctx)
}

// STDIOServer wraps Server + STDIOTransport for simple usage.
type STDIOServer struct {
	mcpServer *Server
	transport *STDIOTransport
}

func NewSTDIOServer(mcpServer *Server) *STDIOServer {
	return &STDIOServer{
		mcpServer: mcpServer,
		transport: NewSTDIOTransport(mcpServer),
	}
}

func (s *STDIOServer) Run(ctx context.Context) error {
	return s.transport.Start(ctx)
}

func (s *STDIOServer) Stop() {
	s.transport.Stop()
}

func (s *STDIOServer) Server() *Server {
	return s.mcpServer
}
