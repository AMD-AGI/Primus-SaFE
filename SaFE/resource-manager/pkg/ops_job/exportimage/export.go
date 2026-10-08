/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package exportimage saves a running container as a new image without touching the node.
//
// Everything it needs from the container comes through pods/exec: a listing of the root
// file system and a tar of the paths that changed. The layer is assembled in this process,
// put on top of the image the container was started from (read from the registry by the
// digest the runtime reported), and pushed with credentials that never enter the
// container. A container on a kubelet and one on a virtual kubelet take the same path.
package exportimage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/stream"
)

// ErrNotRoot is returned for a container that does not run as root. Such a process cannot
// read every file of its own root file system, so the export would silently miss some.
var ErrNotRoot = errors.New("the container does not run as root")

// LauncherRunFile is where the platform launcher writes the user's entry point, relative to
// the container's working directory, once its own bootstrap (driver builds, sshd, socat,
// certificates) is done and immediately before it starts the entry point. Its change time
// is the boundary between what the platform wrote and what the user did.
const LauncherRunFile = ".run.sh"

const launcherScript = "/shared-data/launcher.sh"

const mountinfoMarker = "--- mountinfo"

// probeScript reports what the export depends on. It runs in the container's working
// directory, which is where the launcher wrote its entry point.
var probeScript = `echo "uid=$(id -u)"
if find --version 2>/dev/null | head -n 1 | grep -q GNU; then echo gnufind=1; fi
if tar --version 2>/dev/null | head -n 1 | grep -q GNU; then echo gnutar=1; fi
if [ -e ` + launcherScript + ` ]; then echo launcher=1; fi
if [ -e ./` + LauncherRunFile + ` ]; then find ./` + LauncherRunFile + ` -maxdepth 0 -printf 'runfile=%C@\n'; fi
echo "` + mountinfoMarker + `"
cat /proc/self/mountinfo
`

// Probe is what the container reported about itself.
type Probe struct {
	UID         int
	GNUFind     bool
	GNUTar      bool
	Launcher    bool
	RunFile     *Timestamp
	MountPoints []string
}

// ParseProbe parses probeScript's output.
func ParseProbe(out string) (Probe, error) {
	p := Probe{UID: -1}
	head, mountinfo, ok := strings.Cut(out, mountinfoMarker+"\n")
	if !ok {
		return p, fmt.Errorf("the container did not report its mount table")
	}
	for _, line := range strings.Split(head, "\n") {
		k, v, _ := strings.Cut(strings.TrimSpace(line), "=")
		switch k {
		case "uid":
			if n, err := strconv.Atoi(v); err == nil {
				p.UID = n
			}
		case "gnufind":
			p.GNUFind = true
		case "gnutar":
			p.GNUTar = true
		case "launcher":
			p.Launcher = true
		case "runfile":
			ts, err := ParseTimestamp(v)
			if err != nil {
				return p, err
			}
			p.RunFile = &ts
		}
	}
	p.MountPoints = ParseMountPoints(mountinfo)
	return p, nil
}

// Check refuses a container the export cannot read completely.
func (p Probe) Check() error {
	switch {
	case p.UID < 0:
		return fmt.Errorf("cannot determine the container's user id")
	case p.UID != 0:
		return fmt.Errorf("%w (uid %d): exporting it would leave out the files it cannot read", ErrNotRoot, p.UID)
	case !p.GNUFind || !p.GNUTar:
		return fmt.Errorf("the container has no GNU find and GNU tar, which the export runs inside it")
	}
	return nil
}

// Threshold returns the change time after which a file belongs to the user. In a container
// the launcher started, that is when the launcher handed over to the user's entry point:
// everything it installed before then (a few thousand files, SSH host keys among them) is
// platform state, not part of the user's image. A container without the launcher has only
// the runtime's start time.
func (p Probe) Threshold(startedAt time.Time) (Timestamp, string, error) {
	if p.Launcher {
		if p.RunFile == nil {
			return Timestamp{}, "", fmt.Errorf("the platform launcher ran in this container but its %s is not "+
				"in the working directory, so what the launcher installed cannot be told apart from the user's changes", LauncherRunFile)
		}
		return *p.RunFile, "launcher hand-over", nil
	}
	if startedAt.IsZero() {
		return Timestamp{}, "", fmt.Errorf("the container has no start time")
	}
	return Timestamp{Sec: startedAt.Unix()}, "container start", nil
}

// Request is one export.
type Request struct {
	Exec Execer
	// ImageID is the container status imageID: the digest the container was started from.
	ImageID string
	// StartedAt is the container's start time.
	StartedAt time.Time
	Target    name.Tag
	Keychain  authn.Keychain
	Transport http.RoundTripper
	Platform  v1.Platform
	Logf      func(format string, args ...any)
}

// Result describes a pushed image.
type Result struct {
	// Digest is the manifest digest of the pushed image.
	Digest    string
	Base      string
	Threshold string
	Changed   int
	Deleted   int
	Skipped   int
	Layer     LayerStats
}

// Export saves the container as Target.
func Export(ctx context.Context, req Request) (*Result, error) {
	logf := req.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var probeOut bytes.Buffer
	if err := runCaptured(ctx, req.Exec, []string{"sh", "-c", probeScript}, nil, &probeOut); err != nil {
		return nil, fmt.Errorf("probing the container: %w", err)
	}
	probe, err := ParseProbe(probeOut.String())
	if err != nil {
		return nil, err
	}
	if err := probe.Check(); err != nil {
		return nil, err
	}
	since, sinceSource, err := probe.Threshold(req.StartedAt)
	if err != nil {
		return nil, err
	}
	logf("export threshold %d.%09d (%s), %d mount points excluded", since.Sec, since.Nsec, sinceSource, len(probe.MountPoints))

	baseRef, err := ParseImageID(req.ImageID)
	if err != nil {
		return nil, err
	}
	opts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(req.Keychain),
		remote.WithTransport(req.Transport),
		remote.WithPlatform(req.Platform),
	}
	base, baseUsed, err := ResolveBase(ctx, BaseCandidates(baseRef, req.Target.RegistryStr()), opts...)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	baseFiles, err := BaseFileSet(base)
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", baseUsed, err)
	}
	logf("base %s: %d paths (%s)", baseUsed, len(baseFiles), time.Since(start).Round(time.Millisecond))

	current, err := listContainer(ctx, req.Exec)
	if err != nil {
		return nil, err
	}
	changes := ComputeChanges(baseFiles, current, since, NewFilter(probe.MountPoints))
	logf("container: %d paths, %d changed, %d deleted, %d skipped", len(current), len(changes.Changed), len(changes.Deleted), changes.Skipped)

	stats, digest, err := pushLayer(ctx, req, base, baseFiles, changes, opts)
	if err != nil {
		return nil, err
	}
	if len(stats.DroppedPackages) > 0 {
		logf("dpkg records left out (their files are not in the image): %s", strings.Join(stats.DroppedPackages, " "))
	}
	return &Result{
		Digest:    digest,
		Base:      baseUsed.String(),
		Threshold: sinceSource,
		Changed:   len(changes.Changed),
		Deleted:   len(changes.Deleted),
		Skipped:   changes.Skipped,
		Layer:     stats,
	}, nil
}

func listContainer(ctx context.Context, ex Execer) ([]Entry, error) {
	pr, pw := io.Pipe()
	stderr := &limitedBuffer{max: 4096}
	done := make(chan error, 1)
	go func() {
		err := ex.Exec(ctx, listCommand, nil, pw, stderr)
		pw.CloseWithError(err)
		done <- err
	}()
	entries, perr := ParseListing(pr)
	pr.CloseWithError(errors.New("listing parser stopped"))
	if err := <-done; err != nil {
		return nil, fmt.Errorf("listing the container's files: %w: %s", err, stderr.String())
	}
	if perr != nil {
		return nil, perr
	}
	return entries, nil
}

func pushLayer(ctx context.Context, req Request, base v1.Image, baseFiles map[string]bool, ch Changes, opts []remote.Option) (LayerStats, string, error) {
	inImage := ImageContains(baseFiles, ch)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	layerR, layerW := io.Pipe()
	type built struct {
		stats LayerStats
		err   error
	}
	builtCh := make(chan built, 1)
	go func() {
		archR, archW := io.Pipe()
		stderr := &limitedBuffer{max: 4096}
		execErr := make(chan error, 1)
		go func() {
			err := req.Exec.Exec(ctx, []string{"sh", "-c", tarScript}, TarInput(ch.Changed), archW, stderr)
			archW.CloseWithError(err)
			execErr <- err
		}()
		st, err := WriteLayer(layerW, archR, ch.Changed, ch.Deleted, inImage)
		if err == nil {
			// The archive ends with padding the tar reader does not consume.
			_, err = io.Copy(io.Discard, archR)
		}
		archR.CloseWithError(errors.New("layer writer stopped"))
		if e := <-execErr; e != nil && err == nil {
			err = fmt.Errorf("archiving the changed files in the container: %w: %s", e, stderr.String())
		}
		layerW.CloseWithError(err)
		builtCh <- built{st, err}
	}()

	img, err := mutate.Append(base, mutate.Addendum{
		Layer: stream.NewLayer(layerR),
		History: v1.History{
			Created:   v1.Time{Time: time.Now().UTC()},
			CreatedBy: "primus-safe exportimage",
			Comment:   "files changed in the running container",
		},
	})
	if err == nil {
		err = remote.Write(req.Target, img, opts...)
	}
	if err != nil {
		layerR.CloseWithError(err)
		cancel()
		b := <-builtCh
		if b.err != nil {
			return b.stats, "", fmt.Errorf("pushing %s: %w (layer: %v)", req.Target, err, b.err)
		}
		return b.stats, "", fmt.Errorf("pushing %s: %w", req.Target, err)
	}
	b := <-builtCh
	if b.err != nil {
		return b.stats, "", b.err
	}
	d, err := img.Digest()
	if err != nil {
		return b.stats, "", err
	}
	return b.stats, d.String(), nil
}

func runCaptured(ctx context.Context, ex Execer, cmd []string, stdin io.Reader, stdout io.Writer) error {
	stderr := &limitedBuffer{max: 4096}
	if err := ex.Exec(ctx, cmd, stdin, stdout, stderr); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

// limitedBuffer keeps the first max bytes written to it, for error messages.
type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.max - l.buf.Len(); room > 0 {
		if len(p) > room {
			l.buf.Write(p[:room])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string { return strings.TrimSpace(l.buf.String()) }
