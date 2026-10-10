/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMountinfo = "1 0 0:1 / / rw - overlay overlay rw\n" +
	"2 1 0:2 / /shared-data rw - tmpfs tmpfs rw\n" +
	"3 1 0:3 / /data/vol rw - tmpfs tmpfs rw\n"

func write(t *testing.T, root, p, data string) {
	t.Helper()
	full := filepath.Join(root, p)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(data), 0o644))
}

// tick waits past the file system's timestamp granularity, so that what happens next has
// a later change time than what happened before.
func tick() { time.Sleep(30 * time.Millisecond) }

// container builds a root file system started from an image, records the baseline, lets
// the launcher install something and hand over, and then makes the user's changes.
func container(t *testing.T) Env {
	t.Helper()
	root := t.TempDir()
	for p, data := range map[string]string{
		"etc/debian_version":          "12\n",
		"etc/issue":                   "Debian\n",
		"usr/share/doc/tar/README":    "tar",
		"usr/share/doc/gzip/README":   "gzip",
		"var/lib/dpkg/status":         "Package: base-files\nStatus: install ok installed\nArchitecture: amd64\n\n",
		"var/lib/dpkg/info/base.list": "/etc/issue\n",
		"data/vol/scratch":            "volume data, never in the image",
		"shared-data/launcher.sh":     "#!/bin/sh",
	} {
		write(t, root, p, data)
	}
	baseline := filepath.Join(root, "shared-data/save-image.base")
	_, err := Record(baseline, root, testMountinfo)
	require.NoError(t, err)

	tick()
	write(t, root, "usr/sbin/sshd", "installed by the launcher")
	write(t, root, "etc/ssh/ssh_host_rsa_key", "PRIVATE")
	write(t, root, ".run.sh", "sleep infinity")
	tick()
	write(t, root, "etc/issue", "Debian, changed\n")
	write(t, root, "root/hello.txt", "hello")
	require.NoError(t, os.Symlink("/root/hello.txt", filepath.Join(root, "root/link")))
	require.NoError(t, os.Link(filepath.Join(root, "root/hello.txt"), filepath.Join(root, "root/hard")))
	require.NoError(t, os.Remove(filepath.Join(root, "etc/debian_version")))
	require.NoError(t, os.RemoveAll(filepath.Join(root, "usr/share/doc/tar")))
	return Env{
		Root:      root,
		Baseline:  baseline,
		RunFile:   filepath.Join(root, ".run.sh"),
		Mountinfo: testMountinfo,
		UID:       0,
	}
}

type tlsRegistry struct {
	host string
	ca   string
	dial func(ctx context.Context, network, addr string) (net.Conn, error)
	srv  *httptest.Server
}

func newTLSRegistry(t *testing.T) *tlsRegistry {
	t.Helper()
	srv := httptest.NewTLSServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	_, port, _ := net.SplitHostPort(addr)
	return &tlsRegistry{
		host: "registry.example.com:" + port,
		ca:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})),
		dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		srv: srv,
	}
}

func (r *tlsRegistry) request() Request {
	return Request{
		Registry:    r.host,
		Repository:  "save-staging/job-1",
		Token:       "not-checked-by-this-registry",
		TokenExpiry: time.Now().Add(30 * time.Minute),
		CA:          r.ca,
		Deadline:    time.Now().Add(time.Minute),
	}
}

// noRenewal is a token source for a registry that never asks for a new token.
type noRenewal struct{}

func (noRenewal) Renew(context.Context) (string, time.Time, error) {
	return "", time.Time{}, errors.New("no renewal expected")
}

func layerMembers(t *testing.T, r *tlsRegistry, resp *Response) map[string]string {
	t.Helper()
	ref, err := name.NewDigest(r.host + "/save-staging/job-1@" + resp.Digest)
	require.NoError(t, err)
	tr, err := httpsOnlyTransport(r.ca, r.dial)
	require.NoError(t, err)
	l, err := remote.Layer(ref, remote.WithTransport(tr))
	require.NoError(t, err)
	rc, err := l.Uncompressed()
	require.NoError(t, err)
	defer rc.Close()
	out := map[string]string{}
	tr2 := tar.NewReader(rc)
	for {
		hdr, err := tr2.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		require.NoError(t, err)
		data, _ := io.ReadAll(tr2)
		switch hdr.Typeflag {
		case tar.TypeSymlink:
			out[hdr.Name] = "-> " + hdr.Linkname
		case tar.TypeLink:
			out[hdr.Name] = "=> " + hdr.Linkname
		case tar.TypeDir:
			out[hdr.Name] = "dir"
		default:
			out[hdr.Name] = string(data)
		}
		assert.Empty(t, hdr.Uname, hdr.Name)
	}
}

func TestExportUploadsTheChanges(t *testing.T) {
	env := container(t)
	r := newTLSRegistry(t)
	env.Dial = r.dial
	resp, err := Export(context.Background(), r.request(), env, noRenewal{})
	require.NoError(t, err)

	got := layerMembers(t, r, resp)
	var names []string
	for n := range got {
		names = append(names, n)
	}
	sort.Strings(names)
	assert.Equal(t, []string{
		"etc/", "etc/.wh.debian_version", "etc/issue",
		"root/", "root/hard", "root/hello.txt", "root/link",
		"usr/share/doc/", "usr/share/doc/.wh.tar",
	}, names, "the launcher's files, host keys, mounts and the run file stay out")
	assert.Equal(t, "Debian, changed\n", got["etc/issue"])
	assert.Equal(t, "-> /root/hello.txt", got["root/link"])
	assert.Equal(t, "=> root/hard", got["root/hello.txt"], "the second name of an inode is a hard link")
	assert.Equal(t, 2, resp.Deleted)
}

func TestExportRefusals(t *testing.T) {
	r := newTLSRegistry(t)
	for _, tc := range []struct {
		name   string
		change func(*Env, *Request)
		want   string
		is     error
	}{
		{name: "not root", change: func(e *Env, _ *Request) { e.UID = 1000 }, is: ErrNotRoot},
		{name: "no baseline", change: func(e *Env, _ *Request) { require.NoError(t, os.Remove(e.Baseline)) }, is: ErrNoBaseline},
		{name: "record turned off", change: func(e *Env, _ *Request) {
			require.NoError(t, os.Remove(e.Baseline))
			require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(e.Baseline), "save-image.norecord"), nil, 0o644))
		}, is: ErrRecordDisabled},
		{name: "still recording", change: func(e *Env, _ *Request) {
			require.NoError(t, os.Rename(e.Baseline, e.Baseline+RecordingSuffix))
		}, is: ErrRecording},
		{name: "baseline of another format", change: func(e *Env, _ *Request) {
			require.NoError(t, os.WriteFile(e.Baseline, []byte("primus-safe save-image baseline v1\n/etc\x00\x00end\x00"), 0o600))
		}, want: "unknown format"},
		{name: "truncated baseline", change: func(e *Env, _ *Request) {
			b, err := os.ReadFile(e.Baseline)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(e.Baseline, b[:len(b)/2], 0o600))
		}, want: "incomplete"},
		{name: "no run file", change: func(e *Env, _ *Request) { require.NoError(t, os.Remove(e.RunFile)) }, want: LauncherRunFile, is: ErrNoRunFile},
		{name: "no run file where the launcher recorded it", change: func(e *Env, _ *Request) {
			e.RunMarker = filepath.Join(e.Root, "shared-data/save-image.run")
			require.NoError(t, os.WriteFile(e.RunMarker, []byte("/tmp/run.Ab12Cd\n"), 0o644))
		}, want: "run.Ab12Cd", is: ErrNoRunFile},
		{name: "a run file recorded as a relative path", change: func(e *Env, _ *Request) {
			e.RunMarker = filepath.Join(e.Root, "shared-data/save-image.run")
			require.NoError(t, os.WriteFile(e.RunMarker, []byte(".run.sh\n"), 0o644))
		}, want: "not an absolute path"},
		{name: "no token", change: func(_ *Env, q *Request) { q.Token = "" }, want: "token"},
		{name: "another CA", change: func(_ *Env, q *Request) { q.CA = otherCA(t) }, want: "certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := container(t)
			env.Dial = r.dial
			req := r.request()
			tc.change(&env, &req)
			_, err := Export(context.Background(), req, env, noRenewal{})
			require.Error(t, err)
			if tc.is != nil {
				assert.ErrorIs(t, err, tc.is)
			}
			assert.Contains(t, err.Error(), tc.want)
			assert.NotContains(t, err.Error(), "not-checked-by-this-registry", "the token is never echoed")
		})
	}
}

// otherCA is a CA that did not sign the registry's certificate.
func otherCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "other"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// The token is only ever sent over TLS: a registry that answers in plain HTTP is not
// retried insecurely, even when it is named by a loopback or private address (for which
// the registry client library would fall back to HTTP).
func TestExportNeverFallsBackToPlainHTTP(t *testing.T) {
	plain := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	defer plain.Close()
	addr := plain.Listener.Addr().String()
	_, port, _ := net.SplitHostPort(addr)
	env := container(t)
	env.Dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	_, err := Export(context.Background(), Request{
		Registry: "127.0.0.1:" + port, Repository: "save-staging/job-1",
		Token: "t", TokenExpiry: time.Now().Add(time.Hour), Deadline: time.Now().Add(time.Minute),
	}, env, noRenewal{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP response to HTTPS client")

	tr, err := httpsOnlyTransport("", nil)
	require.NoError(t, err)
	_, err = tr.RoundTrip(httptest.NewRequest("GET", "http://"+addr+"/v2/", nil))
	assert.ErrorContains(t, err, "refusing to reach the registry over http")
}

func readAll(t *testing.T, file string) ([]Entry, error) {
	t.Helper()
	f, err := os.Open(file)
	require.NoError(t, err)
	defer f.Close()
	r, err := NewBaselineReader(f)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for {
		e, ok, err := r.Next()
		if err != nil || !ok {
			return out, err
		}
		out = append(out, e)
	}
}

func TestBaselineRoundTrip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "base")
	in := []Entry{
		{Path: "/", Type: 'd', Ctime: Timestamp{Sec: 1, Nsec: 2}},
		{Path: "/etc", Type: 'd', Ctime: Timestamp{Sec: 3}},
		{Path: "/etc/a b\nc", Type: 'f', Ctime: Timestamp{Sec: 1700000000, Nsec: 999999999}},
	}
	require.NoError(t, WriteBaseline(file, in))
	got, err := readAll(t, file)
	require.NoError(t, err)
	assert.Equal(t, in, got)

	full, err := os.ReadFile(file)
	require.NoError(t, err)
	for _, bad := range []string{
		"", baselineHeader, "x" + string(full[1:]),
		string(full[:len(full)-1]),                      // truncated
		string(full) + "f1.0 /z\x00",                    // past the end
		baselineHeader + "f1.0 etc\x00E1\x00",           // relative
		baselineHeader + "f1.0 /b\x00f1.0 /a\x00E2\x00", // out of order
		baselineHeader + "f1.0 /a\x00E2\x00",            // wrong count
	} {
		f := filepath.Join(t.TempDir(), "bad")
		require.NoError(t, os.WriteFile(f, []byte(bad), 0o600))
		_, err := readAll(t, f)
		assert.Error(t, err, fmt.Sprintf("%q", bad))
	}
	assert.Error(t, WriteBaseline(file, []Entry{{Path: "/b"}, {Path: "/a"}}), "entries out of walk order")
}

// A record that fails leaves none behind, rather than an earlier container's.
func TestRecordRemovesTheOldRecordFirst(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/a", "x")
	file := filepath.Join(t.TempDir(), "base")
	n, err := Record(file, root, "")
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	_, err = os.Stat(file)
	require.NoError(t, err)

	_, err = Record(file, filepath.Join(root, "missing"), "")
	require.Error(t, err)
	_, err = os.Stat(file)
	assert.True(t, os.IsNotExist(err), "the earlier record is gone")
	_, err = os.Stat(file + RecordingSuffix)
	assert.True(t, os.IsNotExist(err), "a failed record leaves nothing that reads as in progress")
}

func TestWalkSkipsExcludedAndOtherFileSystems(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/b", "x")
	write(t, root, "data/vol/inner", "x")
	write(t, root, "proc/1/status", "x")
	var got []string
	require.NoError(t, Walk(root, NewFilter([]string{"/data/vol"}), func(e Entry) error {
		got = append(got, fmt.Sprintf("%c %s", e.Type, e.Path))
		return nil
	}))
	assert.Equal(t, []string{"d /", "d /a", "f /a/b", "d /data"}, got)
}

func TestWriteLayerWhiteoutsAndHeaders(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/a", "a")
	require.NoError(t, os.Chmod(filepath.Join(root, "etc/a"), 0o600))
	var out bytes.Buffer
	st, err := WriteLayer(&out, root, []string{"/etc", "/etc/a", "/etc/gone"}, []string{"/usr/share/doc/tar", "/x"}, nil)
	require.NoError(t, err)
	assert.Equal(t, LayerStats{Entries: 2, Whiteouts: 2, Vanished: 1, Bytes: int64(out.Len())}, st)

	var got []string
	tr := tar.NewReader(&out)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		got = append(got, fmt.Sprintf("%s|%c|%o|%s", hdr.Name, hdr.Typeflag, hdr.Mode&0o777, hdr.Uname))
		assert.True(t, hdr.AccessTime.IsZero() && hdr.ChangeTime.IsZero(), hdr.Name)
	}
	assert.Equal(t, []string{
		"etc/|5|755|",
		"etc/a|0|600|",
		"usr/share/doc/.wh.tar|0|644|",
		".wh.x|0|644|",
	}, got)
}

func TestReconcileDpkgStatus(t *testing.T) {
	in := map[string]bool{
		"/var/lib/dpkg/info/a.list":       true,
		"/var/lib/dpkg/info/b:amd64.list": true,
	}
	status := "Package: a\nStatus: install ok installed\nArchitecture: amd64\n\n" +
		"Package: b\nStatus: install ok installed\nArchitecture: amd64\nMulti-Arch: same\n\n" +
		"Package: c\nStatus: install ok installed\nArchitecture: all\n\n" +
		"Package: d\nStatus: deinstall ok config-files\nArchitecture: amd64\n"
	out, dropped := ReconcileDpkgStatus([]byte(status), func(p string) bool { return in[p] })
	assert.Equal(t, []string{"c"}, dropped)
	assert.Equal(t, "Package: a\nStatus: install ok installed\nArchitecture: amd64\n\n"+
		"Package: b\nStatus: install ok installed\nArchitecture: amd64\nMulti-Arch: same\n\n"+
		"Package: d\nStatus: deinstall ok config-files\nArchitecture: amd64\n\n", string(out))
}
