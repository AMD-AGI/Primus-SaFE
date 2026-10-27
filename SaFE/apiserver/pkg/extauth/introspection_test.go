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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testClientID = "svc-safe"
	testSecret   = "s3cret-0123456789"
)

// introspectionCall is what the fake server saw of one request.
type introspectionCall struct {
	Method, ContentType, Accept string
	BasicID, BasicSecret        string
	HasBasic                    bool
	Form                        map[string]string
}

// fakeIntrospector is an RFC 7662 server. A token in answers is active with those members;
// any other token is {"active": false}. mode switches it to failing.
type fakeIntrospector struct {
	srv     *httptest.Server
	mu      sync.Mutex
	answers map[string]map[string]any
	// discovery, when set, is served at /.well-known/openid-configuration.
	discovery atomic.Value // map[string]any
	mode      atomic.Value // "ok" | "503" | "401" | "403" | "garbage" | "noactive"
	calls     atomic.Int64
	last      atomic.Value // introspectionCall
	delay     time.Duration
}

func newFakeIntrospector(t *testing.T) *fakeIntrospector {
	f := &fakeIntrospector{answers: map[string]map[string]any{}}
	f.mode.Store("ok")
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			doc, _ := f.discovery.Load().(map[string]any)
			if doc == nil {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(doc)
			return
		}
		f.calls.Add(1)
		_ = r.ParseForm()
		call := introspectionCall{Method: r.Method, ContentType: r.Header.Get("Content-Type"), Accept: r.Header.Get("Accept"), Form: map[string]string{}}
		call.BasicID, call.BasicSecret, call.HasBasic = r.BasicAuth()
		for k := range r.PostForm {
			call.Form[k] = r.PostForm.Get(k)
		}
		f.last.Store(call)
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
		case "403":
			http.Error(w, "not allowed to introspect", http.StatusForbidden)
			return
		case "garbage":
			_, _ = w.Write([]byte("<html>"))
			return
		case "noactive":
			_, _ = w.Write([]byte(`{"sub":"alice-id","username":"alice"}`))
			return
		}
		// What the real endpoint does first: it knows its callers.
		if !call.HasBasic || call.BasicID != testClientID || call.BasicSecret != testSecret {
			http.Error(w, "who are you", http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		answer, ok := f.answers[call.Form["token"]]
		f.mu.Unlock()
		out := map[string]any{"active": false}
		if ok {
			out = map[string]any{"active": true}
			for k, v := range answer {
				out[k] = v
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIntrospector) accept(token string, answer map[string]any) {
	f.mu.Lock()
	f.answers[token] = answer
	f.mu.Unlock()
}

func (f *fakeIntrospector) revoke(token string) {
	f.mu.Lock()
	delete(f.answers, token)
	f.mu.Unlock()
}

func (f *fakeIntrospector) lastCall() introspectionCall { return f.last.Load().(introspectionCall) }

var aliceAnswer = map[string]any{
	"sub": "alice-id", "username": "alice@example.com", "name": "Alice", "email": "alice@example.com", "groups": []string{"platform-admins"},
}

type testClock struct{ t atomic.Int64 }

func newTestClock() *testClock {
	c := &testClock{}
	c.t.Store(time.Now().UnixNano())
	return c
}
func (c *testClock) now() time.Time          { return time.Unix(0, c.t.Load()) }
func (c *testClock) advance(d time.Duration) { c.t.Add(int64(d)) }

// secretFile writes the client secret where the authenticator will read it.
func secretFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "client-secret")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func newIntrospection(t *testing.T, f *fakeIntrospector, clock *testClock, mod func(*IntrospectionConfig)) *IntrospectionAuthenticator {
	cfg := IntrospectionConfig{URL: f.srv.URL, ClientID: testClientID, ClientSecretFile: secretFile(t, testSecret+"\n"), Now: clock.now}
	if mod != nil {
		mod(&cfg)
	}
	a, err := NewIntrospectionAuthenticator(cfg)
	require.NoError(t, err)
	return a
}

func introspect(a *IntrospectionAuthenticator, token, cookie string) (*Identity, error) {
	return a.Authenticate(context.Background(), Credentials{Bearer: token, Cookie: cookie, ClientIP: "10.1.2.3"})
}

func TestIntrospectionRequestAndIdentity(t *testing.T) {
	f := newFakeIntrospector(t)
	exp := time.Now().Add(time.Hour).Unix()
	f.accept("tok", map[string]any{
		"sub": "alice-id", "username": "alice@example.com", "name": "Alice", "email": "alice@example.com",
		"groups": []string{"g1", "g2"}, "roles": []string{"g2", "default"}, "scope": "safe.read safe.api", "jti": "j-9",
		"exp": exp, "iat": exp - 300, "token_type": "access_token", "restricted": true,
		"act": map[string]any{"sub": "svc-bot", "client_id": "svc-bot"},
	})
	a := newIntrospection(t, f, newTestClock(), nil)
	id, err := introspect(a, "tok", "")
	require.NoError(t, err)
	require.Equal(t, "alice-id", id.ID)
	require.Equal(t, "Alice", id.Name)
	require.Equal(t, "alice@example.com", id.Email)
	require.Equal(t, []string{"g1", "g2", "default"}, id.Groups, "groups and roles are both group names, once each")
	require.Equal(t, "svc-bot", id.Actor)
	require.True(t, id.Delegated())
	require.Equal(t, []string{"safe.read", "safe.api"}, id.Scopes)
	require.Equal(t, "j-9", id.Jti)
	require.True(t, id.Restricted)
	require.Equal(t, exp, id.Expiry.Unix())

	call := f.lastCall()
	require.Equal(t, http.MethodPost, call.Method)
	require.Equal(t, "application/x-www-form-urlencoded", call.ContentType)
	require.Equal(t, "application/json", call.Accept)
	require.True(t, call.HasBasic)
	require.Equal(t, testClientID, call.BasicID)
	require.Equal(t, testSecret, call.BasicSecret, "the secret is read from the file, without its trailing newline")
	require.Equal(t, map[string]string{"token": "tok", "client_ip": "10.1.2.3"}, call.Form)
}

// The way each member of the answer becomes the identity SaFE authorizes.
func TestIntrospectionMapsTheAnswer(t *testing.T) {
	cases := []struct {
		name   string
		answer map[string]any
		check  func(t *testing.T, id *Identity)
	}{
		{"name falls back to username, then to the id", map[string]any{"sub": "u1", "username": "uname"}, func(t *testing.T, id *Identity) {
			require.Equal(t, "uname", id.Name)
		}},
		{"nothing but a sub", map[string]any{"sub": "u1"}, func(t *testing.T, id *Identity) {
			require.Equal(t, "u1", id.Name)
			require.Empty(t, id.Email)
			require.Empty(t, id.Groups)
			require.False(t, id.Delegated())
			require.True(t, id.Expiry.IsZero())
		}},
		{"an email username is the email when none is given", map[string]any{"sub": "u1", "username": "u@example.com"}, func(t *testing.T, id *Identity) {
			require.Equal(t, "u@example.com", id.Email)
		}},
		{"a username that is no email is not an email", map[string]any{"sub": "u1", "username": "uname"}, func(t *testing.T, id *Identity) {
			require.Empty(t, id.Email)
		}},
		{"roles alone are groups too", map[string]any{"sub": "u1", "roles": []string{"system-admin", "default"}}, func(t *testing.T, id *Identity) {
			require.Equal(t, []string{"system-admin", "default"}, id.Groups)
		}},
		{"act by client_id", map[string]any{"sub": "u1", "act": map[string]any{"client_id": "svc-a"}}, func(t *testing.T, id *Identity) {
			require.Equal(t, "svc-a", id.Actor)
			require.True(t, id.Delegated())
		}},
		{"act by sub (RFC 8693)", map[string]any{"sub": "u1", "act": map[string]any{"sub": "svc-b"}}, func(t *testing.T, id *Identity) {
			require.Equal(t, "svc-b", id.Actor)
		}},
		{"no act: not delegated", map[string]any{"sub": "u1", "token_type": "api_key"}, func(t *testing.T, id *Identity) {
			require.False(t, id.Delegated())
			require.Empty(t, id.Scopes)
		}},
		{"exp as a fractional number", map[string]any{"sub": "u1", "exp": 1893456000.0}, func(t *testing.T, id *Identity) {
			require.Equal(t, int64(1893456000), id.Expiry.Unix())
		}},
		{"scope with extra spaces", map[string]any{"sub": "u1", "scope": " safe.read  safe.api "}, func(t *testing.T, id *Identity) {
			require.Equal(t, []string{"safe.read", "safe.api"}, id.Scopes)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeIntrospector(t)
			f.accept("tok", tc.answer)
			id, err := introspect(newIntrospection(t, f, newTestClock(), nil), "tok", "")
			require.NoError(t, err)
			tc.check(t, id)
		})
	}
}

// An answer that cannot be turned into an identity SaFE can safely authorize is an error,
// never a pass and never a cached rejection.
func TestIntrospectionRefusesAnswersItCannotTrust(t *testing.T) {
	cases := map[string]map[string]any{
		"no sub":                         {"username": "alice"},
		"an act that names no actor":     {"sub": "u1", "act": map[string]any{}},
		"an act with only blank members": {"sub": "u1", "act": map[string]any{"client_id": "", "sub": ""}},
		"a malformed exp":                {"sub": "u1", "exp": "soon"},
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeIntrospector(t)
			f.accept("tok", answer)
			a := newIntrospection(t, f, newTestClock(), nil)
			_, err := introspect(a, "tok", "")
			var u *UnavailableError
			require.ErrorAs(t, err, &u)
			require.NotErrorIs(t, err, ErrNotApplicable)
			require.NotErrorIs(t, err, ErrUnauthenticated)
		})
	}
}

func TestIntrospectionInactiveIsRejected(t *testing.T) {
	f := newFakeIntrospector(t)
	a := newIntrospection(t, f, newTestClock(), nil)
	_, err := introspect(a, "unknown", "")
	require.ErrorIs(t, err, ErrUnauthenticated)

	// An inactive answer is a rejection whatever else it says: the members of an
	// active answer next to "active": false do not make the credential good.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"active": false, "sub": "alice-id", "username": "alice", "roles": ["system-admin"]}`))
	}))
	defer srv.Close()
	a2 := newIntrospection(t, f, newTestClock(), func(c *IntrospectionConfig) { c.URL = srv.URL })
	id, err := introspect(a2, "tok", "")
	require.ErrorIs(t, err, ErrUnauthenticated)
	require.Nil(t, id)

	// An answer without the member is not an RFC 7662 answer: it is neither a pass nor a rejection.
	f.mode.Store("noactive")
	_, err = introspect(newIntrospection(t, f, newTestClock(), nil), "tok", "")
	var u *UnavailableError
	require.ErrorAs(t, err, &u)
}

func TestIntrospectionCarriesCookiesAndUserType(t *testing.T) {
	f := newFakeIntrospector(t)
	f.accept("session-cookie", aliceAnswer)
	f.accept("the-bearer", aliceAnswer)
	a := newIntrospection(t, f, newTestClock(), nil)

	id, err := a.Authenticate(context.Background(), Credentials{Cookie: "session-cookie", UserType: "sso"})
	require.NoError(t, err)
	require.Equal(t, "alice-id", id.ID)
	require.Equal(t, map[string]string{"token": "session-cookie", "user_type": "sso"}, f.lastCall().Form, "the cookie's value is the token; no address, none given")

	_, err = a.Authenticate(context.Background(), Credentials{Bearer: "the-bearer", Cookie: "other-cookie"})
	require.NoError(t, err)
	require.Equal(t, "the-bearer", f.lastCall().Form["token"], "an explicit bearer token beats an ambient cookie")

	_, err = a.Authenticate(context.Background(), Credentials{})
	require.ErrorIs(t, err, ErrNotApplicable, "nothing presented, nothing to ask")
}

func TestIntrospectionPositiveAndNegativeCache(t *testing.T) {
	f := newFakeIntrospector(t)
	f.accept("good", aliceAnswer)
	clock := newTestClock()
	a := newIntrospection(t, f, clock, func(c *IntrospectionConfig) { c.CacheTTL, c.CacheNegativeTTL = 30*time.Second, 5*time.Second })

	for i := 0; i < 5; i++ {
		_, err := introspect(a, "good", "")
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, f.calls.Load(), "an accepted credential is introspected once per TTL")

	for i := 0; i < 5; i++ {
		_, err := introspect(a, "bad", "")
		require.ErrorIs(t, err, ErrUnauthenticated)
	}
	require.EqualValues(t, 2, f.calls.Load(), "a rejected credential is introspected once per negative TTL")

	clock.advance(6 * time.Second)
	f.accept("bad", aliceAnswer) // it became valid; the negative entry has expired
	_, err := introspect(a, "bad", "")
	require.NoError(t, err)
	require.EqualValues(t, 3, f.calls.Load())

	clock.advance(25 * time.Second) // "good" is now past its TTL
	f.revoke("good")
	_, err = introspect(a, "good", "")
	require.ErrorIs(t, err, ErrUnauthenticated, "revocation is seen after the TTL")
}

func TestIntrospectionSingleflight(t *testing.T) {
	f := newFakeIntrospector(t)
	f.accept("good", aliceAnswer)
	f.delay = 150 * time.Millisecond
	a := newIntrospection(t, f, newTestClock(), nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := introspect(a, "good", "")
			require.NoError(t, err)
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, f.calls.Load(), "a burst with one credential is one introspection")
}

func TestIntrospectionStaleIfError(t *testing.T) {
	setup := func(t *testing.T, mod func(*IntrospectionConfig)) (*fakeIntrospector, *testClock, *IntrospectionAuthenticator) {
		f := newFakeIntrospector(t)
		f.accept("good", aliceAnswer)
		clock := newTestClock()
		a := newIntrospection(t, f, clock, func(c *IntrospectionConfig) {
			c.CacheTTL = 30 * time.Second
			if mod != nil {
				mod(c)
			}
		})
		_, err := introspect(a, "good", "")
		require.NoError(t, err)
		return f, clock, a
	}

	t.Run("default window is 30 minutes", func(t *testing.T) {
		require.Equal(t, 30*time.Minute, DefaultStaleIfError)
		f, clock, a := setup(t, nil)
		f.mode.Store("503")
		clock.advance(30*time.Second + 29*time.Minute)
		id, err := introspect(a, "good", "")
		require.NoError(t, err, "the endpoint is down but the credential was good a moment ago")
		require.Equal(t, "alice-id", id.ID)

		clock.advance(2 * time.Minute) // past ttl + 30m
		_, err = introspect(a, "good", "")
		var u *UnavailableError
		require.ErrorAs(t, err, &u, "beyond the window it is unavailable, never a pass")
	})

	t.Run("configured window", func(t *testing.T) {
		f, clock, a := setup(t, func(c *IntrospectionConfig) { c.StaleIfError = 5 * time.Minute })
		f.mode.Store("503")
		clock.advance(30*time.Second + 4*time.Minute)
		_, err := introspect(a, "good", "")
		require.NoError(t, err)
		clock.advance(2 * time.Minute)
		_, err = introspect(a, "good", "")
		require.Error(t, err)
	})

	t.Run("off", func(t *testing.T) {
		f, clock, a := setup(t, func(c *IntrospectionConfig) { c.StaleIfError = -1 })
		f.mode.Store("503")
		clock.advance(time.Minute)
		_, err := introspect(a, "good", "")
		require.Error(t, err)
	})

	t.Run("a live answer beats the stale entry", func(t *testing.T) {
		f, clock, a := setup(t, nil)
		f.revoke("good")
		clock.advance(time.Minute)
		_, err := introspect(a, "good", "")
		require.ErrorIs(t, err, ErrUnauthenticated, "stale is only for errors, not for a revocation")
	})

	t.Run("never for a credential that was not accepted", func(t *testing.T) {
		f, clock, a := setup(t, nil)
		_, err := introspect(a, "bad", "")
		require.ErrorIs(t, err, ErrUnauthenticated)
		f.mode.Store("503")
		clock.advance(10 * time.Second)
		_, err = introspect(a, "bad", "")
		var u *UnavailableError
		require.ErrorAs(t, err, &u)
		_, err = introspect(a, "never-seen", "")
		require.ErrorAs(t, err, &u)
	})

	t.Run("not past the credential's own expiry", func(t *testing.T) {
		f := newFakeIntrospector(t)
		clock := newTestClock()
		f.accept("short", map[string]any{"sub": "alice-id", "exp": clock.now().Add(time.Minute).Unix()})
		a := newIntrospection(t, f, clock, func(c *IntrospectionConfig) { c.CacheTTL = 10 * time.Second })
		_, err := introspect(a, "short", "")
		require.NoError(t, err)
		f.mode.Store("503")
		clock.advance(30 * time.Second)
		_, err = introspect(a, "short", "")
		require.NoError(t, err, "still inside its own lifetime")
		clock.advance(2 * time.Minute)
		_, err = introspect(a, "short", "")
		require.Error(t, err, "an expired credential is not revived by an outage")
	})
}

// Whatever the endpoint answers other than a 200 JSON object, the introspection did not happen.
func TestIntrospectionErrorsAreNotAnswers(t *testing.T) {
	for _, mode := range []string{"503", "401", "403", "garbage"} {
		t.Run(mode, func(t *testing.T) {
			f := newFakeIntrospector(t)
			f.mode.Store(mode)
			f.accept("good", aliceAnswer)
			a := newIntrospection(t, f, newTestClock(), nil)
			_, err := introspect(a, "good", "")
			var u *UnavailableError
			require.ErrorAs(t, err, &u)
			// And it was not remembered as a rejection: once the endpoint recovers the
			// very next request succeeds.
			f.mode.Store("ok")
			_, err = introspect(a, "good", "")
			require.NoError(t, err)
		})
	}
}

func TestIntrospectionUnreachable(t *testing.T) {
	f := newFakeIntrospector(t)
	url := f.srv.URL
	f.srv.Close()
	a, err := NewIntrospectionAuthenticator(IntrospectionConfig{URL: url, ClientID: testClientID, ClientSecretFile: secretFile(t, testSecret), Timeout: time.Second})
	require.NoError(t, err)
	_, err = introspect(a, "tok", "")
	var u *UnavailableError
	require.ErrorAs(t, err, &u)
}

// The client authenticates to the endpoint; the secret comes from the file each time, so a
// rotated Secret is picked up, and it is never part of an error.
func TestIntrospectionClientSecretFromFile(t *testing.T) {
	f := newFakeIntrospector(t)
	f.accept("tok", aliceAnswer)
	path := secretFile(t, "wrong-secret")
	clock := newTestClock()
	a := newIntrospection(t, f, clock, func(c *IntrospectionConfig) { c.ClientSecretFile = path })

	_, err := introspect(a, "tok", "")
	var u *UnavailableError
	require.ErrorAs(t, err, &u, "the endpoint refuses this client: that is an outage of authentication, not a bad credential")
	require.NotContains(t, err.Error(), "wrong-secret")

	require.NoError(t, os.WriteFile(path, []byte(testSecret), 0o600)) // rotated
	_, err = introspect(a, "tok", "")
	require.NoError(t, err)
	require.Equal(t, testSecret, f.lastCall().BasicSecret)

	require.NoError(t, os.Remove(path))
	clock.advance(time.Minute)
	_, err = introspect(a, "other", "")
	require.ErrorAs(t, err, &u, "a secret that cannot be read is never an answer")
	require.NotContains(t, err.Error(), testSecret)
}

func TestIntrospectionConfigValidation(t *testing.T) {
	good := IntrospectionConfig{URL: "https://iam.example.com/oauth/introspect", ClientID: "c", ClientSecretFile: secretFile(t, "s")}
	_, err := NewIntrospectionAuthenticator(good)
	require.NoError(t, err)
	for name, mod := range map[string]func(*IntrospectionConfig){
		"no url or issuer":      func(c *IntrospectionConfig) { c.URL = "" },
		"no client id":          func(c *IntrospectionConfig) { c.ClientID = "" },
		"no secret file":        func(c *IntrospectionConfig) { c.ClientSecretFile = "" },
		"secret file is absent": func(c *IntrospectionConfig) { c.ClientSecretFile = "/no/such/file" },
		"secret file is empty":  func(c *IntrospectionConfig) { c.ClientSecretFile = secretFile(t, " \n") },
		"secret file is huge":   func(c *IntrospectionConfig) { c.ClientSecretFile = secretFile(t, strings.Repeat("x", 5000)) },
		"unreadable CA bundle":  func(c *IntrospectionConfig) { c.CABundle = "/no/such/file" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := good
			mod(&cfg)
			_, err := NewIntrospectionAuthenticator(cfg)
			require.Error(t, err)
		})
	}
	// An issuer alone is enough: the endpoint comes from its discovery document.
	cfg := good
	cfg.URL, cfg.Issuer = "", "https://iam.example.com"
	_, err = NewIntrospectionAuthenticator(cfg)
	require.NoError(t, err)
}

func TestIntrospectionDiscovery(t *testing.T) {
	f := newFakeIntrospector(t)
	f.accept("tok", aliceAnswer)
	doc := func(issuer, endpoint string) map[string]any {
		d := map[string]any{"issuer": issuer}
		if endpoint != "" {
			d["introspection_endpoint"] = endpoint
		}
		return d
	}
	newA := func(mod func(*IntrospectionConfig)) *IntrospectionAuthenticator {
		return newIntrospection(t, f, newTestClock(), func(c *IntrospectionConfig) {
			c.URL, c.Issuer = "", f.srv.URL+"/"
			if mod != nil {
				mod(c)
			}
		})
	}

	// Not published yet: unavailable (and retried), not a rejection.
	a := newA(nil)
	_, err := introspect(a, "tok", "")
	var u *UnavailableError
	require.ErrorAs(t, err, &u)
	require.EqualValues(t, 0, f.calls.Load(), "no introspection without an endpoint")

	f.discovery.Store(doc(f.srv.URL, f.srv.URL+"/oauth/introspect"))
	_, err = introspect(a, "tok", "")
	require.NoError(t, err, "a failed discovery is retried")
	require.EqualValues(t, 1, f.calls.Load())
	_, err = introspect(a, "tok2", "")
	require.ErrorIs(t, err, ErrUnauthenticated)
	f.discovery.Store(map[string]any(nil))
	_, err = introspect(a, "tok3", "")
	require.ErrorIs(t, err, ErrUnauthenticated, "the endpoint is discovered once")

	for name, d := range map[string]map[string]any{
		"another issuer":      doc("https://other.example.com", f.srv.URL+"/oauth/introspect"),
		"no endpoint in it":   doc(f.srv.URL, ""),
		"a document of junk":  {"issuer": 5},
		"an issuer with path": doc(f.srv.URL+"/tenant", f.srv.URL+"/oauth/introspect"),
	} {
		t.Run(name, func(t *testing.T) {
			f.discovery.Store(d)
			_, err := introspect(newA(nil), "tok", "")
			var u *UnavailableError
			require.ErrorAs(t, err, &u)
		})
	}
}

// A token of another issuer is not one this authenticator may vouch for, even if the
// endpoint says it is active.
func TestIntrospectionChecksTheIssuer(t *testing.T) {
	f := newFakeIntrospector(t)
	f.accept("ours", map[string]any{"sub": "u1", "iss": "https://iam.example.com"})
	f.accept("ours-slash", map[string]any{"sub": "u1", "iss": "https://iam.example.com/"})
	f.accept("theirs", map[string]any{"sub": "u1", "iss": "https://elsewhere.example.com"})
	f.accept("silent", map[string]any{"sub": "u1"})
	a := newIntrospection(t, f, newTestClock(), func(c *IntrospectionConfig) { c.Issuer = "https://iam.example.com/" })
	for _, tok := range []string{"ours", "ours-slash", "silent"} {
		_, err := introspect(a, tok, "")
		require.NoError(t, err, tok)
	}
	_, err := introspect(a, "theirs", "")
	require.ErrorIs(t, err, ErrUnauthenticated)

	// With no issuer configured nothing is compared.
	b := newIntrospection(t, f, newTestClock(), nil)
	_, err = introspect(b, "theirs", "")
	require.NoError(t, err)
}

// The authenticator may apply address policy, user types and cookies mean different
// things, so an acceptance for one caller context is not an acceptance for another.
func TestIntrospectionCacheKeyIncludesTheCallerContext(t *testing.T) {
	f := newFakeIntrospector(t)
	f.accept("good", aliceAnswer)
	a := newIntrospection(t, f, newTestClock(), nil)
	ctx := context.Background()

	steps := []struct {
		name  string
		cred  Credentials
		calls int64 // total after this step
	}{
		{"first address", Credentials{Bearer: "good", ClientIP: "10.0.0.1"}, 1},
		{"second address is asked on its own", Credentials{Bearer: "good", ClientIP: "10.0.0.2"}, 2},
		{"first address again is cached", Credentials{Bearer: "good", ClientIP: "10.0.0.1"}, 2},
		{"another user type is asked on its own", Credentials{Bearer: "good", ClientIP: "10.0.0.1", UserType: "sso"}, 3},
		{"the same string as a cookie is asked on its own", Credentials{Cookie: "good", ClientIP: "10.0.0.1"}, 4},
		{"no address is asked on its own", Credentials{Bearer: "good"}, 5},
		{"and each of them is cached", Credentials{Cookie: "good", ClientIP: "10.0.0.1"}, 5},
	}
	for _, st := range steps {
		_, err := a.Authenticate(ctx, st.cred)
		require.NoError(t, err, st.name)
		require.Equal(t, st.calls, f.calls.Load(), st.name)
	}

	// The key cannot be confused by moving text between fields.
	require.NotEqual(t, cacheKey(Credentials{Bearer: "ab", Cookie: "c"}), cacheKey(Credentials{Bearer: "a", Cookie: "bc"}))
}

// A call that runs into its timeout must not be judged against the time the request began.
func TestIntrospectionStaleWindowIsMeasuredAfterTheFailedCall(t *testing.T) {
	f := newFakeIntrospector(t)
	f.accept("good", aliceAnswer)
	clock := newTestClock()
	a := newIntrospection(t, f, clock, func(c *IntrospectionConfig) { c.CacheTTL, c.StaleIfError = 10*time.Second, time.Minute })
	_, err := introspect(a, "good", "")
	require.NoError(t, err)

	// The cache entry is 69s old when the request starts: inside the window. The failing call
	// takes a further 3s on the clock, so by the time it has failed the entry is outside it.
	clock.advance(69 * time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clock.advance(3 * time.Second)
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	a.cfg.URL = srv.URL
	_, err = introspect(a, "good", "")
	var u *UnavailableError
	require.ErrorAs(t, err, &u, "the window is measured from the end of the failed call")
}
