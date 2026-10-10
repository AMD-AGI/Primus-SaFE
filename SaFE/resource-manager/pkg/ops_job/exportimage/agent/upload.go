/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// A chunk is how much of the layer one request carries. Each is held until the registry
// has it, so that it can be sent again after a failure, and two are held at once, one
// being filled while the other is sent. They are held in files on the Pod's shared volume
// (DefaultSpoolChunkSize), which the export leaves out; a registry takes a fraction of a
// second for each request, so fewer, larger chunks upload faster. Without a place for
// them they are held in memory (DefaultChunkSize).
const (
	DefaultChunkSize      = 16 << 20
	DefaultSpoolChunkSize = 512 << 20
)

const (
	// renewBefore is how long before the token expires a new one is asked for.
	renewBefore = 2 * time.Minute
	// maxAttempts bounds the tries of one request that keeps failing.
	maxAttempts = 8
)

// TokenSource hands out registry tokens for the staging repository.
type TokenSource interface {
	// Renew returns a fresh token and when it expires.
	Renew(ctx context.Context) (string, time.Time, error)
}

// uploader sends one blob to a registry in chunks (the OCI distribution "chunked upload":
// POST opens a session, each PATCH appends a range, PUT closes it with the digest). A
// chunk that fails is sent again from where the registry says the upload is; a token
// that expires, or that the registry no longer accepts, is replaced. The layer stream
// itself cannot be replayed, so an upload session the registry has dropped is fatal.
type uploader struct {
	client *http.Client
	base   *url.URL // https://<registry>
	repo   string
	tokens TokenSource
	token  string
	expiry time.Time
	// lifetime is how long the current token was valid for when it was received.
	lifetime time.Duration
	location *url.URL
	offset   int64
	// sleep waits between attempts; tests shorten it.
	sleep func(ctx context.Context, attempt int) error
	// Renewals counts the tokens obtained after the first.
	Renewals int
	// Retries counts requests sent again after a failure.
	Retries int
}

func backoff(ctx context.Context, attempt int) error {
	d := time.Second << min(attempt, 5)
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// registryError is a registry's answer that is not success.
type registryError struct {
	status int
	body   string
}

func (e *registryError) Error() string {
	if e.body == "" {
		return fmt.Sprintf("the registry answered %d", e.status)
	}
	return fmt.Sprintf("the registry answered %d: %s", e.status, e.body)
}

// renewalError is a failure to get a new token. It is not retried: the controller has
// already retried what it could.
type renewalError struct{ err error }

func (e *renewalError) Error() string { return "renewing the upload token: " + e.err.Error() }
func (e *renewalError) Unwrap() error { return e.err }

// transient reports whether a failed request may succeed when sent again.
func transient(err error) bool {
	var re *registryError
	if errors.As(err, &re) {
		return re.status == http.StatusRequestTimeout || re.status == http.StatusTooManyRequests ||
			re.status >= 500
	}
	var re2 *renewalError
	if err == nil || errors.As(err, &re2) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// A registry this side does not trust, or one that does not speak TLS, will not change
	// its mind.
	var unknownCA x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var hostname x509.HostnameError
	var verify *tls.CertificateVerificationError
	var record tls.RecordHeaderError
	if errors.As(err, &unknownCA) || errors.As(err, &invalid) || errors.As(err, &hostname) ||
		errors.As(err, &verify) || errors.As(err, &record) || strings.Contains(err.Error(), "HTTP response to HTTPS client") {
		return false
	}
	// No answer: the connection failed, or the answer was lost.
	return true
}

func unauthorized(err error) bool {
	var re *registryError
	return errors.As(err, &re) && re.status == http.StatusUnauthorized
}

// do sends one request with the current token, renewing it first when it is about to
// expire and once more when the registry refuses it.
// payload is a request body that can be read again for each attempt.
type payload struct {
	r   io.ReaderAt
	off int64
	n   int64
}

func (u *uploader) do(ctx context.Context, method string, target *url.URL, body *payload, header http.Header, want ...int) (*http.Response, error) {
	if time.Until(u.expiry) < u.renewMargin() {
		if err := u.renew(ctx); err != nil {
			return nil, err
		}
	}
	for renewed := false; ; renewed = true {
		resp, err := u.send(ctx, method, target, body, header, want)
		if unauthorized(err) && !renewed {
			if err := u.renew(ctx); err != nil {
				return nil, err
			}
			continue
		}
		return resp, err
	}
}

func (u *uploader) renew(ctx context.Context) error {
	tok, exp, err := u.tokens.Renew(ctx)
	if err != nil {
		return &renewalError{err}
	}
	u.token, u.expiry, u.lifetime = tok, exp, time.Until(exp)
	u.Renewals++
	return nil
}

// renewMargin is how long before its expiry a token is replaced: renewBefore, or a
// quarter of the token's life when the registry hands out shorter-lived tokens, so that
// each token is used for most of its life.
func (u *uploader) renewMargin() time.Duration {
	if u.lifetime > 0 && u.lifetime/4 < renewBefore {
		return u.lifetime / 4
	}
	return renewBefore
}

func (u *uploader) send(ctx context.Context, method string, target *url.URL, body *payload, header http.Header, want []int) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = io.NewSectionReader(body.r, body.off, body.n)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), rd)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if body == nil {
		req.Body = http.NoBody
	} else {
		req.ContentLength = body.n
	}
	req.Header.Set("Authorization", "Bearer "+u.token)
	resp, err := u.client.Do(req)
	if err != nil {
		// The URL holds no secret, but the error is reported upward; keep it short.
		var ue *url.Error
		if errors.As(err, &ue) {
			return nil, fmt.Errorf("%s: %w", method, ue.Err)
		}
		return nil, err
	}
	for _, w := range want {
		if resp.StatusCode == w {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			return resp, nil
		}
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	resp.Body.Close()
	return nil, &registryError{status: resp.StatusCode, body: strings.TrimSpace(string(b))}
}

// resolve reads a Location header against the registry.
func (u *uploader) resolve(resp *http.Response) (*url.URL, error) {
	loc := resp.Header.Get("Location")
	if loc == "" {
		return nil, fmt.Errorf("the registry named no upload location")
	}
	l, err := u.base.Parse(loc)
	if err != nil {
		return nil, fmt.Errorf("the registry named an invalid upload location: %w", err)
	}
	if l.Scheme != "https" || l.Host != u.base.Host {
		// The token goes wherever the location points.
		return nil, fmt.Errorf("the registry named an upload location on another host")
	}
	return l, nil
}

// start opens the upload session.
func (u *uploader) start(ctx context.Context) error {
	target := u.base.JoinPath("v2", u.repo, "blobs", "uploads") // trailing slash added below
	target.Path += "/"
	var resp *http.Response
	err := u.retry(ctx, func() error {
		var err error
		resp, err = u.do(ctx, http.MethodPost, target, nil, nil, http.StatusAccepted)
		return err
	}, nil)
	if err != nil {
		return fmt.Errorf("starting the upload: %w", err)
	}
	u.location, err = u.resolve(resp)
	return err
}

// retry runs fn until it succeeds, fails for good or has been tried maxAttempts times.
// Before each new try, recover (when set) is run to find out where to resume.
func (u *uploader) retry(ctx context.Context, fn func() error, recover func() error) error {
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			u.Retries++
			if serr := u.sleep(ctx, attempt); serr != nil {
				return fmt.Errorf("%w (after %v)", serr, err)
			}
			if recover != nil {
				if rerr := recover(); rerr != nil {
					if transient(rerr) {
						err = rerr
						continue
					}
					return rerr
				}
			}
		}
		if err = fn(); err == nil || !transient(err) {
			return err
		}
	}
	return err
}

// write appends one chunk of n bytes, which starts at offset start of the blob.
func (u *uploader) write(ctx context.Context, chunk io.ReaderAt, n, start int64) error {
	end := start + n
	send := func() error {
		sent := u.offset - start
		if sent < 0 || sent > n {
			return fmt.Errorf("the registry holds %d bytes of the layer, outside the chunk at %d", u.offset, start)
		}
		if sent == n {
			return nil
		}
		h := http.Header{}
		h.Set("Content-Type", "application/octet-stream")
		h.Set("Content-Range", fmt.Sprintf("%d-%d", u.offset, end-1))
		resp, err := u.do(ctx, http.MethodPatch, u.location, &payload{r: chunk, off: sent, n: n - sent}, h,
			http.StatusAccepted, http.StatusNoContent)
		if err != nil {
			var re *registryError
			if errors.As(err, &re) && re.status == http.StatusRequestedRangeNotSatisfiable {
				// The registry is elsewhere in the upload than this side thought; ask it.
				return &registryError{status: http.StatusServiceUnavailable, body: re.body}
			}
			return err
		}
		loc, err := u.resolve(resp)
		if err != nil {
			return err
		}
		got, ok := parseRange(resp.Header.Get("Range"))
		if ok && got != end {
			return fmt.Errorf("the registry holds %d bytes of the layer after a chunk that ends at %d", got, end)
		}
		u.location, u.offset = loc, end
		return nil
	}
	if err := u.retry(ctx, send, func() error { return u.status(ctx) }); err != nil {
		return fmt.Errorf("uploading the layer at byte %d: %w", u.offset, err)
	}
	return nil
}

// status asks the registry how much of the upload it holds.
func (u *uploader) status(ctx context.Context) error {
	resp, err := u.do(ctx, http.MethodGet, u.location, nil, nil, http.StatusNoContent)
	if err != nil {
		var re *registryError
		if errors.As(err, &re) && re.status < 500 && re.status != http.StatusTooManyRequests && re.status != http.StatusRequestTimeout {
			return fmt.Errorf("the registry dropped the upload, which cannot be resumed: %w", err)
		}
		return err
	}
	got, ok := parseRange(resp.Header.Get("Range"))
	if !ok {
		return fmt.Errorf("the registry did not say how much of the upload it holds")
	}
	if loc, err := u.resolve(resp); err == nil {
		u.location = loc
	}
	u.offset = got
	return nil
}

// parseRange reads an upload's "Range: 0-<last>" as the number of bytes held. Registries
// answer "0-0" for an empty upload as well as for one byte; the chunks this sends are
// never one byte long, so it is read as empty.
func parseRange(h string) (int64, bool) {
	from, to, ok := strings.Cut(strings.TrimPrefix(h, "bytes="), "-")
	if !ok || from != "0" {
		return 0, false
	}
	last, err := strconv.ParseInt(to, 10, 64)
	if err != nil || last < 0 {
		return 0, false
	}
	if last == 0 {
		return 0, true
	}
	return last + 1, true
}

// finish closes the upload with the blob's digest.
func (u *uploader) finish(ctx context.Context, digest string) error {
	target := *u.location
	q := target.Query()
	q.Set("digest", digest)
	target.RawQuery = q.Encode()
	err := u.retry(ctx, func() error {
		_, err := u.do(ctx, http.MethodPut, &target, nil, nil, http.StatusCreated)
		return err
	}, nil)
	if err != nil {
		// The registry may have completed the upload and the answer been lost; then the
		// session is gone but the blob is there.
		blob := u.base.JoinPath("v2", u.repo, "blobs", digest)
		if _, herr := u.do(ctx, http.MethodHead, blob, nil, nil, http.StatusOK); herr == nil {
			return nil
		}
		return fmt.Errorf("completing the upload: %w", err)
	}
	return nil
}
