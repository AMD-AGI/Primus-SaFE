/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"

	"k8s.io/client-go/tools/remotecommand"
)

// Conn defines the interface for user-service interactive connections.
type Conn interface {
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
	ExitReason() string
	SetExitReason(reason string)
	WindowNotify(ctx context.Context, ch chan *remotecommand.TerminalSize)
}

// SshType represents the type of SSH connection.
type SshType string

const (
	SSH      SshType = "ssh"
	WebShell SshType = "webShell"
)

// SessionInfo holds information about an SSH or WebShell session.
type SessionInfo struct {
	sshType  SshType
	size     chan *remotecommand.TerminalSize
	userConn Conn
	userInfo *UserInfo
	rows     int
	cols     int
	isPty    bool
}

// UserInfo contains parsed user information for session authentication.
type UserInfo struct {
	_         struct{} `regexp:"^"`
	User      string   `regexp:"[a-zA-Z0-9][a-zA-Z0-9_-]*"`
	_         struct{} `regexp:"\\."`
	Pod       string   `regexp:"[a-zA-Z0-9][a-zA-Z0-9_-]*"`
	_         struct{} `regexp:"\\."`
	Container string   `regexp:"[a-zA-Z0-9][a-zA-Z0-9_.-]*"`
	_         struct{} `regexp:"\\."`
	CMD       string   `regexp:"[a-zA-Z0-9][a-zA-Z0-9_-]*"`
	_         struct{} `regexp:"\\."`
	Namespace string   `regexp:"[a-zA-Z0-9][a-zA-Z0-9_-]*"`
	_         struct{} `regexp:"$"`
}

// WebShellRequest represents a request to start a web shell session.
type WebShellRequest struct {
	NameSpace string `json:"nameSpace"`
	Rows      string `json:"rows"`
	Cols      string `json:"cols"`
	Container string `json:"container"`
	CMD       string `json:"cmd"`
}
