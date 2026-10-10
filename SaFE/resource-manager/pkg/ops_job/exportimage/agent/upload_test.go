/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flakyRegistry stands in front of a registry and fails the way a real one does under
// load and with expiring tokens. It answers an upload's status itself, from what the
// registry behind it reported.
type flakyRegistry struct {
	inner http.Handler

	mu      sync.Mutex
	valid   map[string]bool
	held    map[string]string // upload path -> Range the registry last reported
	patches int
	// Per PATCH number: answer 503 without forwarding, drop the connection without
	// reading the body, or forward and then drop the connection (the answer is lost).
	unavailable, reset, lost map[int]bool
	// revokeAt is the PATCH number at which every token stops being accepted.
	revokeAt int
	// dropSession makes the upload's status unknown, as when the registry has dropped it.
	dropSession bool
	// rejected counts requests refused for their token.
	rejected int
}

func newFlakyRegistry() *flakyRegistry {
	return &flakyRegistry{
		inner:       registry.New(registry.Logger(log.New(io.Discard, "", 0))),
		valid:       map[string]bool{"first": true},
		held:        map[string]string{},
		unavailable: map[int]bool{}, reset: map[int]bool{}, lost: map[int]bool{},
	}
}

func (f *flakyRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	n := 0
	if r.Method == http.MethodPatch {
		f.patches++
		n = f.patches
		if n == f.revokeAt {
			f.valid = map[string]bool{}
		}
	}
	if strings.Contains(r.URL.Path, "/blobs/uploads") && !f.valid[tok] {
		f.rejected++
		f.mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/uploads/") {
		rng, ok := f.held[r.URL.Path]
		f.mu.Unlock()
		if !ok || f.dropSession {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Location", r.URL.Path)
		w.Header().Set("Range", rng)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	unavailable, reset, lost := f.unavailable[n], f.reset[n], f.lost[n]
	f.mu.Unlock()
	switch {
	case unavailable:
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	case reset:
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
		return
	}
	rec := httptest.NewRecorder()
	f.inner.ServeHTTP(rec, r)
	if loc := rec.Header().Get("Location"); loc != "" && rec.Header().Get("Range") != "" {
		f.mu.Lock()
		f.held[loc] = rec.Header().Get("Range")
		f.mu.Unlock()
	}
	if lost {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
		return
	}
	for k, v := range rec.Header() {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.Code)
	_, _ = w.Write(rec.Body.Bytes())
}

// grants hands out new tokens and makes the registry accept them.
type grants struct {
	f      *flakyRegistry
	n      int
	expiry time.Duration
}

func (g *grants) Renew(context.Context) (string, time.Time, error) {
	g.f.mu.Lock()
	defer g.f.mu.Unlock()
	g.n++
	tok := fmt.Sprintf("renewed-%d", g.n)
	g.f.valid[tok] = true
	return tok, time.Now().Add(g.expiry), nil
}

func newFlakyTLSRegistry(t *testing.T) (*tlsRegistry, *flakyRegistry) {
	t.Helper()
	f := newFlakyRegistry()
	r := newTLSRegistry(t)
	r.srv.Config.Handler = f
	return r, f
}

func noWait(context.Context, int) error { return nil }

// A layer far larger than one request is uploaded in chunks; the token expiring, the
// registry being briefly unavailable, a connection dropped mid-chunk and an answer lost
// after the registry took the chunk all cost a retry, not the export. The chunks are held
// in files on a volume the export leaves out, or in memory without one.
func TestExportUploadsInChunksThroughExpiryAndFaults(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spool func(root string) string
	}{
		{"in memory", func(string) string { return "" }},
		{"in files on the shared volume", func(root string) string { return filepath.Join(root, "shared-data") }},
		{"in memory, the spool being part of the image", func(root string) string { return filepath.Join(root, "root") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := container(t)
			big := make([]byte, 600<<10)
			_, _ = rand.Read(big)
			write(t, env.Root, "root/model.bin", string(big))
			r, f := newFlakyTLSRegistry(t)
			env.Dial, env.ChunkSize, env.Backoff = r.dial, 32<<10, noWait
			env.SpoolDir = tc.spool(env.Root)
			f.unavailable[3], f.reset[5], f.lost[7] = true, true, true
			f.revokeAt = 9

			req := r.request()
			req.Token = "first"
			req.TokenExpiry = time.Now().Add(time.Hour)
			resp, err := Export(context.Background(), req, env, &grants{f: f, expiry: time.Hour})
			require.NoError(t, err)
			assert.Greater(t, f.patches, 18, "the layer went in many chunks")
			assert.Equal(t, 1, resp.Renewals, "the revoked token was replaced once")
			assert.Equal(t, 3, resp.Retries, "each fault cost one retry")

			got := layerMembers(t, r, resp)
			assert.Equal(t, string(big), got["root/model.bin"], "the registry holds the layer, byte for byte")
			assert.Equal(t, "Debian, changed\n", got["etc/issue"])
			assert.NotContains(t, got, "root/save-image-chunk", "a chunk file is never part of the layer")
			if env.SpoolDir != "" {
				left, err := filepath.Glob(filepath.Join(env.SpoolDir, "save-image-chunk-*"))
				require.NoError(t, err)
				assert.Empty(t, left, "the chunk files are removed")
			}
		})
	}
}

// Chunks go to files only on a volume the export leaves out.
func TestChunkBuffersUseFilesOnlyOutsideTheExport(t *testing.T) {
	env := container(t)
	filter := NewFilter(ParseMountPoints(env.Mountinfo))
	for dir, files := range map[string]bool{
		filepath.Join(env.Root, "shared-data"): true,
		filepath.Join(env.Root, "root"):        false,
		"":                                     false,
		filepath.Join(env.Root, "missing"):     false,
	} {
		env.SpoolDir = dir
		set, err := chunkBuffers(env, filter)
		require.NoError(t, err)
		_, isFile := set.bufs[0].(*fileChunk)
		assert.Equal(t, files, isFile, dir)
		if files {
			assert.Equal(t, int64(DefaultSpoolChunkSize), set.size)
		} else {
			assert.Equal(t, int64(DefaultChunkSize), set.size)
		}
		set.close()
	}
}

// A token about to expire is replaced before it is used, rather than after a refusal.
func TestExportRenewsATokenAboutToExpire(t *testing.T) {
	env := container(t)
	r, f := newFlakyTLSRegistry(t)
	env.Dial, env.Backoff = r.dial, noWait
	req := r.request()
	req.Token = "first"
	req.TokenExpiry = time.Now().Add(renewBefore / 2)
	resp, err := Export(context.Background(), req, env, &grants{f: f, expiry: time.Hour})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Renewals)
	assert.Zero(t, f.rejected, "no request went out with the expiring token")
}

func TestExportFailsWhenTheRegistryDropsTheUpload(t *testing.T) {
	env := container(t)
	big := make([]byte, 200<<10)
	_, _ = rand.Read(big)
	write(t, env.Root, "root/model.bin", string(big))
	r, f := newFlakyTLSRegistry(t)
	env.Dial, env.ChunkSize, env.Backoff = r.dial, 32<<10, noWait
	f.lost[2] = true
	f.dropSession = true
	req := r.request()
	req.Token = "first"
	_, err := Export(context.Background(), req, env, &grants{f: f, expiry: time.Hour})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be resumed")
}

func TestExportRefusesALayerLargerThanAllowed(t *testing.T) {
	env := container(t)
	big := make([]byte, 200<<10)
	_, _ = rand.Read(big)
	write(t, env.Root, "root/model.bin", string(big))
	r := newTLSRegistry(t)
	env.Dial = r.dial
	req := r.request()
	req.MaxSize = 100 << 10
	_, err := Export(context.Background(), req, env, noRenewal{})
	require.ErrorIs(t, err, ErrTooLarge)
}

// The CA the controller sends is added to the container's own roots: a registry with a
// publicly trusted certificate stays reachable when a private CA is sent along.
func TestExportTrustsTheSystemRootsAlongsideTheCA(t *testing.T) {
	r := newTLSRegistry(t)
	old := systemRoots
	t.Cleanup(func() { systemRoots = old })
	systemRoots = func() (*x509.CertPool, error) {
		p := x509.NewCertPool()
		p.AppendCertsFromPEM([]byte(r.ca))
		return p, nil
	}
	env := container(t)
	env.Dial = r.dial
	req := r.request()
	req.CA = otherCA(t)
	_, err := Export(context.Background(), req, env, noRenewal{})
	require.NoError(t, err)
}

// Serve speaks the controller's exchange: the request, a request for a new token and its
// grant, then the result.
func TestServeExchangesTokensOverStandardInputAndOutput(t *testing.T) {
	env := container(t)
	r, f := newFlakyTLSRegistry(t)
	env.Dial, env.Backoff = r.dial, noWait
	f.mu.Lock()
	f.valid["granted"] = true
	f.mu.Unlock()
	req := r.request()
	req.Token = "first"
	req.TokenExpiry = time.Now() // already due

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := Serve(context.Background(), inR, outW, env)
		outW.CloseWithError(err)
		done <- err
	}()
	enc := json.NewEncoder(inW)
	require.NoError(t, enc.Encode(req))
	dec := json.NewDecoder(outR)
	var msgs []Message
	for {
		var m Message
		if err := dec.Decode(&m); err != nil {
			break
		}
		msgs = append(msgs, m)
		if m.Renew {
			require.NoError(t, enc.Encode(Grant{Token: "granted", Expiry: time.Now().Add(time.Hour)}))
		}
	}
	require.NoError(t, <-done)
	require.Len(t, msgs, 2)
	assert.True(t, msgs[0].Renew)
	require.NotNil(t, msgs[1].Result)
	assert.Equal(t, 1, msgs[1].Result.Renewals)
}

func TestServeReportsARefusedGrant(t *testing.T) {
	env := container(t)
	r := newTLSRegistry(t)
	env.Dial = r.dial
	req := r.request()
	req.TokenExpiry = time.Now()
	b, _ := json.Marshal(req)
	g, _ := json.Marshal(Grant{Error: "too many renewals"})
	var out bytes.Buffer
	err := Serve(context.Background(), strings.NewReader(string(b)+"\n"+string(g)+"\n"), &out, env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many renewals")
}
