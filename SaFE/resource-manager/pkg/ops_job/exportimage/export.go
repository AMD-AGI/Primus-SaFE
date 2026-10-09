/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package exportimage saves a running container as a new image without moving its data
// through the control plane.
//
// The container does the heavy part itself. A static program the platform puts on the
// Pod's shared volume (see package agent) finds what changed since the launcher handed
// over to the user, measures deletions against the list of files the launcher recorded
// when the container started, and uploads that as one layer blob to a staging repository
// made for this export, with a short-lived token that can push there and nowhere else.
// This package issues that token, starts the program through pods/exec (the token
// travels on its standard input), and then puts the image together in the registry
// itself: the base image's layers and the staged layer are mounted, and only the config
// and manifest are written. Nothing the container reports is trusted beyond where to
// find its layer: the base is the digest the runtime reported, and what is published is
// what the registry holds. A container on a kubelet and one on a virtual kubelet take the
// same path.
package exportimage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/ops_job/exportimage/agent"
)

var (
	// ErrNotRoot is returned for a container that does not run as root.
	ErrNotRoot = agent.ErrNotRoot
	// ErrPredatesSaveImage is returned for a container started before the platform
	// recorded its files and installed the export program.
	ErrPredatesSaveImage = errors.New("the container was started before it could be saved as an image; " +
		"restart the workload, then save it again")
)

const (
	// maxLayerSize bounds the layer a container may stage.
	maxLayerSize = 500 << 30
	// tokenMargin is how long before its token expires the agent stops.
	tokenMargin = time.Minute
	// maxResponse bounds what the agent may print.
	maxResponse = 64 << 10
)

// probeScript reports what the export depends on.
var probeScript = `echo "uid=$(id -u)"
if [ -x ` + agent.BinaryPath + ` ]; then echo agent=1; fi
if [ -s ` + agent.BaselinePath + ` ]; then echo baseline=1; fi
`

// Probe is what the container reported about itself.
type Probe struct {
	UID      int
	Agent    bool
	Baseline bool
}

// ParseProbe parses probeScript's output.
func ParseProbe(out string) Probe {
	p := Probe{UID: -1}
	for _, line := range strings.Split(out, "\n") {
		k, v, _ := strings.Cut(strings.TrimSpace(line), "=")
		switch k {
		case "uid":
			if n, err := strconv.Atoi(v); err == nil {
				p.UID = n
			}
		case "agent":
			p.Agent = true
		case "baseline":
			p.Baseline = true
		}
	}
	return p
}

// Check refuses a container the export cannot save.
func (p Probe) Check() error {
	switch {
	case !p.Agent || !p.Baseline:
		return ErrPredatesSaveImage
	case p.UID < 0:
		return fmt.Errorf("cannot determine the container's user id")
	case p.UID != 0:
		return fmt.Errorf("%w (uid %d): saving it would leave out the files it cannot read", ErrNotRoot, p.UID)
	}
	return nil
}

// Request is one export.
type Request struct {
	Exec Execer
	// ImageID is the container status imageID: the digest the container was started from.
	ImageID string
	// Staging is the repository, made for this export alone, that the container uploads
	// its layer to.
	Staging name.Repository
	// Assembly is where the image is put together; it is in the staging registry.
	Assembly name.Tag
	// Target is where the saved image is published. When it is in another registry than
	// Assembly, that registry's replication carries the image there, and Export waits up
	// to ReplicationTimeout for it to arrive.
	Target             name.Tag
	ReplicationTimeout time.Duration
	// PollInterval is how often the target is checked while waiting for replication.
	PollInterval time.Duration
	// Keychain and Transport are this process's access to the registries.
	Keychain  authn.Keychain
	Transport http.RoundTripper
	// CA is the PEM bundle the container checks the staging registry's certificate
	// against; empty for a registry its system roots trust.
	CA       []byte
	Platform v1.Platform
	Logf     func(format string, args ...any)
}

// Result describes a published image.
type Result struct {
	// Digest is the manifest digest the target registry reports.
	Digest string
	Base   string
	Layer  agent.Response
}

// Export saves the container as Target.
func Export(ctx context.Context, req Request) (*Result, error) {
	logf := req.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	var probeOut bytes.Buffer
	if err := runCaptured(ctx, req.Exec, []string{"sh", "-c", probeScript}, &probeOut); err != nil {
		return nil, fmt.Errorf("probing the container: %w", err)
	}
	if err := ParseProbe(probeOut.String()).Check(); err != nil {
		return nil, err
	}

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
	base, baseUsed, err := ResolveBase(ctx, BaseCandidates(baseRef, req.Assembly.RegistryStr()), opts...)
	if err != nil {
		return nil, err
	}

	auth, err := req.Keychain.Resolve(req.Staging)
	if err != nil {
		return nil, err
	}
	token, err := IssuePushToken(ctx, req.Staging, auth, req.Transport)
	if err != nil {
		return nil, err
	}
	deadline := token.Expiry.Add(-tokenMargin)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if !deadline.After(time.Now()) {
		return nil, fmt.Errorf("the registry's push token expires too soon (%s)", token.Expiry.UTC().Format(time.RFC3339))
	}
	logf("base %s; container uploads to %s until %s", baseUsed, req.Staging, deadline.UTC().Format(time.RFC3339))

	staged, resp, err := runAgent(ctx, req, token, deadline)
	if err != nil {
		return nil, err
	}
	logf("container: %d changed, %d deleted, %d skipped, %d vanished, %d resized; layer %s, %d bytes",
		resp.Changed, resp.Deleted, resp.Skipped, resp.Vanished, resp.Resized, staged.Digest, staged.Size)
	if len(resp.DroppedPackages) > 0 {
		logf("dpkg records left out (their files are not in the image): %s", strings.Join(resp.DroppedPackages, " "))
	}

	// The layer is located by the container's report, and checked against the registry.
	c, err := newRegistryClient(ctx, req.Staging.Registry, auth, req.Transport, req.Staging.Scope(transport.PullScope))
	if err != nil {
		return nil, err
	}
	size, ok, err := c.BlobSize(ctx, req.Staging, staged.Digest)
	switch {
	case err != nil:
		return nil, fmt.Errorf("checking the uploaded layer: %w", err)
	case !ok:
		return nil, fmt.Errorf("the container reported layer %s, which is not in %s", staged.Digest, req.Staging)
	case size != staged.Size:
		return nil, fmt.Errorf("layer %s in %s has %d bytes, not the %d the container reported", staged.Digest, req.Staging, size, staged.Size)
	case size > maxLayerSize:
		return nil, fmt.Errorf("the layer has %d bytes, more than the %d a saved image may add", size, int64(maxLayerSize))
	}

	digest, err := Assemble(ctx, base, baseUsed.Context(), req.Staging, staged, req.Assembly, auth, req.Transport)
	if err != nil {
		return nil, err
	}
	logf("put together %s@%s", req.Assembly, digest)
	if req.Target.RegistryStr() != req.Assembly.RegistryStr() || req.Target.RepositoryStr() != req.Assembly.RepositoryStr() {
		if err := waitForImage(ctx, req.Target, digest, req.ReplicationTimeout, req.PollInterval, opts); err != nil {
			return nil, err
		}
		logf("replicated to %s", req.Target)
	}
	return &Result{Digest: digest.String(), Base: baseUsed.String(), Layer: *resp}, nil
}

func runAgent(ctx context.Context, req Request, token *PushToken, deadline time.Time) (StagedLayer, *agent.Response, error) {
	in, err := json.Marshal(agent.Request{
		Registry:   req.Staging.RegistryStr(),
		Repository: req.Staging.RepositoryStr(),
		Token:      token.Value,
		CA:         string(req.CA),
		Deadline:   deadline,
	})
	if err != nil {
		return StagedLayer{}, nil, err
	}
	// The agent stops at the deadline on its own; this only bounds the wait for it.
	ctx, cancel := context.WithDeadline(ctx, deadline.Add(tokenMargin/2))
	defer cancel()
	stdout := &limitedBuffer{max: maxResponse}
	stderr := &limitedBuffer{max: 4096}
	err = req.Exec.Exec(ctx, []string{agent.BinaryPath, "export"}, bytes.NewReader(in), stdout, stderr)
	msg := strings.ReplaceAll(stderr.String(), token.Value, "<token>")
	if err != nil {
		return StagedLayer{}, nil, fmt.Errorf("saving in the container: %w: %s", err, msg)
	}
	var resp agent.Response
	if err := json.Unmarshal(stdout.buf.Bytes(), &resp); err != nil || stdout.overflow {
		return StagedLayer{}, nil, fmt.Errorf("the container's answer cannot be read")
	}
	d, err := v1.NewHash(resp.Digest)
	if err != nil {
		return StagedLayer{}, nil, fmt.Errorf("the container reported an invalid layer digest")
	}
	diffID, err := v1.NewHash(resp.DiffID)
	if err != nil {
		return StagedLayer{}, nil, fmt.Errorf("the container reported an invalid layer diff ID")
	}
	return StagedLayer{Digest: d, DiffID: diffID, Size: resp.Size}, &resp, nil
}

// waitForImage waits for target to hold the manifest digest.
func waitForImage(ctx context.Context, target name.Tag, digest v1.Hash, timeout, interval time.Duration, opts []remote.Option) error {
	if timeout <= 0 {
		return fmt.Errorf("%s is in another registry than the one the image was put together in, and no replication wait is configured", target)
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	opts = append(append([]remote.Option{}, opts...), remote.WithContext(ctx))
	last := "not there yet"
	for {
		desc, err := remote.Head(target, opts...)
		switch {
		case err != nil:
			last = err.Error()
		case desc.Digest == digest:
			return nil
		default:
			last = fmt.Sprintf("it holds %s", desc.Digest)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the saved image %s@%s did not arrive at %s within %s (check the registry's replication rule): %s",
				target.Context(), digest, target, timeout, last)
		case <-time.After(interval):
		}
	}
}

func runCaptured(ctx context.Context, ex Execer, cmd []string, stdout *bytes.Buffer) error {
	stderr := &limitedBuffer{max: 4096}
	if err := ex.Exec(ctx, cmd, nil, stdout, stderr); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

// limitedBuffer keeps the first max bytes written to it.
type limitedBuffer struct {
	buf      bytes.Buffer
	max      int
	overflow bool
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	room := l.max - l.buf.Len()
	if len(p) > room {
		l.overflow = true
		if room > 0 {
			l.buf.Write(p[:room])
		}
		return len(p), nil
	}
	l.buf.Write(p)
	return len(p), nil
}

func (l *limitedBuffer) String() string { return strings.TrimSpace(l.buf.String()) }
