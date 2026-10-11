/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// Execer runs a command in the container being exported, the way `kubectl exec` does.
// A non-zero exit status is returned as an error.
type Execer interface {
	Exec(ctx context.Context, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error
}

// PodExecer runs commands through the Kubernetes pods/exec subresource. It needs nothing
// on the node: the same call reaches a container on a kubelet and one on a virtual
// kubelet.
type PodExecer struct {
	Config    *rest.Config
	Client    kubernetes.Interface
	Namespace string
	Pod       string
	Container string
}

// Exec implements Execer. It speaks WebSocket and falls back to SPDY, as kubectl does.
func (e *PodExecer) Exec(ctx context.Context, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	req := e.Client.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(e.Namespace).Name(e.Pod).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: e.Container,
			Command:   cmd,
			Stdin:     stdin != nil,
			Stdout:    stdout != nil,
			Stderr:    stderr != nil,
		}, scheme.ParameterCodec)
	spdy, err := remotecommand.NewSPDYExecutor(e.Config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("creating exec stream: %w", err)
	}
	ws, err := remotecommand.NewWebSocketExecutor(e.Config, "GET", req.URL().String())
	if err != nil {
		return fmt.Errorf("creating exec stream: %w", err)
	}
	exec, err := remotecommand.NewFallbackExecutor(ws, spdy, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})
	if err != nil {
		return fmt.Errorf("creating exec stream: %w", err)
	}
	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: stdin, Stdout: stdout, Stderr: stderr})
}
