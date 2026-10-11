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
	"bytes"
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
	"path/filepath"
	"strconv"
	"strings"
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
	// RunMarkerPath is where the launcher writes (MarkRun) the absolute path of the file it
	// started the entry point from (LauncherRunFile, or a temporary file when the working
	// directory cannot be written) and that file's change time as it handed over. It is on
	// the shared volume, so it is never part of an export.
	RunMarkerPath = "/shared-data/save-image.run"
	// RecordEnv is the environment variable a workload sets to 0 to skip the record, and
	// NoRecordPath is where the launcher notes that it did.
	RecordEnv    = "SAFE_SAVE_IMAGE_RECORD"
	NoRecordPath = "/shared-data/save-image.norecord"
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

// ErrRecordDisabled is returned for a container whose workload turned the record off.
var ErrRecordDisabled = errors.New("this workload was started without the record of its image's files that saving " +
	"it as an image needs (" + RecordEnv + "=0); remove that setting, restart the workload, then save it again")

// ErrNoRunFile is returned when the file the launcher started the entry point from is
// gone: its change time is the boundary between what the platform wrote and what the user
// did, and without it the two cannot be told apart.
var ErrNoRunFile = errors.New("the file the platform launcher started the entry point from is gone, so what the " +
	"launcher installed cannot be told apart from the user's changes; restart the workload, leave that file in " +
	"place, then save it again")

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
	// RunFile is the launcher's entry point file when RunMarker names none.
	RunFile string
	// RunMarker is where the launcher wrote the path of the entry point file it used
	// (RunMarkerPath); that path is read under Root.
	RunMarker string
	// Mountinfo is the container's mount table (/proc/self/mountinfo).
	Mountinfo string
	UID       int
	// Dial replaces the network dialer; tests use it to reach a registry by name.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
	// SpoolDir is where the chunks being uploaded are held: a directory the export leaves
	// out (the Pod's shared volume). Without one, or when it is part of the export, they
	// are held in memory.
	SpoolDir string
	// ChunkSize replaces DefaultChunkSize, or DefaultSpoolChunkSize with a SpoolDir.
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
			return "", time.Time{}, refusedGrant(r.g.Error)
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
		if _, perr := os.Lstat(filepath.Join(filepath.Dir(env.Baseline), filepath.Base(NoRecordPath))); perr == nil {
			return nil, ErrRecordDisabled
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
	runFile, since, recorded, err := runMarkOf(env)
	if err != nil {
		return nil, err
	}
	run, err := os.Lstat(runFile)
	if err != nil {
		return nil, fmt.Errorf("%w (%s: %v)", ErrNoRunFile, runFile, err)
	}
	// The change time the launcher recorded is the boundary. The file's change time now
	// is not: the user's own chmod or chown of it moves it past their earlier changes.
	if !recorded {
		var ok bool
		if since, ok = ctimeOf(run); !ok {
			return nil, fmt.Errorf("cannot read the change time of %s", runFile)
		}
	}

	// The packages the image held before the launcher's bootstrap; a container started
	// by a launcher that did not list them falls back to those in the record.
	pkgs, err := readPackages(filepath.Join(filepath.Dir(env.Baseline), filepath.Base(PackagesPath)))
	if err != nil {
		return nil, err
	}

	filter := NewFilter(ParseMountPoints(env.Mountinfo))
	changes, err := ComputeChanges(base, func(visit func(Entry) error) error {
		return Walk(env.Root, filter, visit)
	}, since, filter)
	if err != nil {
		return nil, err
	}
	bf.Close()
	if pkgs != nil {
		changes.basePackageLists = pkgs
	}

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
	bufs, err := chunkBuffers(env, filter)
	if err != nil {
		return nil, err
	}
	defer bufs.close()
	st, layer, err := upload(ctx, up, bufs, req.MaxSize, func(w io.Writer) (LayerStats, error) {
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

// RunFileOf returns the file the launcher started the entry point from: the path it
// recorded in env.RunMarker, read under env.Root, or env.RunFile when it recorded none.
func RunFileOf(env Env) (string, error) {
	p, _, _, err := runMarkOf(env)
	return p, err
}

// runMarkOf returns the file the launcher started the entry point from (see RunFileOf)
// and, when the launcher recorded it (MarkRun), that file's change time as it handed
// over.
func runMarkOf(env Env) (string, Timestamp, bool, error) {
	if env.RunMarker == "" {
		return env.RunFile, Timestamp{}, false, nil
	}
	b, err := os.ReadFile(env.RunMarker)
	if errors.Is(err, os.ErrNotExist) {
		return env.RunFile, Timestamp{}, false, nil
	}
	if err != nil {
		return "", Timestamp{}, false, fmt.Errorf("reading where the launcher wrote the entry point: %w", err)
	}
	p, rest, _ := strings.Cut(string(b), "\n")
	if !filepath.IsAbs(p) {
		return "", Timestamp{}, false, fmt.Errorf("the launcher recorded %q as its entry point file, which is not an absolute path", p)
	}
	line, _, _ := strings.Cut(rest, "\n")
	if line == "" {
		return filepath.Join(env.Root, p), Timestamp{}, false, nil
	}
	ts, err := parseRunCtime(line)
	if err != nil {
		return "", Timestamp{}, false, fmt.Errorf("the launcher recorded %q as the change time of its entry point file: %w", line, err)
	}
	return filepath.Join(env.Root, p), ts, true, nil
}

// MarkRun records, in marker, the entry point file the launcher starts and that file's
// change time now: the boundary between what the platform wrote and what the user did.
// The record replaces marker whole, so it is never read half written.
func MarkRun(marker, runFile string) error {
	return markRun(marker, "/", runFile)
}

// markRun is MarkRun for a container whose root file system is at root.
func markRun(marker, root, runFile string) error {
	if !filepath.IsAbs(runFile) {
		return fmt.Errorf("the entry point file %q is not an absolute path", runFile)
	}
	info, err := os.Lstat(filepath.Join(root, runFile))
	if err != nil {
		return err
	}
	ts, ok := ctimeOf(info)
	if !ok {
		return fmt.Errorf("cannot read the change time of %s", runFile)
	}
	tmp, err := os.CreateTemp(filepath.Dir(marker), filepath.Base(marker)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := fmt.Fprintf(tmp, "%s\n%d.%09d\n", runFile, ts.Sec, ts.Nsec); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), marker)
}

// parseRunCtime parses the "seconds.nanoseconds" MarkRun writes.
func parseRunCtime(s string) (Timestamp, error) {
	sec, nsec, ok := strings.Cut(s, ".")
	if !ok || len(nsec) != 9 {
		return Timestamp{}, fmt.Errorf("not seconds.nanoseconds")
	}
	a, err := strconv.ParseInt(sec, 10, 64)
	if err != nil {
		return Timestamp{}, err
	}
	b, err := strconv.ParseInt(nsec, 10, 64)
	if err != nil || b < 0 {
		return Timestamp{}, fmt.Errorf("not seconds.nanoseconds")
	}
	return Timestamp{Sec: a, Nsec: b}, nil
}

type uploadedLayer struct {
	digest, diffID string
	size           int64
}

// chunkSet is the two buffers the layer is uploaded through.
type chunkSet struct {
	bufs  [2]chunkBuffer
	size  int64
	close func()
}

// chunkBuffer holds one chunk.
type chunkBuffer interface {
	io.ReaderAt
	// fill replaces the contents with up to n bytes of r, and returns how many it read.
	fill(r io.Reader, n int64) (int64, error)
}

type memChunk struct{ b []byte }

func (m *memChunk) ReadAt(p []byte, off int64) (int, error) {
	return bytes.NewReader(m.b).ReadAt(p, off)
}

func (m *memChunk) fill(r io.Reader, n int64) (int64, error) {
	m.b = m.b[:n]
	k, err := io.ReadFull(r, m.b)
	m.b = m.b[:k]
	return int64(k), err
}

type fileChunk struct{ f *os.File }

func (c *fileChunk) ReadAt(p []byte, off int64) (int, error) { return c.f.ReadAt(p, off) }

func (c *fileChunk) fill(r io.Reader, n int64) (int64, error) {
	if err := c.f.Truncate(0); err != nil {
		return 0, err
	}
	k, err := io.Copy(io.NewOffsetWriter(c.f, 0), io.LimitReader(r, n))
	if err == nil && k < n {
		err = io.EOF
	}
	return k, err
}

// chunkBuffers returns the chunk buffers: two files in env.SpoolDir when it is a place the
// export leaves out, two buffers in memory otherwise.
func chunkBuffers(env Env, filter Filter) (*chunkSet, error) {
	if env.SpoolDir != "" && filter.Excluded(spoolPath(env)) {
		set := &chunkSet{size: int64(DefaultSpoolChunkSize)}
		var files []*os.File
		set.close = func() {
			for _, f := range files {
				f.Close()
				os.Remove(f.Name())
			}
		}
		for i := range set.bufs {
			f, err := os.CreateTemp(env.SpoolDir, "save-image-chunk-*")
			if err != nil {
				set.close()
				files = nil
				break
			}
			files = append(files, f)
			set.bufs[i] = &fileChunk{f: f}
		}
		if len(files) == len(set.bufs) {
			if env.ChunkSize > 0 {
				set.size = int64(env.ChunkSize)
			}
			return set, nil
		}
	}
	set := &chunkSet{size: int64(DefaultChunkSize), close: func() {}}
	if env.ChunkSize > 0 {
		set.size = int64(env.ChunkSize)
	}
	for i := range set.bufs {
		set.bufs[i] = &memChunk{b: make([]byte, 0, set.size)}
	}
	return set, nil
}

// spoolPath is env.SpoolDir as the container's root file system names it.
func spoolPath(env Env) string {
	rel, err := filepath.Rel(env.Root, env.SpoolDir)
	if err != nil || strings.HasPrefix(rel, "..") {
		// Outside the root being saved altogether (tests): never part of the export.
		return "/proc"
	}
	return "/" + filepath.ToSlash(rel)
}

// upload writes the layer through gzip into the registry, chunk by chunk, while it is
// being produced: one chunk is filled while the other is sent.
func upload(ctx context.Context, up *uploader, bufs *chunkSet, maxSize int64, produce func(io.Writer) (LayerStats, error)) (LayerStats, uploadedLayer, error) {
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

	type filled struct {
		buf chunkBuffer
		n   int64
	}
	chunks := make(chan filled)
	free := make(chan chunkBuffer, len(bufs.bufs))
	for _, b := range bufs.bufs {
		free <- b
	}
	readErr := make(chan error, 1)
	go func() {
		defer close(chunks)
		for {
			var buf chunkBuffer
			select {
			case buf = <-free:
			case <-ctx.Done():
				readErr <- ctx.Err()
				return
			}
			n, err := buf.fill(pr, bufs.size)
			if n > 0 {
				select {
				case chunks <- filled{buf, n}:
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
		if upErr = up.write(ctx, c.buf, c.n, off); upErr != nil {
			cancel()
			pr.CloseWithError(errUploadStopped)
			continue
		}
		off += c.n
		free <- c.buf
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
