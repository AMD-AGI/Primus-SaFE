/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"archive/tar"
	"bufio"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/ops_job/exportimage/agent"
)

// fakeHarbor is a registry with a token service, blobs kept per repository and
// cross-repository mounts, which counts the bytes each token moves.
type fakeHarbor struct {
	host  string // name:port
	addr  string
	srv   *httptest.Server
	inner http.Handler

	mu         sync.Mutex
	blobs      map[string]map[string]bool
	tokens     map[string]string // jti -> requested scopes
	moved      map[string]int64  // jti -> request and response body bytes
	issued     int
	noMount    bool
	extraGrant string
	// expireAt is the PATCH request from which every token issued before it is refused,
	// as if it had expired.
	expireAt   int
	patches    int
	cutoff     int
	onManifest func(repo, ref string)
}

const platformUser, platformSecret = "platform", "secret"

func newFakeHarbor(t *testing.T, hostname string) *fakeHarbor {
	t.Helper()
	h := &fakeHarbor{
		inner:  registry.New(registry.Logger(nopLogger())),
		blobs:  map[string]map[string]bool{},
		tokens: map[string]string{},
		moved:  map[string]int64{},
	}
	h.srv = httptest.NewTLSServer(h)
	t.Cleanup(h.srv.Close)
	h.addr = h.srv.Listener.Addr().String()
	_, port, _ := net.SplitHostPort(h.addr)
	h.host = hostname + ":" + port
	return h
}

type grant struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

type claims struct {
	JTI    string  `json:"jti"`
	Exp    int64   `json:"exp"`
	Access []grant `json:"access"`
}

func encodeJWT(c claims) string {
	b, _ := json.Marshal(c)
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"none"}`)) + "." + enc.EncodeToString(b) + ".sig"
}

func (h *fakeHarbor) issue(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	if !ok || user != platformUser || pass != platformSecret {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	c := claims{Exp: time.Now().Add(30 * time.Minute).Unix()}
	scopes := r.URL.Query()["scope"]
	for _, s := range scopes {
		parts := strings.Split(s, ":")
		if len(parts) != 3 {
			continue
		}
		c.Access = append(c.Access, grant{Type: parts[0], Name: parts[1], Actions: strings.Split(parts[2], ",")})
	}
	h.mu.Lock()
	h.issued++
	c.JTI = fmt.Sprint(h.issued)
	h.tokens[c.JTI] = strings.Join(scopes, " ")
	if h.extraGrant != "" {
		c.Access = append(c.Access, grant{Type: "repository", Name: h.extraGrant, Actions: []string{"pull", "push"}})
	}
	h.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"token": encodeJWT(c), "expires_in": 1800})
}

func (h *fakeHarbor) bearer(r *http.Request) (*claims, bool) {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return nil, false
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, false
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var c claims
	return &c, json.Unmarshal(b, &c) == nil
}

func (c *claims) allows(repo, action string) bool {
	for _, a := range c.Access {
		if a.Name == repo {
			for _, act := range a.Actions {
				if act == action {
					return true
				}
			}
		}
	}
	return false
}

// splitPath returns the repository and the rest of a /v2/ path.
func splitPath(p string) (repo, kind, rest string) {
	p = strings.TrimPrefix(p, "/v2/")
	for _, k := range []string{"/blobs/uploads/", "/blobs/", "/manifests/"} {
		if i := strings.LastIndex(p, k); i >= 0 {
			return p[:i], strings.Trim(k, "/"), p[i+len(k):]
		}
	}
	return "", "", p
}

type countingResponse struct {
	http.ResponseWriter
	status int
	n      int64
}

func (c *countingResponse) WriteHeader(s int) { c.status = s; c.ResponseWriter.WriteHeader(s) }
func (c *countingResponse) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	n, err := c.ResponseWriter.Write(b)
	c.n += int64(n)
	return n, err
}

type countingBody struct {
	io.ReadCloser
	n *int64
}

func (c countingBody) Read(b []byte) (int, error) {
	n, err := c.ReadCloser.Read(b)
	*c.n += int64(n)
	return n, err
}

func (h *fakeHarbor) has(repo, d string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.blobs[repo][d]
}

func (h *fakeHarbor) add(repo, d string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.blobs[repo] == nil {
		h.blobs[repo] = map[string]bool{}
	}
	h.blobs[repo][d] = true
}

func (h *fakeHarbor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/service/token" {
		h.issue(w, r)
		return
	}
	c, ok := h.bearer(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/service/token",service="harbor-registry"`, h.host))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/v2/" || r.URL.Path == "/v2" {
		w.WriteHeader(http.StatusOK)
		return
	}
	h.mu.Lock()
	if r.Method == http.MethodPatch {
		h.patches++
		if h.patches == h.expireAt {
			h.cutoff = h.issued
		}
	}
	jti, _ := strconv.Atoi(c.JTI)
	expired := jti <= h.cutoff
	h.mu.Unlock()
	if expired {
		// As a registry answers an expired token: with the challenge, so that a client
		// that can get a new token does.
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/service/token",service="harbor-registry",error="invalid_token"`, h.host))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	repo, kind, rest := splitPath(r.URL.Path)
	action := "pull"
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		action = "push"
	}
	if !c.allows(repo, action) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var in int64
	if r.Body != nil {
		r.Body = countingBody{r.Body, &in}
	}
	cw := &countingResponse{ResponseWriter: w}
	defer func() {
		h.mu.Lock()
		h.moved[c.JTI] += in + cw.n
		h.mu.Unlock()
	}()

	switch {
	case kind == "blobs/uploads" && r.Method == http.MethodPost && r.URL.Query().Get("mount") != "":
		d, from := r.URL.Query().Get("mount"), r.URL.Query().Get("from")
		if !h.noMount && c.allows(from, "pull") && h.has(from, d) {
			h.add(repo, d)
			cw.Header().Set("Location", "/v2/"+repo+"/blobs/"+d)
			cw.WriteHeader(http.StatusCreated)
			return
		}
		q := r.URL.Query()
		q.Del("mount")
		q.Del("from")
		r.URL.RawQuery = q.Encode()
	case kind == "blobs" && !h.has(repo, rest):
		cw.WriteHeader(http.StatusNotFound)
		_, _ = cw.Write([]byte(`{"errors":[{"code":"BLOB_UNKNOWN","message":"blob unknown"}]}`))
		return
	case kind == "manifests" && r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		var m v1.Manifest
		if err := json.Unmarshal(body, &m); err != nil {
			cw.WriteHeader(http.StatusBadRequest)
			return
		}
		// An index names manifests, which the inner registry checks itself.
		for _, d := range append([]v1.Descriptor{m.Config}, m.Layers...) {
			if !m.MediaType.IsIndex() && !h.has(repo, d.Digest.String()) {
				cw.WriteHeader(http.StatusBadRequest)
				_, _ = cw.Write([]byte(`{"errors":[{"code":"BLOB_UNKNOWN","message":"blob unknown to registry"}]}`))
				return
			}
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		h.inner.ServeHTTP(cw, r)
		if h.onManifest != nil && cw.status == http.StatusCreated {
			h.onManifest(repo, rest)
		}
		return
	}
	h.inner.ServeHTTP(cw, r)
	if kind == "blobs/uploads" && r.Method == http.MethodPut && cw.status == http.StatusCreated {
		h.add(repo, r.URL.Query().Get("digest"))
	}
}

// movedBy sums the bytes moved with tokens whose requested scopes satisfy pick.
func (h *fakeHarbor) movedBy(pick func(scopes string) bool) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	var n int64
	for jti, b := range h.moved {
		if pick(h.tokens[jti]) {
			n += b
		}
	}
	return n
}

// network reaches the fake registries by name and trusts their certificate.
type network struct {
	addrs map[string]string // host:port -> listener
	ca    []byte
}

func (n *network) dial(ctx context.Context, netw, addr string) (net.Conn, error) {
	if a, ok := n.addrs[addr]; ok {
		addr = a
	}
	return (&net.Dialer{}).DialContext(ctx, netw, addr)
}

func (n *network) transport() http.RoundTripper {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(n.ca)
	t := remote.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig.RootCAs = pool
	t.DialContext = n.dial
	return t
}

func (n *network) keychain(t *testing.T) authn.Keychain {
	auth := base64.StdEncoding.EncodeToString([]byte(platformUser + ":" + platformSecret))
	auths := map[string]any{}
	for host := range n.addrs {
		auths[host] = map[string]string{"auth": auth}
	}
	b, _ := json.Marshal(map[string]any{"auths": auths})
	kc, err := NewConfigKeychain(b)
	require.NoError(t, err)
	return kc
}

// fakeContainer answers the probe and runs the agent in this process, on a directory that
// stands for the container's root file system.
type fakeContainer struct {
	env        agent.Env
	uid        int
	noAgent    bool
	protocol   int
	noBaseline bool
	recording  bool
	noRecord   bool
	probeExtra string
	tamper     func(*agent.Response)
	echoToken  bool
	ran        []string
	request    agent.Request
	messages   []agent.Message
}

func (c *fakeContainer) Exec(ctx context.Context, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	switch {
	case len(cmd) == 3 && cmd[0] == "sh" && cmd[2] == probeScript:
		c.ran = append(c.ran, "probe")
		fmt.Fprintf(stdout, "uid=%d\n", c.uid)
		if !c.noAgent {
			fmt.Fprintln(stdout, "agent=1")
			p := c.protocol
			if p == 0 {
				p = agent.ProtocolVersion
			}
			fmt.Fprintf(stdout, "protocol=%d\n", p)
		}
		if !c.noBaseline {
			fmt.Fprintln(stdout, "baseline=1")
		}
		if c.recording {
			fmt.Fprintln(stdout, "recording=1")
		}
		if c.noRecord {
			fmt.Fprintln(stdout, "norecord=1")
		}
		// The entry point file is looked for the way the agent looks for it.
		if f, err := agent.RunFileOf(c.env); err == nil {
			fmt.Fprintf(stdout, "runpath=%s\n", f)
			if _, err := os.Lstat(f); err == nil {
				fmt.Fprintln(stdout, "runfile=1")
			}
		}
		fmt.Fprint(stdout, c.probeExtra)
		return nil
	case len(cmd) == 2 && cmd[0] == agent.BinaryPath && cmd[1] == "export":
		c.ran = append(c.ran, "export")
		in := bufio.NewReader(stdin)
		first, err := in.ReadString('\n')
		if err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(first), &c.request); err != nil {
			return err
		}
		if c.echoToken {
			fmt.Fprintf(stderr, "save-image: failed with token %s\n", c.request.Token)
			return errors.New("command terminated with exit code 1")
		}
		err = agent.Serve(ctx, io.MultiReader(strings.NewReader(first), in), &messageTap{c: c, w: stdout}, c.env)
		if err != nil {
			fmt.Fprintln(stderr, "save-image:", err)
			return errors.New("command terminated with exit code 1")
		}
		return nil
	}
	return fmt.Errorf("unexpected command %q", cmd)
}

// messageTap records, and may change, what the agent prints: each message is one write.
type messageTap struct {
	c *fakeContainer
	w io.Writer
}

func (m *messageTap) Write(p []byte) (int, error) {
	var msg agent.Message
	if err := json.Unmarshal(p, &msg); err == nil {
		if msg.Result != nil && m.c.tamper != nil {
			m.c.tamper(msg.Result)
			b, _ := json.Marshal(msg)
			if _, err := m.w.Write(append(b, '\n')); err != nil {
				return 0, err
			}
			m.c.messages = append(m.c.messages, msg)
			return len(p), nil
		}
		m.c.messages = append(m.c.messages, msg)
	}
	return m.w.Write(p)
}

const dpkgStatusBase = "Package: base-files\nStatus: install ok installed\nArchitecture: amd64\n\n"

const dpkgStatusAfterApt = dpkgStatusBase +
	"Package: openssh-server\nStatus: install ok installed\nArchitecture: amd64\n\n" +
	"Package: jq\nStatus: install ok installed\nArchitecture: amd64\n\n"

var baseFiles = map[string]string{
	"etc/debian_version":                "12\n",
	"etc/issue":                         "Debian\n",
	"usr/share/doc/tar/README":          "tar",
	"usr/share/doc/gzip/README":         "gzip",
	"data/vol/inner":                    "in the image, hidden by a volume",
	"var/lib/dpkg/status":               dpkgStatusBase,
	"var/lib/dpkg/info/base-files.list": "/etc/issue\n",
}

const testMountinfo = "1 0 0:1 / / rw - overlay overlay rw\n" +
	"2 1 0:2 / /shared-data rw - tmpfs tmpfs rw\n" +
	"3 1 0:3 / /data/vol rw - tmpfs tmpfs rw\n" +
	"4 1 0:4 / /etc/hosts rw - ext4 /dev/sda rw\n"

func writeFile(t *testing.T, root, p, data string) {
	t.Helper()
	full := filepath.Join(root, p)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(data), 0o644))
}

// tick waits past the file system's timestamp granularity.
func tick() { time.Sleep(30 * time.Millisecond) }

// newContainer is a root container started from the base image: the launcher recorded its
// files, installed sshd and handed over, and then the user worked in it.
func newContainer(t *testing.T, bigFile int) *fakeContainer {
	t.Helper()
	return newContainerRunningFrom(t, bigFile, "")
}

// newContainerRunningFrom is newContainer whose launcher wrote the entry point to runFile
// (a path in the container) and recorded where, as it does when the working directory
// cannot be written; "" for the working directory's .run.sh, recorded nowhere.
func newContainerRunningFrom(t *testing.T, bigFile int, runFile string) *fakeContainer {
	t.Helper()
	root := t.TempDir()
	for p, data := range baseFiles {
		writeFile(t, root, p, data)
	}
	// At start the volume is already mounted over /data/vol, and /etc/hosts is the runtime's.
	require.NoError(t, os.Remove(filepath.Join(root, "data/vol/inner")))
	writeFile(t, root, "data/vol/scratch", "volume data")
	writeFile(t, root, "etc/hosts", "10.0.0.1 me\n")
	writeFile(t, root, "shared-data/launcher.sh", "#!/bin/sh")
	baseline := filepath.Join(root, "shared-data/save-image.base")
	_, err := agent.Record(baseline, root, testMountinfo)
	require.NoError(t, err)

	tick()
	writeFile(t, root, "usr/sbin/sshd", "launcher-installed")
	writeFile(t, root, "var/lib/dpkg/info/openssh-server.list", "/usr/sbin/sshd\n")
	writeFile(t, root, "etc/ssh/ssh_host_rsa_key", "PRIVATE")
	marker := ""
	if runFile == "" {
		writeFile(t, root, ".run.sh", "sleep infinity")
	} else {
		writeFile(t, root, runFile, "sleep infinity")
		marker = filepath.Join(root, "shared-data/save-image.run")
		writeFile(t, root, "shared-data/save-image.run", runFile+"\n")
	}
	tick()
	writeFile(t, root, "etc/issue", "Debian, changed\n")
	writeFile(t, root, "root/hello.txt", "hello")
	require.NoError(t, os.Symlink("/root/hello.txt", filepath.Join(root, "root/link")))
	writeFile(t, root, "etc/ssh/ssh_host_ecdsa_key", "PRIVATE, regenerated by the user")
	writeFile(t, root, "var/lib/dpkg/status", dpkgStatusAfterApt)
	writeFile(t, root, "var/lib/dpkg/info/jq.list", "/usr/bin/jq\n")
	require.NoError(t, os.Remove(filepath.Join(root, "etc/debian_version")))
	require.NoError(t, os.RemoveAll(filepath.Join(root, "usr/share/doc/tar")))
	if bigFile > 0 {
		b := make([]byte, bigFile)
		_, _ = rand.Read(b)
		writeFile(t, root, "root/model.bin", string(b))
	}
	return &fakeContainer{env: agent.Env{
		Root:      root,
		Baseline:  baseline,
		RunFile:   filepath.Join(root, ".run.sh"),
		RunMarker: marker,
		Mountinfo: testMountinfo,
	}}
}

type world struct {
	staging *fakeHarbor
	net     *network
	baseID  string
	request Request
	c       *fakeContainer
}

// newWorld pushes the base image to the registry, under the repository the node
// pulled it from, and returns a request that saves c into the same registry.
func newWorld(t *testing.T, c *fakeContainer) *world {
	t.Helper()
	staging := newFakeHarbor(t, "registry.example.com")
	n := &network{
		addrs: map[string]string{staging.host: staging.addr},
		ca:    []byte(certPEM(staging.srv)),
	}
	c.env.Dial = n.dial

	layer, err := crane.Layer(func() map[string][]byte {
		m := map[string][]byte{}
		for p, d := range baseFiles {
			m[p] = []byte(d)
		}
		return m
	}())
	require.NoError(t, err)
	base, err := mutate.AppendLayers(empty.Image, layer)
	require.NoError(t, err)
	baseTag, err := name.NewTag(staging.host + "/proxy/library/python:3.12")
	require.NoError(t, err)
	require.NoError(t, remote.Write(baseTag, base, remote.WithTransport(n.transport()),
		remote.WithAuth(&authn.Basic{Username: platformUser, Password: "secret"})))
	d, err := base.Digest()
	require.NoError(t, err)

	staged, err := name.NewRepository(staging.host + "/save-staging/export-1")
	require.NoError(t, err)
	target, err := name.NewTag(staging.host + "/custom/library/python:20261008000000-abcdef")
	require.NoError(t, err)
	return &world{
		staging: staging,
		net:     n,
		baseID:  baseTag.Context().Digest(d.String()).String(),
		c:       c,
		request: Request{
			Exec:            c,
			Registry:        staging.host,
			ImageID:         "docker-pullable://" + baseTag.Context().Digest(d.String()).String(),
			Staging:         staged,
			Target:          target,
			Keychain:        n.keychain(t),
			StagingKeychain: n.keychain(t),
			Transport:       n.transport(),
			CA:              n.ca,
			Platform:        v1.Platform{OS: "linux", Architecture: "amd64"},
		},
	}
}

func certPEM(srv *httptest.Server) string {
	return "-----BEGIN CERTIFICATE-----\n" + base64.StdEncoding.EncodeToString(srv.Certificate().Raw) + "\n-----END CERTIFICATE-----\n"
}

func (w *world) options() []remote.Option {
	return []remote.Option{remote.WithTransport(w.net.transport()), remote.WithAuthFromKeychain(w.request.Keychain)}
}

func (w *world) flatten(t *testing.T, ref name.Reference) map[string]string {
	t.Helper()
	img, err := remote.Image(ref, w.options()...)
	require.NoError(t, err)
	rc := mutate.Extract(img)
	defer rc.Close()
	out := map[string]string{}
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		require.NoError(t, err)
		data, _ := io.ReadAll(tr)
		p := path.Clean("/" + hdr.Name)
		if hdr.Typeflag == tar.TypeSymlink {
			out[p] = "-> " + hdr.Linkname
		} else {
			out[p] = string(data)
		}
	}
}

// stagingPush picks the token the container got: the one asked for the staging
// repository's push scope alone.
func (w *world) stagingPush(scopes string) bool {
	return scopes == w.request.Staging.Scope("push,pull")
}

func TestExportEndToEnd(t *testing.T) {
	const big = 2 << 20
	w := newWorld(t, newContainer(t, big))
	res, err := Export(context.Background(), w.request)
	require.NoError(t, err)
	assert.Equal(t, w.baseID, res.Base)
	assert.Equal(t, []string{"probe", "export"}, w.c.ran)

	// The layer went from the container to the registry; this process moved kilobytes.
	assert.Greater(t, w.staging.movedBy(w.stagingPush), int64(big), "the container uploaded the layer")
	controller := w.staging.movedBy(func(s string) bool { return !w.stagingPush(s) && !strings.Contains(s, "python:push") })
	assert.Less(t, controller, int64(64<<10), "the controller moves no image data")

	pushed, err := remote.Head(w.request.Target, w.options()...)
	require.NoError(t, err)
	assert.Equal(t, pushed.Digest.String(), res.Digest, "the result is the registry's digest")

	fs := w.flatten(t, w.request.Target)
	// Added and modified.
	assert.Equal(t, "hello", fs["/root/hello.txt"])
	assert.Equal(t, "-> /root/hello.txt", fs["/root/link"])
	assert.Equal(t, "Debian, changed\n", fs["/etc/issue"])
	assert.Len(t, fs["/root/model.bin"], big)
	// Deleted.
	assert.NotContains(t, fs, "/etc/debian_version")
	assert.NotContains(t, fs, "/usr/share/doc/tar/README")
	assert.Contains(t, fs, "/usr/share/doc/gzip/README", "a sibling of a deleted directory survives")
	// A volume hides image content; it does not delete it, and its own content stays out.
	assert.Equal(t, "in the image, hidden by a volume", fs["/data/vol/inner"])
	assert.NotContains(t, fs, "/data/vol/scratch")
	// What the launcher wrote before handing over, the runtime's files and host keys.
	assert.NotContains(t, fs, "/usr/sbin/sshd")
	assert.NotContains(t, fs, "/etc/ssh/ssh_host_rsa_key")
	assert.NotContains(t, fs, "/etc/ssh/ssh_host_ecdsa_key")
	assert.NotContains(t, fs, "/etc/hosts")
	assert.NotContains(t, fs, "/shared-data/save-image.base")
	assert.NotContains(t, fs, "/.run.sh")
	// The package database describes the files the image holds.
	assert.Equal(t, dpkgStatusBase+"Package: jq\nStatus: install ok installed\nArchitecture: amd64\n\n", fs["/var/lib/dpkg/status"])
	assert.Contains(t, fs, "/var/lib/dpkg/info/jq.list")
	assert.NotContains(t, fs, "/var/lib/dpkg/info/openssh-server.list")
	// The token went in on standard input, with the CA.
	assert.Equal(t, w.request.Staging.RepositoryStr(), w.c.request.Repository)
	assert.Equal(t, string(w.net.ca), w.c.request.CA)
	assert.True(t, w.c.request.Deadline.After(time.Now()))
	assert.True(t, w.c.request.TokenExpiry.After(time.Now()))
	assert.Equal(t, int64(maxLayerSize), w.c.request.MaxSize)
}

// A registry that enforces the token's grant keeps the container's layer in the staging
// repository. (Harbor does not: it gives a token its minting account's power, which is
// why the token is minted with an account limited to the staging project.)
func TestTheContainersTokenCannotWriteElsewhere(t *testing.T) {
	w := newWorld(t, newContainer(t, 0))
	_, err := Export(context.Background(), w.request)
	require.NoError(t, err)
	tok := w.c.request.Token
	for _, repo := range []string{"custom/library/python", "proxy/library/python"} {
		ref, err := name.NewTag(w.staging.host + "/" + repo + ":evil")
		require.NoError(t, err)
		img, err := mutate.AppendLayers(empty.Image)
		require.NoError(t, err)
		err = remote.Write(ref, img, remote.WithTransport(w.net.transport()),
			remote.WithAuth(authn.FromConfig(authn.AuthConfig{RegistryToken: tok})))
		assert.Error(t, err, repo)
	}
}

func TestExportRefusesAStagingRepositoryInAnotherRegistry(t *testing.T) {
	w := newWorld(t, newContainer(t, 0))
	other, err := name.NewRepository("elsewhere.example.com/save-staging/export-1")
	require.NoError(t, err)
	w.request.Staging = other
	_, err = Export(context.Background(), w.request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both in the configured registry")
	assert.Empty(t, w.c.ran)
}

// A name the registry client reads as Docker Hub ("myregistry/save-staging/x" without a
// dot or port) is not the configured registry: no credential goes there.
func TestExportUsesNoCredentialOutsideTheConfiguredRegistry(t *testing.T) {
	w := newWorld(t, newContainer(t, 0))
	hub, err := name.NewRepository("myregistry/save-staging/export-1")
	require.NoError(t, err)
	require.Equal(t, "index.docker.io", hub.RegistryStr())
	hubTag, err := name.NewTag("myregistry/custom/library/python:x")
	require.NoError(t, err)
	w.request.Registry = "myregistry"
	w.request.Staging, w.request.Target = hub, hubTag
	used := false
	w.request.StagingKeychain = keychainFunc(func(authn.Resource) (authn.Authenticator, error) {
		used = true
		return authn.Anonymous, nil
	})
	_, err = Export(context.Background(), w.request)
	require.Error(t, err)
	assert.False(t, used, "the staging credential was not even looked up")
	assert.Empty(t, w.c.ran)
}

type keychainFunc func(authn.Resource) (authn.Authenticator, error)

func (k keychainFunc) Resolve(r authn.Resource) (authn.Authenticator, error) { return k(r) }

// The layer outlives the registry's tokens: the container asks for new ones and gets
// them, minted the same way.
func TestExportRenewsTheContainersToken(t *testing.T) {
	c := newContainer(t, 1<<20)
	c.env.ChunkSize = 64 << 10
	w := newWorld(t, c)
	w.staging.expireAt = 3
	res, err := Export(context.Background(), w.request)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Layer.Renewals)
	var renewals int
	for _, m := range w.c.messages {
		if m.Renew {
			renewals++
		}
	}
	assert.Equal(t, 1, renewals)
	assert.Equal(t, 2, w.stagingTokens(), "the renewal is a token for the staging repository alone")
	assert.Greater(t, w.staging.patches, 10)
	fs := w.flatten(t, w.request.Target)
	assert.Len(t, fs["/root/model.bin"], 1<<20)
}

func (w *world) stagingTokens() int {
	w.staging.mu.Lock()
	defer w.staging.mu.Unlock()
	n := 0
	for _, scopes := range w.staging.tokens {
		if w.stagingPush(scopes) {
			n++
		}
	}
	return n
}

// A container asking for tokens in a loop is not uploading; it is refused.
func TestExportLimitsHowOftenTheContainerGetsATokenOut(t *testing.T) {
	c := newContainer(t, 0)
	w := newWorld(t, c)
	inR, inW := io.Pipe()
	defer inW.Close()
	w.request.Exec = execFunc(func(ctx context.Context, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
		if cmd[0] == "sh" {
			return c.Exec(ctx, cmd, stdin, stdout, stderr)
		}
		go func() { _, _ = io.Copy(inW, stdin) }()
		lines := bufio.NewScanner(inR)
		lines.Scan() // the request
		var errs []string
		for i := 0; i < 3; i++ {
			fmt.Fprintln(stdout, `{"renew":true}`)
			lines.Scan()
			var g agent.Grant
			_ = json.Unmarshal(lines.Bytes(), &g)
			errs = append(errs, g.Error)
		}
		fmt.Fprintln(stderr, strings.Join(errs, "|"))
		return errors.New("command terminated with exit code 1")
	})
	_, err := Export(context.Background(), w.request)
	require.Error(t, err)
	assert.Equal(t, 2, w.stagingTokens(), "the first token and one renewal")
	assert.Contains(t, err.Error(), "|a new token was granted")
}

type execFunc func(ctx context.Context, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error

func (f execFunc) Exec(ctx context.Context, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return f(ctx, cmd, stdin, stdout, stderr)
}

// The probe's output is the container's, and is bounded like the agent's.
func TestExportBoundsWhatTheProbePrints(t *testing.T) {
	c := newContainer(t, 0)
	c.probeExtra = strings.Repeat("x", 2*maxProbe)
	w := newWorld(t, c)
	_, err := Export(context.Background(), w.request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "printed more than")
	assert.Equal(t, []string{"probe"}, c.ran)
}

func TestExportWaitsForTheRecord(t *testing.T) {
	c := newContainer(t, 0)
	c.noBaseline, c.recording = true, true
	w := newWorld(t, c)
	_, err := Export(context.Background(), w.request)
	require.ErrorIs(t, err, agent.ErrRecording)
	assert.Equal(t, []string{"probe"}, c.ran)
}

func TestExportRefusesAContainerThatPredatesSaveImage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fakeContainer)
	}{
		{"no record of its files", func(c *fakeContainer) { c.noBaseline = true }},
		{"no export program", func(c *fakeContainer) { c.noAgent = true }},
		{"an export program of another protocol", func(c *fakeContainer) { c.protocol = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newContainer(t, 0)
			tc.change(c)
			w := newWorld(t, c)
			_, err := Export(context.Background(), w.request)
			require.ErrorIs(t, err, ErrPredatesSaveImage)
			assert.Contains(t, err.Error(), "restart")
			assert.Equal(t, []string{"probe"}, c.ran)
			assert.Zero(t, w.staging.movedBy(w.stagingPush), "no token is issued")
		})
	}
}

func TestExportRefusesNonRoot(t *testing.T) {
	c := newContainer(t, 0)
	c.uid = 1000
	w := newWorld(t, c)
	_, err := Export(context.Background(), w.request)
	require.ErrorIs(t, err, ErrNotRoot)
	assert.Equal(t, []string{"probe"}, c.ran)
}

// The image is put together by mounting the base's layers; a base the registry
// does not hold is refused rather than copied.
func TestExportRefusesABaseNotInTheRegistry(t *testing.T) {
	w := newWorld(t, newContainer(t, 0))
	w.request.ImageID = "elsewhere.example.com/proxy/library/python@sha256:" + strings.Repeat("a", 64)
	_, err := Export(context.Background(), w.request)
	require.ErrorIs(t, err, ErrBaseNotInRegistry)
	assert.Equal(t, []string{"probe"}, w.c.ran)
}

func TestExportRefusesWithoutAStagingCredential(t *testing.T) {
	for _, kc := range []authn.Keychain{nil, authn.NewMultiKeychain()} {
		w := newWorld(t, newContainer(t, 0))
		w.request.StagingKeychain = kc
		_, err := Export(context.Background(), w.request)
		require.ErrorIs(t, err, ErrNoStagingCredential)
		assert.Equal(t, []string{"probe"}, w.c.ran)
	}
}

func TestExportRefusesATokenThatGrantsMore(t *testing.T) {
	w := newWorld(t, newContainer(t, 0))
	w.staging.extraGrant = "custom/library/python"
	_, err := Export(context.Background(), w.request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not only")
	assert.Equal(t, []string{"probe"}, w.c.ran, "the token never reaches the container")
}

// What the container reports locates its layer; the registry decides whether it is there.
func TestExportChecksTheLayerAgainstTheRegistry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tamper func(*agent.Response)
		want   string
	}{
		{"wrong size", func(r *agent.Response) { r.Size++ }, "not the"},
		{"layer not uploaded", func(r *agent.Response) { r.Digest = "sha256:" + strings.Repeat("c", 64) }, "not in"},
		{"garbage digest", func(r *agent.Response) { r.Digest = "x" }, "invalid layer digest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newContainer(t, 0)
			c.tamper = tc.tamper
			w := newWorld(t, c)
			_, err := Export(context.Background(), w.request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			_, err = remote.Head(w.request.Target, w.options()...)
			assert.Error(t, err, "nothing is published")
		})
	}
}

// A registry that will not mount gets no copy instead: that would move the data through
// this process.
func TestExportNeverCopiesWhenTheRegistryWillNotMount(t *testing.T) {
	w := newWorld(t, newContainer(t, 1<<20))
	w.staging.noMount = true
	_, err := Export(context.Background(), w.request)
	require.ErrorIs(t, err, ErrNotMounted)
	_, err = remote.Head(w.request.Target, w.options()...)
	assert.Error(t, err)
	controller := w.staging.movedBy(func(s string) bool { return !w.stagingPush(s) && !strings.Contains(s, "python:push") })
	assert.Less(t, controller, int64(64<<10))
}

func TestExportKeepsTheTokenOutOfErrors(t *testing.T) {
	c := newContainer(t, 0)
	c.echoToken = true
	w := newWorld(t, c)
	_, err := Export(context.Background(), w.request)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), c.request.Token)
	assert.Contains(t, err.Error(), "<token>")
}

func TestTokenGrantCheck(t *testing.T) {
	ok := &tokenClaims{}
	require.NoError(t, json.Unmarshal([]byte(`{"access":[{"type":"repository","name":"s/1","actions":["pull","push"]}]}`), ok))
	assert.NoError(t, ok.grantsOnlyPushTo("s/1"))
	// Harbor adds delete to an administrator's push grant; it reaches nothing but s/1.
	withDelete := &tokenClaims{}
	require.NoError(t, json.Unmarshal([]byte(`{"access":[{"type":"repository","name":"s/1","actions":["delete","pull","push"]}]}`), withDelete))
	assert.NoError(t, withDelete.grantsOnlyPushTo("s/1"))
	assert.Error(t, ok.grantsOnlyPushTo("s/2"))

	for _, bad := range []string{
		`{"access":[]}`,
		`{"access":[{"type":"repository","name":"s/1","actions":["pull"]}]}`,
		`{"access":[{"type":"repository","name":"s/1","actions":["push"]},{"type":"repository","name":"t","actions":["pull"]}]}`,
		`{"access":[{"type":"registry","name":"catalog","actions":["*"]}]}`,
	} {
		c := &tokenClaims{}
		require.NoError(t, json.Unmarshal([]byte(bad), c))
		assert.Error(t, c.grantsOnlyPushTo("s/1"), bad)
	}
	_, err := parseTokenClaims("opaque-token")
	assert.Error(t, err)
}

func TestParseImageID(t *testing.T) {
	d, err := ParseImageID("docker-pullable://reg.example.com/proxy/library/python@sha256:" + strings.Repeat("a", 64))
	require.NoError(t, err)
	assert.Equal(t, "reg.example.com", d.RegistryStr())
	assert.Equal(t, "proxy/library/python", d.RepositoryStr())

	_, err = ParseImageID("sha256:" + strings.Repeat("a", 64))
	assert.Error(t, err, "a local image ID names no registry")
}

func TestBaseCandidates(t *testing.T) {
	d, err := name.NewDigest("node-registry.example.com/proxy/library/python@sha256:" + strings.Repeat("b", 64))
	require.NoError(t, err)
	var got []string
	for _, c := range BaseCandidates(d, "staging.example.com") {
		got = append(got, c.String())
	}
	assert.Equal(t, []string{"staging.example.com/proxy/library/python@sha256:" + strings.Repeat("b", 64)}, got,
		"only the export registry's copy can be mounted")
	assert.Equal(t, []name.Digest{d}, BaseCandidates(d, "node-registry.example.com"))
}

func TestConfigKeychain(t *testing.T) {
	auth := base64.StdEncoding.EncodeToString([]byte("u:p"))
	kc, err := NewConfigKeychain([]byte(`{"auths":{"https://reg.example.com/":{"auth":"` + auth + `"}}}`))
	require.NoError(t, err)
	reg, _ := name.NewRegistry("reg.example.com")
	a, err := kc.Resolve(reg)
	require.NoError(t, err)
	cfg, err := a.Authorization()
	require.NoError(t, err)
	assert.Equal(t, auth, cfg.Auth)

	other, _ := name.NewRegistry("other.example.com")
	a, err = kc.Resolve(other)
	require.NoError(t, err)
	assert.Equal(t, authn.Anonymous, a)

	_, err = NewConfigKeychain([]byte("not json"))
	assert.Error(t, err)
}

func TestNewTransportRejectsAnUnusableCA(t *testing.T) {
	_, err := NewTransport([]byte("not a certificate"))
	assert.Error(t, err)
	_, err = NewTransport(nil)
	assert.NoError(t, err)
}

// A launcher that cannot write the working directory starts the entry point from a
// temporary file, and records where: its change time still separates what the launcher
// installed from what the user did.
func TestExportFindsTheEntryPointFileTheLauncherFellBackTo(t *testing.T) {
	w := newWorld(t, newContainerRunningFrom(t, 0, "/tmp/run.Ab12Cd"))
	_, err := Export(context.Background(), w.request)
	require.NoError(t, err)
	fs := w.flatten(t, w.request.Target)
	assert.Equal(t, "hello", fs["/root/hello.txt"])
	assert.Equal(t, "Debian, changed\n", fs["/etc/issue"])
	assert.NotContains(t, fs, "/etc/debian_version")
	assert.NotContains(t, fs, "/usr/sbin/sshd", "what the launcher installed stays out")
	assert.NotContains(t, fs, "/tmp/run.Ab12Cd")
}

// Without the entry point file, the export is refused before any token is issued.
func TestExportRefusesAContainerWithoutItsEntryPointFile(t *testing.T) {
	for _, tc := range []struct {
		name, runFile, missing string
	}{
		{"in the working directory", "", ".run.sh"},
		{"where the launcher fell back to", "/tmp/run.Ab12Cd", "tmp/run.Ab12Cd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newContainerRunningFrom(t, 0, tc.runFile)
			require.NoError(t, os.Remove(filepath.Join(c.env.Root, tc.missing)))
			w := newWorld(t, c)
			_, err := Export(context.Background(), w.request)
			require.ErrorIs(t, err, ErrNoRunFile)
			assert.Contains(t, err.Error(), "restart the workload")
			assert.Contains(t, err.Error(), tc.missing)
			assert.Equal(t, []string{"probe"}, c.ran)
			assert.Zero(t, w.stagingTokens(), "no token is issued")
		})
	}
}

// The probe is a shell script; run it, as the container would, against files laid out
// the way the launcher leaves them.
func TestProbeScriptFindsTheEntryPointFile(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	dir := t.TempDir()
	shared, cwd := filepath.Join(dir, "shared-data"), filepath.Join(dir, "work")
	require.NoError(t, os.MkdirAll(shared, 0o755))
	require.NoError(t, os.MkdirAll(cwd, 0o755))
	binary := filepath.Join(shared, "save-image")
	require.NoError(t, os.WriteFile(binary, []byte(fmt.Sprintf("#!/bin/sh\necho %d\n", agent.ProtocolVersion)), 0o755))
	baseline := filepath.Join(shared, "save-image.base")
	require.NoError(t, os.WriteFile(baseline, []byte("x"), 0o644))
	marker := filepath.Join(shared, "save-image.run")
	script := probeScriptFor(binary, baseline, marker, filepath.Join(shared, "save-image.norecord"))
	probe := func() Probe {
		cmd := exec.Command("sh", "-c", script)
		cmd.Dir = cwd
		out, err := cmd.Output()
		require.NoError(t, err)
		p := ParseProbe(string(out))
		p.UID = 0 // the user the test runs as is not what is tested here
		return p
	}

	// No record of where: the working directory's .run.sh.
	assert.ErrorIs(t, probe().Check(), ErrNoRunFile)
	require.NoError(t, os.WriteFile(filepath.Join(cwd, ".run.sh"), nil, 0o755))
	assert.NoError(t, probe().Check())

	// The launcher fell back to a temporary file and recorded it.
	fallback := filepath.Join(dir, "tmp", "run.Ab12Cd")
	require.NoError(t, os.WriteFile(marker, []byte(fallback+"\n"), 0o644))
	p := probe()
	assert.Equal(t, fallback, p.RunPath)
	assert.ErrorIs(t, p.Check(), ErrNoRunFile, "the working directory's file is not the one recorded")
	require.NoError(t, os.MkdirAll(filepath.Dir(fallback), 0o755))
	require.NoError(t, os.WriteFile(fallback, nil, 0o700))
	assert.NoError(t, probe().Check())
}

// A workload that turned the record off is told so, before any token is issued.
func TestExportRefusesAWorkloadThatTurnedTheRecordOff(t *testing.T) {
	c := newContainer(t, 0)
	c.noBaseline, c.noRecord = true, true
	w := newWorld(t, c)
	_, err := Export(context.Background(), w.request)
	require.ErrorIs(t, err, ErrRecordDisabled)
	assert.Contains(t, err.Error(), "SAFE_SAVE_IMAGE_RECORD=0")
	assert.Equal(t, []string{"probe"}, c.ran)
	assert.Zero(t, w.stagingTokens(), "no token is issued")
}

func TestProbeScriptReportsTheRecordTurnedOff(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "save-image")
	require.NoError(t, os.WriteFile(binary, []byte(fmt.Sprintf("#!/bin/sh\necho %d\n", agent.ProtocolVersion)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".run.sh"), nil, 0o755))
	noRecord := filepath.Join(dir, "save-image.norecord")
	probe := func() Probe {
		cmd := exec.Command("sh", "-c", probeScriptFor(binary, filepath.Join(dir, "save-image.base"),
			filepath.Join(dir, "save-image.run"), noRecord))
		cmd.Dir = dir
		out, err := cmd.Output()
		require.NoError(t, err)
		p := ParseProbe(string(out))
		p.UID = 0
		return p
	}
	assert.ErrorIs(t, probe().Check(), ErrPredatesSaveImage)
	require.NoError(t, os.WriteFile(noRecord, nil, 0o644))
	assert.ErrorIs(t, probe().Check(), ErrRecordDisabled)
}

// A container started from a multi-platform image runs the node's platform's image; the
// saved image is put together on that one.
func TestExportUsesTheBaseForTheNodesPlatform(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			w := newWorld(t, newContainer(t, 0))
			baseID, err := ParseImageID(w.request.ImageID)
			require.NoError(t, err)
			auth := remote.WithAuth(&authn.Basic{Username: platformUser, Password: platformSecret})
			base, err := remote.Image(baseID, remote.WithTransport(w.net.transport()), auth)
			require.NoError(t, err)
			idx := v1.ImageIndex(empty.Index)
			for _, a := range []string{"amd64", "arm64"} {
				layer, err := crane.Layer(map[string][]byte{"etc/arch": []byte(a)})
				require.NoError(t, err)
				img, err := mutate.AppendLayers(base, layer)
				require.NoError(t, err)
				require.NoError(t, remote.Write(baseID.Context().Tag(a), img, remote.WithTransport(w.net.transport()), auth))
				idx = mutate.AppendManifests(idx, mutate.IndexAddendum{Add: img, Descriptor: v1.Descriptor{
					Platform: &v1.Platform{OS: "linux", Architecture: a},
				}})
			}
			tag := baseID.Context().Tag("multi")
			require.NoError(t, remote.WriteIndex(tag, idx, remote.WithTransport(w.net.transport()), auth))
			d, err := idx.Digest()
			require.NoError(t, err)
			w.request.ImageID = baseID.Context().Digest(d.String()).String()
			w.request.Platform = v1.Platform{OS: "linux", Architecture: arch}

			_, err = Export(context.Background(), w.request)
			require.NoError(t, err)
			fs := w.flatten(t, w.request.Target)
			assert.Equal(t, arch, fs["/etc/arch"])
			assert.Equal(t, "hello", fs["/root/hello.txt"])
		})
	}
}
