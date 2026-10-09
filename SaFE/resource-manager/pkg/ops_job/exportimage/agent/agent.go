/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package agent is what runs inside the container being saved. It finds what changed
// since the platform launcher handed over to the user, writes that as one layer and
// uploads the layer to a staging repository with a short-lived token minted with a
// credential limited to the staging project. The image itself is put together by the controller, from what the
// registry holds; nothing this package reports is trusted beyond locating the layer.
package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/stream"
)

const (
	// BinaryPath is where the platform's init container puts this program.
	BinaryPath = "/shared-data/save-image"
	// BaselinePath is where the launcher records the files the container started with.
	// It is on the shared volume, which the export leaves out like every other mount.
	BaselinePath = "/shared-data/save-image.base"
	// LauncherRunFile is where the platform launcher writes the user's entry point,
	// relative to the container's working directory, once its own bootstrap (driver
	// builds, sshd, socat, certificates) is done and immediately before it starts the
	// entry point. Its change time is the boundary between what the platform wrote and
	// what the user did.
	LauncherRunFile = ".run.sh"
)

var (
	// ErrNoBaseline is returned for a container that has no record of the files it
	// started with: it was started before the platform recorded them.
	ErrNoBaseline = errors.New("this container has no record of the files its image held")
	// ErrNotRoot is returned for a container that does not run as root. Such a process
	// cannot read every file of its own root file system, so the export would silently
	// miss some.
	ErrNotRoot = errors.New("the container does not run as root")
)

var errUploadStopped = errors.New("upload stopped")

// Request is what the controller sends on the agent's standard input. It never appears
// in the command line, the environment or the Pod spec.
type Request struct {
	// Registry is the host (and port) of the registry.
	Registry string `json:"registry"`
	// Repository is the staging repository, without the registry.
	Repository string `json:"repository"`
	// Token is a short-lived registry bearer token for Repository.
	Token string `json:"token"`
	// CA is the PEM bundle the registry's certificate is checked against. When empty,
	// the container's system roots are used.
	CA string `json:"ca,omitempty"`
	// Deadline is when the token expires; the agent gives up then.
	Deadline time.Time `json:"deadline"`
}

// Response is what the agent prints on success.
type Response struct {
	// Digest, DiffID and Size describe the uploaded layer blob.
	Digest          string   `json:"digest"`
	DiffID          string   `json:"diffID"`
	Size            int64    `json:"size"`
	Changed         int      `json:"changed"`
	Deleted         int      `json:"deleted"`
	Skipped         int      `json:"skipped"`
	Vanished        int      `json:"vanished"`
	Resized         int      `json:"resized"`
	DroppedPackages []string `json:"droppedPackages,omitempty"`
}

// Env is the container the agent reads. Only tests set anything but the defaults.
type Env struct {
	// Root is the root of the file system to save ("/").
	Root string
	// Baseline is the launcher's record of the files the container started with.
	Baseline string
	// RunFile is the launcher's entry point file.
	RunFile string
	// Mountinfo is the container's mount table (/proc/self/mountinfo).
	Mountinfo string
	UID       int
	// Dial replaces the network dialer; tests use it to reach a registry by name.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// Export computes the layer and uploads it.
func Export(ctx context.Context, req Request, env Env) (*Response, error) {
	if env.UID != 0 {
		return nil, fmt.Errorf("%w (uid %d): saving it would leave out the files it cannot read", ErrNotRoot, env.UID)
	}
	if req.Registry == "" || req.Repository == "" || req.Token == "" || req.Deadline.IsZero() {
		return nil, fmt.Errorf("the request names no registry, repository, token or deadline")
	}
	ctx, cancel := context.WithDeadline(ctx, req.Deadline)
	defer cancel()

	bf, err := os.Open(env.Baseline)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoBaseline
	}
	if err != nil {
		return nil, fmt.Errorf("reading the record of the image's files: %w", err)
	}
	base, err := ReadBaseline(bf)
	bf.Close()
	if err != nil {
		return nil, err
	}
	run, err := os.Lstat(env.RunFile)
	if err != nil {
		return nil, fmt.Errorf("the platform launcher's %s is not in the working directory, so what the launcher "+
			"installed cannot be told apart from the user's changes: %w", LauncherRunFile, err)
	}
	since, ok := ctimeOf(run)
	if !ok {
		return nil, fmt.Errorf("cannot read the change time of %s", LauncherRunFile)
	}

	filter := NewFilter(ParseMountPoints(env.Mountinfo))
	current, err := Walk(env.Root, filter)
	if err != nil {
		return nil, err
	}
	changes := ComputeChanges(base, current, since, filter)

	repo, err := name.NewRepository(req.Registry+"/"+req.Repository, name.StrictValidation)
	if err != nil {
		return nil, fmt.Errorf("invalid staging repository: %w", err)
	}
	tr, err := httpsOnlyTransport(req.CA, env.Dial)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	type built struct {
		stats LayerStats
		err   error
	}
	done := make(chan built, 1)
	go func() {
		st, err := WriteLayer(pw, env.Root, changes.Changed, changes.Deleted, ImageContains(base, changes))
		pw.CloseWithError(err)
		done <- built{st, err}
	}()
	layer := stream.NewLayer(pr)
	err = remote.WriteLayer(repo, layer,
		remote.WithContext(ctx),
		remote.WithAuth(authn.FromConfig(authn.AuthConfig{RegistryToken: req.Token})),
		remote.WithTransport(tr))
	pr.CloseWithError(errUploadStopped)
	b := <-done
	if b.err != nil && !errors.Is(b.err, errUploadStopped) {
		// The upload failed because the layer could not be written.
		return nil, fmt.Errorf("writing the layer: %w", b.err)
	}
	if err != nil {
		return nil, fmt.Errorf("uploading the layer: %w", err)
	}
	digest, err := layer.Digest()
	if err != nil {
		return nil, err
	}
	diffID, err := layer.DiffID()
	if err != nil {
		return nil, err
	}
	size, err := layer.Size()
	if err != nil {
		return nil, err
	}
	return &Response{
		Digest:          digest.String(),
		DiffID:          diffID.String(),
		Size:            size,
		Changed:         len(changes.Changed),
		Deleted:         len(changes.Deleted),
		Skipped:         changes.Skipped,
		Vanished:        b.stats.Vanished,
		Resized:         b.stats.Resized,
		DroppedPackages: b.stats.DroppedPackages,
	}, nil
}

// httpsOnlyTransport trusts only the given CA (or the system roots when there is none)
// and refuses to send anything over plain HTTP, so the token never leaves unencrypted
// and an unreachable registry is never retried insecurely.
func httpsOnlyTransport(caPEM string, dial func(ctx context.Context, network, addr string) (net.Conn, error)) (http.RoundTripper, error) {
	var pool *x509.CertPool
	if caPEM != "" {
		pool = x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caPEM)) {
			return nil, fmt.Errorf("the registry CA holds no usable certificate")
		}
	}
	t := remote.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	if dial != nil {
		t.DialContext = dial
	}
	return httpsOnly{t}, nil
}

type httpsOnly struct{ inner http.RoundTripper }

func (h httpsOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" {
		return nil, fmt.Errorf("refusing to reach the registry over %s", r.URL.Scheme)
	}
	return h.inner.RoundTrip(r)
}
