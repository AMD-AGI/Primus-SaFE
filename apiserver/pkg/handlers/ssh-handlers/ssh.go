/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/alexflint/go-restructure"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// SshHandler handles SSH connections and related operations.
type SshHandler struct {
	ctx context.Context
	client.Client
	dbClient      dbclient.Interface
	clientManager *commonutils.ObjectManager
	config        *ssh.ServerConfig
	auth          *authority.Authorizer
	timeout       time.Duration
	upgrader      *websocket.Upgrader
}

var (
	once       sync.Once
	sshHandler *SshHandler
)

// NewSshHandler creates and initializes a new SshHandler singleton.
func NewSshHandler(ctx context.Context, mgr ctrlruntime.Manager) (*SshHandler, error) {
	var err error
	once.Do(func() {
		klog.Infof("init ssh handler")

		var dbClient *dbclient.Client
		if commonconfig.IsDBEnable() {
			if dbClient = dbclient.NewClient(); dbClient == nil {
				err = fmt.Errorf("failed to new db client")
				return
			}
		}

		config := &ssh.ServerConfig{}
		privateData := commonconfig.GetSSHRsaPrivate()
		if len(privateData) == 0 {
			err = fmt.Errorf("id_rsa is empty")
			return
		}
		var private ssh.Signer
		private, err = ssh.ParsePrivateKey([]byte(privateData))
		if err != nil {
			return
		}
		config.AddHostKey(private)

		sshHandler = &SshHandler{
			ctx:           ctx,
			Client:        mgr.GetClient(),
			dbClient:      dbClient,
			clientManager: commonutils.NewObjectManagerSingleton(),
			config:        config,
			auth:          authority.NewAuthorizer(mgr.GetClient()),
			timeout:       time.Hour * 48,
			upgrader: &websocket.Upgrader{
				HandshakeTimeout: 3 * time.Second,
				ReadBufferSize:   4096,
				WriteBufferSize:  4096,
				CheckOrigin: func(r *http.Request) bool {
					return true
				},
			},
		}
		sshHandler.config.PublicKeyCallback = sshHandler.publicCallback
	})

	return sshHandler, err
}

// publicCallback validates the public key for SSH authentication.
func (h *SshHandler) publicCallback(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	if !commonconfig.IsDBEnable() {
		return nil, fmt.Errorf("db is not enable")
	}
	parsedUser, ok := ParseUserInfo(conn.User())
	klog.Infof("parse user info: %+v, ok: %v", parsedUser, ok)
	if !ok {
		return nil, fmt.Errorf("invalid user")
	}

	publicKeys, err := h.dbClient.GetPublicKeyByUserId(context.Background(), parsedUser.User)
	if err != nil {
		return nil, err
	}
	isFound := false
	for _, p := range publicKeys {
		pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(p.PublicKey))
		if err != nil {
			continue
		}
		if bytes.Equal(pub.Marshal(), key.Marshal()) {
			isFound = true
			break
		}
	}
	if !isFound {
		return nil, fmt.Errorf("invalid public key")
	}

	return nil, nil
}

// HandleConnection handles an incoming SSH connection.
func (h *SshHandler) HandleConnection(conn net.Conn) {
	sshConn, newChannel, reqs, err := ssh.NewServerConn(conn, h.config)
	if err != nil {
		klog.ErrorS(err, "failed to handshake")
		return
	}
	defer sshConn.Close()

	klog.Infof("ssh connection started, user:%s, from: %s", sshConn.User(), conn.RemoteAddr())

	ctx, cancel := context.WithTimeout(h.ctx, h.timeout)
	defer cancel()

	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Add(1)
	go func() {
		ssh.DiscardRequests(reqs)
		wg.Done()
	}()

	for ch := range newChannel {
		switch ch.ChannelType() {
		case "session":
			go h.startSessionHandler(ctx, sshConn, ch)
		case "direct-tcpip":
			go h.handleDirectIp(ctx, sshConn, ch)
		default:
			ch.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
	}
	klog.Infof("ssh connection closed, user: %s, from %s", sshConn.User(), conn.RemoteAddr())
}

// startSessionHandler starts a session handler for an SSH channel.
func (h *SshHandler) startSessionHandler(ctx context.Context, conn *ssh.ServerConn, newChan ssh.NewChannel) {
	ch, reqs, err := newChan.Accept()
	if err != nil {
		klog.ErrorS(err, "failed to accept channel")
		return
	}
	//fmt.Fprintf(ch, "\r\x1b[93mConnecting ...\x1b[0m\r\n")
	s := &session{
		ctx:     ctx,
		Channel: ch,
		conn:    conn,
		handler: h.handleSession,
		subsystemHandlers: map[string]SubsystemHandler{
			"sftp": h.handleSftp,
		},
	}
	s.handleRequests(reqs)
}

// handleSession processes a session request for a user.
func (h *SshHandler) handleSession(s Session) {
	userInfo, ok := ParseUserInfo(s.User())
	if !ok {
		sendError(s, fmt.Sprintf("Invalid user %v", s.User()))
		return
	}

	_, _, isPty := s.Pty()
	sessionInfo := h.NewSessionInfo(userInfo, newSSHConn(s), 1800, 40, SSH, isPty)
	if err := h.SessionConn(s.Context(), sessionInfo); err != nil {
		sendError(s, err.Error())
	}

	return
}

// ParseUserInfo parses the user string into a UserInfo struct.
func ParseUserInfo(user string) (*UserInfo, bool) {
	info := &UserInfo{}
	ok, _ := restructure.Find(info, user)
	return info, ok
}

// SSHConn implements the Conn interface for SSH sessions.
type SSHConn struct {
	s          Session
	exitReason string
	closeCh    chan struct{}
	once       sync.Once
}

// newSSHConn creates a new SSHConn from a Session.
func newSSHConn(s Session) Conn {
	return &SSHConn{
		s:       s,
		closeCh: make(chan struct{}),
	}
}

// Read reads data from the SSH session.
func (conn *SSHConn) Read(p []byte) (n int, err error) {
	select {
	case <-conn.closeCh:
		return 0, fmt.Errorf("ssh session closed")
	default:
	}
	n, err = conn.s.Read(p)
	if err != nil && err == io.EOF {
		conn.SetExitReason("User actively disconnected")
		_ = conn.Close()
	}
	return n, err
}

// Write writes data to the SSH session.
func (conn *SSHConn) Write(p []byte) (n int, err error) {
	select {
	case <-conn.closeCh:
		return 0, fmt.Errorf("ssh session closed")
	default:
	}
	return conn.s.Write(p)
}

// Close closes the SSH session.
func (conn *SSHConn) Close() error {
	conn.once.Do(func() {
		close(conn.closeCh)
	})
	return nil
}

// ExitReason returns the reason for session exit.
func (conn *SSHConn) ExitReason() string {
	return conn.exitReason
}

// SetExitReason sets the reason for session exit.
func (conn *SSHConn) SetExitReason(reason string) {
	conn.exitReason = reason
}

// WindowNotify notifies about terminal window size changes.
func (conn *SSHConn) WindowNotify(ctx context.Context, ch chan *remotecommand.TerminalSize) {
	_, windowCh, ok := conn.s.Pty()
	if !ok {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case window := <-windowCh:
			ch <- &remotecommand.TerminalSize{
				Width:  uint16(window.Width),
				Height: uint16(window.Height),
			}
		}
	}
}

// ClosedChan returns a channel that is closed when the connection is closed.
func (conn *SSHConn) ClosedChan() chan struct{} {
	return conn.closeCh
}

// RawCommand returns the raw command string.
func (conn *SSHConn) RawCommand() string {
	return conn.s.RawCommand()
}
