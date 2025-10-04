/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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

	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

func (h *SshHandler) handleSession(s Session) {
	userInfo, ok := ParseUserInfo(s.User())
	if !ok {
		sendError(s, fmt.Sprintf("Invalid user %v", s.User()))
		return
	}

	workload, k8sClients, err := h.getWorkloadAndClients(s.Context(), userInfo)
	if err != nil {
		sendError(s, err.Error())
		return
	}
	if err = h.authUser(s.Context(), userInfo, workload); err != nil {
		sendError(s, err.Error())
		return
	}

	req := k8sClients.ClientSet().CoreV1().RESTClient().Post().
		Resource("pods").
		Name(userInfo.Pod).
		Namespace(userInfo.Namespace).
		SubResource("exec").VersionedParams(&corev1.PodExecOptions{
		Container: v1.GetMainContainer(workload),
		Command:   []string{"sh"},
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(k8sClients.RestConfig(), "POST", req.URL())
	if err != nil {
		sendError(s, fmt.Sprintf("failed to create SPDY executor: %s", err.Error()))
		return
	}
	err = executor.StreamWithContext(s.Context(), remotecommand.StreamOptions{
		Stdin:             s,
		Stdout:            s,
		Stderr:            s,
		TerminalSizeQueue: nil,
		Tty:               true,
	})
	if err != nil {
		message := ""
		if errors.Is(err, context.DeadlineExceeded) {
			message = fmt.Sprintf("\r\n[INFO] Connection timed out (%f hour)", h.timeout.Hours())
		} else {
			message = err.Error()
		}
		sendError(s, message)
	}
	klog.Infof("Connection to the Pod(%s/%s) has ended.", workload.Spec.Workspace, userInfo.Pod)
}

func (h *SshHandler) handleSftp(s Session) {
	userInfo, ok := ParseUserInfo(s.User())
	if !ok {
		klog.Errorf("failed to parse ssh info, user: %s", s.User())
		return
	}

	workload, k8sClients, err := h.getWorkloadAndClients(s.Context(), userInfo)
	if err != nil {
		klog.Error(err)
		return
	}
	if err = h.authUser(s.Context(), userInfo, workload); err != nil {
		klog.Error(err)
		return
	}

	req := k8sClients.ClientSet().CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(userInfo.Pod).
		Namespace(userInfo.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: v1.GetMainContainer(workload),
			Command:   []string{"/usr/lib/openssh/sftp-server"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(k8sClients.RestConfig(), "POST", req.URL())
	if err != nil {
		klog.ErrorS(err, "failed to create SFTP executor")
		return
	}

	err = exec.StreamWithContext(s.Context(), remotecommand.StreamOptions{
		Stdin:  s,
		Stdout: s,
		Stderr: s.Stderr(),
		Tty:    false,
	})
	if err != nil {
		klog.Error(err, "failed to stream SFTP command")
		return
	}
}

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

type forwardChannelData struct {
	DestAddr   string
	DestPort   uint32
	OriginAddr string
	OriginPort uint32
}

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
			go func() {
				defer ch.Close()
				defer destConn.Close()
				_, _ = io.Copy(ch, destConn)
				doneCancel()
			}()
			go func() {
				defer ch.Close()
				defer destConn.Close()
				_, _ = io.Copy(destConn, ch)
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

func (h *SshHandler) getWorkloadAndClients(ctx context.Context, userInfo *UserInfo) (*v1.Workload, *commonclient.ClientFactory, error) {
	workspace := &v1.Workspace{}
	err := h.Get(ctx, client.ObjectKey{Name: userInfo.Namespace}, workspace)
	if err != nil {
		err = fmt.Errorf("failed to get namespace, %s", err.Error())
		return nil, nil, err
	}

	k8sClients, err := apiutils.GetK8sClientFactory(h.clientManager, workspace.Spec.Cluster)
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

func (h *SshHandler) authUser(ctx context.Context, userInfo *UserInfo, workload *v1.Workload) error {
	if err := h.auth.Authorize(authority.Input{
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

func sendError(w io.Writer, msg string) {
	klog.Error(msg)
	_, _ = w.Write([]byte(msg + "\n"))
}
