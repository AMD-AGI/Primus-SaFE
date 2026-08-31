/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/viper"
	testifyassert "github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/remotecommand"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"

	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

const testForwardUser = "root.pod-0.main.bash.ns"

// enableReverseForward turns the feature on for the duration of a test.
func enableReverseForward(t *testing.T, values map[string]any) {
	t.Helper()
	wasEnabled := viper.Get(sshReverseForwardEnableKey)
	viper.Set(sshReverseForwardEnableKey, true)
	t.Cleanup(func() { viper.Set(sshReverseForwardEnableKey, wasEnabled) })
	for key, value := range values {
		key, value := key, value
		previous := viper.Get(key)
		viper.Set(key, value)
		t.Cleanup(func() { viper.Set(key, previous) })
	}
}

// Config keys mirrored from common/pkg/config so tests can toggle them.
const (
	sshReverseForwardEnableKey   = "ssh.reverse_forward.enable"
	sshReverseForwardPortMinKey  = "ssh.reverse_forward.port_min"
	sshReverseForwardPortMaxKey  = "ssh.reverse_forward.port_max"
	sshReverseForwardMaxPerSess  = "ssh.reverse_forward.max_forwards_per_session"
	sshReverseForwardBindAddrKey = "ssh.reverse_forward.bind_addresses"
)

// --- policy ---------------------------------------------------------------

func TestReverseForwardPolicyValidate(t *testing.T) {
	policy := reverseForwardPolicy{
		enabled:       true,
		bindAddresses: []string{"127.0.0.1"},
		portMin:       10000,
		portMax:       19999,
		maxForwards:   8,
	}

	cases := []struct {
		name    string
		addr    string
		port    uint32
		want    string
		wantErr string
	}{
		{name: "loopback", addr: "127.0.0.1", port: 10001, want: "127.0.0.1"},
		{name: "localhost is normalized", addr: "localhost", port: 10001, want: "127.0.0.1"},
		{name: "empty bind means all interfaces", addr: "", port: 10001, wantErr: "not allowed"},
		{name: "star means all interfaces", addr: "*", port: 10001, wantErr: "not allowed"},
		{name: "other address", addr: "10.0.0.5", port: 10001, wantErr: "not allowed"},
		{name: "hostname", addr: "example.com", port: 10001, wantErr: "not a literal IP"},
		{name: "dynamic port", addr: "127.0.0.1", port: 0, wantErr: "server-allocated"},
		{name: "below range", addr: "127.0.0.1", port: 22, wantErr: "outside the permitted range"},
		{name: "above range", addr: "127.0.0.1", port: 20000, wantErr: "outside the permitted range"},
		{name: "not a port", addr: "127.0.0.1", port: 70000, wantErr: "out of range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := policy.validate(tc.addr, tc.port)
			if tc.wantErr != "" {
				testifyassert.Error(t, err)
				testifyassert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			testifyassert.NoError(t, err)
			testifyassert.Equal(t, tc.want, got)
		})
	}
}

func TestReverseForwardPolicyDisabled(t *testing.T) {
	_, err := reverseForwardPolicy{bindAddresses: []string{"127.0.0.1"}}.validate("127.0.0.1", 10001)
	testifyassert.Error(t, err)
	testifyassert.Contains(t, err.Error(), "disabled")
}

func TestReverseForwardPolicyAllowsConfiguredWildcard(t *testing.T) {
	policy := reverseForwardPolicy{
		enabled:       true,
		bindAddresses: []string{"0.0.0.0"},
		portMin:       10000,
		portMax:       19999,
	}
	got, err := policy.validate("", 10001)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "0.0.0.0", got)
}

// TestLoadReverseForwardPolicyDefaults pins the defaults the production getters
// produce when nothing is configured.
func TestLoadReverseForwardPolicyDefaults(t *testing.T) {
	policy := loadReverseForwardPolicy()
	testifyassert.True(t, policy.enabled)
	testifyassert.Equal(t, []string{"127.0.0.1"}, policy.bindAddresses)
	testifyassert.Equal(t, uint32(1024), policy.portMin)
	testifyassert.Equal(t, uint32(65535), policy.portMax)
	testifyassert.Equal(t, 8, policy.maxForwards)
}

// TestLoadReverseForwardPolicyFromConfig pins that the config keys are actually read.
func TestLoadReverseForwardPolicyFromConfig(t *testing.T) {
	enableReverseForward(t, map[string]any{
		sshReverseForwardPortMinKey:  20000,
		sshReverseForwardPortMaxKey:  20010,
		sshReverseForwardMaxPerSess:  2,
		sshReverseForwardBindAddrKey: []string{"127.0.0.1", "0.0.0.0"},
	})
	policy := loadReverseForwardPolicy()
	testifyassert.True(t, policy.enabled)
	testifyassert.Equal(t, []string{"127.0.0.1", "0.0.0.0"}, policy.bindAddresses)
	testifyassert.Equal(t, uint32(20000), policy.portMin)
	testifyassert.Equal(t, uint32(20010), policy.portMax)
	testifyassert.Equal(t, 2, policy.maxForwards)
}

// TestReverseForwardDefaultPortRange pins what the shipped defaults actually allow.
// The motivating case is a proxy on the conventional 7890, so a default range that
// refused it would make the documented example fail on a stock install; the floor is
// there to stop a forward shadowing a privileged service inside the pod.
func TestReverseForwardDefaultPortRange(t *testing.T) {
	enableReverseForward(t, nil)
	policy := loadReverseForwardPolicy()

	for _, port := range []uint32{7890, 1080, 8080, 1024, 65535} {
		_, err := policy.validate("127.0.0.1", port)
		testifyassert.NoErrorf(t, err, "port %d should be allowed by default", port)
	}
	for _, port := range []uint32{22, 53, 443, 1023} {
		_, err := policy.validate("127.0.0.1", port)
		testifyassert.Errorf(t, err, "privileged port %d must be refused by default", port)
	}
}

// --- pod listener plumbing ------------------------------------------------

func TestParseConnAnnouncement(t *testing.T) {
	conn, ok := parseConnAnnouncement("SAFE-RFWD-CONN 4242 127.0.0.1 51234")
	testifyassert.True(t, ok)
	testifyassert.Equal(t, "4242", conn.id)
	testifyassert.Equal(t, "127.0.0.1", conn.peerAddr)
	testifyassert.Equal(t, uint32(51234), conn.peerPort)

	for _, line := range []string{
		"",
		"SAFE-RFWD-CONN",
		"SAFE-RFWD-CONN 4242 127.0.0.1",
		"SAFE-RFWD-CONN 4242 127.0.0.1 51234 extra",
		"SAFE-RFWD-CONN ../../etc/passwd 127.0.0.1 51234",
		"SAFE-RFWD-CONN 42;rm 127.0.0.1 51234",
		"SAFE-RFWD-CONN 4242 127.0.0.1 notaport",
		"SAFE-RFWD-CONN 4242 127.0.0.1 70000",
		// An overlong id would still be digits-only, so the length bound is what
		// keeps it from becoming an unbounded path component.
		"SAFE-RFWD-CONN 123456789012345678901234567890123 127.0.0.1 51234",
		"SAFE-RFWD-READY",
	} {
		_, ok := parseConnAnnouncement(line)
		testifyassert.Falsef(t, ok, "expected %q to be rejected", line)
	}
}

func TestAcceptorScript(t *testing.T) {
	script := acceptorScript("/tmp/.safe-rfwd-abcd", "127.0.0.1", 10001, 12)
	testifyassert.Contains(t, script, "TCP-LISTEN:10001,bind=127.0.0.1,reuseaddr,fork")
	// Every relay socat needs the half-close grace: the default folds one direction
	// ending into closing the whole connection half a second later.
	testifyassert.Equal(t, 2, strings.Count(script, "socat -t 120"))
	testifyassert.Contains(t, script, rfwdReadyMarker)
	testifyassert.Contains(t, script, rfwdConnMarker)
	testifyassert.Contains(t, script, rfwdErrMarker)
	// The listener must be cleaned up when the exec stream goes away, by all three
	// routes: the trap, the stdin watcher, and the watcher that outlives a SIGKILL.
	testifyassert.Contains(t, script, `trap 'kill "$SPID" "$WPID" 2>/dev/null || true; rm -rf "$D"' EXIT INT TERM`)
	testifyassert.Contains(t, script, `( cat <&3 >/dev/null 2>&1; kill "$SPID" 2>/dev/null )`)
	testifyassert.Contains(t, script, `while [ -e "/proc/$MPID" ] && [ -e "/proc/$SPID" ]`)
	// Both cleanup routes kill processes that have usually exited already, and the
	// script runs under set -e: without the || true that failed kill would end the
	// route before it removes the rendezvous directory.
	testifyassert.Equal(t, 2, strings.Count(script, `kill "$SPID" "$WPID" 2>/dev/null || true`))

	// Both scripts hand their own stdin to a background socat: a shell would
	// otherwise give it /dev/null, and the relay would carry an instant EOF.
	testifyassert.Contains(t, script, "socat -t 120 - UNIX-LISTEN:\"$S\" <&3 &")
	testifyassert.Equal(t, 2, strings.Count(script, "exec 3<&0"))

	// socat splits address strings on commas, so a comma anywhere in the SYSTEM:
	// command truncates it into an unparsable address.
	start := strings.Index(script, "SYSTEM:'")
	testifyassert.NotEqual(t, -1, start)
	command := script[start+len("SYSTEM:'"):]
	command = command[:strings.Index(command, "'")]
	testifyassert.NotContains(t, command, ",")
}

func TestConnectScript(t *testing.T) {
	testifyassert.Equal(t,
		"exec socat -t 120 - UNIX-CONNECT:/tmp/.safe-rfwd-abcd/4242/s,retry=100,interval=0.1",
		connectScript("/tmp/.safe-rfwd-abcd", "4242"))
}

// --- fakes ----------------------------------------------------------------

// fakePodListener stands in for a socat relay running inside a Pod.
type fakePodListener struct {
	conns      chan podConn
	closed     chan struct{}
	closeOnce  sync.Once
	closeCalls int32
}

func newFakePodListener() *fakePodListener {
	return &fakePodListener{conns: make(chan podConn, 4), closed: make(chan struct{})}
}

func (f *fakePodListener) Accept(ctx context.Context) (podConn, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.closed:
		return nil, io.EOF
	case c := <-f.conns:
		return c, nil
	}
}

func (f *fakePodListener) Close() error {
	atomic.AddInt32(&f.closeCalls, 1)
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

// closes reports how many times the forward was torn down. Anything but one means
// the audit trail gained a close line the "established" line has no room for.
func (f *fakePodListener) closes() int32 { return atomic.LoadInt32(&f.closeCalls) }

func (f *fakePodListener) isClosed() bool {
	select {
	case <-f.closed:
		return true
	default:
		return false
	}
}

// push queues a connection and returns the Pod-side end of it.
func (f *fakePodListener) push() net.Conn {
	podSide, appSide := net.Pipe()
	f.conns <- &fakePodConn{Conn: podSide}
	return appSide
}

type fakePodConn struct{ net.Conn }

func (c *fakePodConn) CloseWrite() error  { return nil }
func (c *fakePodConn) OriginAddr() string { return "127.0.0.1" }
func (c *fakePodConn) OriginPort() uint32 { return 51234 }

// --- end-to-end over a real SSH connection --------------------------------

// forwardTestRig is a real SSH client and server talking over an in-memory pipe,
// with the Pod side replaced by a fake listener.
type forwardTestRig struct {
	client     *ssh.Client
	manager    *reverseForwardManager
	listeners  chan *fakePodListener
	factoryErr error
	cancel     context.CancelFunc
	closeConn  func()
}

func newForwardTestRig(t *testing.T) *forwardTestRig {
	t.Helper()
	return newForwardTestRigWith(t, nil)
}

// newForwardTestRigWith builds the rig, letting a caller replace the Pod side before
// the manager starts answering requests.
func newForwardTestRigWith(t *testing.T, configure func(*reverseForwardManager)) *forwardTestRig {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	testifyassert.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	testifyassert.NoError(t, err)

	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)

	// A real loopback socket, not net.Pipe: the SSH version exchange writes on
	// both sides before reading, which deadlocks on an unbuffered pipe.
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	testifyassert.NoError(t, err)
	t.Cleanup(func() { _ = tcpListener.Close() })

	ctx, cancel := context.WithCancel(context.Background())

	rig := &forwardTestRig{
		listeners: make(chan *fakePodListener, 4),
		cancel:    cancel,
	}

	managerCh := make(chan *reverseForwardManager, 1)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		serverPipe, acceptErr := tcpListener.Accept()
		if acceptErr != nil {
			managerCh <- nil
			return
		}
		sshConn, chans, reqs, err := ssh.NewServerConn(serverPipe, serverConfig)
		if err != nil {
			managerCh <- nil
			return
		}
		defer sshConn.Close()

		m := newReverseForwardManager(ctx, nil, sshConn)
		m.resolve = func(context.Context, *UserInfo) (*commonclient.ClientFactory, error) { return nil, nil }
		m.newListener = func(context.Context, *UserInfo, *commonclient.ClientFactory,
			string, uint32) (podListener, error) {
			if rig.factoryErr != nil {
				return nil, rig.factoryErr
			}
			l := newFakePodListener()
			rig.listeners <- l
			return l, nil
		}
		if configure != nil {
			configure(m)
		}
		defer m.closeAll()
		managerCh <- m

		go func() {
			for ch := range chans {
				_ = ch.Reject(ssh.UnknownChannelType, "not used in this test")
			}
		}()
		m.handleGlobalRequests(reqs)
	}()

	clientPipe, err := net.Dial("tcp", tcpListener.Addr().String())
	testifyassert.NoError(t, err)

	clientConn, clientChans, clientReqs, err := ssh.NewClientConn(clientPipe, "pipe", &ssh.ClientConfig{
		User:            testForwardUser,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	testifyassert.NoError(t, err)

	rig.client = ssh.NewClient(clientConn, clientChans, clientReqs)
	rig.manager = <-managerCh
	testifyassert.NotNil(t, rig.manager)
	rig.closeConn = func() {
		_ = rig.client.Close()
		<-serverDone
	}

	t.Cleanup(func() {
		_ = rig.client.Close()
		cancel()
		<-serverDone
	})
	return rig
}

// TestReverseForwardEndToEnd drives a real `ssh -R` from the client library:
// tcpip-forward is accepted, a Pod-side connection becomes a forwarded-tcpip
// channel, and bytes flow both ways.
func TestReverseForwardEndToEnd(t *testing.T) {
	enableReverseForward(t, nil)
	rig := newForwardTestRig(t)

	listener, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.NoError(t, err)

	podListener := <-rig.listeners
	appSide := podListener.push()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			close(accepted)
			return
		}
		accepted <- conn
	}()

	// Pod -> client.
	go func() {
		_, _ = appSide.Write([]byte("from-pod"))
	}()

	var clientSide net.Conn
	select {
	case clientSide = <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for forwarded-tcpip channel")
	}
	testifyassert.NotNil(t, clientSide)

	// The channel must carry the address the client asked us to listen on, so the
	// client library can route it to the right listener - which it just did.
	testifyassert.Equal(t, "127.0.0.1:10001", clientSide.LocalAddr().String())
	testifyassert.Equal(t, "127.0.0.1:51234", clientSide.RemoteAddr().String())

	buf := make([]byte, len("from-pod"))
	_ = clientSide.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = io.ReadFull(clientSide, buf)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "from-pod", string(buf))

	// Client -> Pod.
	_, err = clientSide.Write([]byte("from-client"))
	testifyassert.NoError(t, err)
	back := make([]byte, len("from-client"))
	_ = appSide.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = io.ReadFull(appSide, back)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "from-client", string(back))

	_ = clientSide.Close()
	_ = appSide.Close()
}

// TestReverseForwardCancelClosesPodListener verifies cancel-tcpip-forward tears the
// Pod-side listener down and frees the port for a new request.
func TestReverseForwardCancelClosesPodListener(t *testing.T) {
	enableReverseForward(t, nil)
	rig := newForwardTestRig(t)

	listener, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.NoError(t, err)
	first := <-rig.listeners

	testifyassert.NoError(t, listener.Close())
	waitFor(t, func() bool { return first.isClosed() }, "pod listener closed after cancel")

	// The same port can be requested again once it has been released.
	listener, err = rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.NoError(t, err)
	second := <-rig.listeners
	testifyassert.NotEqual(t, first, second)
	_ = listener.Close()
}

// TestReverseForwardSessionCloseClosesPodListener verifies a dropped SSH connection
// removes the Pod-side listener.
func TestReverseForwardSessionCloseClosesPodListener(t *testing.T) {
	enableReverseForward(t, nil)
	rig := newForwardTestRig(t)

	_, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.NoError(t, err)
	podListener := <-rig.listeners
	testifyassert.False(t, podListener.isClosed())

	rig.closeConn()
	testifyassert.True(t, podListener.isClosed())
}

// TestReverseForwardRejectedWhenDisabled verifies a deployment that turns the
// feature off refuses `-R`, exactly as the gateway did before it existed.
func TestReverseForwardRejectedWhenDisabled(t *testing.T) {
	previous := viper.Get(sshReverseForwardEnableKey)
	viper.Set(sshReverseForwardEnableKey, false)
	t.Cleanup(func() { viper.Set(sshReverseForwardEnableKey, previous) })

	rig := newForwardTestRig(t)
	_, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.Error(t, err)
}

// TestReverseForwardRejectsPortOutsideRange verifies the port policy is enforced on
// the wire, not just in validate().
func TestReverseForwardRejectsPortOutsideRange(t *testing.T) {
	enableReverseForward(t, nil)
	rig := newForwardTestRig(t)
	_, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22})
	testifyassert.Error(t, err)
	testifyassert.Empty(t, rig.listeners)
}

// TestReverseForwardRejectsDuplicatePort verifies one session cannot bind the same
// Pod port twice.
func TestReverseForwardRejectsDuplicatePort(t *testing.T) {
	enableReverseForward(t, nil)
	rig := newForwardTestRig(t)

	_, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.NoError(t, err)
	<-rig.listeners

	_, err = rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.Error(t, err)
}

// TestReverseForwardEnforcesSessionLimit verifies max_forwards_per_session.
func TestReverseForwardEnforcesSessionLimit(t *testing.T) {
	enableReverseForward(t, map[string]any{sshReverseForwardMaxPerSess: 1})
	rig := newForwardTestRig(t)

	_, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.NoError(t, err)
	<-rig.listeners

	_, err = rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10002})
	testifyassert.Error(t, err)
	testifyassert.Empty(t, rig.listeners)
}

// TestReverseForwardListenerStartFailureReleasesPort verifies a failed listener does
// not leak its reservation.
func TestReverseForwardListenerStartFailureReleasesPort(t *testing.T) {
	enableReverseForward(t, nil)
	rig := newForwardTestRig(t)

	rig.factoryErr = errSSH("socat is not installed in the container")
	_, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.Error(t, err)

	rig.factoryErr = nil
	_, err = rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.NoError(t, err)
	<-rig.listeners
}

// TestReverseForwardUnknownGlobalRequestIsRefused verifies keepalives and unknown
// global requests still get a failure reply rather than hanging the client.
func TestReverseForwardUnknownGlobalRequestIsRefused(t *testing.T) {
	rig := newForwardTestRig(t)
	ok, _, err := rig.client.SendRequest("keepalive@openssh.com", true, nil)
	testifyassert.NoError(t, err)
	testifyassert.False(t, ok)
}

// TestReverseForwardCancelUnknownPort verifies cancelling a forward we never made
// is answered with a failure instead of panicking.
func TestReverseForwardCancelUnknownPort(t *testing.T) {
	enableReverseForward(t, nil)
	rig := newForwardTestRig(t)

	payload := ssh.Marshal(tcpipForwardPayload{BindAddr: "127.0.0.1", BindPort: 10001})
	ok, _, err := rig.client.SendRequest(cancelTCPIPForwardRequest, true, payload)
	testifyassert.NoError(t, err)
	testifyassert.False(t, ok)
}

// TestReverseForwardMalformedPayload verifies a malformed request is refused.
func TestReverseForwardMalformedPayload(t *testing.T) {
	enableReverseForward(t, nil)
	rig := newForwardTestRig(t)

	ok, _, err := rig.client.SendRequest(tcpipForwardRequest, true, []byte{0x00, 0x01})
	testifyassert.NoError(t, err)
	testifyassert.False(t, ok)
}

// TestReverseForwardInvalidUser verifies a username that does not name a Pod is
// refused before any Pod-side work happens.
func TestReverseForwardInvalidUser(t *testing.T) {
	enableReverseForward(t, nil)

	m := &reverseForwardManager{
		policy:   loadReverseForwardPolicy(),
		conn:     &ssh.ServerConn{Conn: fakeSSHConn{user: "invalid-format"}},
		ctx:      context.Background(),
		forwards: map[string]*reverseForward{},
		resolve: func(context.Context, *UserInfo) (*commonclient.ClientFactory, error) {
			t.Fatal("resolve must not be reached for an invalid user")
			return nil, nil
		},
		newListener: func(context.Context, *UserInfo, *commonclient.ClientFactory,
			string, uint32) (podListener, error) {
			t.Fatal("listener must not be created for an invalid user")
			return nil, nil
		},
	}
	m.handleForward(&ssh.Request{
		Type:    tcpipForwardRequest,
		Payload: ssh.Marshal(tcpipForwardPayload{BindAddr: "127.0.0.1", BindPort: 10001}),
	})
	testifyassert.Empty(t, m.forwards)
}

// fakeSSHConn is a minimal ssh.Conn exposing only the login user.
type fakeSSHConn struct {
	ssh.Conn
	user string
}

func (f fakeSSHConn) User() string { return f.user }

// waitFor polls until cond holds or the test times out.
func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// --- whole stack, real relay scripts ---------------------------------------

// localRelayExec runs a relay script as a local process. It stands in for the
// Kubernetes exec transport so the scripts, socat and the shell that the apiserver
// actually depends on are exercised end to end.
type localRelayExec struct{ script string }

func (e *localRelayExec) Stream(opts remotecommand.StreamOptions) error {
	return e.StreamWithContext(context.Background(), opts)
}

func (e *localRelayExec) StreamWithContext(ctx context.Context, opts remotecommand.StreamOptions) error {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", e.script)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = opts.Stdin, opts.Stdout, opts.Stderr
	// The relay is killed by tearing its stream down, so it must not outlive ctx.
	cmd.WaitDelay = time.Second
	return cmd.Run()
}

// TestReverseForwardThroughRealRelay drives the whole feature: a real SSH client
// asks for `-R`, the real relay scripts open the listen socket, and an HTTP request
// made against that socket is served by the client side of the SSH connection.
// Only the Kubernetes exec transport is replaced.
func TestReverseForwardThroughRealRelay(t *testing.T) {
	if _, err := exec.LookPath("socat"); err != nil {
		t.Skip("socat is not installed")
	}

	// The policy is a port range, so pin it to one port that is free right now.
	port := freeTCPPort(t)
	enableReverseForward(t, map[string]any{
		sshReverseForwardPortMinKey: int(port),
		sshReverseForwardPortMaxKey: int(port),
	})

	previous := newPodExecutor
	newPodExecutor = func(_ *execPodListener, script string, _ bool) (remotecommand.Executor, error) {
		return &localRelayExec{script: script}, nil
	}
	t.Cleanup(func() { newPodExecutor = previous })

	rig := newForwardTestRigWith(t, func(m *reverseForwardManager) {
		m.resolve = func(context.Context, *UserInfo) (*commonclient.ClientFactory, error) { return nil, nil }
		m.newListener = newExecPodListener
	})

	// The client end of `-R`: whatever reaches the forward is served from here, the
	// way a proxy on the developer's machine would be.
	listener, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: int(port)})
	testifyassert.NoError(t, err)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "served from the developer's machine: "+r.URL.Path)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	// The pod end: a process inside the pod talks to the loopback port the relay
	// opened, exactly as `curl -x` or `git` would.
	client := &http.Client{Timeout: 30 * time.Second}
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

	// Dropping the SSH connection must take the pod-side listen socket with it.
	rig.closeConn()
	waitForPortFree(t, port)
}

// TestReverseForwardSessionClosingDuringSetupClosesListener covers the race where the
// SSH connection ends while the Pod-side listener is still starting: the listener is
// already running by then, so failing to close it would leak a socat in the Pod.
func TestReverseForwardSessionClosingDuringSetupClosesListener(t *testing.T) {
	enableReverseForward(t, nil)

	listener := newFakePodListener()
	m := &reverseForwardManager{
		policy:   loadReverseForwardPolicy(),
		conn:     &ssh.ServerConn{Conn: fakeSSHConn{user: testForwardUser}},
		ctx:      context.Background(),
		forwards: map[string]*reverseForward{},
		resolve: func(context.Context, *UserInfo) (*commonclient.ClientFactory, error) {
			return nil, nil
		},
	}
	m.newListener = func(context.Context, *UserInfo, *commonclient.ClientFactory,
		string, uint32) (podListener, error) {
		// The session ends while the listener is coming up.
		m.mu.Lock()
		m.closed = true
		m.mu.Unlock()
		return listener, nil
	}

	m.handleForward(&ssh.Request{
		Type:    tcpipForwardRequest,
		Payload: ssh.Marshal(tcpipForwardPayload{BindAddr: "127.0.0.1", BindPort: 10001}),
	})

	testifyassert.True(t, listener.isClosed(), "the pod-side listener must not be left running")
	testifyassert.Empty(t, m.forwards, "the reservation must not be left behind")
}

// TestReverseForwardClosesExactlyOnce pins the property the audit trail depends on:
// one "established" line has one matching close. Closing a forward is also what ends
// its accept loop, so every teardown reaches forget() on the way out - and if forget
// closed unconditionally, a cancelled or session-ended forward would be counted and
// logged twice.
func TestReverseForwardClosesExactlyOnce(t *testing.T) {
	t.Run("ssh session ended", func(t *testing.T) {
		enableReverseForward(t, nil)
		rig := newForwardTestRig(t)

		_, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
		testifyassert.NoError(t, err)
		podListener := <-rig.listeners

		// closeConn returns only once the manager's closeAll has waited on the
		// accept loop, so any second teardown has already happened by now.
		rig.closeConn()
		testifyassert.Equal(t, int32(1), podListener.closes())
	})

	t.Run("cancelled by client", func(t *testing.T) {
		enableReverseForward(t, nil)
		rig := newForwardTestRig(t)

		listener, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
		testifyassert.NoError(t, err)
		podListener := <-rig.listeners

		testifyassert.NoError(t, listener.Close())
		waitFor(t, func() bool { return podListener.isClosed() }, "the forward to be cancelled")
		// Give the accept loop room to reach forget() before counting.
		rig.closeConn()
		testifyassert.Equal(t, int32(1), podListener.closes())
	})
}

// halfClosedPodConn is a pod-side connection that stops producing on demand while
// still accepting what comes back, so a half-close can be driven from a test.
type halfClosedPodConn struct {
	readEOF     chan struct{}
	toRead      chan []byte
	writeClosed bool
	mu          sync.Mutex
	written     []byte
	got         chan struct{}
	closed      bool
}

func newHalfClosedPodConn() *halfClosedPodConn {
	return &halfClosedPodConn{
		readEOF: make(chan struct{}),
		toRead:  make(chan []byte, 8),
		got:     make(chan struct{}, 8),
	}
}

func (c *halfClosedPodConn) Read(p []byte) (int, error) {
	// Hand over everything queued before reporting the end. Both cases can be ready
	// at once, and select would then pick between them at random, losing a reply
	// that was queued just before the half-close.
	select {
	case chunk := <-c.toRead:
		return copy(p, chunk), nil
	default:
	}
	select {
	case chunk := <-c.toRead:
		return copy(p, chunk), nil
	case <-c.readEOF:
		return 0, io.EOF
	}
}

// deliver queues bytes for the bridge to carry to the client.
func (c *halfClosedPodConn) deliver(s string) { c.toRead <- []byte(s) }

// CloseWrite records that the client finished sending, without ending the read side.
func (c *halfClosedPodConn) CloseWrite() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeClosed = true
	return nil
}

func (c *halfClosedPodConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, io.ErrClosedPipe
	}
	c.written = append(c.written, p...)
	select {
	case c.got <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (c *halfClosedPodConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func (c *halfClosedPodConn) OriginAddr() string { return "127.0.0.1" }
func (c *halfClosedPodConn) OriginPort() uint32 { return 51234 }

func (c *halfClosedPodConn) receivedAll() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.written)
}

// TestReverseForwardHalfCloseKeepsReplyFlowing pins that the pod finishing its
// request does not tear the pair down. A client that half-closes after sending -
// ordinary for a request/response protocol - must still receive the reply, and the
// only signal the pod is done writing is a half-close on the channel.
func TestReverseForwardHalfCloseKeepsReplyFlowing(t *testing.T) {
	enableReverseForward(t, nil)
	rig := newForwardTestRig(t)

	listener, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.NoError(t, err)
	podListener := <-rig.listeners

	pc := newHalfClosedPodConn()
	podListener.conns <- pc

	clientSide, err := listener.Accept()
	testifyassert.NoError(t, err)
	defer clientSide.Close()

	// The pod finishes sending; the client should see EOF, not a dead connection.
	close(pc.readEOF)
	_ = clientSide.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1)
	_, err = clientSide.Read(buf)
	testifyassert.ErrorIs(t, err, io.EOF, "the pod's half-close should reach the client as EOF")

	// Now the reply comes back the other way. This is what a proxy on the developer's
	// machine does after reading the whole request.
	_, err = clientSide.Write([]byte("reply-after-half-close"))
	testifyassert.NoError(t, err)

	select {
	case <-pc.got:
	case <-time.After(5 * time.Second):
		t.Fatal("the reply never reached the pod: the half-close tore the pair down")
	}
	testifyassert.Equal(t, "reply-after-half-close", pc.receivedAll())
}

// TestActivateRegistersServeBeforeReleasingLock pins that a forward's accept loop is
// registered with the WaitGroup under the same lock closeAll takes. Registering it
// afterwards lets closeAll's Wait return first, which both lets the goroutine escape
// teardown and is the documented way to panic a WaitGroup.
func TestActivateRegistersServeBeforeReleasingLock(t *testing.T) {
	enableReverseForward(t, nil)

	fake := newFakePodListener()
	m := &reverseForwardManager{
		conn:     &ssh.ServerConn{Conn: fakeSSHConn{user: testForwardUser}},
		ctx:      context.Background(),
		policy:   loadReverseForwardPolicy(),
		forwards: map[string]*reverseForward{},
	}
	fwd := &reverseForward{
		bindAddr:  "127.0.0.1",
		bindPort:  10001,
		listener:  fake,
		cancel:    func() {},
		userInfo:  &UserInfo{User: "root", Namespace: "ns", Pod: "pod-0"},
		startedAt: time.Now(),
	}
	testifyassert.True(t, m.activate(forwardKey("127.0.0.1", 10001), fwd))

	closed := make(chan struct{})
	go func() { m.closeAll(); close(closed) }()

	select {
	case <-closed:
		t.Fatal("closeAll returned while the accept loop was still outstanding")
	case <-time.After(200 * time.Millisecond):
	}

	// Standing in for the accept loop finishing.
	m.wg.Done()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("closeAll never returned")
	}
}

// TestReverseForwardPolicyRejectsIPv6 covers a bind address that passes as an IP but
// cannot be bound by the pod-side listener, which builds an IPv4 socket. Accepting it
// here turns a config mistake into a failure inside the pod.
func TestReverseForwardPolicyRejectsIPv6(t *testing.T) {
	policy := reverseForwardPolicy{
		enabled:       true,
		bindAddresses: []string{"::1"},
		portMin:       1024,
		portMax:       65535,
	}
	_, err := policy.validate("::1", 10001)
	testifyassert.Error(t, err)
	testifyassert.Contains(t, err.Error(), "IPv4")
}

// TestLoadReverseForwardPolicyRejectsUnusablePorts pins that a misconfigured range
// falls back instead of wrapping. A negative port_min became 4294967295 as a uint32,
// which refused every forward and reported a range nobody had configured.
func TestLoadReverseForwardPolicyRejectsUnusablePorts(t *testing.T) {
	for _, tc := range []struct {
		name             string
		min, max         int
		wantMin, wantMax uint32
	}{
		{name: "negative min", min: -1, max: 65535, wantMin: 1024, wantMax: 65535},
		{name: "min above a port", min: 70000, max: 65535, wantMin: 1024, wantMax: 65535},
		{name: "max above a port", min: 1024, max: 70000, wantMin: 1024, wantMax: 65535},
		{name: "inverted range", min: 40000, max: 30000, wantMin: 1024, wantMax: 65535},
		{name: "usable range kept", min: 20000, max: 20010, wantMin: 20000, wantMax: 20010},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enableReverseForward(t, map[string]any{
				sshReverseForwardPortMinKey: tc.min,
				sshReverseForwardPortMaxKey: tc.max,
			})
			policy := loadReverseForwardPolicy()
			testifyassert.Equal(t, tc.wantMin, policy.portMin)
			testifyassert.Equal(t, tc.wantMax, policy.portMax)
		})
	}
}

// TestReverseForwardEchoesRequestedBindAddr pins that the forwarded-tcpip channel
// names the bind address the client asked for, not the address we resolved it to.
// OpenSSH compares that string against the one it sent with strcmp, so answering
// "localhost" with "127.0.0.1" makes it refuse its own forward with
// "administratively prohibited" and no data ever reaches the client.
func TestReverseForwardEchoesRequestedBindAddr(t *testing.T) {
	for _, tc := range []struct {
		name, requested, want string
		binds                 []string
	}{
		{name: "localhost", requested: "localhost", want: "localhost"},
		{name: "literal ip", requested: "127.0.0.1", want: "127.0.0.1"},
		// OpenSSH folds a wildcard to the empty string in its own copy before
		// comparing, so both spellings have to come back as the empty string.
		{name: "star", requested: "*", want: "", binds: []string{"0.0.0.0"}},
		{name: "empty", requested: "", want: "", binds: []string{"0.0.0.0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]any{}
			if tc.binds != nil {
				values[sshReverseForwardBindAddrKey] = tc.binds
			}
			enableReverseForward(t, values)
			rig := newForwardTestRig(t)

			// Take the channel opens directly rather than through ListenTCP, so the
			// payload can be inspected as the client library would see it.
			channels := rig.client.HandleChannelOpen(forwardedTCPIPChannel)

			ok, _, err := rig.client.SendRequest(tcpipForwardRequest, true,
				ssh.Marshal(tcpipForwardPayload{BindAddr: tc.requested, BindPort: 10001}))
			testifyassert.NoError(t, err)
			testifyassert.True(t, ok, "the forward should be accepted")

			podListener := <-rig.listeners
			podListener.push()

			select {
			case newCh := <-channels:
				var data forwardChannelData
				testifyassert.NoError(t, ssh.Unmarshal(newCh.ExtraData(), &data))
				testifyassert.Equal(t, tc.want, data.DestAddr)
				testifyassert.Equal(t, uint32(10001), data.DestPort)
				_ = newCh.Reject(ssh.Prohibited, "inspected")
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for the forwarded-tcpip channel")
			}
		})
	}
}

// --- the authorization a forward request actually goes through ---------------

// newResolveTestHandler builds a handler whose workspace, pod and workload resolve,
// with only adminUser permitted to reach the workload.
func newResolveTestHandler(t *testing.T, adminUser string) *SshHandler {
	t.Helper()

	scheme := runtime.NewScheme()
	testifyassert.NoError(t, v1.AddToScheme(scheme))

	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ns"},
		Spec:       v1.WorkspaceSpec{Cluster: "c1"},
	}
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-1"},
		Spec:       v1.WorkloadSpec{Workspace: "ns"},
	}
	admin := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: adminUser},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
	}
	adminRole := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.SystemAdminRole)},
		Rules: []v1.PolicyRule{{
			Resources:    []string{authority.AllResource},
			Verbs:        []v1.RoleVerb{v1.AllVerb},
			GrantedUsers: []string{authority.GrantedAllUser},
		}},
	}
	ctrlClient := ctrlfake.NewClientBuilder().WithScheme(scheme).
		WithObjects(workspace, workload, admin, adminRole).Build()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "pod-0",
		Namespace: "ns",
		Labels:    map[string]string{v1.WorkloadIdLabel: "wl-1"},
	}}
	clientManager := commonutils.NewObjectManagerSingleton()
	clientManager.Add("c1", commonclient.NewClientFactoryWithOnlyClient(
		context.Background(), "c1", k8sfake.NewSimpleClientset(pod)))

	return &SshHandler{
		Client:           ctrlClient,
		clientManager:    clientManager,
		accessController: authority.NewAccessController(ctrlClient),
	}
}

// TestResolveForwardTargetAuthorizes covers the check that stands between a
// tcpip-forward request and someone else's pod. Every other test in this file
// replaces it, so without this the guard is only ever exercised by a stub.
func TestResolveForwardTargetAuthorizes(t *testing.T) {
	h := newResolveTestHandler(t, "admin")

	clients, err := h.resolveForwardTarget(context.Background(),
		&UserInfo{User: "admin", Namespace: "ns", Pod: "pod-0", Container: "main"})
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, clients)

	// A user with no claim on the workload cannot open a forward into its pod.
	_, err = h.resolveForwardTarget(context.Background(),
		&UserInfo{User: "someone-else", Namespace: "ns", Pod: "pod-0", Container: "main"})
	testifyassert.Error(t, err)

	// A pod outside any workspace we know about resolves to nothing at all.
	_, err = h.resolveForwardTarget(context.Background(),
		&UserInfo{User: "admin", Namespace: "other-ns", Pod: "pod-0", Container: "main"})
	testifyassert.Error(t, err)
}

// TestReverseForwardRejectsUnauthorizedUser drives the same guard over a real SSH
// connection: the client asks for a forward and is refused before any pod-side work.
func TestReverseForwardRejectsUnauthorizedUser(t *testing.T) {
	enableReverseForward(t, nil)

	h := newResolveTestHandler(t, "admin")
	rig := newForwardTestRigWith(t, func(m *reverseForwardManager) {
		m.resolve = h.resolveForwardTarget
	})

	// testForwardUser logs in as "root", which owns nothing here.
	_, err := rig.client.ListenTCP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001})
	testifyassert.Error(t, err)
	testifyassert.Empty(t, rig.listeners)
}

// TestReverseForwardBoundsAuthorizationWait pins that a target cluster which stops
// answering cannot hold the connection's request loop. Global requests are answered
// in order, so an unbounded wait here stalls keepalives and everything behind it -
// the session's context runs for twelve hours, which is not a bound worth having.
func TestReverseForwardBoundsAuthorizationWait(t *testing.T) {
	enableReverseForward(t, nil)

	previous := forwardResolveTimeout
	forwardResolveTimeout = 150 * time.Millisecond
	t.Cleanup(func() { forwardResolveTimeout = previous })

	resolving := make(chan struct{}, 1)
	m := &reverseForwardManager{
		conn:     &ssh.ServerConn{Conn: fakeSSHConn{user: testForwardUser}},
		ctx:      context.Background(),
		policy:   loadReverseForwardPolicy(),
		forwards: map[string]*reverseForward{},
		resolve: func(ctx context.Context, _ *UserInfo) (*commonclient.ClientFactory, error) {
			resolving <- struct{}{}
			<-ctx.Done() // a cluster that never answers
			return nil, ctx.Err()
		},
		newListener: func(context.Context, *UserInfo, *commonclient.ClientFactory,
			string, uint32) (podListener, error) {
			t.Error("no listener should be started when authorization never answered")
			return nil, nil
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.handleForward(&ssh.Request{
			Type:    tcpipForwardRequest,
			Payload: ssh.Marshal(tcpipForwardPayload{BindAddr: "127.0.0.1", BindPort: 10001}),
		})
	}()

	<-resolving
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleForward never gave up on the unreachable cluster")
	}
	testifyassert.Empty(t, m.forwards, "a request that timed out must not hold a reservation")
}

// slowClosePodListener stands in for a relay that takes its time letting go of the
// pod-side port, which is what Close now waits for.
type slowClosePodListener struct {
	*fakePodListener
	delay time.Duration
}

func (l *slowClosePodListener) Close() error {
	time.Sleep(l.delay)
	return l.fakePodListener.Close()
}

// TestCloseAllClosesForwardsTogether pins that a session's teardown costs one relay
// shutdown, not one per forward. A session may hold eight, and closing each in turn
// would multiply the wait by eight before the connection goroutine can finish.
func TestCloseAllClosesForwardsTogether(t *testing.T) {
	const delay = 200 * time.Millisecond

	m := &reverseForwardManager{
		conn:     &ssh.ServerConn{Conn: fakeSSHConn{user: testForwardUser}},
		ctx:      context.Background(),
		forwards: map[string]*reverseForward{},
	}
	for port := uint32(10001); port <= 10004; port++ {
		fwd := &reverseForward{
			bindAddr:  "127.0.0.1",
			bindPort:  port,
			listener:  &slowClosePodListener{fakePodListener: newFakePodListener(), delay: delay},
			cancel:    func() {},
			userInfo:  &UserInfo{User: "root", Namespace: "ns", Pod: "pod-0"},
			startedAt: time.Now(),
		}
		testifyassert.True(t, m.activate(forwardKey("127.0.0.1", port), fwd))
		m.wg.Done() // no accept loop in this test
	}

	start := time.Now()
	m.closeAll()
	testifyassert.Less(t, time.Since(start), 3*delay,
		"four forwards were closed one after another instead of together")
}

// ctxWatchingListener records whether the forward's context had already been
// cancelled by the time the listener was asked to close.
type ctxWatchingListener struct {
	*fakePodListener
	ctx                  context.Context
	cancelledBeforeClose bool
}

func (l *ctxWatchingListener) Close() error {
	l.cancelledBeforeClose = l.ctx.Err() != nil
	return l.fakePodListener.Close()
}

// TestCloseForwardClosesTheListenerBeforeCancelling pins the order teardown depends
// on. The forward's context is the parent of the listener's exec stream, so
// cancelling first tears that stream down where it stands: Close then has no stdin
// left to end and nothing to wait for, and the pod keeps the port a moment longer -
// exactly the reconnect failure the wait was added to prevent. Closing the listener
// directly, as an earlier test did, cannot see this.
func TestCloseForwardClosesTheListenerBeforeCancelling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	listener := &ctxWatchingListener{fakePodListener: newFakePodListener(), ctx: ctx}
	fwd := &reverseForward{
		bindAddr:  "127.0.0.1",
		bindPort:  10001,
		listener:  listener,
		ctx:       ctx,
		cancel:    cancel,
		userInfo:  &UserInfo{User: "root", Namespace: "ns", Pod: "pod-0"},
		startedAt: time.Now(),
	}

	closeForward(fwd, "ssh session ended")

	testifyassert.False(t, listener.cancelledBeforeClose,
		"the exec stream was cancelled before the listener could shut the relay down")
	testifyassert.True(t, listener.isClosed())
	testifyassert.Error(t, ctx.Err(), "the forward's context must still end up cancelled")
}

// TestReverseForwardHalfCloseFromTheClientKeepsPodDataFlowing is the mirror of the
// pod-side half-close: the machine at the other end finishes writing and waits for
// the rest of the pod's data. A proxy does exactly this once it has sent a whole
// response, so tearing the pair down on the client's EOF truncates the pod's side of
// the exchange.
func TestReverseForwardHalfCloseFromTheClientKeepsPodDataFlowing(t *testing.T) {
	enableReverseForward(t, nil)
	rig := newForwardTestRig(t)

	// Take the channel directly: the net.Conn the client library hands back does
	// not expose the half-close this test has to perform.
	channels := rig.client.HandleChannelOpen(forwardedTCPIPChannel)

	ok, _, err := rig.client.SendRequest(tcpipForwardRequest, true,
		ssh.Marshal(tcpipForwardPayload{BindAddr: "127.0.0.1", BindPort: 10001}))
	testifyassert.NoError(t, err)
	testifyassert.True(t, ok)

	podListener := <-rig.listeners
	pc := newHalfClosedPodConn()
	podListener.conns <- pc

	newCh := <-channels
	ch, reqs, err := newCh.Accept()
	testifyassert.NoError(t, err)
	go ssh.DiscardRequests(reqs)

	// The client is done sending and waits for the pod.
	testifyassert.NoError(t, ch.CloseWrite())

	// The pod answers afterwards; it must still get through.
	pc.deliver("late-reply-from-pod")
	close(pc.readEOF)

	read := make(chan string, 1)
	go func() {
		body, _ := io.ReadAll(ch)
		read <- string(body)
	}()
	select {
	case got := <-read:
		testifyassert.Equal(t, "late-reply-from-pod", got,
			"the client's half-close cut the pod off mid-reply")
	case <-time.After(5 * time.Second):
		t.Fatal("the pod's reply never arrived after the client half-closed")
	}
}

// TestReverseForwardRefusesWhenNoBindAddressIsAllowed pins the end of the chain an
// operator pulls when they empty the bind list: no address is permitted, so no
// forward is. Restoring loopback here would hand back the one address they removed.
func TestReverseForwardRefusesWhenNoBindAddressIsAllowed(t *testing.T) {
	enableReverseForward(t, map[string]any{sshReverseForwardBindAddrKey: []string{}})

	policy := loadReverseForwardPolicy()
	testifyassert.Empty(t, policy.bindAddresses)
	for _, addr := range []string{"127.0.0.1", "localhost", "0.0.0.0"} {
		_, err := policy.validate(addr, 10001)
		testifyassert.Errorf(t, err, "%q was allowed although no bind address is configured", addr)
	}
}

// stopObservingListener samples the forward's own account of why it is stopping at
// the instant the accept loop would wake: when the listener is closed.
type stopObservingListener struct {
	*fakePodListener
	fwd               *reverseForward
	unexpectedAtClose bool
}

func (l *stopObservingListener) Close() error {
	l.unexpectedAtClose = l.fwd.unexpectedStop()
	return l.fakePodListener.Close()
}

// TestForwardKnowsWhoStoppedIt pins how the accept loop tells a listener that died
// from one we shut down. It used to read that off the forward's context, which only
// worked while teardown cancelled before closing; now the listener is closed first -
// so the pod lets go of the port before we report it gone - and the context is still
// live at the moment Accept returns. Reading the context there would log every
// ordinary disconnect as a listener failure, burying the real ones.
func TestForwardKnowsWhoStoppedIt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fwd := &reverseForward{
		bindAddr:  "127.0.0.1",
		bindPort:  10001,
		ctx:       ctx,
		cancel:    cancel,
		userInfo:  &UserInfo{User: "root", Namespace: "ns", Pod: "pod-0"},
		startedAt: time.Now(),
	}
	listener := &stopObservingListener{fakePodListener: newFakePodListener(), fwd: fwd}
	fwd.listener = listener

	// A listener that stops while nobody asked it to is worth reporting.
	testifyassert.True(t, fwd.unexpectedStop())

	closeForward(fwd, "ssh session ended")

	testifyassert.False(t, listener.unexpectedAtClose,
		"at the moment the accept loop wakes, our own teardown looked like a listener failure")
	testifyassert.False(t, fwd.unexpectedStop())
}

// TestLoadReverseForwardPolicyRejectsAnUnusableLimit pins what a per-session count
// means at each end. Zero is a decision - no forwards on this deployment - and the
// chart goes out of its way to let it be written, so it must survive the getters;
// only a value that cannot be a count at all falls back.
func TestLoadReverseForwardPolicyRejectsAnUnusableLimit(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  int
		want int
	}{
		// Zero is the one value the chart takes trouble to let an operator write.
		{name: "zero means none", set: 0, want: 0},
		{name: "negative", set: -1, want: 8},
		{name: "usable kept", set: 3, want: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enableReverseForward(t, map[string]any{sshReverseForwardMaxPerSess: tc.set})
			testifyassert.Equal(t, tc.want, loadReverseForwardPolicy().maxForwards)
		})
	}
}

// TestReverseForwardRefusesWhenNoForwardsArePermitted pins the end of the chain an
// operator pulls by setting the per-session count to zero. Silently restoring the
// default here would hand them eight of the thing they asked for none of.
func TestReverseForwardRefusesWhenNoForwardsArePermitted(t *testing.T) {
	enableReverseForward(t, map[string]any{sshReverseForwardMaxPerSess: 0})

	m := &reverseForwardManager{
		conn:     &ssh.ServerConn{Conn: fakeSSHConn{user: testForwardUser}},
		ctx:      context.Background(),
		policy:   loadReverseForwardPolicy(),
		forwards: map[string]*reverseForward{},
	}
	testifyassert.Equal(t, 0, m.policy.maxForwards)
	err := m.reserve(forwardKey("127.0.0.1", 10001))
	testifyassert.Error(t, err)
	testifyassert.Contains(t, err.Error(), "no forwards")
	testifyassert.Empty(t, m.forwards)
}
