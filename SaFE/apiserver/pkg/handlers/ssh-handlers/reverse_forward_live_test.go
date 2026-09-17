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
	"strings"
	"sync"
	"sync/atomic"
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

	// The test process runs inside the target Pod, so a multiplexer built here is
	// one that pod can run - and building it means the live tests do not depend on
	// the image build having already replaced the committed placeholder.
	injectHostMux(t)

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
	dir := installDir(t, listener)

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

	// A half-close in one direction must leave the other one flowing, which is what
	// every request followed by its reply depends on.
	testifyassert.NoError(t, conn.CloseWrite())
	_, err = podSide.Write([]byte("after-half-close\n"))
	testifyassert.NoError(t, err)
	tail := make([]byte, len("after-half-close\n"))
	_, err = io.ReadFull(conn, tail)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "after-half-close\n", string(tail))

	// Closing the listener must free the port inside the Pod.
	testifyassert.NoError(t, listener.Close())
	waitForPortFree(t, port)
	// The multiplexer removes its own files, so a forward leaves the container as
	// it found it.
	waitFor(t, func() bool { return installRemoved(dir) },
		"the injected multiplexer to be removed from the pod")
}

// installDir is where the listener under test put its multiplexer.
//
// The check is against this one directory rather than every `.safe-rfwd-*` under the
// install directories: a pod may well be carrying other forwards of its own, and
// this test is not entitled to an opinion about those.
func installDir(t *testing.T, listener podListener) string {
	t.Helper()
	l, ok := listener.(*execPodListener)
	testifyassert.True(t, ok)
	if !ok {
		return ""
	}
	testifyassert.NotEmpty(t, l.dir, "the listener never recorded where it installed")
	return l.dir
}

// installRemoved reports whether the multiplexer has taken its own files away. The
// live tests run inside the target Pod, so this is the Pod's filesystem.
func installRemoved(dir string) bool {
	_, err := os.Stat(dir)
	return os.IsNotExist(err)
}

// TestLiveReverseForwardBurstLeavesNothingBehind is the load the single-exec
// design exists for. The socat relay took one exec per connection, capped at
// thirty-two, and left the pod holding rendezvous directories and unattached
// children; a burst here has to leave one exec, no leftover files, and a listen
// port that is free the moment the forward closes.
func TestLiveReverseForwardBurstLeavesNothingBehind(t *testing.T) {
	clients, userInfo := liveTarget(t)

	port := freeTCPPort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	listener, err := newExecPodListener(ctx, userInfo, clients, "127.0.0.1", port)
	testifyassert.NoError(t, err)
	if err != nil {
		return
	}
	defer listener.Close()
	dir := installDir(t, listener)

	// Serve every accepted connection with one line and hang up, so the burst is a
	// burst of connections rather than of bytes.
	served := make(chan struct{}, 256)
	go func() {
		for {
			conn, acceptErr := listener.Accept(ctx)
			if acceptErr != nil {
				return
			}
			go func(c podConn) {
				defer c.Close()
				_, _ = io.Copy(c, strings.NewReader("ok\n"))
				_ = c.CloseWrite()
				_, _ = io.Copy(io.Discard, c)
				served <- struct{}{}
			}(conn)
		}
	}()

	const connections = 150
	var failures atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < connections; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, dialErr := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", itoa(port)), 30*time.Second)
			if dialErr != nil {
				failures.Add(1)
				return
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(60 * time.Second))
			if _, writeErr := c.Write([]byte("ping\n")); writeErr != nil {
				failures.Add(1)
				return
			}
			body, readErr := io.ReadAll(c)
			if readErr != nil || string(body) != "ok\n" {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	testifyassert.Zero(t, failures.Load(), "connections were lost across one forward")

	// Nothing is left holding a stream once the burst is over.
	l, ok := listener.(*execPodListener)
	testifyassert.True(t, ok)
	if ok {
		waitFor(t, func() bool { return l.session.NumStreams() == 0 }, "every stream to be released")
	}

	testifyassert.NoError(t, listener.Close())
	waitForPortFree(t, port)
	waitFor(t, func() bool { return installRemoved(dir) },
		"the pod to be left with no reverse forward files")
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
