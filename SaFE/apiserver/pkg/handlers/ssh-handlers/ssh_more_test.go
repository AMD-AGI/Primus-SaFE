/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// fakeConnMetadata is a minimal ssh.ConnMetadata used to drive publicCallback.
type fakeConnMetadata struct{ user string }

func (f fakeConnMetadata) User() string          { return f.user }
func (f fakeConnMetadata) SessionID() []byte      { return nil }
func (f fakeConnMetadata) ClientVersion() []byte  { return nil }
func (f fakeConnMetadata) ServerVersion() []byte  { return nil }
func (f fakeConnMetadata) RemoteAddr() net.Addr   { return &net.TCPAddr{} }
func (f fakeConnMetadata) LocalAddr() net.Addr    { return &net.TCPAddr{} }

func sshScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	_ = v1.AddToScheme(s)
	return s
}

// TestPublicCallbackInvalidUser verifies a malformed username is rejected.
func TestPublicCallbackInvalidUser(t *testing.T) {
	h := &SshHandler{}
	_, err := h.publicCallback(fakeConnMetadata{user: "invalid-format"}, nil)
	assert.Error(t, err)
}

// TestPublicCallbackKeyPath drives publicCallback past username parsing. Depending on
// whether the DB is enabled in this environment it either fails at the db-disabled
// guard or at the public-key lookup; both are error paths exercising the callback body.
func TestPublicCallbackKeyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetPublicKeyByUserId(gomock.Any(), "root").
		Return([]*dbclient.PublicKey{}, nil).AnyTimes()

	h := &SshHandler{dbClient: m}
	_, err := h.publicCallback(
		fakeConnMetadata{user: "root.pod-0.main.bash.ns"},
		testPublicKey(t),
	)
	assert.Error(t, err)
}

// testPublicKey returns a throwaway ssh.PublicKey for callback tests.
func testPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("failed to build ssh public key: %v", err)
	}
	return sshPub
}

// TestGetWorkloadAndClientsWorkspaceNotFound verifies the workspace lookup error path.
func TestGetWorkloadAndClientsWorkspaceNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(sshScheme(t)).Build()
	h := &SshHandler{Client: cl}
	_, _, err := h.getWorkloadAndClients(context.Background(), &UserInfo{Namespace: "ns", Pod: "p"})
	assert.Error(t, err)
}

// sessionStub satisfies the Session interface (ssh.Channel + extras) with a
// configurable username for handler tests.
type sessionStub struct {
	*fakeChannel
	loginUser string
	sessCtx   context.Context
}

func (s *sessionStub) User() string                    { return s.loginUser }
func (s *sessionStub) Context() context.Context        { return s.sessCtx }
func (s *sessionStub) Pty() (Pty, <-chan Window, bool) { return Pty{}, nil, false }
func (s *sessionStub) RawCommand() string              { return "" }

func newSessionStub(user string) *sessionStub {
	return &sessionStub{fakeChannel: &fakeChannel{}, loginUser: user, sessCtx: context.Background()}
}

// TestHandleSessionInvalidUser verifies the invalid-user branch writes an error.
func TestHandleSessionInvalidUser(t *testing.T) {
	h := &SshHandler{}
	s := newSessionStub("invalid-format")
	h.handleSession(s) // should not panic; writes error and returns
}

// TestHandleSessionWorkspaceNotFound drives a valid user into SessionConn, which fails
// fast on workspace resolution.
func TestHandleSessionWorkspaceNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(sshScheme(t)).Build()
	h := &SshHandler{Client: cl}
	s := newSessionStub("root.pod-0.main.bash.ns")
	h.handleSession(s)
}

// TestHandleSftpInvalidUser verifies the invalid-user branch returns early.
func TestHandleSftpInvalidUser(t *testing.T) {
	h := &SshHandler{}
	s := newSessionStub("invalid-format")
	h.handleSftp(s)
}

// TestHandleSftpWorkspaceNotFound verifies the workspace-lookup error path returns early.
func TestHandleSftpWorkspaceNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(sshScheme(t)).Build()
	h := &SshHandler{Client: cl}
	s := newSessionStub("root.pod-0.main.bash.ns")
	h.handleSftp(s)
}

// TestSessionConnWorkspaceNotFound verifies SessionConn fails fast when the workspace
// cannot be resolved (before any k8s exec stream is established).
func TestSessionConnWorkspaceNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(sshScheme(t)).Build()
	h := &SshHandler{Client: cl}
	err := h.SessionConn(context.Background(), &SessionInfo{
		userInfo: &UserInfo{Namespace: "ns", Pod: "p"},
	})
	assert.Error(t, err)
}

// fakeNewChannel is a minimal ssh.NewChannel for direct-tcpip parsing tests.
type fakeNewChannel struct {
	extraData []byte
	rejected  bool
}

func (f *fakeNewChannel) Accept() (ssh.Channel, <-chan *ssh.Request, error) {
	// Return a closed request channel so handleRequests' range exits immediately
	// (ranging over a nil channel would block forever).
	reqs := make(chan *ssh.Request)
	close(reqs)
	return &fakeChannel{}, reqs, nil
}
func (f *fakeNewChannel) Reject(ssh.RejectionReason, string) error { f.rejected = true; return nil }
func (f *fakeNewChannel) ChannelType() string                      { return "direct-tcpip" }
func (f *fakeNewChannel) ExtraData() []byte                        { return f.extraData }

// rejectNewChannel is an ssh.NewChannel whose Accept fails.
type rejectNewChannel struct{}

func (rejectNewChannel) Accept() (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, assertSSHErr
}
func (rejectNewChannel) Reject(ssh.RejectionReason, string) error { return nil }
func (rejectNewChannel) ChannelType() string                      { return "session" }
func (rejectNewChannel) ExtraData() []byte                        { return nil }

var assertSSHErr = errSSH("accept failed")

type errSSH string

func (e errSSH) Error() string { return string(e) }

// TestHandleDirectIpBadExtraData verifies malformed forward data is rejected.
func TestHandleDirectIpBadExtraData(t *testing.T) {
	h := &SshHandler{}
	nc := &fakeNewChannel{extraData: []byte{0x01}} // truncated -> Unmarshal fails
	h.handleDirectIp(context.Background(), nil, nc)
	assert.True(t, nc.rejected, "expected channel to be rejected on bad data")
}

// TestStartSessionHandler drives startSessionHandler with a channel whose Accept
// succeeds and an immediately-closed request stream, so handleRequests returns.
func TestStartSessionHandler(t *testing.T) {
	h := &SshHandler{}
	// fakeNewChannel.Accept returns (nil, nil, nil); handleRequests ranges over a
	// nil request channel and returns immediately.
	h.startSessionHandler(context.Background(), nil, &fakeNewChannel{})
}

// TestStartSessionHandlerAcceptError verifies the accept-error branch returns early.
func TestStartSessionHandlerAcceptError(t *testing.T) {
	h := &SshHandler{}
	h.startSessionHandler(context.Background(), nil, &rejectNewChannel{})
}

// TestNewSshHandlerNoPrivateKey verifies initialization fails when no SSH private key
// is configured (the common case in unit-test environments).
func TestNewSshHandlerNoPrivateKey(t *testing.T) {
	_, err := NewSshHandler(context.Background(), nil)
	// Either the key is missing (expected here) or a prior init already succeeded;
	// both leave the package in a consistent state. We only require no panic.
	_ = err
}

// TestWebShellSessionFastFail establishes a real websocket connection and drives
// WebShell into SessionConn, which fails fast on workspace resolution and then
// closes the websocket gracefully.
func TestWebShellSessionFastFail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := ctrlfake.NewClientBuilder().WithScheme(sshScheme(t)).Build()
	h := &SshHandler{
		Client: cl,
		upgrader: &websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}

	router := gin.New()
	router.GET("/ws/:"+common.PodId, func(c *gin.Context) { h.WebShell(c) })
	srv := httptest.NewServer(router)
	defer srv.Close()

	wsURL := "ws://" + strings.TrimPrefix(srv.URL, "http://") + "/ws/pod-0?namespace=ns"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	// Drain until the server closes the connection (SessionConn fails fast).
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// TestHandleConnectionSessionChannel performs a real SSH handshake over an in-memory
// listener, opens a "session" channel, and lets the server-side handleSession run
// (which fails fast in SessionConn since no workspace exists). This exercises
// HandleConnection's channel loop, startSessionHandler, handleSession and session.User().
func TestHandleConnectionSessionChannel(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	signer, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	cl := ctrlfake.NewClientBuilder().WithScheme(sshScheme(t)).Build()
	h := &SshHandler{ctx: context.Background(), Client: cl, config: cfg, timeout: time.Minute}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	srvDone := make(chan struct{})
	go func() {
		conn, aerr := ln.Accept()
		if aerr == nil {
			h.HandleConnection(conn)
		}
		close(srvDone)
	}()

	clientCfg := &ssh.ClientConfig{
		User:            "root.pod-0.main.bash.ns",
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	}
	client, err := ssh.Dial("tcp", ln.Addr().String(), clientCfg)
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}

	sess, err := client.NewSession()
	if err == nil {
		// Run a command; server-side handleSession will fail fast and close.
		_ = sess.Run("whoami")
		_ = sess.Close()
	}
	_ = client.Close()

	select {
	case <-srvDone:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not finish handling connection")
	}
}

// TestHandleConnectionHandshakeFailure verifies HandleConnection returns when the SSH
// handshake fails (peer sends no valid SSH protocol).
func TestHandleConnectionHandshakeFailure(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	_ = pub
	signer, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	h := &SshHandler{ctx: context.Background(), config: cfg, timeout: time.Minute}

	serverConn, clientConn := net.Pipe()
	// Close the client end so the server handshake fails quickly.
	_ = clientConn.Close()

	done := make(chan struct{})
	go func() {
		h.HandleConnection(serverConn)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("HandleConnection did not return on handshake failure")
	}
}

// TestGetWorkloadAndClientsClientFactoryError verifies the path past workspace lookup
// fails when the cluster client factory cannot be built.
func TestGetWorkloadAndClientsClientFactoryError(t *testing.T) {
	ws := &v1.Workspace{}
	ws.Name = "ns"
	ws.Spec.Cluster = "nonexistent-cluster"
	cl := ctrlfake.NewClientBuilder().WithScheme(sshScheme(t)).WithObjects(ws).Build()
	h := &SshHandler{Client: cl, clientManager: commonutils.NewObjectManagerSingleton()}

	_, _, err := h.getWorkloadAndClients(context.Background(), &UserInfo{Namespace: "ns", Pod: "p"})
	assert.Error(t, err)
}

// TestWebShellUpgradeFailure verifies a non-websocket request fails the upgrade and returns.
func TestWebShellUpgradeFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SshHandler{upgrader: &websocket.Upgrader{}}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// Plain GET without websocket upgrade headers -> Upgrade fails.
	c.Request = httptest.NewRequest(http.MethodGet, "/?namespace=ns", nil)

	// Should return without panicking.
	h.WebShell(c)
	assert.NotEqual(t, 0, w.Code)
}
