/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

// SSH global request and channel names for remote port forwarding (RFC 4254 §7).
const (
	tcpipForwardRequest       = "tcpip-forward"
	cancelTCPIPForwardRequest = "cancel-tcpip-forward"
	forwardedTCPIPChannel     = "forwarded-tcpip"
)

// tcpipForwardPayload is the body of both tcpip-forward and cancel-tcpip-forward.
type tcpipForwardPayload struct {
	BindAddr string
	BindPort uint32
}

// reverseForwardPolicy is the resolved configuration applied to one SSH session.
type reverseForwardPolicy struct {
	enabled       bool
	bindAddresses []string
	portMin       uint32
	portMax       uint32
	maxForwards   int
}

// Defaults applied when the configured port range cannot be used.
const (
	defaultForwardPortMin = 1024
	defaultForwardPortMax = 65535
)

// usablePort converts a configured port, falling back when it could not be one.
// The conversion matters: a negative value becomes 4294967295 as a uint32, which
// silently refuses every forward and reports a range nobody configured.
func usablePort(value, fallback int) uint32 {
	if value < 1 || value > 65535 {
		klog.Warningf("ssh reverse forward port %d is not a usable port, using %d instead", value, fallback)
		return uint32(fallback)
	}
	return uint32(value)
}

// loadReverseForwardPolicy snapshots the reverse forwarding configuration.
func loadReverseForwardPolicy() reverseForwardPolicy {
	portMin := usablePort(commonconfig.GetSSHReverseForwardPortMin(), defaultForwardPortMin)
	portMax := usablePort(commonconfig.GetSSHReverseForwardPortMax(), defaultForwardPortMax)
	if portMin > portMax {
		klog.Warningf("ssh reverse forward port range %d-%d is inverted, using %d-%d instead",
			portMin, portMax, defaultForwardPortMin, defaultForwardPortMax)
		portMin, portMax = defaultForwardPortMin, defaultForwardPortMax
	}
	return reverseForwardPolicy{
		enabled:       commonconfig.IsSSHReverseForwardEnable(),
		bindAddresses: commonconfig.GetSSHReverseForwardBindAddresses(),
		portMin:       portMin,
		portMax:       portMax,
		maxForwards:   commonconfig.GetSSHReverseForwardMaxPerSession(),
	}
}

// normalizeBindAddr maps the RFC 4254 bind address spellings onto a literal address.
func normalizeBindAddr(addr string) string {
	switch addr {
	case "", "*":
		return "0.0.0.0"
	case "localhost":
		return "127.0.0.1"
	default:
		return addr
	}
}

// validate checks a requested bind address and port against the policy and returns
// the literal address the Pod-side listener should bind.
func (p reverseForwardPolicy) validate(addr string, port uint32) (string, error) {
	if !p.enabled {
		return "", fmt.Errorf("remote port forwarding is disabled on this server")
	}
	bindAddr := normalizeBindAddr(addr)
	ip := net.ParseIP(bindAddr)
	if ip == nil {
		return "", fmt.Errorf("bind address %q is not a literal IP address", addr)
	}
	if ip.To4() == nil {
		// The relay binds with socat's TCP-LISTEN, which is IPv4. Refusing here
		// turns a config mistake into a clear answer instead of a listener that
		// fails inside the pod.
		return "", fmt.Errorf("bind address %q is not IPv4, and the pod-side listener binds an IPv4 socket", addr)
	}
	allowed := false
	for _, a := range p.bindAddresses {
		if normalizeBindAddr(a) == bindAddr {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("bind address %q is not allowed, permitted addresses: %v", bindAddr, p.bindAddresses)
	}
	if port == 0 {
		return "", fmt.Errorf("server-allocated listen ports are not supported, request an explicit port")
	}
	if port > 65535 {
		return "", fmt.Errorf("listen port %d is out of range", port)
	}
	if port < p.portMin || port > p.portMax {
		return "", fmt.Errorf("listen port %d is outside the permitted range %d-%d", port, p.portMin, p.portMax)
	}
	return bindAddr, nil
}

// reverseForward is one active `-R` listener owned by a session.
type reverseForward struct {
	// requestedAddr is the bind address as the client spelled it, kept apart from
	// the resolved bindAddr because the two serve different ends: bindAddr is what
	// the pod-side listener binds, requestedAddr is what goes back on the wire.
	requestedAddr string
	bindAddr      string
	bindPort      uint32
	listener      podListener
	ctx           context.Context
	cancel        context.CancelFunc
	// userInfo and startedAt exist so the close of a forward can be audited with
	// the same identity the open was logged under, plus how long it was up.
	userInfo  *UserInfo
	startedAt time.Time
}

// reverseForwardManager owns every remote forward requested over a single SSH
// connection and tears them all down when that connection ends.
type reverseForwardManager struct {
	h           *SshHandler
	conn        *ssh.ServerConn
	ctx         context.Context
	policy      reverseForwardPolicy
	resolve     podTargetResolver
	newListener podListenerFactory

	mu       sync.Mutex
	forwards map[string]*reverseForward
	closed   bool

	wg sync.WaitGroup
}

// podTargetResolver authorizes the session user against their target Pod and
// returns the clients used to reach it.
type podTargetResolver func(ctx context.Context, userInfo *UserInfo) (*commonclient.ClientFactory, error)

// resolveForwardTarget authorizes the user for the workload backing their Pod.
func (h *SshHandler) resolveForwardTarget(ctx context.Context, userInfo *UserInfo) (*commonclient.ClientFactory, error) {
	workload, k8sClients, err := h.getWorkloadAndClients(ctx, userInfo)
	if err != nil {
		return nil, err
	}
	if err = h.authUser(ctx, userInfo, workload); err != nil {
		return nil, err
	}
	return k8sClients, nil
}

// newReverseForwardManager creates a forward registry scoped to one SSH connection.
func newReverseForwardManager(ctx context.Context, h *SshHandler, conn *ssh.ServerConn) *reverseForwardManager {
	return &reverseForwardManager{
		h:           h,
		conn:        conn,
		ctx:         ctx,
		policy:      loadReverseForwardPolicy(),
		resolve:     h.resolveForwardTarget,
		newListener: newExecPodListener,
		forwards:    map[string]*reverseForward{},
	}
}

// forwardKey identifies a forward within a session.
func forwardKey(addr string, port uint32) string {
	return net.JoinHostPort(addr, strconv.FormatUint(uint64(port), 10))
}

// handleGlobalRequests services the connection-level request channel. It replaces
// ssh.DiscardRequests so that `-R` is answered instead of silently refused.
func (m *reverseForwardManager) handleGlobalRequests(reqs <-chan *ssh.Request) {
	for req := range reqs {
		switch req.Type {
		case tcpipForwardRequest:
			m.handleForward(req)
		case cancelTCPIPForwardRequest:
			m.handleCancel(req)
		default:
			// Keepalives and unknown requests: a failure reply is the correct
			// answer and is what DiscardRequests used to send.
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

// handleForward answers a tcpip-forward request by creating a Pod-side listener.
func (m *reverseForwardManager) handleForward(req *ssh.Request) {
	var payload tcpipForwardPayload
	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		m.reject(req, fmt.Errorf("failed to parse tcpip-forward payload: %v", err))
		return
	}

	bindAddr, err := m.policy.validate(payload.BindAddr, payload.BindPort)
	if err != nil {
		m.reject(req, err)
		return
	}

	userInfo, ok := ParseUserInfo(m.conn.User())
	if !ok {
		m.reject(req, fmt.Errorf("invalid user %s", m.conn.User()))
		return
	}
	k8sClients, err := m.resolve(m.ctx, userInfo)
	if err != nil {
		m.reject(req, err)
		return
	}

	key := forwardKey(bindAddr, payload.BindPort)
	if err = m.reserve(key); err != nil {
		m.reject(req, err)
		return
	}

	fwdCtx, cancel := context.WithCancel(m.ctx)
	listener, err := m.newListener(fwdCtx, userInfo, k8sClients, bindAddr, payload.BindPort)
	if err != nil {
		cancel()
		m.release(key)
		m.reject(req, err)
		return
	}

	fwd := &reverseForward{
		requestedAddr: payload.BindAddr,
		bindAddr:      bindAddr,
		bindPort:      payload.BindPort,
		listener:      listener,
		ctx:           fwdCtx,
		cancel:        cancel,
		userInfo:      userInfo,
		startedAt:     time.Now(),
	}
	if !m.activate(key, fwd) {
		// The session ended while the listener was starting up.
		closeForward(fwd, "ssh connection closing")
		m.reject(req, fmt.Errorf("ssh connection is closing"))
		return
	}

	klog.Infof("reverse forward established, user: %s, pod: %s/%s, listen: %s, requested: %q",
		userInfo.User, userInfo.Namespace, userInfo.Pod, key, payload.BindAddr)

	if req.WantReply {
		// RFC 4254: the reply carries a port only when the client asked the
		// server to allocate one, which we do not support.
		_ = req.Reply(true, nil)
	}

	// activate already registered this goroutine with the WaitGroup.
	go func() {
		defer m.wg.Done()
		m.serve(userInfo, fwd)
	}()
}

// handleCancel answers a cancel-tcpip-forward request by removing the listener.
func (m *reverseForwardManager) handleCancel(req *ssh.Request) {
	var payload tcpipForwardPayload
	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		m.reject(req, fmt.Errorf("failed to parse cancel-tcpip-forward payload: %v", err))
		return
	}
	key := forwardKey(normalizeBindAddr(payload.BindAddr), payload.BindPort)

	m.mu.Lock()
	fwd, ok := m.forwards[key]
	// A nil entry is a reservation whose listener is still starting. Global requests
	// are answered one at a time, so this cannot be reached today; leaving the
	// reservation in place keeps it from turning into a nil dereference if that
	// ever changes.
	if ok && fwd != nil {
		delete(m.forwards, key)
	}
	m.mu.Unlock()

	if !ok || fwd == nil {
		m.reject(req, fmt.Errorf("no active forward for %s", key))
		return
	}
	closeForward(fwd, "cancelled by client")
	if req.WantReply {
		_ = req.Reply(true, nil)
	}
}

// reserve claims a forward key, enforcing the per-session limit. The slot is held
// by a nil entry until the listener is running so two concurrent requests for the
// same port cannot both start a listener.
func (m *reverseForwardManager) reserve(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("ssh connection is closing")
	}
	if _, exists := m.forwards[key]; exists {
		return fmt.Errorf("%s is already forwarded by this session", key)
	}
	if m.policy.maxForwards > 0 && len(m.forwards) >= m.policy.maxForwards {
		return fmt.Errorf("this session already holds the maximum of %d remote forwards", m.policy.maxForwards)
	}
	m.forwards[key] = nil
	return nil
}

// release drops a reservation that never became a running forward.
func (m *reverseForwardManager) release(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fwd, ok := m.forwards[key]; ok && fwd == nil {
		delete(m.forwards, key)
	}
}

// activate publishes a started listener against its reservation. It reports false
// if the session was closed while the listener was starting.
func (m *reverseForwardManager) activate(key string, fwd *reverseForward) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		delete(m.forwards, key)
		return false
	}
	m.forwards[key] = fwd
	// The accept loop is registered here rather than by the caller: an Add that
	// lands after closeAll has released the lock can also land after its Wait has
	// returned, which lets the goroutine outlive teardown and is the documented way
	// to panic a WaitGroup.
	m.wg.Add(1)
	return true
}

// serve accepts Pod-side connections and bridges each one back to the client.
func (m *reverseForwardManager) serve(userInfo *UserInfo, fwd *reverseForward) {
	for {
		pc, err := fwd.listener.Accept(fwd.ctx)
		if err != nil {
			if fwd.ctx.Err() == nil {
				klog.ErrorS(err, "reverse forward listener stopped",
					"pod", userInfo.Pod, "listen", forwardKey(fwd.bindAddr, fwd.bindPort))
			}
			m.forget(fwd)
			return
		}
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.bridge(fwd, pc)
		}()
	}
}

// forget removes a forward whose listener died on its own, so the client can
// request the same port again.
//
// Closing a forward is also what stops its listener, so this runs on the way out of
// every teardown - the accept loop ends because someone else already closed it. Only
// the goroutine that still owned the entry may close and account for it, or a
// cancelled or session-ended forward would be reported closed twice.
func (m *reverseForwardManager) forget(fwd *reverseForward) {
	key := forwardKey(fwd.bindAddr, fwd.bindPort)
	m.mu.Lock()
	current, ok := m.forwards[key]
	owned := ok && current == fwd
	if owned {
		delete(m.forwards, key)
	}
	m.mu.Unlock()
	if !owned {
		return
	}
	closeForward(fwd, "listener stopped")
}

// bridge opens a forwarded-tcpip channel and copies bytes both ways.
func (m *reverseForwardManager) bridge(fwd *reverseForward, pc podConn) {
	payload := ssh.Marshal(forwardChannelData{
		DestAddr:   channelBindAddr(fwd.requestedAddr),
		DestPort:   fwd.bindPort,
		OriginAddr: pc.OriginAddr(),
		OriginPort: pc.OriginPort(),
	})
	ch, reqs, err := m.conn.OpenChannel(forwardedTCPIPChannel, payload)
	if err != nil {
		klog.ErrorS(err, "failed to open forwarded-tcpip channel")
		_ = pc.Close()
		return
	}
	go ssh.DiscardRequests(reqs)

	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(done) }) }

	// Tearing the forward down must unblock both copies, not just wait on them.
	go func() {
		select {
		case <-fwd.ctx.Done():
		case <-done:
		}
		_ = ch.Close()
		_ = pc.Close()
	}()

	go func() {
		_, copyErr := io.Copy(ch, pc)
		// Half-close so the client sees the Pod side finish writing. A clean end
		// here is the pod saying "request sent" - the reply still has to come back
		// the other way, so the pair stays up until the client is done too. An
		// error is a broken pod side, and then holding the pair open would strand
		// the client on a connection nothing can answer.
		_ = ch.CloseWrite()
		if copyErr != nil {
			finish()
		}
	}()
	_, _ = io.Copy(pc, ch)
	finish()
}

// channelBindAddr is the address to name in a forwarded-tcpip channel. RFC 4254
// calls it "address that was connected", and OpenSSH compares it against the string
// it sent in tcpip-forward with strcmp - so it has to be echoed back as the client
// spelled it, or the client refuses its own forward as administratively prohibited.
// The one transformation OpenSSH applies to its own copy before comparing is
// folding a wildcard to the empty string, so do the same.
func channelBindAddr(requested string) string {
	if requested == "*" {
		return ""
	}
	return requested
}

// reject logs why a forward request failed and tells the client it failed.
func (m *reverseForwardManager) reject(req *ssh.Request, err error) {
	klog.Errorf("%s rejected for user %s: %v", req.Type, m.conn.User(), err)
	if req.WantReply {
		_ = req.Reply(false, nil)
	}
}

// closeAll tears down every forward held by the session and waits for the
// goroutines bridging them to finish.
func (m *reverseForwardManager) closeAll() {
	m.mu.Lock()
	m.closed = true
	forwards := make([]*reverseForward, 0, len(m.forwards))
	for key, fwd := range m.forwards {
		delete(m.forwards, key)
		if fwd != nil {
			forwards = append(forwards, fwd)
		}
	}
	m.mu.Unlock()

	for _, fwd := range forwards {
		closeForward(fwd, "ssh session ended")
	}
	m.wg.Wait()
}

// closeForward stops a forward's Pod-side listener and every connection under it.
// Every route out of a forward passes through here, so an "established" line in the
// audit trail always has exactly one matching close.
func closeForward(fwd *reverseForward, reason string) {
	fwd.cancel()
	_ = fwd.listener.Close()
	klog.Infof("reverse forward closed (%s), user: %s, pod: %s/%s, listen: %s, duration: %s",
		reason, fwd.userInfo.User, fwd.userInfo.Namespace, fwd.userInfo.Pod,
		forwardKey(fwd.bindAddr, fwd.bindPort), time.Since(fwd.startedAt).Truncate(time.Second))
}
