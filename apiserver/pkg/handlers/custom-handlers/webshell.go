/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow Cross-Origin Access
		return true
	},
}

const (
	sshUser     = "root"
	sshPassword = "root"
)

func (h *Handler) SSHPod(c *gin.Context) {
	handle(c, h.sshPod)
}

func (h *Handler) sshPod(c *gin.Context) (interface{}, error) {
	workload, err := h.getAdminWorkload(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if !workload.Spec.IsSSHEnabled {
		return nil, fmt.Errorf("ssh function is not enabled")
	}
	if !workload.IsRunning() || workload.Spec.Resource.SSHPort == 0 {
		return nil, fmt.Errorf("the workload is not running")
	}
	podId := strings.TrimSpace(c.Param(types.PodId))

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return nil, err
	}

	sshClient, err := connectToSSH(podId, workload)
	if err != nil {
		message := fmt.Sprintf("failed to ssh pod %s.", podId)
		klog.ErrorS(err, message)
		conn.WriteMessage(websocket.TextMessage, []byte(message))
		conn.Close()
		return nil, err
	}

	return nil, handleSSHSession(conn, sshClient)
}

func connectToSSH(podId string, workload *v1.Workload) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPassword),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	sshIp := podId
	if net.ParseIP(podId) != nil {
		for _, p := range workload.Status.Pods {
			if p.PodId == podId {
				sshIp = p.PodIp
			}
		}
	}
	sshAddress := fmt.Sprintf("%s:%d", sshIp, workload.Spec.Resource.SSHPort)
	client, err := ssh.Dial("tcp", sshAddress, config)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func handleSSHSession(conn *websocket.Conn, sshClient *ssh.Client) error {
	session, err := sshClient.NewSession()
	if err != nil {
		klog.ErrorS(err, "failed to create session")
		return err
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	err = session.RequestPty("xterm", 80, 40, modes)
	if err != nil {
		klog.ErrorS(err, "fail to request for pseudo terminal")
		return err
	}

	stdin, _ := session.StdinPipe()
	stdout, _ := session.StdoutPipe()
	stderr, _ := session.StderrPipe()

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			conn.WriteMessage(websocket.TextMessage, append(scanner.Bytes(), '\n'))
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			conn.WriteMessage(websocket.TextMessage, append(scanner.Bytes(), '\n'))
		}
	}()

	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				session.Close()
				return
			}
			stdin.Write(msg)
		}
	}()

	err = session.Shell()
	if err != nil {
		klog.ErrorS(err, "failed to start shell")
		return err
	}
	return nil
}
