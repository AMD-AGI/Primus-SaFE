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
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/klauspost/compress/gzip"
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

// ErrRecording is returned while the record of the files the container started with is
// still being made.
var ErrRecording = errors.New("this container is still recording the files its image held; save it again in a minute")

// ProtocolVersion is what "save-image protocol" prints: the controller only talks to an
// agent that speaks this version of the exchange on its standard input and output.
const ProtocolVersion = 2

// Request is the first line the controller sends on the agent's standard input. It never
// appears in the command line, the environment or the Pod spec.
type Request struct {
	// Registry is the host (and port) of the registry.
	Registry string `json:"registry"`
	// Repository is the staging repository, without the registry.
	Repository string `json:"repository"`
	// Token is a short-lived registry bearer token for Repository, and TokenExpiry when it
	// expires. The agent asks for a new one before then (see Grant).
	Token       string    `json:"token"`
	TokenExpiry time.Time `json:"tokenExpiry"`
	// CA is a PEM bundle the registry's certificate may also be signed by, on top of the
	// container's system roots; empty for a registry those roots trust.
	CA string `json:"ca,omitempty"`
	// Deadline is when the agent gives up.
	Deadline time.Time `json:"deadline"`
	// MaxSize bounds the compressed layer; zero for no bound.
	MaxSize int64 `json:"maxSize,omitempty"`
}

// Grant is each later line on the agent's standard input: the answer to a Message that
// asks for a new token.
type Grant struct {
	Token  string    `json:"token,omitempty"`
	Expiry time.Time `json:"expiry,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// Message is one line the agent prints on its standard output: a request for a new token,
// or, last, the result.
type Message struct {
	Renew  bool      `json:"renew,omitempty"`
	Result *Response `json:"result,omitempty"`
}

// Response is what the agent reports on success.
type Response struct {
	// Digest, DiffID and Size describe the uploaded layer blob.
	Digest    string `json:"digest"`
	DiffID    string `json:"diffID"`
	Size      int64  `json:"size"`
	Changed   int    `json:"changed"`
	Deleted   int    `json:"deleted"`
	Skipped   int    `json:"skipped"`
	Vanished  int    `json:"vanished"`
	Resized   int    `json:"resized"`
	Unsettled int    `json:"unsettled,omitempty"`
	Renewals  int    `json:"renewals,omitempty"`
	Retries   int    `json:"retries,omitempty"`
	// PeakMemory is the most memory the agent held, in bytes.
	PeakMemory      int64    `json:"peakMemory,omitempty"`
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
	// ChunkSize replaces DefaultChunkSize.
	ChunkSize int
	// Backoff replaces the wait between attempts of a failed request.
	Backoff func(ctx context.Context, attempt int) error
}

// maxLine bounds one line of the exchange on standard input.
const maxLine = 1 << 20

// Serve runs one export the way the controller drives it: the Request is the first line
// of in, each Grant a later one, and every Message a line of out.
func Serve(ctx context.Context, in io.Reader, out io.Writer, env Env) error {
	lines := bufio.NewScanner(in)
	lines.Buffer(make([]byte, 64<<10), maxLine)
	if !lines.Scan() {
		return fmt.Errorf("reading the request: %v", errOr(lines.Err(), io.ErrUnexpectedEOF))
	}
	var req Request
	if err := json.Unmarshal(lines.Bytes(), &req); err != nil {
		return fmt.Errorf("reading the request: %w", err)
	}
	enc := json.NewEncoder(out)
	resp, err := Export(ctx, req, env, &lineTokens{lines: lines, enc: enc})
	if err != nil {
		return err
	}
	resp.PeakMemory = PeakMemory()
	return enc.Encode(Message{Result: resp})
}

func errOr(err, def error) error {
	if err != nil {
		return err
	}
	return def
}

// lineTokens asks the controller for tokens over standard input and output.
type lineTokens struct {
	lines *bufio.Scanner
	enc   *json.Encoder
}

func (l *lineTokens) Renew(ctx context.Context) (string, time.Time, error) {
	if err := l.enc.Encode(Message{Renew: true}); err != nil {
		return "", time.Time{}, err
	}
	type line struct {
		g   Grant
		err error
	}
	got := make(chan line, 1)
	go func() {
		if !l.lines.Scan() {
			got <- line{err: errOr(l.lines.Err(), io.ErrUnexpectedEOF)}
			return
		}
		var g Grant
		err := json.Unmarshal(l.lines.Bytes(), &g)
		got <- line{g: g, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", time.Time{}, ctx.Err()
	case r := <-got:
		switch {
		case r.err != nil:
			return "", time.Time{}, r.err
		case r.g.Error != "":
			return "", time.Time{}, errors.New(r.g.Error)
		case r.g.Token == "":
			return "", time.Time{}, errors.New("no token was granted")
		}
		return r.g.Token, r.g.Expiry, nil
	}
}

// Export computes the layer and uploads it, asking tokens for new tokens as they expire.
func Export(ctx context.Context, req Request, env Env, tokens TokenSource) (*Response, error) {
	if env.UID != 0 {
		return nil, fmt.Errorf("%w (uid %d): saving it would leave out the files it cannot read", ErrNotRoot, env.UID)
	}
	if req.Registry == "" || req.Repository == "" || req.Token == "" || req.Deadline.IsZero() {
		return nil, fmt.Errorf("the request names no registry, repository, token or deadline")
	}
	ctx, cancel := context.WithDeadline(ctx, req.Deadline)
	defer cancel()

	repo, err := name.NewRepository(req.Registry+"/"+req.Repository, name.StrictValidation)
	if err != nil {
		return nil, fmt.Errorf("invalid staging repository: %w", err)
	}
	tr, err := httpsOnlyTransport(req.CA, env.Dial)
	if err != nil {
		return nil, err
	}

	bf, err := os.Open(env.Baseline)
	if errors.Is(err, os.ErrNotExist) {
		if _, perr := os.Lstat(env.Baseline + RecordingSuffix); perr == nil {
			return nil, ErrRecording
		}
		return nil, ErrNoBaseline
	}
	if err != nil {
		return nil, fmt.Errorf("reading the record of the image's files: %w", err)
	}
	defer bf.Close()
	base, err := NewBaselineReader(bf)
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
	changes, err := ComputeChanges(base, func(visit func(Entry) error) error {
		return Walk(env.Root, filter, visit)
	}, since, filter)
	if err != nil {
		return nil, err
	}
	bf.Close()

	backoffFn := env.Backoff
	if backoffFn == nil {
		backoffFn = backoff
	}
	up := &uploader{
		client: &http.Client{Transport: tr},
		base:   &url.URL{Scheme: "https", Host: repo.RegistryStr()},
		repo:   repo.RepositoryStr(),
		tokens: tokens,
		token:  req.Token,
		expiry: req.TokenExpiry,
		// The first token's life is not known: it may have been minted a while ago.
		sleep: backoffFn,
	}
	chunkSize := env.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	st, layer, err := upload(ctx, up, chunkSize, req.MaxSize, func(w io.Writer) (LayerStats, error) {
		return WriteLayer(w, env.Root, changes.Changed, changes.Deleted, ImageContains(changes))
	})
	if err != nil {
		return nil, err
	}
	return &Response{
		Digest:          layer.digest,
		DiffID:          layer.diffID,
		Size:            layer.size,
		Changed:         len(changes.Changed),
		Deleted:         len(changes.Deleted),
		Skipped:         changes.Skipped,
		Vanished:        st.Vanished,
		Resized:         st.Resized,
		Unsettled:       changes.Unsettled,
		Renewals:        up.Renewals,
		Retries:         up.Retries,
		DroppedPackages: st.DroppedPackages,
	}, nil
}

type uploadedLayer struct {
	digest, diffID string
	size           int64
}

// upload writes the layer through gzip into the registry, chunk by chunk, while it is
// being produced. Two chunks are in memory: one being filled, one being sent.
func upload(ctx context.Context, up *uploader, chunkSize int, maxSize int64, produce func(io.Writer) (LayerStats, error)) (LayerStats, uploadedLayer, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := up.start(ctx); err != nil {
		return LayerStats{}, uploadedLayer{}, err
	}

	pr, pw := io.Pipe()
	diffID, digest := sha256.New(), sha256.New()
	compressed := &countingWriter{w: io.MultiWriter(pw, digest)}
	type built struct {
		stats LayerStats
		err   error
	}
	done := make(chan built, 1)
	go func() {
		zw, _ := gzip.NewWriterLevel(&limitWriter{w: compressed, max: maxSize}, gzip.BestSpeed)
		st, err := produce(io.MultiWriter(zw, diffID))
		if err == nil {
			err = zw.Close()
		}
		pw.CloseWithError(err)
		done <- built{st, err}
	}()

	chunks := make(chan []byte)
	free := make(chan []byte, 2)
	free <- make([]byte, chunkSize)
	free <- make([]byte, chunkSize)
	readErr := make(chan error, 1)
	go func() {
		defer close(chunks)
		for {
			var buf []byte
			select {
			case buf = <-free:
			case <-ctx.Done():
				readErr <- ctx.Err()
				return
			}
			n, err := io.ReadFull(pr, buf)
			if n > 0 {
				select {
				case chunks <- buf[:n]:
				case <-ctx.Done():
					readErr <- ctx.Err()
					return
				}
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				readErr <- nil
				return
			}
			if err != nil {
				readErr <- err
				return
			}
		}
	}()

	var off int64
	var upErr error
	for c := range chunks {
		if upErr != nil {
			continue
		}
		if upErr = up.write(ctx, c, off); upErr != nil {
			cancel()
			pr.CloseWithError(errUploadStopped)
			continue
		}
		off += int64(len(c))
		free <- c[:cap(c)]
	}
	rerr := <-readErr
	b := <-done
	if b.err != nil && !errors.Is(b.err, errUploadStopped) {
		// The upload stopped because the layer could not be written.
		return b.stats, uploadedLayer{}, fmt.Errorf("writing the layer: %w", b.err)
	}
	if upErr != nil {
		return b.stats, uploadedLayer{}, upErr
	}
	if rerr != nil {
		return b.stats, uploadedLayer{}, rerr
	}
	l := uploadedLayer{
		digest: "sha256:" + hex.EncodeToString(digest.Sum(nil)),
		diffID: "sha256:" + hex.EncodeToString(diffID.Sum(nil)),
		size:   compressed.n,
	}
	if off != l.size {
		return b.stats, uploadedLayer{}, fmt.Errorf("uploaded %d bytes of a %d-byte layer", off, l.size)
	}
	if err := up.finish(ctx, l.digest); err != nil {
		return b.stats, uploadedLayer{}, err
	}
	return b.stats, l, nil
}

// ErrTooLarge is returned for a layer larger than the request allows.
var ErrTooLarge = errors.New("the changes are larger than a saved image may add")

type limitWriter struct {
	w   io.Writer
	max int64
	n   int64
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.max > 0 && l.n+int64(len(p)) > l.max {
		return 0, fmt.Errorf("%w (%d bytes)", ErrTooLarge, l.max)
	}
	n, err := l.w.Write(p)
	l.n += int64(n)
	return n, err
}

// httpsOnlyTransport trusts the container's system roots and, on top of them, the given
// CA (a registry signed by a private CA), and refuses to send anything over plain HTTP,
// so the token never leaves unencrypted and an unreachable registry is never retried
// insecurely.
func httpsOnlyTransport(caPEM string, dial func(ctx context.Context, network, addr string) (net.Conn, error)) (http.RoundTripper, error) {
	pool, err := systemRoots()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if caPEM != "" && !pool.AppendCertsFromPEM([]byte(caPEM)) {
		return nil, fmt.Errorf("the registry CA holds no usable certificate")
	}
	t := remote.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	if dial != nil {
		t.DialContext = dial
	}
	return httpsOnly{t}, nil
}

// PeakMemory is the most resident memory this process has held, in bytes.
func PeakMemory() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return ru.Maxrss << 10
}

// systemRoots returns the container's trusted roots; tests replace it.
var systemRoots = x509.SystemCertPool

type httpsOnly struct{ inner http.RoundTripper }

func (h httpsOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" {
		return nil, fmt.Errorf("refusing to reach the registry over %s", r.URL.Scheme)
	}
	return h.inner.RoundTrip(r)
}
