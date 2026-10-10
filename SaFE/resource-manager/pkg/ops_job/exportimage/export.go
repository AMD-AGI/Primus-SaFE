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
// made for this export, with a short-lived token minted with a credential limited to the
// staging project (never the platform's own).
// This package issues that token, starts the program through pods/exec (the token
// travels on its standard input), and then puts the image together in the registry
// itself: the base image's layers and the staged layer are mounted, and only the config
// and manifest are written. Nothing the container reports is trusted beyond where to
// find its layer: the base is the digest the runtime reported, and what is published is
// what the registry holds. A container on a kubelet and one on a virtual kubelet take the
// same path.
package exportimage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/ops_job/exportimage/agent"
)

var (
	// ErrNotRoot is returned for a container that does not run as root.
	ErrNotRoot = agent.ErrNotRoot
	// ErrNoStagingCredential is returned when no credential limited to the staging
	// project is configured for the registry.
	ErrNoStagingCredential = errors.New("no registry credential limited to the staging project is configured, " +
		"and the platform's own credential is never handed to a container")
	// ErrPredatesSaveImage is returned for a container started before the platform
	// recorded its files and installed the export program.
	ErrPredatesSaveImage = errors.New("the container was started before it could be saved as an image; " +
		"restart the workload, then save it again")
)

const (
	// maxLayerSize bounds the layer a container may stage.
	maxLayerSize = 500 << 30
	// stopMargin is how long before the job's deadline the agent stops, so that its
	// failure, not the deadline, is what is reported.
	stopMargin = time.Minute
	// defaultExportTime bounds an export whose job has no deadline.
	defaultExportTime = 12 * time.Hour
	// maxMessage bounds one line the agent prints, and maxOutput all of them.
	maxMessage = 64 << 10
	maxOutput  = 1 << 20
	// maxProbe bounds what the probe may print: a few short lines.
	maxProbe = 4096
	// minRenewInterval is how often the agent may ask for a new token: a token lives for
	// many minutes, so a container that asks more often is not uploading.
	minRenewInterval = 20 * time.Second
)

// probeScript reports what the export depends on.
var probeScript = `echo "uid=$(id -u)"
if [ -x ` + agent.BinaryPath + ` ]; then
  echo agent=1
  p=$(` + agent.BinaryPath + ` protocol 2>/dev/null) && echo "protocol=$p"
fi
if [ -s ` + agent.BaselinePath + ` ]; then echo baseline=1; fi
if [ -e ` + agent.BaselinePath + agent.RecordingSuffix + ` ]; then echo recording=1; fi
`

// Probe is what the container reported about itself.
type Probe struct {
	UID       int
	Agent     bool
	Protocol  int
	Baseline  bool
	Recording bool
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
		case "protocol":
			p.Protocol, _ = strconv.Atoi(v)
		case "baseline":
			p.Baseline = true
		case "recording":
			p.Recording = true
		}
	}
	return p
}

// Check refuses a container the export cannot save.
func (p Probe) Check() error {
	switch {
	case !p.Agent || p.Protocol != agent.ProtocolVersion:
		return ErrPredatesSaveImage
	case !p.Baseline && p.Recording:
		return agent.ErrRecording
	case !p.Baseline:
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
	// Registry is the configured registry host. Staging and Target must be in it: no
	// credential is used, and no token minted, for any other.
	Registry string
	// ImageID is the container status imageID: the digest the container was started from.
	ImageID string
	// Staging is the repository, made for this export alone, that the container uploads
	// its layer to.
	Staging name.Repository
	// Target is where the saved image is published, in the same registry as Staging: the
	// image is put together there by mounting blobs, which works within one registry only.
	Target name.Tag
	// Keychain and Transport are this process's access to the registries.
	Keychain authn.Keychain
	// StagingKeychain holds the credential the container's upload token is minted with.
	// The registry gives a token the power of the account that minted it, whatever
	// repository it names, so this must be an account that can push to the staging
	// project alone; it is never Keychain's.
	StagingKeychain authn.Keychain
	Transport       http.RoundTripper
	// CA is a PEM bundle the container trusts for the registry on top of its system roots;
	// empty for a registry those roots trust.
	CA       []byte
	Platform v1.Platform
	Logf     func(format string, args ...any)
}

// Result describes a published image.
type Result struct {
	// Digest is the manifest digest the registry reports.
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
	if req.Registry == "" || req.Staging.RegistryStr() != req.Registry || req.Target.RegistryStr() != req.Registry {
		return nil, fmt.Errorf("the staging repository %s and the image %s are not both in the configured registry %q",
			req.Staging, req.Target, req.Registry)
	}
	probeOut := &limitedBuffer{max: maxProbe}
	if err := runCaptured(ctx, req.Exec, []string{"sh", "-c", probeScript}, probeOut); err != nil {
		return nil, fmt.Errorf("probing the container: %w", err)
	}
	if probeOut.overflow {
		return nil, fmt.Errorf("probing the container: it printed more than %d bytes", maxProbe)
	}
	if err := ParseProbe(probeOut.buf.String()).Check(); err != nil {
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
	base, baseUsed, err := ResolveBase(ctx, BaseCandidates(baseRef, req.Target.RegistryStr()), opts...)
	if err != nil {
		return nil, err
	}

	auth, err := req.Keychain.Resolve(req.Staging)
	if err != nil {
		return nil, err
	}
	if req.StagingKeychain == nil {
		return nil, ErrNoStagingCredential
	}
	stagingAuth, err := req.StagingKeychain.Resolve(req.Staging)
	if err != nil {
		return nil, err
	}
	if stagingAuth == authn.Anonymous {
		return nil, ErrNoStagingCredential
	}
	mint := func(ctx context.Context) (*PushToken, error) {
		return IssuePushToken(ctx, req.Staging, stagingAuth, req.Transport)
	}
	token, err := mint(ctx)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(defaultExportTime)
	if d, ok := ctx.Deadline(); ok {
		deadline = d.Add(-stopMargin)
	}
	if !deadline.After(time.Now()) {
		return nil, fmt.Errorf("the job has no time left to save the container")
	}
	logf("base %s; container uploads to %s until %s", baseUsed, req.Staging, deadline.UTC().Format(time.RFC3339))

	staged, resp, err := runAgent(ctx, req, token, mint, deadline)
	if err != nil {
		return nil, err
	}
	logf("container: %d changed, %d deleted, %d skipped, %d vanished, %d resized; layer %s, %d bytes; "+
		"%d token renewals, %d retried requests; peak memory %d MiB",
		resp.Changed, resp.Deleted, resp.Skipped, resp.Vanished, resp.Resized, staged.Digest, staged.Size,
		resp.Renewals, resp.Retries, resp.PeakMemory>>20)
	if resp.Unsettled > 0 {
		logf("%d directories had changed before the container recorded them; "+
			"files deleted from them before then are still in the image", resp.Unsettled)
	}
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

	digest, err := Assemble(ctx, base, baseUsed.Context(), req.Staging, staged, req.Target, auth, req.Transport)
	if err != nil {
		return nil, err
	}
	logf("put together %s@%s", req.Target, digest)
	return &Result{Digest: digest.String(), Base: baseUsed.String(), Layer: *resp}, nil
}

// runAgent runs the agent and answers its requests for new tokens, until it reports its
// layer. What it prints is bounded and checked; it only ever gets tokens for the staging
// repository, and not more often than one every minRenewInterval.
func runAgent(ctx context.Context, req Request, token *PushToken, mint func(context.Context) (*PushToken, error),
	deadline time.Time) (StagedLayer, *agent.Response, error) {
	first, err := json.Marshal(agent.Request{
		Registry:    req.Staging.RegistryStr(),
		Repository:  req.Staging.RepositoryStr(),
		Token:       token.Value,
		TokenExpiry: token.Expiry,
		CA:          string(req.CA),
		Deadline:    deadline,
		MaxSize:     maxLayerSize,
	})
	if err != nil {
		return StagedLayer{}, nil, err
	}
	// The agent stops at the deadline on its own; this only bounds the wait for it.
	ctx, cancel := context.WithDeadline(ctx, deadline.Add(stopMargin/2))
	defer cancel()

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	stderr := &limitedBuffer{max: 4096}
	execDone := make(chan error, 1)
	go func() {
		err := req.Exec.Exec(ctx, []string{agent.BinaryPath, "export"}, inR, outW, stderr)
		// The reader of its output sees the end of it; a write to its input fails.
		outW.Close()
		inR.CloseWithError(errors.New("the agent exited"))
		execDone <- err
	}()
	send := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = inW.Write(append(b, '\n'))
		return err
	}
	secrets := []string{token.Value}
	result, protoErr := func() (*agent.Response, error) {
		if err := send(json.RawMessage(first)); err != nil {
			return nil, nil // the agent exited; its error says why
		}
		lines := bufio.NewScanner(io.LimitReader(outR, maxOutput))
		lines.Buffer(make([]byte, 4096), maxMessage)
		var lastRenew time.Time
		for lines.Scan() {
			var m agent.Message
			if err := json.Unmarshal(lines.Bytes(), &m); err != nil {
				return nil, fmt.Errorf("the container's answer cannot be read")
			}
			switch {
			case m.Result != nil:
				_ = inW.Close()
				return m.Result, nil
			case m.Renew:
				var g agent.Grant
				if wait := minRenewInterval - time.Since(lastRenew); wait > 0 {
					g.Error = fmt.Sprintf("a new token was granted %s ago", time.Since(lastRenew).Round(time.Second))
				} else if t, err := mint(ctx); err != nil {
					g.Error = "the registry did not grant a new token: " + err.Error()
				} else {
					lastRenew = time.Now()
					secrets = append(secrets, t.Value)
					g.Token, g.Expiry = t.Value, t.Expiry
				}
				if err := send(g); err != nil {
					return nil, nil
				}
			default:
				return nil, fmt.Errorf("the container's answer cannot be read")
			}
		}
		if errors.Is(lines.Err(), bufio.ErrTooLong) {
			return nil, fmt.Errorf("the container's answer cannot be read")
		}
		// The agent exited without a result; its error says why.
		return nil, nil
	}()
	_ = inW.CloseWithError(errors.New("the export is over"))
	if protoErr != nil {
		cancel()
	}
	_, _ = io.Copy(io.Discard, outR)
	execErr := <-execDone
	msg := stderr.String()
	for _, s := range secrets {
		msg = strings.ReplaceAll(msg, s, "<token>")
	}
	switch {
	case protoErr != nil:
		return StagedLayer{}, nil, protoErr
	case execErr != nil:
		return StagedLayer{}, nil, fmt.Errorf("saving in the container: %w: %s", execErr, msg)
	case result == nil:
		return StagedLayer{}, nil, fmt.Errorf("the container's answer cannot be read")
	}
	d, err := v1.NewHash(result.Digest)
	if err != nil {
		return StagedLayer{}, nil, fmt.Errorf("the container reported an invalid layer digest")
	}
	diffID, err := v1.NewHash(result.DiffID)
	if err != nil {
		return StagedLayer{}, nil, fmt.Errorf("the container reported an invalid layer diff ID")
	}
	return StagedLayer{Digest: d, DiffID: diffID, Size: result.Size}, result, nil
}

func runCaptured(ctx context.Context, ex Execer, cmd []string, stdout io.Writer) error {
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
