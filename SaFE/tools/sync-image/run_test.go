/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"io"
	"path"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/types"
	"github.com/distribution/reference"
)

func TestComputeDestSuffix(t *testing.T) {
	named, err := reference.ParseNormalizedNamed("docker.io/library/busybox:latest")
	if err != nil {
		t.Fatal(err)
	}
	dref, err := docker.NewReference(named)
	if err != nil {
		t.Fatal(err)
	}

	// scoped keeps the full docker reference
	if got := computeDestSuffix(dref, "", true); got != "docker.io/library/busybox:latest" {
		t.Errorf("scoped docker = %q", got)
	}
	// unscoped reduces to the base name
	if got := computeDestSuffix(dref, "", false); got != "busybox:latest" {
		t.Errorf("unscoped docker = %q", got)
	}

	// directory reference whose path equals the base path yields the base name
	base := t.TempDir()
	dirRef, err := directory.Transport.ParseReference(filepath.Join(base, "img"))
	if err != nil {
		t.Fatal(err)
	}
	full := dirRef.StringWithinTransport()
	if got := computeDestSuffix(dirRef, full, true); got != path.Base(full) {
		t.Errorf("dir empty-suffix = %q, want %q", got, path.Base(full))
	}
}

func TestApplyProgress(t *testing.T) {
	e := &UpstreamEvent{Data: make(map[string]types.ProgressProperties)}

	if !e.applyProgress(types.ProgressProperties{Event: types.ProgressEventSkipped, Artifact: types.BlobInfo{Digest: "sha256:a"}}) {
		t.Error("skipped event should be terminal")
	}
	if e.SkipLayerCount != 1 || e.SyncLayerCount != 1 {
		t.Errorf("skip counters = %d/%d", e.SkipLayerCount, e.SyncLayerCount)
	}

	if !e.applyProgress(types.ProgressProperties{Event: types.ProgressEventDone, Artifact: types.BlobInfo{Digest: "sha256:b"}}) {
		t.Error("done event should be terminal")
	}
	if e.ComplexLayerCount != 1 || e.SyncLayerCount != 2 {
		t.Errorf("done counters = %d/%d", e.ComplexLayerCount, e.SyncLayerCount)
	}

	// a non-terminal event updates data but not counters
	if e.applyProgress(types.ProgressProperties{Event: types.ProgressEventNewArtifact, Artifact: types.BlobInfo{Digest: "sha256:c"}}) {
		t.Error("new-artifact event should not be terminal")
	}
	if len(e.Data) != 3 {
		t.Errorf("data entries = %d, want 3", len(e.Data))
	}
}

// newSyncOptions returns a fully wired docker->docker syncOptions for run tests.
func newSyncOptions() *syncOptions {
	return &syncOptions{
		Global:              &GlobalOptions{InsecurePolicy: true},
		DeprecatedTLSVerify: &DeprecatedTLSVerifyOption{},
		SrcImage: &ImageOptions{
			DockerImageOptions: DockerImageOptions{Shared: &SharedImageOptions{}},
		},
		DestImage: &ImageDestOptions{
			ImageOptions: &ImageOptions{
				DockerImageOptions: DockerImageOptions{Shared: &SharedImageOptions{}},
			},
		},
		RetryOpts:   &reTryOptions{MaxRetry: 0, Delay: 0},
		Source:      TransportDocker,
		Destination: TransportDocker,
	}
}

func TestRunArgsError(t *testing.T) {
	opts := newSyncOptions()
	if err := opts.run([]string{"only-one"}, io.Discard); err == nil {
		t.Fatal("expected error for wrong argument count")
	}
}

func TestRunInvalidSource(t *testing.T) {
	opts := newSyncOptions()
	opts.Source = "bogus"
	args := []string{"docker.io/library/busybox:latest", "registry.example.com/repo/busybox:latest"}
	if err := opts.run(args, io.Discard); err == nil {
		t.Fatal("expected error for invalid source transport")
	}
}

func TestRunConflictingSignOptions(t *testing.T) {
	opts := newSyncOptions()
	opts.SignPassphraseFile = "/tmp/passphrase"
	opts.SignByFingerprint = "fingerprint"
	opts.SignBySigstorePrivateKey = "/tmp/key"
	args := []string{"docker.io/library/busybox:latest", "registry.example.com/repo/busybox:latest"}
	if err := opts.run(args, io.Discard); err == nil {
		t.Fatal("expected error for conflicting signing options")
	}
}

func TestRunDryRun(t *testing.T) {
	opts := newSyncOptions()
	opts.DryRun = true
	// destination ends with the unscoped suffix so destinationReference uses it directly
	args := []string{"docker.io/library/busybox:latest", "registry.example.com/repo/busybox:latest"}
	if err := opts.run(args, io.Discard); err != nil {
		t.Fatalf("run dry-run: %v", err)
	}
}
