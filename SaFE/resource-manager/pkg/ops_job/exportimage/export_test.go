/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"path"
	"sort"
	"strings"
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
)

type fakeFile struct {
	typ   byte
	ctime int64
	data  string
	link  string
}

// fakeContainer answers the three commands the export runs, the way a container with GNU
// find and GNU tar would.
type fakeContainer struct {
	uid        int
	gnu        bool
	launcher   bool
	runfile    int64 // 0: no .run.sh
	mounts     []string
	files      map[string]fakeFile
	tarErr     error
	tarExtra   string // a member the tar step adds without being asked
	ranTar     bool
	ranListing bool
}

func (c *fakeContainer) Exec(_ context.Context, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	switch {
	case len(cmd) == 3 && cmd[2] == probeScript:
		fmt.Fprintf(stdout, "uid=%d\n", c.uid)
		if c.gnu {
			fmt.Fprint(stdout, "gnufind=1\ngnutar=1\n")
		}
		if c.launcher {
			fmt.Fprint(stdout, "launcher=1\n")
		}
		if c.runfile != 0 {
			fmt.Fprintf(stdout, "runfile=%d.0000000000\n", c.runfile)
		}
		fmt.Fprintln(stdout, mountinfoMarker)
		fmt.Fprintln(stdout, "1 0 0:1 / / rw - overlay overlay rw")
		for i, m := range c.mounts {
			fmt.Fprintf(stdout, "%d 1 0:%d / %s rw - tmpfs tmpfs rw\n", i+2, i+2, m)
		}
		return nil
	case len(cmd) > 0 && cmd[0] == "find":
		c.ranListing = true
		for _, p := range c.sorted() {
			if c.underMount(p) {
				continue
			}
			f := c.files[p]
			fmt.Fprintf(stdout, "%c %d.0000000000 %s\x00", f.typ, f.ctime, p)
		}
		return nil
	case len(cmd) == 3 && cmd[2] == tarScript:
		c.ranTar = true
		all, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		tw := tar.NewWriter(stdout)
		for _, n := range strings.Split(strings.TrimSuffix(string(all), "\x00"), "\x00") {
			if n == "" {
				continue
			}
			p := path.Clean("/" + n)
			f, ok := c.files[p]
			if !ok {
				continue // vanished
			}
			writeFakeMember(tw, n, f)
		}
		if c.tarExtra != "" {
			writeFakeMember(tw, c.tarExtra, fakeFile{typ: 'f', data: "x"})
		}
		_ = tw.Close()
		return c.tarErr
	}
	return fmt.Errorf("unexpected command %q", cmd)
}

func writeFakeMember(tw *tar.Writer, n string, f fakeFile) {
	hdr := &tar.Header{Name: n, Mode: 0o644, Uid: 0, Gid: 0, Uname: "root", ModTime: time.Unix(f.ctime, 0)}
	switch f.typ {
	case 'd':
		hdr.Typeflag, hdr.Name, hdr.Mode = tar.TypeDir, strings.TrimSuffix(n, "/")+"/", 0o755
	case 'l':
		hdr.Typeflag, hdr.Linkname = tar.TypeSymlink, f.link
	default:
		hdr.Typeflag, hdr.Size = tar.TypeReg, int64(len(f.data))
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte(f.data))
}

func (c *fakeContainer) sorted() []string {
	var out []string
	for p := range c.files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (c *fakeContainer) underMount(p string) bool {
	for _, m := range c.mounts {
		if strings.HasPrefix(p, m+"/") {
			return true
		}
	}
	return false
}

type world struct {
	host    string
	baseID  string
	target  name.Tag
	request Request
}

// newWorld pushes a base image to an in-memory registry and returns a request that exports
// a container started from it.
func newWorld(t *testing.T, c *fakeContainer) *world {
	t.Helper()
	srv := httptest.NewServer(registry.New(registry.Logger(nopLogger())))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")

	layer, err := crane.Layer(map[string][]byte{
		"etc/debian_version":                []byte("12\n"),
		"etc/issue":                         []byte("Debian\n"),
		"usr/share/doc/tar/README":          []byte("tar"),
		"usr/share/doc/gzip/README":         []byte("gzip"),
		"data/vol/inner":                    []byte("in the image, hidden by a volume"),
		"var/lib/dpkg/status":               []byte(dpkgStatusBase),
		"var/lib/dpkg/info/base-files.list": []byte("/etc/issue\n"),
	})
	require.NoError(t, err)
	base, err := mutate.AppendLayers(empty.Image, layer)
	require.NoError(t, err)
	baseTag, err := name.NewTag(host + "/proxy/library/python:3.12")
	require.NoError(t, err)
	require.NoError(t, remote.Write(baseTag, base))
	d, err := base.Digest()
	require.NoError(t, err)

	target, err := name.NewTag(host + "/custom/library/python:20261008000000-abcdef")
	require.NoError(t, err)
	tr, err := NewTransport()
	require.NoError(t, err)
	return &world{
		host:   host,
		baseID: baseTag.Context().Digest(d.String()).String(),
		target: target,
		request: Request{
			Exec:      c,
			ImageID:   "docker-pullable://" + baseTag.Context().Digest(d.String()).String(),
			StartedAt: time.Unix(50, 0),
			Target:    target,
			Keychain:  authn.NewMultiKeychain(),
			Transport: tr,
			Platform:  v1.Platform{OS: "linux", Architecture: "amd64"},
		},
	}
}

// containerFromBase is a root container started from newWorld's base, whose launcher
// handed over at 200 after installing sshd, and in which the user then worked.
func containerFromBase() *fakeContainer {
	return &fakeContainer{
		uid: 0, gnu: true, launcher: true, runfile: 200,
		mounts: []string{"/shared-data", "/data/vol", "/etc/hosts"},
		files: map[string]fakeFile{
			"/":                           {typ: 'd', ctime: 300},
			"/etc":                        {typ: 'd', ctime: 300},
			"/etc/issue":                  {typ: 'f', ctime: 300, data: "Debian, changed\n"},
			"/etc/hosts":                  {typ: 'f', ctime: 300, data: "10.0.0.1 me\n"},
			"/etc/ssh":                    {typ: 'd', ctime: 150},
			"/etc/ssh/ssh_host_rsa_key":   {typ: 'f', ctime: 150, data: "PRIVATE"},
			"/etc/ssh/ssh_host_ecdsa_key": {typ: 'f', ctime: 300, data: "PRIVATE, regenerated by the user"},
			"/usr":                        {typ: 'd', ctime: 10},
			"/usr/sbin":                   {typ: 'd', ctime: 150},
			"/usr/sbin/sshd":              {typ: 'f', ctime: 150, data: "launcher-installed"},
			"/usr/share":                  {typ: 'd', ctime: 10},
			"/usr/share/doc":              {typ: 'd', ctime: 300},
			"/usr/share/doc/gzip":         {typ: 'd', ctime: 10},
			"/usr/share/doc/gzip/README":  {typ: 'f', ctime: 10, data: "gzip"},
			"/data":                       {typ: 'd', ctime: 10},
			"/data/vol":                   {typ: 'd', ctime: 300},
			"/data/vol/scratch":           {typ: 'f', ctime: 300, data: "volume data"},
			"/shared-data":                {typ: 'd', ctime: 300},
			"/shared-data/launcher.sh":    {typ: 'f', ctime: 300, data: "#!/bin/sh"},
			"/.run.sh":                    {typ: 'f', ctime: 200, data: "sleep"},
			"/root":                       {typ: 'd', ctime: 300},
			"/root/hello.txt":             {typ: 'f', ctime: 300, data: "hello"},
			"/root/link":                  {typ: 'l', ctime: 300, link: "/root/hello.txt"},
			// The user ran apt after the launcher had: the status file records both.
			"/var":                                   {typ: 'd', ctime: 10},
			"/var/lib":                               {typ: 'd', ctime: 10},
			"/var/lib/dpkg":                          {typ: 'd', ctime: 300},
			"/var/lib/dpkg/status":                   {typ: 'f', ctime: 300, data: dpkgStatusAfterApt},
			"/var/lib/dpkg/info":                     {typ: 'd', ctime: 300},
			"/var/lib/dpkg/info/base-files.list":     {typ: 'f', ctime: 10, data: "/etc/issue\n"},
			"/var/lib/dpkg/info/openssh-server.list": {typ: 'f', ctime: 150, data: "/usr/sbin/sshd\n"},
			"/var/lib/dpkg/info/libc6:amd64.list":    {typ: 'f', ctime: 150, data: "/lib\n"},
			"/var/lib/dpkg/info/jq.list":             {typ: 'f', ctime: 300, data: "/usr/bin/jq\n"},
		},
	}
}

const dpkgStatusBase = `Package: base-files
Status: install ok installed
Architecture: amd64

`

const dpkgStatusAfterApt = `Package: base-files
Status: install ok installed
Architecture: amd64

Package: openssh-server
Status: install ok installed
Architecture: amd64
Description: secure shell server
 Package: not-a-field

Package: libc6
Status: install ok installed
Architecture: amd64
Multi-Arch: same

Package: jq
Status: install ok installed
Architecture: amd64
`

func flatten(t *testing.T, ref name.Reference) map[string]string {
	t.Helper()
	img, err := remote.Image(ref)
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

func TestExportEndToEnd(t *testing.T) {
	c := containerFromBase()
	w := newWorld(t, c)

	res, err := Export(context.Background(), w.request)
	require.NoError(t, err)
	assert.Equal(t, w.baseID, res.Base)
	assert.Equal(t, "launcher hand-over", res.Threshold)

	pushed, err := remote.Head(w.target)
	require.NoError(t, err)
	assert.Equal(t, pushed.Digest.String(), res.Digest, "the reported digest is the pushed manifest's")

	fs := flatten(t, w.target)
	// Added and modified.
	assert.Equal(t, "hello", fs["/root/hello.txt"])
	assert.Equal(t, "-> /root/hello.txt", fs["/root/link"])
	assert.Equal(t, "Debian, changed\n", fs["/etc/issue"])
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
	assert.NotContains(t, fs, "/shared-data/launcher.sh")
	assert.NotContains(t, fs, "/.run.sh")
	// The package database describes the files the image holds.
	assert.Equal(t, "Package: base-files\nStatus: install ok installed\nArchitecture: amd64\n\n"+
		"Package: jq\nStatus: install ok installed\nArchitecture: amd64\n\n", fs["/var/lib/dpkg/status"])
	assert.Contains(t, fs, "/var/lib/dpkg/info/jq.list")
	assert.NotContains(t, fs, "/var/lib/dpkg/info/openssh-server.list")
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

func TestExportRefusesNonRoot(t *testing.T) {
	c := containerFromBase()
	c.uid = 1000
	w := newWorld(t, c)

	_, err := Export(context.Background(), w.request)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotRoot), err.Error())
	assert.False(t, c.ranListing || c.ranTar, "nothing is read from a container that cannot be read completely")
	_, err = remote.Head(w.target)
	assert.Error(t, err, "no image is pushed")
}

func TestExportRefusesUnknownLauncherHandover(t *testing.T) {
	c := containerFromBase()
	c.runfile = 0
	w := newWorld(t, c)
	_, err := Export(context.Background(), w.request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), LauncherRunFile)
	assert.False(t, c.ranTar)
}

func TestExportWithoutLauncherUsesContainerStart(t *testing.T) {
	c := containerFromBase()
	c.launcher, c.runfile = false, 0
	w := newWorld(t, c)
	w.request.StartedAt = time.Unix(100, 0)
	res, err := Export(context.Background(), w.request)
	require.NoError(t, err)
	assert.Equal(t, "container start", res.Threshold)
	fs := flatten(t, w.target)
	assert.Contains(t, fs, "/usr/sbin/sshd", "without a launcher, everything after the start is the user's")
	assert.NotContains(t, fs, "/etc/ssh/ssh_host_rsa_key", "host keys never are")
}

func TestExportFailsWhenTarFails(t *testing.T) {
	c := containerFromBase()
	c.tarErr = errors.New("command terminated with exit code 2")
	w := newWorld(t, c)
	_, err := Export(context.Background(), w.request)
	require.Error(t, err)
	_, err = remote.Head(w.target)
	assert.Error(t, err, "a partial archive is never pushed")
}

func TestExportRejectsUnrequestedMember(t *testing.T) {
	c := containerFromBase()
	c.tarExtra = "./etc/shadow"
	w := newWorld(t, c)
	_, err := Export(context.Background(), w.request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not requested")
	_, err = remote.Head(w.target)
	assert.Error(t, err)
}

func TestExportNeedsGNUTools(t *testing.T) {
	c := containerFromBase()
	c.gnu = false
	w := newWorld(t, c)
	_, err := Export(context.Background(), w.request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GNU")
}

func TestWriteLayerWhiteoutsAndHeaders(t *testing.T) {
	var in bytes.Buffer
	tw := tar.NewWriter(&in)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "./etc/", Typeflag: tar.TypeDir, Mode: 0o755, Uname: "root"}))
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "./etc/a", Typeflag: tar.TypeReg, Size: 1, Mode: 0o600,
		Uid: 7, Gid: 8, Uname: "seven", PAXRecords: map[string]string{
			"SCHILY.xattr.security.capability": "cap", "LIBARCHIVE.creationtime": "1"}}))
	_, _ = tw.Write([]byte("a"))
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "./etc/b", Typeflag: tar.TypeLink, Linkname: "./etc/a"}))
	require.NoError(t, tw.Close())

	var out bytes.Buffer
	st, err := WriteLayer(&out, &in, []string{"/etc", "/etc/a", "/etc/b"}, []string{"/usr/share/doc/tar", "/x"}, nil)
	require.NoError(t, err)
	assert.Equal(t, LayerStats{Entries: 3, Whiteouts: 2, Bytes: int64(out.Len())}, st)

	var got []string
	tr := tar.NewReader(&out)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		got = append(got, fmt.Sprintf("%s|%c|%s|%d:%d|%s", hdr.Name, hdr.Typeflag, hdr.Linkname, hdr.Uid, hdr.Gid, hdr.Uname))
		if hdr.Name == "etc/a" {
			assert.Equal(t, map[string]string{"SCHILY.xattr.security.capability": "cap"}, hdr.PAXRecords)
		}
	}
	assert.Equal(t, []string{
		"etc/|5||0:0|",
		"etc/a|0||7:8|",
		"etc/b|1|etc/a|0:0|",
		"usr/share/doc/.wh.tar|0||0:0|",
		".wh.x|0||0:0|",
	}, got)
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
	for _, c := range BaseCandidates(d, "push-registry.example.com") {
		got = append(got, c.String())
	}
	assert.Equal(t, []string{
		"push-registry.example.com/proxy/library/python@sha256:" + strings.Repeat("b", 64),
		"node-registry.example.com/proxy/library/python@sha256:" + strings.Repeat("b", 64),
	}, got)
	assert.Len(t, BaseCandidates(d, "node-registry.example.com"), 1)
}

// The base image is read from the push registry when the node's registry is unusable,
// and the push then reuses it.
func TestExportFallsBackToTheTargetRegistryForTheBase(t *testing.T) {
	c := containerFromBase()
	w := newWorld(t, c)
	// The node pulled from a registry this process cannot reach.
	w.request.ImageID = strings.Replace(w.baseID, w.host, "unreachable.invalid", 1)
	res, err := Export(context.Background(), w.request)
	require.NoError(t, err)
	assert.Equal(t, w.baseID, res.Base)
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

func TestFileSetFromTarAddsParents(t *testing.T) {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "./a/b/c", Typeflag: tar.TypeReg}))
	require.NoError(t, tw.Close())
	set, err := fileSetFromTar(&b)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"/a": true, "/a/b": true, "/a/b/c": true}, set)
}
