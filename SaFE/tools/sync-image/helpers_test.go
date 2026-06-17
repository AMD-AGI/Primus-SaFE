/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestParseManifestFormat(t *testing.T) {
	cases := map[string]string{
		"oci":  imgspecv1.MediaTypeImageManifest,
		"v2s1": manifest.DockerV2Schema1SignedMediaType,
		"v2s2": manifest.DockerV2Schema2MediaType,
	}
	for in, want := range cases {
		got, err := parseManifestFormat(in)
		if err != nil || got != want {
			t.Errorf("parseManifestFormat(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := parseManifestFormat("bogus"); err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestParseCreds(t *testing.T) {
	if _, _, err := parseCreds(""); err == nil {
		t.Error("expected error for empty creds")
	}
	if _, _, err := parseCreds(":pass"); err == nil {
		t.Error("expected error for empty username")
	}
	u, p, err := parseCreds("user:pass")
	if err != nil || u != "user" || p != "pass" {
		t.Errorf("parseCreds(user:pass) = %q,%q,%v", u, p, err)
	}
	u, p, err = parseCreds("user")
	if err != nil || u != "user" || p != "" {
		t.Errorf("parseCreds(user) = %q,%q,%v", u, p, err)
	}
}

func TestGetDockerAuth(t *testing.T) {
	auth, err := getDockerAuth("user:pass")
	if err != nil || auth.Username != "user" || auth.Password != "pass" {
		t.Errorf("getDockerAuth = %+v, %v", auth, err)
	}
	if _, err := getDockerAuth(""); err == nil {
		t.Error("expected error for empty creds")
	}
}

func TestParseStr(t *testing.T) {
	got := parseStr([]string{"a\nb", " c\t", "\td\n"})
	want := []string{"ab", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("parseStr len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseStr[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNoteCloseFailure(t *testing.T) {
	// nil base error wraps the close error
	if err := noteCloseFailure(nil, "closing", errors.New("ce")); err == nil ||
		!strings.Contains(err.Error(), "closing") {
		t.Errorf("noteCloseFailure(nil) = %v", err)
	}
	// existing error is preserved and annotated
	base := errors.New("orig")
	err := noteCloseFailure(base, "closing", errors.New("ce"))
	if err == nil || !strings.Contains(err.Error(), "orig") {
		t.Errorf("noteCloseFailure(base) = %v", err)
	}
}

func TestReqValid(t *testing.T) {
	if err := reqValid("", "dir"); err == nil {
		t.Error("expected error for empty source")
	}
	if err := reqValid("bogus", "dir"); err == nil {
		t.Error("expected error for invalid source")
	}
	if err := reqValid(TransportDocker, ""); err == nil {
		t.Error("expected error for empty destination")
	}
	if err := reqValid(TransportDocker, "bogus"); err == nil {
		t.Error("expected error for invalid destination")
	}
	if err := reqValid(TransportDir, TransportDir); err == nil {
		t.Error("expected error for dir-to-dir sync")
	}
	if err := reqValid(TransportDocker, TransportDir); err != nil {
		t.Errorf("reqValid(docker,dir) = %v, want nil", err)
	}
}

func TestGlobalNewSystemContext(t *testing.T) {
	ctx := (&GlobalOptions{OverrideArch: "amd64"}).newSystemContext()
	if ctx.ArchitectureChoice != "amd64" {
		t.Errorf("ArchitectureChoice = %q", ctx.ArchitectureChoice)
	}
	if ctx.DockerInsecureSkipTLSVerify != types.OptionalBoolUndefined {
		t.Errorf("expected undefined TLS verify when TLSVerify is false")
	}
	// TLSVerify true sets the skip flag
	ctx = (&GlobalOptions{TLSVerify: true}).newSystemContext()
	if ctx.DockerInsecureSkipTLSVerify == types.OptionalBoolUndefined {
		t.Errorf("expected TLS verify flag to be set")
	}
}

func TestGetPolicyContext(t *testing.T) {
	// insecure policy always succeeds
	pc, err := (&GlobalOptions{InsecurePolicy: true}).getPolicyContext()
	if err != nil || pc == nil {
		t.Fatalf("getPolicyContext(insecure) = %v", err)
	}
	_ = pc.Destroy()

	// a non-existent policy file returns an error
	if _, err := (&GlobalOptions{PolicyPath: filepath.Join(t.TempDir(), "nope.json")}).getPolicyContext(); err == nil {
		t.Error("expected error for missing policy file")
	}
}

func TestCommandTimeoutContext(t *testing.T) {
	ctx, cancel := (&GlobalOptions{}).commandTimeoutContext()
	cancel()
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		t.Error("expected no deadline when timeout is zero")
	}

	ctx, cancel = (&GlobalOptions{CommandTimeout: time.Second}).commandTimeoutContext()
	defer cancel()
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		t.Error("expected deadline when timeout is set")
	}
}

func TestWarnIfUsed(t *testing.T) {
	(&DeprecatedTLSVerifyOption{TLSVerify: true}).warnIfUsed([]string{"--src-tls-verify"})
	(&DeprecatedTLSVerifyOption{TLSVerify: false}).warnIfUsed([]string{"--src-tls-verify"})
}

func TestWarnAboutIneffectiveOptions(t *testing.T) {
	opts := &ImageDestOptions{
		DirForceCompression:   true,
		DirForceDecompression: true,
		ImageDestFlagPrefix:   "dest-",
	}
	// non-dir transport triggers the warnings
	opts.warnAboutIneffectiveOptions(docker.Transport)
	// dir transport is fine
	opts.warnAboutIneffectiveOptions(directory.Transport)
}

func TestNewClient(t *testing.T) {
	c := NewClient()
	if c == nil || c.Timeout != DefaultTimeout {
		t.Errorf("NewClient timeout = %v", c.Timeout)
	}
}

// newImageOpts returns a minimally valid *ImageOptions for newSystemContext tests.
func newImageOpts() *ImageOptions {
	return &ImageOptions{
		DockerImageOptions: DockerImageOptions{
			Global: &GlobalOptions{},
			Shared: &SharedImageOptions{},
		},
	}
}

func TestImageNewSystemContextErrors(t *testing.T) {
	type mutate func(*ImageOptions)
	cases := []mutate{
		func(o *ImageOptions) { o.CredsOption = "u:p"; o.NoCreds = true },
		func(o *ImageOptions) { o.UserName = "u"; o.NoCreds = true },
		func(o *ImageOptions) { o.CredsOption = "u:p"; o.UserName = "u" },
		func(o *ImageOptions) { o.UserName = "u" }, // password missing
		func(o *ImageOptions) { o.Password = "p" }, // username missing
		func(o *ImageOptions) { o.CredsOption = ":onlypass" }, // invalid creds
	}
	for i, m := range cases {
		o := newImageOpts()
		m(o)
		if _, err := o.newSystemContext(); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestImageNewSystemContextSuccess(t *testing.T) {
	// creds option
	o := newImageOpts()
	o.CredsOption = "user:pass"
	o.RegistryToken = "tok"
	o.TlsVerify = true
	o.DeprecatedTLSVerify = &DeprecatedTLSVerifyOption{TLSVerify: true}
	ctx, err := o.newSystemContext()
	if err != nil || ctx.DockerAuthConfig == nil || ctx.DockerAuthConfig.Username != "user" {
		t.Fatalf("creds path: ctx=%+v err=%v", ctx, err)
	}
	if ctx.DockerBearerRegistryToken != "tok" {
		t.Errorf("registry token not set")
	}

	// username + password
	o = newImageOpts()
	o.UserName = "user"
	o.Password = "pass"
	ctx, err = o.newSystemContext()
	if err != nil || ctx.DockerAuthConfig.Username != "user" {
		t.Fatalf("username path: %+v %v", ctx, err)
	}

	// no creds
	o = newImageOpts()
	o.NoCreds = true
	ctx, err = o.newSystemContext()
	if err != nil || ctx.DockerAuthConfig == nil {
		t.Fatalf("nocreds path: %+v %v", ctx, err)
	}
}

func TestDestinationReference(t *testing.T) {
	// invalid transport
	if _, err := destinationReference("dest", "bogus"); err == nil {
		t.Error("expected error for invalid transport")
	}

	// docker transport
	if _, err := destinationReference("example.com/repo", TransportDocker); err != nil {
		t.Errorf("docker transport: %v", err)
	}

	// directory transport, new directory is created
	newDir := filepath.Join(t.TempDir(), "image")
	if _, err := destinationReference(newDir, TransportDir); err != nil {
		t.Errorf("dir transport (new): %v", err)
	}

	// directory transport, existing directory is refused
	if _, err := destinationReference(t.TempDir(), TransportDir); err == nil {
		t.Error("expected error when destination directory exists")
	}
}

func TestUpstreamData(t *testing.T) {
	// 200 response succeeds
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	domain := strings.TrimPrefix(ok.URL, "http://")
	if err := upstreamData(domain, "repo/image", &UpstreamEvent{}); err != nil {
		t.Errorf("upstreamData(200) = %v", err)
	}

	// non-200 response returns an error
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	if err := upstreamData(strings.TrimPrefix(bad.URL, "http://"), "repo/image", &UpstreamEvent{}); err == nil {
		t.Error("expected error for non-200 status")
	}

	// unreachable host returns an error
	if err := upstreamData("127.0.0.1:0", "repo/image", &UpstreamEvent{}); err == nil {
		t.Error("expected error for unreachable host")
	}
}

func TestImagesToCopy(t *testing.T) {
	// non-docker transport returns no descriptors without error
	descs, err := imagesToCopy("whatever", TransportDir, &types.SystemContext{})
	if err != nil || len(descs) != 0 {
		t.Errorf("imagesToCopy(dir) = %v, %v", descs, err)
	}

	// docker transport with a tagged image builds one reference (no network)
	descs, err = imagesToCopy("docker.io/library/busybox:latest", TransportDocker, &types.SystemContext{})
	if err != nil || len(descs) != 1 || len(descs[0].ImageRefs) != 1 {
		t.Errorf("imagesToCopy(tagged) = %v, %v", descs, err)
	}

	// docker transport with an invalid reference returns an error
	if _, err := imagesToCopy("INVALID IMAGE!!", TransportDocker, &types.SystemContext{}); err == nil {
		t.Error("expected error for invalid reference")
	}
}

func TestPromptForPassphraseNonTTY(t *testing.T) {
	// a pipe is not a TTY, so the prompt must fail fast
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	if _, err := promptForPassphrase("key.pem", r, w); err == nil {
		t.Error("expected error when stdin is not a TTY")
	}
}

func TestParseTemplateConfig(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(orig)

	// missing config file returns an error
	if _, err := parseTemplateConfig(); err == nil {
		t.Error("expected error for missing config file")
	}

	// valid config parses successfully
	if err := os.WriteFile(filepath.Join(dir, templateConfigName), []byte("global:\n  debug: true\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := parseTemplateConfig()
	if err != nil || cfg.Global == nil || !cfg.Global.Debug {
		t.Fatalf("parseTemplateConfig(valid) = %+v, %v", cfg, err)
	}

	// invalid yaml returns an error
	if err := os.WriteFile(filepath.Join(dir, templateConfigName), []byte("global:\n\t- bad"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := parseTemplateConfig(); err == nil {
		t.Error("expected error for invalid yaml")
	}
}
