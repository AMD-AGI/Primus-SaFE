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

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// STDIOTransport implements the MCP STDIO transport protocol.
// It reads JSON-RPC messages from stdin and writes responses to stdout.
// This is the preferred transport for IDE integrations like Cursor.
type STDIOTransport struct {
	server *Server
	reader io.Reader
	writer io.Writer

	// For graceful shutdown
	done chan struct{}
	wg   sync.WaitGroup
}

// NewSTDIOTransport creates a new STDIO transport for the given MCP server.
// It uses os.Stdin and os.Stdout by default.
func NewSTDIOTransport(server *Server) *STDIOTransport {
	return &STDIOTransport{
		server: server,
		reader: os.Stdin,
		writer: os.Stdout,
		done:   make(chan struct{}),
	}
}

// NewSTDIOTransportWithIO creates a new STDIO transport with custom IO.
// This is useful for testing.
func NewSTDIOTransportWithIO(server *Server, reader io.Reader, writer io.Writer) *STDIOTransport {
	return &STDIOTransport{
		server: server,
		reader: reader,
		writer: writer,
		done:   make(chan struct{}),
	}
}

// Start starts the STDIO transport, reading messages from stdin and writing responses to stdout.
// It blocks until the context is cancelled or an error occurs.
func (t *STDIOTransport) Start(ctx context.Context) error {
	log.Info("MCP STDIO: Starting transport")

	scanner := bufio.NewScanner(t.reader)
	// Increase buffer size for large messages
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		<-ctx.Done()
		close(t.done)
	}()

	for {
		select {
		case <-t.done:
			log.Info("MCP STDIO: Shutting down")
			return nil
		default:
		}

		// Read next line (JSON-RPC message)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				log.Errorf("MCP STDIO: Read error: %v", err)
				return err
			}
			// EOF reached
			log.Info("MCP STDIO: EOF reached")
			return nil
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		log.Debugf("MCP STDIO: Received: %s", string(line))

		// Handle the message
		response, err := t.server.HandleMessage(ctx, line)
		if err != nil {
			log.Errorf("MCP STDIO: Handle error: %v", err)
			// Send error response
			errResp := NewErrorResponse(nil, ErrorCodeInternalError, err.Error(), nil)
			response, _ = json.Marshal(errResp)
		}

		// Write response (if any)
		if response != nil {
			log.Debugf("MCP STDIO: Sending: %s", string(response))
			if err := t.writeResponse(response); err != nil {
				log.Errorf("MCP STDIO: Write error: %v", err)
				return err
			}
		}
	}
}

// writeResponse writes a JSON-RPC response to stdout.
// Each response is written as a single line followed by a newline.
func (t *STDIOTransport) writeResponse(data []byte) error {
	// Write the response followed by a newline
	if _, err := t.writer.Write(data); err != nil {
		return err
	}
	if _, err := t.writer.Write([]byte("\n")); err != nil {
		return err
	}

	// Flush if the writer supports it
	if f, ok := t.writer.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	if f, ok := t.writer.(*os.File); ok {
		return f.Sync()
	}

	return nil
}

// Stop stops the STDIO transport gracefully.
func (t *STDIOTransport) Stop() {
	select {
	case <-t.done:
		// Already stopped
	default:
		close(t.done)
	}
	t.wg.Wait()
}

// RunStdio is a convenience function to start an MCP server with STDIO transport.
// This is the main entry point for running the MCP server in STDIO mode.
func RunStdio(ctx context.Context, server *Server) error {
	transport := NewSTDIOTransport(server)
	return transport.Start(ctx)
}

// STDIOServer is a wrapper that provides a simple API for running an MCP server
// with STDIO transport. This is designed for easy integration with the main binary.
type STDIOServer struct {
	mcpServer *Server
	transport *STDIOTransport
}

// NewSTDIOServer creates a new STDIO server with the given MCP server.
func NewSTDIOServer(mcpServer *Server) *STDIOServer {
	return &STDIOServer{
		mcpServer: mcpServer,
		transport: NewSTDIOTransport(mcpServer),
	}
}

// Run starts the STDIO server and blocks until the context is cancelled.
func (s *STDIOServer) Run(ctx context.Context) error {
	return s.transport.Start(ctx)
}

// Stop stops the STDIO server gracefully.
func (s *STDIOServer) Stop() {
	s.transport.Stop()
}

// Server returns the underlying MCP server.
func (s *STDIOServer) Server() *Server {
	return s.mcpServer
}
