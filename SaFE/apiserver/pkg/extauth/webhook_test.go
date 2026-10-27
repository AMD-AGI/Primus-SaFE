/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package extauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeReviewer is a TokenReview server. Tokens in users are accepted; any other
// is "not authenticated". mode switches it to failing.
type fakeReviewer struct {
	srv   *httptest.Server
	mu    sync.Mutex
	users map[string]map[string]any // token -> status.user
	mode  atomic.Value              // "ok" | "503" | "401" | "garbage" | "slow"
	calls atomic.Int64
	last  atomic.Value // last request body (tokenReview)
	delay time.Duration
}

func newFakeReviewer(t *testing.T) *fakeReviewer {
	f := &fakeReviewer{users: map[string]map[string]any{}}
	f.mode.Store("ok")
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		var in tokenReview
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.last.Store(in)
		if f.delay > 0 {
			time.Sleep(f.delay)
		}
		switch f.mode.Load().(string) {
		case "503":
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		case "401":
			http.Error(w, "who are you", http.StatusUnauthorized)
			return
		case "garbage":
			_, _ = w.Write([]byte("<html>"))
			return
		}
		f.mu.Lock()
		user, ok := f.users[in.Spec.Token+"|"+in.Spec.Cookie]
		f.mu.Unlock()
		status := map[string]any{"authenticated": ok}
		if ok {
			status["user"] = user
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": tokenReviewAPIVersion, "kind": tokenReviewKind, "status": status})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeReviewer) accept(token, cookie string, user map[string]any) {
	f.mu.Lock()
	f.users[token+"|"+cookie] = user
	f.mu.Unlock()
}

func (f *fakeReviewer) revoke(token, cookie string) {
	f.mu.Lock()
	delete(f.users, token+"|"+cookie)
	f.mu.Unlock()
}

var aliceUser = map[string]any{
	"uid": "alice-id", "username": "alice", "groups": []string{"platform-admins"},
	"extra": map[string][]string{"email": {"alice@example.com"}},
}

type testClock struct{ t atomic.Int64 }

func newTestClock() *testClock {
	c := &testClock{}
	c.t.Store(time.Now().UnixNano())
	return c
}
func (c *testClock) now() time.Time          { return time.Unix(0, c.t.Load()) }
func (c *testClock) advance(d time.Duration) { c.t.Add(int64(d)) }

func newWebhook(t *testing.T, f *fakeReviewer, clock *testClock, mod func(*WebhookConfig)) *WebhookAuthenticator {
	cfg := WebhookConfig{URL: f.srv.URL, Now: clock.now}
	if mod != nil {
		mod(&cfg)
	}
	w, err := NewWebhookAuthenticator(cfg)
	require.NoError(t, err)
	return w
}

func review(w *WebhookAuthenticator, token, cookie string) (*Identity, error) {
	return w.Authenticate(context.Background(), Credentials{Bearer: token, Cookie: cookie, ClientIP: "10.1.2.3"})
}

func TestWebhookTokenReviewExchange(t *testing.T) {
	f := newFakeReviewer(t)
	f.accept("tok", "", map[string]any{
		"uid": "alice-id", "username": "alice", "groups": []string{"g1", "g2"},
		"extra": map[string][]string{
			"email": {"alice@example.com"}, "actor": {"svc-bot"}, "scopes": {"safe.read"}, "jti": {"j-9"},
			"exp": {strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)}, "restricted": {"true"},
		},
	})
	w := newWebhook(t, f, newTestClock(), nil)
	id, err := review(w, "tok", "")
	require.NoError(t, err)
	require.Equal(t, "alice-id", id.ID)
	require.Equal(t, "alice", id.Name)
	require.Equal(t, "alice@example.com", id.Email)
	require.Equal(t, []string{"g1", "g2"}, id.Groups)
	require.Equal(t, "svc-bot", id.Actor)
	require.Equal(t, []string{"safe.read"}, id.Scopes)
	require.Equal(t, "j-9", id.Jti)
	require.True(t, id.Restricted)
	require.False(t, id.Expiry.IsZero())

	in := f.last.Load().(tokenReview)
	require.Equal(t, "authentication.k8s.io/v1", in.APIVersion)
	require.Equal(t, "TokenReview", in.Kind)
	require.Equal(t, "tok", in.Spec.Token)
	require.Equal(t, "10.1.2.3", in.Spec.ClientIP)
}

func TestWebhookCarriesCookies(t *testing.T) {
	f := newFakeReviewer(t)
	f.accept("", "session-cookie", aliceUser)
	w := newWebhook(t, f, newTestClock(), nil)
	id, err := w.Authenticate(context.Background(), Credentials{Cookie: "session-cookie", UserType: "sso"})
	require.NoError(t, err)
	require.Equal(t, "alice-id", id.ID)
	require.Equal(t, "sso", f.last.Load().(tokenReview).Spec.UserType)

	_, err = w.Authenticate(context.Background(), Credentials{})
	require.ErrorIs(t, err, ErrNotApplicable, "nothing presented, nothing to ask")
}

func TestWebhookPositiveAndNegativeCache(t *testing.T) {
	f := newFakeReviewer(t)
	f.accept("good", "", aliceUser)
	clock := newTestClock()
	w := newWebhook(t, f, clock, func(c *WebhookConfig) { c.CacheTTL, c.CacheNegativeTTL = 30*time.Second, 5*time.Second })

	for i := 0; i < 5; i++ {
		_, err := review(w, "good", "")
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, f.calls.Load(), "an accepted credential is reviewed once per TTL")

	for i := 0; i < 5; i++ {
		_, err := review(w, "bad", "")
		require.ErrorIs(t, err, ErrUnauthenticated)
	}
	require.EqualValues(t, 2, f.calls.Load(), "a rejected credential is reviewed once per negative TTL")

	clock.advance(6 * time.Second)
	f.accept("bad", "", aliceUser) // it became valid; the negative entry has expired
	_, err := review(w, "bad", "")
	require.NoError(t, err)
	require.EqualValues(t, 3, f.calls.Load())

	clock.advance(25 * time.Second) // "good" is now past its TTL
	f.revoke("good", "")
	_, err = review(w, "good", "")
	require.ErrorIs(t, err, ErrUnauthenticated, "revocation is seen after the TTL")
}

func TestWebhookSingleflight(t *testing.T) {
	f := newFakeReviewer(t)
	f.accept("good", "", aliceUser)
	f.delay = 150 * time.Millisecond
	w := newWebhook(t, f, newTestClock(), nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := review(w, "good", "")
			require.NoError(t, err)
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, f.calls.Load(), "a burst with one credential is one review")
}

func TestWebhookStaleIfError(t *testing.T) {
	setup := func(t *testing.T, mod func(*WebhookConfig)) (*fakeReviewer, *testClock, *WebhookAuthenticator) {
		f := newFakeReviewer(t)
		f.accept("good", "", aliceUser)
		clock := newTestClock()
		w := newWebhook(t, f, clock, func(c *WebhookConfig) {
			c.CacheTTL = 30 * time.Second
			if mod != nil {
				mod(c)
			}
		})
		_, err := review(w, "good", "")
		require.NoError(t, err)
		return f, clock, w
	}

	t.Run("default window is 30 minutes", func(t *testing.T) {
		require.Equal(t, 30*time.Minute, DefaultStaleIfError)
		f, clock, w := setup(t, nil)
		f.mode.Store("503")
		clock.advance(30*time.Second + 29*time.Minute)
		id, err := review(w, "good", "")
		require.NoError(t, err, "the webhook is down but the credential was good a moment ago")
		require.Equal(t, "alice-id", id.ID)

		clock.advance(2 * time.Minute) // past ttl + 30m
		_, err = review(w, "good", "")
		var u *UnavailableError
		require.ErrorAs(t, err, &u, "beyond the window it is unavailable, never a pass")
	})

	t.Run("configured window", func(t *testing.T) {
		f, clock, w := setup(t, func(c *WebhookConfig) { c.StaleIfError = 5 * time.Minute })
		f.mode.Store("503")
		clock.advance(30*time.Second + 4*time.Minute)
		_, err := review(w, "good", "")
		require.NoError(t, err)
		clock.advance(2 * time.Minute)
		_, err = review(w, "good", "")
		require.Error(t, err)
	})

	t.Run("off", func(t *testing.T) {
		f, clock, w := setup(t, func(c *WebhookConfig) { c.StaleIfError = -1 })
		f.mode.Store("503")
		clock.advance(time.Minute)
		_, err := review(w, "good", "")
		require.Error(t, err)
	})

	t.Run("a live answer beats the stale entry", func(t *testing.T) {
		f, clock, w := setup(t, nil)
		f.revoke("good", "")
		clock.advance(time.Minute)
		_, err := review(w, "good", "")
		require.ErrorIs(t, err, ErrUnauthenticated, "stale is only for errors, not for a revocation")
	})

	t.Run("never for a credential that was not accepted", func(t *testing.T) {
		f, clock, w := setup(t, nil)
		_, err := review(w, "bad", "")
		require.ErrorIs(t, err, ErrUnauthenticated)
		f.mode.Store("503")
		clock.advance(10 * time.Second)
		_, err = review(w, "bad", "")
		var u *UnavailableError
		require.ErrorAs(t, err, &u)
		_, err = review(w, "never-seen", "")
		require.ErrorAs(t, err, &u)
	})

	t.Run("not past the credential's own expiry", func(t *testing.T) {
		f := newFakeReviewer(t)
		clock := newTestClock()
		f.accept("short", "", map[string]any{"uid": "alice-id", "extra": map[string][]string{
			"exp": {strconv.FormatInt(clock.now().Add(time.Minute).Unix(), 10)}}})
		w := newWebhook(t, f, clock, func(c *WebhookConfig) { c.CacheTTL = 10 * time.Second })
		_, err := review(w, "short", "")
		require.NoError(t, err)
		f.mode.Store("503")
		clock.advance(30 * time.Second)
		_, err = review(w, "short", "")
		require.NoError(t, err, "still inside its own lifetime")
		clock.advance(2 * time.Minute)
		_, err = review(w, "short", "")
		require.Error(t, err, "an expired credential is not revived by an outage")
	})
}

func TestWebhookErrorsAreNotAnswers(t *testing.T) {
	for _, mode := range []string{"503", "401", "garbage"} {
		t.Run(mode, func(t *testing.T) {
			f := newFakeReviewer(t)
			f.mode.Store(mode)
			f.accept("good", "", aliceUser)
			w := newWebhook(t, f, newTestClock(), nil)
			_, err := review(w, "good", "")
			var u *UnavailableError
			require.ErrorAs(t, err, &u)
			// And it was not remembered as a rejection: once the webhook recovers the
			// very next request succeeds.
			f.mode.Store("ok")
			_, err = review(w, "good", "")
			require.NoError(t, err)
		})
	}
}

func TestWebhookRefusesAnAcceptanceWithoutAUserId(t *testing.T) {
	f := newFakeReviewer(t)
	f.accept("tok", "", map[string]any{"username": "alice"})
	w := newWebhook(t, f, newTestClock(), nil)
	_, err := review(w, "tok", "")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNotApplicable)
}

func TestWebhookUnreachable(t *testing.T) {
	f := newFakeReviewer(t)
	url := f.srv.URL
	f.srv.Close()
	w, err := NewWebhookAuthenticator(WebhookConfig{URL: url, Timeout: time.Second})
	require.NoError(t, err)
	_, err = review(w, "tok", "")
	var u *UnavailableError
	require.ErrorAs(t, err, &u)
}

func TestWebhookConfigValidation(t *testing.T) {
	_, err := NewWebhookAuthenticator(WebhookConfig{})
	require.Error(t, err)
	_, err = NewWebhookAuthenticator(WebhookConfig{URL: "https://x.example.com", CABundle: "/no/such/file"})
	require.Error(t, err)
}

// The authenticator may apply address policy, so an acceptance for one address
// is not an acceptance for another.
func TestWebhookCacheIsPerClientAddress(t *testing.T) {
	f := newFakeReviewer(t)
	f.accept("good", "", aliceUser)
	w := newWebhook(t, f, newTestClock(), nil)
	_, err := w.Authenticate(context.Background(), Credentials{Bearer: "good", ClientIP: "10.0.0.1"})
	require.NoError(t, err)
	_, err = w.Authenticate(context.Background(), Credentials{Bearer: "good", ClientIP: "10.0.0.2"})
	require.NoError(t, err)
	require.EqualValues(t, 2, f.calls.Load(), "each address is reviewed on its own")
	_, err = w.Authenticate(context.Background(), Credentials{Bearer: "good", ClientIP: "10.0.0.1"})
	require.NoError(t, err)
	require.EqualValues(t, 2, f.calls.Load())
}

// A review that runs into its timeout must not be judged against the time the
// request began.
func TestWebhookStaleWindowIsMeasuredAfterTheFailedCall(t *testing.T) {
	f := newFakeReviewer(t)
	f.accept("good", "", aliceUser)
	clock := newTestClock()
	w := newWebhook(t, f, clock, func(c *WebhookConfig) { c.CacheTTL, c.StaleIfError = 10*time.Second, time.Minute })
	_, err := review(w, "good", "")
	require.NoError(t, err)

	clock.advance(50 * time.Second) // inside ttl + stale (70s) when the request begins
	f.mode.Store("503")
	w.cfg.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		clock.advance(30 * time.Second) // the call fails slowly: 80s since the entry was fetched
		return nil, context.DeadlineExceeded
	})}
	_, err = review(w, "good", "")
	var u *UnavailableError
	require.ErrorAs(t, err, &u, "the window had passed by the time the call failed")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
