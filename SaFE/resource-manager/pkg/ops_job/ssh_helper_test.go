/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"crypto/rand"
	"crypto/rsa"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"
)

// startInMemorySSHServer launches a minimal SSH server that accepts any auth and
// answers every "exec" request with a successful empty result. It returns a
// connected client and a cleanup function. Used to exercise SSH-driven code
// paths without a real remote host.
func startInMemorySSHServer(t *testing.T) (*ssh.Client, func()) {
	t.Helper()
	return startInMemorySSHServerWithExecHandler(t, func(_ *ssh.Request, channel ssh.Channel) {
		// Send exit-status 0 and close.
		_, _ = channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
		_ = channel.Close()
	})
}

func startInMemorySSHServerWithoutExitStatus(t *testing.T) (*ssh.Client, func()) {
	t.Helper()
	return startInMemorySSHServerWithExecHandler(t, func(_ *ssh.Request, channel ssh.Channel) {
		// A real reboot can drop the SSH session before an exit status is sent.
		_ = channel.Close()
	})
}

func startInMemorySSHServerWithExecHandler(t *testing.T, execHandler func(*ssh.Request, ssh.Channel)) (*ssh.Client, func()) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}

	serverConf := &ssh.ServerConfig{NoClientAuth: true}
	serverConf.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			sconn, chans, reqs, serr := ssh.NewServerConn(conn, serverConf)
			if serr != nil {
				continue
			}
			go ssh.DiscardRequests(reqs)
			go handleSSHChannels(chans, execHandler)
			_ = sconn
		}
	}()

	opsJobSSHAddr = ln.Addr().String()
	client, err := dialOpsJobSSH()
	if err != nil {
		ln.Close()
		t.Fatal(err)
	}
	return client, func() {
		client.Close()
		ln.Close()
		opsJobSSHAddr = ""
	}
}

// opsJobSSHAddr holds the in-memory SSH server address so the GetSSHClient
// patch can dial a fresh connection per call.
var opsJobSSHAddr string

func dialOpsJobSSH() (*ssh.Client, error) {
	clientConf := &ssh.ClientConfig{
		User:            "root",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	return ssh.Dial("tcp", opsJobSSHAddr, clientConf)
}

func handleSSHChannels(chans <-chan ssh.NewChannel, execHandler func(*ssh.Request, ssh.Channel)) {
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "only session supported")
			continue
		}
		ch, requests, err := newChan.Accept()
		if err != nil {
			continue
		}
		go func(in <-chan *ssh.Request, channel ssh.Channel) {
			for req := range in {
				switch req.Type {
				case "exec":
					if req.WantReply {
						_ = req.Reply(true, nil)
					}
					execHandler(req, channel)
				default:
					if req.WantReply {
						_ = req.Reply(false, nil)
					}
				}
			}
		}(requests, ch)
	}
}
