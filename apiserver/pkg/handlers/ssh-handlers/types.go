package ssh_handlers

import (
	"context"

	"k8s.io/client-go/tools/remotecommand"
)

// Conn 用户与服务的交互连接.
type Conn interface {
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
	ExitReason() string
	SetExitReason(reason string)
	WindowNotify(ctx context.Context, ch chan *remotecommand.TerminalSize)
}

type SshType string

const (
	SSH      = "ssh"
	WebShell = "webShell"
)

type SessionInfo struct {
	sshType  SshType
	size     chan *remotecommand.TerminalSize
	userConn Conn
	userInfo *UserInfo
	rows     int
	cols     int
}

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

type WebShellRequest struct {
	NameSpace string `json:"nameSpace"`
	Rows      string `json:"rows"`
	Cols      string `json:"cols"`
	Container string `json:"container"`
	CMD       string `json:"cmd"`
}
