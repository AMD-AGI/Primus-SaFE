/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

// Environment for the live test. It stays skipped unless a cluster and a target Pod
// are named, so an ordinary `go test` run never reaches a real API server.
const (
	liveKubeconfigEnv = "SAFE_LIVE_KUBECONFIG"
	liveNamespaceEnv  = "SAFE_LIVE_NAMESPACE"
	livePodEnv        = "SAFE_LIVE_POD"
	liveContainerEnv  = "SAFE_LIVE_CONTAINER"
)

// liveTarget reads the live-cluster target, skipping the test when it is absent.
func liveTarget(t *testing.T) (*commonclient.ClientFactory, *UserInfo) {
	t.Helper()

	kubeconfig := os.Getenv(liveKubeconfigEnv)
	namespace, pod := os.Getenv(liveNamespaceEnv), os.Getenv(livePodEnv)
	if kubeconfig == "" || namespace == "" || pod == "" {
		t.Skipf("set %s, %s and %s to run against a live cluster",
			liveKubeconfigEnv, liveNamespaceEnv, livePodEnv)
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	testifyassert.NoError(t, err)
	clientSet, err := kubernetes.NewForConfig(restConfig)
	testifyassert.NoError(t, err)

	clients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), "live", clientSet)
	clients.AttachRestConfigForTest(restConfig)
	return clients, &UserInfo{
		User:      "live-test",
		Namespace: namespace,
		Pod:       pod,
		Container: os.Getenv(liveContainerEnv),
	}
}

// TestLivePodListenerOverK8sExec exercises the one layer the offline tests cannot:
// the Kubernetes exec transport. The listen socket is opened inside a real Pod and a
// connection made to it is carried back over a real exec stream.
//
// The test process runs inside the target Pod, so its loopback is the Pod's loopback.
func TestLivePodListenerOverK8sExec(t *testing.T) {
	clients, userInfo := liveTarget(t)

	port := freeTCPPort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	listener, err := newExecPodListener(ctx, userInfo, clients, "127.0.0.1", port)
	testifyassert.NoError(t, err)
	if err != nil {
		return
	}
	defer listener.Close()

	// A process in the Pod connects to the forwarded port.
	podSide, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", itoa(port)))
	testifyassert.NoError(t, err)
	defer podSide.Close()

	conn, err := listener.Accept(ctx)
	testifyassert.NoError(t, err)
	if err != nil {
		return
	}
	defer conn.Close()
	testifyassert.NotEmpty(t, conn.OriginAddr())

	// Pod -> apiserver.
	_, err = podSide.Write([]byte("from-pod\n"))
	testifyassert.NoError(t, err)
	buf := make([]byte, len("from-pod\n"))
	_, err = io.ReadFull(conn, buf)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "from-pod\n", string(buf))

	// apiserver -> Pod.
	_, err = conn.Write([]byte("from-apiserver\n"))
	testifyassert.NoError(t, err)
	_ = podSide.SetReadDeadline(time.Now().Add(30 * time.Second))
	back := make([]byte, len("from-apiserver\n"))
	_, err = io.ReadFull(podSide, back)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "from-apiserver\n", string(back))

	// Closing the listener must free the port inside the Pod.
	testifyassert.NoError(t, listener.Close())
	waitForPortFree(t, port)
}

// TestLiveReverseForwardEndToEnd runs the whole feature against a real Pod: a real
// SSH client asks for `-R`, the listener is created in the Pod over the Kubernetes
// exec transport, and an HTTP request made inside the Pod is served by the SSH
// client's side of the connection - the GitHub-via-local-proxy use case.
func TestLiveReverseForwardEndToEnd(t *testing.T) {
	clients, userInfo := liveTarget(t)

	port := freeTCPPort(t)
	enableReverseForward(t, map[string]any{
		sshReverseForwardPortMinKey: int(port),
		sshReverseForwardPortMaxKey: int(port),
	})

	rig := newForwardTestRigWith(t, func(m *reverseForwardManager) {
		m.resolve = func(context.Context, *UserInfo) (*commonclient.ClientFactory, error) {
			return clients, nil
		}
		m.newListener = func(ctx context.Context, _ *UserInfo, c *commonclient.ClientFactory,
			bindAddr string, bindPort uint32) (podListener, error) {
			// The rig's login name is not a real workload, so the target comes from
			// the environment while everything else stays production code.
			return newExecPodListener(ctx, userInfo, c, bindAddr, bindPort)
		}
	})

	listener, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: int(port)})
	testifyassert.NoError(t, err)

	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "served from the developer's machine: "+r.URL.Path)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", itoa(port)) + "/api/github")
	testifyassert.NoError(t, err)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, http.StatusOK, resp.StatusCode)
	testifyassert.Equal(t, "served from the developer's machine: /api/github", string(body))

	// Dropping the SSH connection must remove the Pod-side listener.
	rig.closeConn()
	waitForPortFree(t, port)
}
