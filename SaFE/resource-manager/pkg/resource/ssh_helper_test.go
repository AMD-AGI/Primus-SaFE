/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"crypto/rand"
	"crypto/rsa"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"
)

// startInMemorySSHServer launches a minimal SSH server that accepts any auth and
// answers every "exec" request with a successful empty result.
func startInMemorySSHServer(t *testing.T) (*ssh.Client, func()) {
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
			go handleSSHChannels(chans)
			_ = sconn
		}
	}()

	addr := ln.Addr().String()
	resourceSSHAddr = addr
	c, err := dialResourceSSH()
	if err != nil {
		ln.Close()
		t.Fatal(err)
	}
	return c, func() {
		c.Close()
		ln.Close()
		resourceSSHAddr = ""
	}
}

// resourceSSHAddr holds the address of the running in-memory SSH server so the
// gomonkey patch for GetSSHClient can dial a fresh connection on each call
// (production code closes the client via defer after every operation).
var resourceSSHAddr string

func dialResourceSSH() (*ssh.Client, error) {
	clientConf := &ssh.ClientConfig{
		User:            "root",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	return ssh.Dial("tcp", resourceSSHAddr, clientConf)
}

func handleSSHChannels(chans <-chan ssh.NewChannel) {
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
					_, _ = channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
					_ = channel.Close()
				default:
					if req.WantReply {
						_ = req.Reply(false, nil)
					}
				}
			}
		}(requests, ch)
	}
}
