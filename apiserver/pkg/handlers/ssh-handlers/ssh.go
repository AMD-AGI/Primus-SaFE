/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/alexflint/go-restructure"
	"golang.org/x/crypto/ssh"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

type SshHandler struct {
	ctx context.Context
	client.Client
	clientManager *commonutils.ObjectManager
	config        *ssh.ServerConfig
	auth          *authority.Authorizer
	timeout       time.Duration
}

func NewSshHandler(ctx context.Context, mgr ctrlruntime.Manager) (*SshHandler, error) {
	// TODO: validate user's public-key
	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	privateData := commonconfig.GetSSHRsaPrivate()
	if len(privateData) == 0 {
		return nil, fmt.Errorf("id_rsa is empty")
	}

	private, err := ssh.ParsePrivateKey([]byte(privateData))
	if err != nil {
		return nil, err
	}
	config.AddHostKey(private)

	h := &SshHandler{
		ctx:           ctx,
		Client:        mgr.GetClient(),
		clientManager: commonutils.NewObjectManagerSingleton(),
		config:        config,
		auth:          authority.NewAuthorizer(mgr.GetClient()),
		timeout:       time.Hour * 48,
	}
	return h, nil
}

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

func (h *SshHandler) startSessionHandler(ctx context.Context, conn *ssh.ServerConn, newChan ssh.NewChannel) {
	ch, reqs, err := newChan.Accept()
	if err != nil {
		klog.ErrorS(err, "failed to accept channel")
		return
	}
	fmt.Fprintf(ch, "\r\x1b[93mConnecting ...\x1b[0m\r\n")
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

type UserInfo struct {
	_         struct{} `regexp:"^"`
	User      string   `regexp:"[a-zA-Z0-9][a-zA-Z0-9_-]*"`
	_         struct{} `regexp:"\\."`
	Pod       string   `regexp:"[a-zA-Z0-9][a-zA-Z0-9_-]*"`
	_         struct{} `regexp:"\\."`
	Namespace string   `regexp:"[a-zA-Z0-9][a-zA-Z0-9_-]*"`
	_         struct{} `regexp:"$"`
}

func ParseUserInfo(user string) (*UserInfo, bool) {
	info := &UserInfo{}
	ok, _ := restructure.Find(info, user)
	return info, ok
}
