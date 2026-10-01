/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
	k8sexec "k8s.io/client-go/util/exec"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// SessionConn establishes an SSH session to a Kubernetes pod and records the session.
// The returned code is the remote process exit status (or 1 when the stream fails without one).
func (h *SshHandler) SessionConn(ctx context.Context, sessionInfo *SessionInfo) (uint32, error) {
	workload, k8sClients, err := h.getWorkloadAndClients(ctx, sessionInfo.userInfo)
	if err != nil {
		return 1, err
	}
	if err = h.authUser(ctx, sessionInfo.userInfo, workload); err != nil {
		return 1, err
	}

	rawCmd := sessionInfo.userConn.RawCommand()
	isInteractive := sessionInfo.isPty || IsShellCommand(rawCmd)

	execOptions := &corev1.PodExecOptions{
		Container: sessionInfo.userInfo.Container,
		Command:   []string{sessionInfo.userInfo.CMD},
		// Every exec attaches stdin so clients that stream a body (rsync, tar, cat >file)
		// are not silently truncated to zero bytes.
		Stdin:  true,
		Stdout: true,
		Stderr: true,
		TTY:    sessionInfo.isPty,
	}
	if !isInteractive {
		execOptions.Command = append(execOptions.Command, "-c", rawCmd)
	}

	req := k8sClients.ClientSet().CoreV1().RESTClient().Post().
		Resource("pods").
		Name(sessionInfo.userInfo.Pod).
		Namespace(sessionInfo.userInfo.Namespace).
		SubResource("exec").
		Timeout(time.Hour).
		VersionedParams(execOptions, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(k8sClients.RestConfig(), "POST", req.URL())
	if err != nil {
		return 1, fmt.Errorf("failed to create SPDY executor: %v", err)
	}
	sessionInfo.size <- &remotecommand.TerminalSize{
		Width:  uint16(sessionInfo.cols),
		Height: uint16(sessionInfo.rows),
	}

	if isInteractive {
		go sessionInfo.userConn.WindowNotify(ctx, sessionInfo.size)
	}

	nowTime := dbutils.NullMetaV1Time(&metav1.Time{Time: time.Now().UTC()})
	recordId, err := h.dbClient.InsertSshSessionRecord(ctx, &dbclient.SshSessionRecords{
		UserId:        sessionInfo.userInfo.User,
		SshType:       string(sessionInfo.sshType),
		Namespace:     sessionInfo.userInfo.Namespace,
		PodId:         sessionInfo.userInfo.Pod,
		ContainerName: sessionInfo.userInfo.Container,
		CreateTime:    nowTime,
	})
	if err != nil {
		return 1, fmt.Errorf("create ssh session record err: %v", err)
	}
	defer func() {
		if err := h.dbClient.SetSshDisconnect(context.Background(), recordId, sessionInfo.userConn.ExitReason()); err != nil {
			klog.Errorf("set ssh session record disconnect reason err: %v", err)
		}
	}()

	stderr := io.Writer(sessionInfo.userConn)
	if sw, ok := sessionInfo.userConn.(interface{ Stderr() io.Writer }); ok && !sessionInfo.isPty {
		stderr = sw.Stderr()
	}

	errCh := make(chan error, 1)
	go func() {
		options := remotecommand.StreamOptions{
			Stdin:             sessionInfo.userConn,
			Stdout:            sessionInfo.userConn,
			Stderr:            stderr,
			TerminalSizeQueue: sessionInfo,
			Tty:               sessionInfo.isPty,
		}
		if sessionInfo.isPty {
			// With a TTY the kubelet merges stderr into the stdout stream.
			options.Stderr = nil
		}
		if !isInteractive {
			options.TerminalSizeQueue = nil
		}
		errCh <- executor.StreamWithContext(ctx, options)
	}()

	var streamErr error
	select {
	case <-ctx.Done():
		streamErr = ctx.Err()
		sessionInfo.userConn.SetExitReason(fmt.Sprintf("\r\n[INFO] Connection idle timed out (%s)", h.timeout))
	case streamErr = <-errCh:
		message := "The underlying connection is disconnected normally"
		if streamErr != nil {
			if errors.Is(streamErr, context.DeadlineExceeded) || errors.Is(streamErr, context.Canceled) {
				message = fmt.Sprintf("\r\n[INFO] Connection idle timed out (%s)", h.timeout)
			} else if _, ok := streamExitError(streamErr); ok {
				message = "The underlying connection is disconnected normally"
			} else {
				message = fmt.Sprintf("The underlying connection is abnormally disconnected：%s", streamErr.Error())
			}
		}
		sessionInfo.userConn.SetExitReason(message)
	case <-sessionInfo.userConn.ClosedChan():
	}

	code := streamExitCode(streamErr)
	klog.Infof("Connection to the Pod(%s/%s) has ended, reason: %s, exit: %d", workload.Spec.Workspace,
		sessionInfo.userInfo.Pod, sessionInfo.userConn.ExitReason(), code)
	return code, nil
}

// streamExitError reports whether err carries a remote process exit status.
func streamExitError(err error) (k8sexec.CodeExitError, bool) {
	var exitErr k8sexec.CodeExitError
	if err == nil || !errors.As(err, &exitErr) {
		return k8sexec.CodeExitError{}, false
	}
	return exitErr, true
}

// streamExitCode returns the remote process exit status, or 1 when the stream failed
// without one. A nil error is exit 0.
func streamExitCode(err error) uint32 {
	if err == nil {
		return 0
	}
	if exitErr, ok := streamExitError(err); ok {
		if exitErr.Code < 0 {
			return 1
		}
		return uint32(exitErr.Code)
	}
	return 1
}

// handleSftp handles SFTP requests over SSH for a Kubernetes pod.
func (h *SshHandler) handleSftp(s Session) uint32 {
	userInfo, ok := ParseUserInfo(s.User())
	if !ok {
		klog.Errorf("failed to parse ssh info, user: %s", s.User())
		return 1
	}

	workload, k8sClients, err := h.getWorkloadAndClients(s.Context(), userInfo)
	if err != nil {
		klog.Error(err)
		return 1
	}
	if err = h.authUser(s.Context(), userInfo, workload); err != nil {
		klog.Error(err)
		return 1
	}
	req := k8sClients.ClientSet().CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(userInfo.Pod).
		Namespace(userInfo.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: commonworkload.GetMainContainerByPod(workload, workload.SpecKind(), userInfo.Pod),
			Command:   []string{"/usr/lib/openssh/sftp-server"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(k8sClients.RestConfig(), "POST", req.URL())
	if err != nil {
		klog.ErrorS(err, "failed to create SFTP executor")
		return 1
	}

	err = exec.StreamWithContext(s.Context(), remotecommand.StreamOptions{
		Stdin:  s,
		Stdout: s,
		Stderr: s.Stderr(),
		Tty:    false,
	})
	if err != nil {
		klog.Error(err, "failed to stream SFTP command")
		return streamExitCode(err)
	}
	return 0
}

// handleDirectIp handles direct IP forwarding requests over SSH.
func (h *SshHandler) handleDirectIp(ctx context.Context, sshConn *ssh.ServerConn, newChan ssh.NewChannel) {
	forwardData := forwardChannelData{}
	if err := ssh.Unmarshal(newChan.ExtraData(), &forwardData); err != nil {
		err = fmt.Errorf("failed to parse forward data: %s", err.Error())
		klog.Error(err.Error())
		_ = newChan.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	userInfo, ok := ParseUserInfo(sshConn.User())
	if !ok {
		klog.Errorf("failed to parse ssh info, user: %s", sshConn.User())
		return
	}
	workload, k8sClients, err := h.getWorkloadAndClients(ctx, userInfo)
	if err != nil {
		klog.Error(err)
		return
	}
	if err = h.authUser(ctx, userInfo, workload); err != nil {
		klog.Error(err)
		return
	}

	req := k8sClients.ClientSet().CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(userInfo.Namespace).
		Name(userInfo.Pod).
		SubResource("portforward")
	transport, upgrader, err := spdy.RoundTripperFor(k8sClients.RestConfig())
	if err != nil {
		klog.ErrorS(err, "failed to create roundtripper")
		return
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())

	if err = h.forward(ctx, dialer, forwardData, newChan); err != nil {
		klog.ErrorS(err, "failed to forward to pod")
	}
}

// forward establishes port forwarding between SSH and Kubernetes pod.
func (h *SshHandler) forward(ctx context.Context, dialer httpstream.Dialer,
	forwardData forwardChannelData, newChan ssh.NewChannel) error {
	ports := []string{fmt.Sprintf("%d:%d", forwardData.OriginPort, forwardData.DestPort)}
	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{})
	forwarder, err := portforward.New(dialer, ports, stopChan, readyChan, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to create port forward: %v", err)
	}

	go func() {
		if err = forwarder.ForwardPorts(); err != nil {
			klog.ErrorS(err, "failed to forward port")
		}
	}()

	select {
	case <-readyChan:
		go func() {
			dest := net.JoinHostPort(forwardData.OriginAddr, strconv.FormatInt(int64(forwardData.OriginPort), 10))
			var dialer net.Dialer
			destConn, err := dialer.DialContext(ctx, "tcp", dest)
			if err != nil {
				_ = newChan.Reject(ssh.ConnectionFailed, err.Error())
				return
			}

			ch, reqs, err := newChan.Accept()
			if err != nil {
				_ = destConn.Close()
				return
			}
			go ssh.DiscardRequests(reqs)

			doneCtx, doneCancel := context.WithCancel(ctx)
			idle := idleFromContext(ctx)
			go func() {
				defer ch.Close()
				defer destConn.Close()
				_, _ = io.Copy(touchingWriter{w: ch, idle: idle}, touchingReader{r: destConn, idle: idle})
				doneCancel()
			}()
			go func() {
				defer ch.Close()
				defer destConn.Close()
				_, _ = io.Copy(touchingWriter{w: destConn, idle: idle}, touchingReader{r: ch, idle: idle})
				doneCancel()
			}()
			select {
			case <-doneCtx.Done():
				close(stopChan)
			}
		}()
	case <-time.After(15 * time.Second):
		return fmt.Errorf("ssh port forward timeout")
	}
	return nil
}

// getWorkloadAndClients retrieves the workload and Kubernetes client factory for the user.
func (h *SshHandler) getWorkloadAndClients(ctx context.Context, userInfo *UserInfo) (*v1.Workload, *commonclient.ClientFactory, error) {
	workspace := &v1.Workspace{}
	err := h.Get(ctx, client.ObjectKey{Name: userInfo.Namespace}, workspace)
	if err != nil {
		err = fmt.Errorf("failed to get namespace, %s", err.Error())
		return nil, nil, err
	}

	k8sClients, err := commonutils.GetK8sClientFactory(h.clientManager, workspace.Spec.Cluster)
	if err != nil {
		return nil, nil, err
	}

	pod, err := k8sClients.ClientSet().CoreV1().Pods(userInfo.Namespace).
		Get(ctx, userInfo.Pod, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("failed to get pod, %s", err.Error())
		return nil, nil, err
	}
	workloadId := v1.GetWorkloadId(pod)
	if workloadId == "" {
		err = fmt.Errorf("failed to get workload id. pod: %s", pod.Name)
		return nil, nil, err
	}
	workload := &v1.Workload{}
	err = h.Get(ctx, client.ObjectKey{Name: workloadId}, workload)
	if err != nil {
		err = fmt.Errorf("failed to get workload, %s", err.Error())
		return nil, nil, err
	}
	return workload, k8sClients, nil
}

// authUser authorizes the user for the given workload.
func (h *SshHandler) authUser(ctx context.Context, userInfo *UserInfo, workload *v1.Workload) error {
	if err := h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   workload,
		Verb:       v1.GetVerb,
		Workspaces: []string{workload.Spec.Workspace},
		UserId:     userInfo.User,
	}); err != nil {
		return err
	}
	return nil
}

// sendError writes an error message to the writer and logs it.
func sendError(w io.Writer, msg string) {
	klog.Error(msg)
	_, _ = w.Write([]byte(msg + "\n"))
}

// forwardChannelData holds data for port forwarding.
type forwardChannelData struct {
	DestAddr   string
	DestPort   uint32
	OriginAddr string
	OriginPort uint32
}

// IsShellCommand checks if the given command is a valid shell command.
func IsShellCommand(cmd string) bool {
	shells := []string{"sh", "bash", "zsh", "ash", "ksh", "csh", "tcsh", "bash --login -c bash"}
	for _, shell := range shells {
		if cmd == shell {
			return true
		}
	}
	return false
}
