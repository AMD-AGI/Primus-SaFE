/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// opsScheme builds a scheme carrying only the SAFE API types.
func opsScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// fullScheme builds a scheme carrying the SAFE API types plus the core,
// apps and batch groups needed by controllers that touch native resources.
func fullScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// newBaseReconciler returns a base reconciler backed by a fake client that
// only knows the SAFE API types.
func newBaseReconciler(t *testing.T, objs ...client.Object) *OpsJobBaseReconciler {
	t.Helper()
	cl := ctrlfake.NewClientBuilder().
		WithScheme(opsScheme(t)).
		WithStatusSubresource(&v1.OpsJob{}).
		WithObjects(objs...).
		Build()
	return &OpsJobBaseReconciler{Client: cl}
}

// newBaseWithObjs returns a base reconciler backed by a fake client that also
// knows the core, apps and batch groups.
func newBaseWithObjs(t *testing.T, objs ...client.Object) *OpsJobBaseReconciler {
	t.Helper()
	cl := ctrlfake.NewClientBuilder().
		WithScheme(fullScheme(t)).
		WithStatusSubresource(&v1.OpsJob{}, &v1.Workload{}, &v1.Model{}).
		WithObjects(objs...).
		Build()
	return &OpsJobBaseReconciler{Client: cl}
}

// newTestOpsJob returns a finalizer-bearing OpsJob usable by any controller test.
func newTestOpsJob(name string) *v1.OpsJob {
	return &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name, Finalizers: []string{v1.OpsJobFinalizer}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobRebootType},
	}
}

// opsWorkQueue builds a rate-limiting queue accepted by the event handlers.
func opsWorkQueue() v1.RequestWorkQueue {
	return workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
}

// endedWorkload returns a succeeded workload labelled for the given ops job type.
func endedWorkload(opsType v1.OpsJobType) *v1.Workload {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "wl1",
			Labels: map[string]string{
				v1.OpsJobIdLabel:   "j1",
				v1.OpsJobTypeLabel: string(opsType),
			},
		},
		Status: v1.WorkloadStatus{Phase: v1.WorkloadSucceeded},
	}
	wl.Status.EndTime = &metav1.Time{Time: metav1.Now().Time}
	return wl
}

// runWorkloadEventHandler drives the create and update paths of a workload
// event handler with a transition from running to ended.
func runWorkloadEventHandler(t *testing.T, h interface {
	Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
	Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
}, wl *v1.Workload) {
	t.Helper()
	q := opsWorkQueue()
	defer q.ShutDown()
	h.Create(context.Background(), event.CreateEvent{Object: wl}, q)
	old := wl.DeepCopy()
	old.Status.Phase = v1.WorkloadRunning
	old.Status.EndTime = nil
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: old, ObjectNew: wl}, q)
}

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
