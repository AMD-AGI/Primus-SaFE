/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commoncrypto "github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

const autoTestEmail = "test@amd.com"

// ── Fakes ─────────────────────────────────────────────────────────────────

// bindingStore is a primary-key keyed stand-in for the binding table. Creating
// a row that exists fails, as the primary key would.
type bindingStore struct {
	mu        sync.Mutex
	rows      map[string]dbclient.LLMGatewayUserBinding
	creates   int // CreateLLMBinding attempts, including failed ones
	updates   int
	getErr    error
	createErr error
}

func newBindingStore(seed ...*dbclient.LLMGatewayUserBinding) *bindingStore {
	s := &bindingStore{rows: map[string]dbclient.LLMGatewayUserBinding{}}
	for _, b := range seed {
		s.rows[b.UserEmail] = *b
	}
	return s
}

func (s *bindingStore) get(_ context.Context, email string) (*dbclient.LLMGatewayUserBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return nil, s.getErr
	}
	row, ok := s.rows[email]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (s *bindingStore) create(_ context.Context, b *dbclient.LLMGatewayUserBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates++
	if s.createErr != nil {
		return s.createErr
	}
	if _, ok := s.rows[b.UserEmail]; ok {
		return errors.New("duplicate key value violates unique constraint")
	}
	b.CreatedAt = time.Now()
	s.rows[b.UserEmail] = *b
	return nil
}

func (s *bindingStore) update(_ context.Context, b *dbclient.LLMGatewayUserBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates++
	s.rows[b.UserEmail] = *b
	return nil
}

func (s *bindingStore) row(email string) (dbclient.LLMGatewayUserBinding, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[email]
	return row, ok
}

func (s *bindingStore) counts() (rows, creates, updates int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows), s.creates, s.updates
}

func (s *bindingStore) mockDB(ctrl *gomock.Controller) *mock_client.MockInterface {
	db := mock_client.NewMockInterface(ctrl)
	db.EXPECT().GetLLMBindingByEmail(gomock.Any(), gomock.Any()).DoAndReturn(s.get).AnyTimes()
	db.EXPECT().CreateLLMBinding(gomock.Any(), gomock.Any()).DoAndReturn(s.create).AnyTimes()
	db.EXPECT().UpdateLLMBinding(gomock.Any(), gomock.Any()).DoAndReturn(s.update).AnyTimes()
	return db
}

// advisoryLocks emulates pg_try_advisory_xact_lock across the replicas that
// share one database: a second taker of a held key is refused, not queued.
type advisoryLocks struct {
	mu   sync.Mutex
	held map[string]bool
	busy atomic.Int32 // refusals handed out
}

func (l *advisoryLocks) tryLock(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held == nil {
		l.held = map[string]bool{}
	}
	if l.held[key] {
		l.busy.Add(1)
		return false
	}
	l.held[key] = true
	return true
}

func (l *advisoryLocks) unlock(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.held, key)
}

// lockingDB is a database client that also serializes per-user binding work,
// as the real client does.
type lockingDB struct {
	*mock_client.MockInterface
	locks *advisoryLocks
}

func (d *lockingDB) WithLLMBindingAdvisoryLock(ctx context.Context, email string, fn func(context.Context) error) error {
	key := strings.ToLower(email)
	if !d.locks.tryLock(key) {
		return dbclient.ErrLLMBindingLockBusy
	}
	defer d.locks.unlock(key)
	return fn(ctx)
}

// upstreamCall is one proxied request as LiteLLM received it.
type upstreamCall struct {
	auth         string
	extraHeaders map[string]string
	hasExtra     bool
}

// fakeLiteLLM serves both the admin API and the proxied model endpoint.
type fakeLiteLLM struct {
	srv *httptest.Server

	mu          sync.Mutex
	userNew     int
	keyGenerate int
	keyUpdate   int
	keyDelete   int
	generated   []map[string]string // metadata of each /key/generate
	updated     []map[string]string // metadata of each /key/update
	deleted     [][]string          // keys of each /key/delete
	upstream    []upstreamCall

	// gate, when set, holds /key/generate until it is closed.
	gate chan struct{}
	// failGenerate makes /key/generate answer 503.
	failGenerate atomic.Bool
	// aliasTaken makes /key/generate answer "alias exists" and runs onAliasTaken.
	aliasTaken   atomic.Bool
	onAliasTaken func()
}

func newFakeLiteLLM(t *testing.T) *fakeLiteLLM {
	f := &fakeLiteLLM{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve(t)))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLiteLLM) serve(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/user/new":
			f.mu.Lock()
			f.userNew++
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/key/generate":
			if f.gate != nil {
				<-f.gate
			}
			var req CreateKeyRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.mu.Lock()
			f.keyGenerate++
			n := f.keyGenerate
			f.generated = append(f.generated, req.Metadata)
			f.mu.Unlock()
			if f.failGenerate.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			if f.aliasTaken.Load() {
				if f.onAliasTaken != nil {
					f.onAliasTaken()
				}
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"Key with alias 'x' already exists."}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(CreateKeyResponse{
				Key:     fmt.Sprintf("vkey-%d", n),
				TokenID: fmt.Sprintf("hash-%d", n),
			})
		case r.URL.Path == "/key/update":
			var req UpdateKeyRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.mu.Lock()
			f.keyUpdate++
			f.updated = append(f.updated, req.Metadata)
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/key/delete":
			var req DeleteKeyRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.mu.Lock()
			f.keyDelete++
			f.deleted = append(f.deleted, req.Keys)
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/v1/"):
			raw, _ := io.ReadAll(r.Body)
			call := upstreamCall{auth: r.Header.Get("Authorization")}
			var body struct {
				Extra map[string]string `json:"extra_headers"`
			}
			if json.Unmarshal(raw, &body) == nil && body.Extra != nil {
				call.hasExtra = true
				call.extraHeaders = body.Extra
			}
			f.mu.Lock()
			f.upstream = append(f.upstream, call)
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Errorf("unexpected LiteLLM path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func (f *fakeLiteLLM) snapshot() (userNew, generate, update, del int, upstream []upstreamCall) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.userNew, f.keyGenerate, f.keyUpdate, f.keyDelete, append([]upstreamCall(nil), f.upstream...)
}

// apimHeader returns the APIM subscription header the upstream request carried.
func (c upstreamCall) apimHeader() (string, bool) {
	for name, value := range c.extraHeaders {
		if strings.EqualFold(name, apimKeyHeader) {
			return value, true
		}
	}
	return "", false
}

func newAutoHandler(db dbclient.Interface, f *fakeLiteLLM) *Handler {
	proxy, err := newLLMProxy(f.srv.URL)
	if err != nil {
		panic(err)
	}
	return &Handler{
		dbClient:      db,
		litellmClient: NewLiteLLMClient(f.srv.URL, "test-master-key", "test-team-id"),
		crypto:        &commoncrypto.Crypto{},
		proxy:         proxy,
	}
}

// newGatewayServer serves the proxy and the binding endpoints for one user.
func newGatewayServer(t *testing.T, h *Handler, email string) *httptest.Server {
	router := gin.New()
	as := func(fn gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			setUserContext(c, "user1", email)
			fn(c)
		}
	}
	router.Any("/api/v1/llm-proxy/*proxyPath", as(h.ProxyLLMRequest))
	router.POST("/binding", as(h.CreateBinding))
	router.PUT("/binding", as(h.UpdateBinding))
	router.GET("/binding", as(h.GetBinding))
	router.GET("/binding/apim-key", as(h.RevealApimKey))
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func chat(t *testing.T, srv *httptest.Server, body string) int {
	t.Helper()
	if body == "" {
		body = `{"model":"m","messages":[]}`
	}
	resp, err := http.Post(srv.URL+"/api/v1/llm-proxy/v1/chat/completions", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func send(t *testing.T, srv *httptest.Server, method, path, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func seededBinding(apimKey string) *dbclient.LLMGatewayUserBinding {
	b := &dbclient.LLMGatewayUserBinding{
		UserEmail:         autoTestEmail,
		LiteLLMVirtualKey: "seed-vkey",
		LiteLLMKeyHash:    "seed-hash",
		KeyAlias:          autoTestEmail,
	}
	if apimKey != "" {
		b.ApimKey = apimKey
		b.ApimKeyHash = dbclient.ComputeApimKeyHash(apimKey)
	}
	return b
}

// ── The proxy resolves the caller's binding ───────────────────────────────

func TestProxyLLMRequest_BindingResolution(t *testing.T) {
	const clientAPIM = `{"model":"m","extra_headers":{"ocp-apim-subscription-key":"from-client","X-Other":"kept"}}`

	tests := []struct {
		name         string
		seed         *dbclient.LLMGatewayUserBinding
		body         string
		requests     int
		wantGenerate int
		wantAuth     string
		wantAPIM     string // "" means the header must be absent
		wantOther    bool   // the client's unrelated extra header survives
	}{
		{
			name:         "no binding: created on the first request and reused afterwards",
			requests:     3,
			wantGenerate: 1,
			wantAuth:     "Bearer vkey-1",
		},
		{
			name:         "no binding: a client-supplied APIM header is not forwarded",
			body:         clientAPIM,
			requests:     1,
			wantGenerate: 1,
			wantAuth:     "Bearer vkey-1",
			wantOther:    true,
		},
		{
			name:         "binding without APIM key: reused, nothing created",
			seed:         seededBinding(""),
			requests:     2,
			wantGenerate: 0,
			wantAuth:     "Bearer seed-vkey",
		},
		{
			name:         "binding with APIM key: forwarded with that key, nothing created",
			seed:         seededBinding("seed-apim"),
			requests:     2,
			wantGenerate: 0,
			wantAuth:     "Bearer seed-vkey",
			wantAPIM:     "seed-apim",
		},
		{
			name:         "binding with APIM key: a client-supplied APIM header is replaced",
			seed:         seededBinding("seed-apim"),
			body:         clientAPIM,
			requests:     1,
			wantGenerate: 0,
			wantAuth:     "Bearer seed-vkey",
			wantAPIM:     "seed-apim",
			wantOther:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			var seeds []*dbclient.LLMGatewayUserBinding
			if tt.seed != nil {
				seeds = append(seeds, tt.seed)
			}
			store := newBindingStore(seeds...)
			fake := newFakeLiteLLM(t)
			srv := newGatewayServer(t, newAutoHandler(store.mockDB(ctrl), fake), autoTestEmail)

			for i := 0; i < tt.requests; i++ {
				require.Equal(t, http.StatusOK, chat(t, srv, tt.body), "request %d", i+1)
			}

			userNew, generate, update, del, upstream := fake.snapshot()
			assert.Equal(t, tt.wantGenerate, generate, "virtual keys generated")
			assert.Equal(t, tt.wantGenerate, userNew, "LiteLLM users ensured")
			assert.Zero(t, update)
			assert.Zero(t, del)
			require.Len(t, upstream, tt.requests)
			for _, call := range upstream {
				assert.Equal(t, tt.wantAuth, call.auth)
				value, present := call.apimHeader()
				assert.Equal(t, tt.wantAPIM != "", present, "APIM header present")
				assert.Equal(t, tt.wantAPIM, value)
				_, other := call.extraHeaders["X-Other"]
				assert.Equal(t, tt.wantOther, other)
			}

			rows, creates, updates := store.counts()
			assert.Equal(t, 1, rows)
			assert.Zero(t, updates)
			row, _ := store.row(autoTestEmail)
			if tt.seed != nil {
				assert.Equal(t, *tt.seed, row, "an existing binding is left untouched")
				assert.Zero(t, creates)
				return
			}
			assert.Equal(t, 1, creates, "binding created once")
			assert.Equal(t, "vkey-1", row.LiteLLMVirtualKey)
			assert.Equal(t, "hash-1", row.LiteLLMKeyHash)
			assert.Equal(t, autoTestEmail, row.KeyAlias)
			assert.Empty(t, row.ApimKey)
			assert.Empty(t, row.ApimKeyHash)
			assert.Equal(t, map[string]string{"safe_user_id": autoTestEmail}, fake.generated[0],
				"the LiteLLM key carries no APIM metadata")
		})
	}
}

// ── Concurrent first requests create one binding ──────────────────────────

func TestProxyLLMRequest_ConcurrentFirstRequestsCreateOneBinding(t *testing.T) {
	tests := []struct {
		name     string
		replicas int
		locker   bool
		// wantBusy: the run must exercise the advisory lock refusing a taker.
		wantBusy bool
	}{
		{name: "one replica, no advisory lock: requests share one creation", replicas: 1},
		{name: "one replica with advisory lock", replicas: 1, locker: true},
		{name: "two replicas contending for the advisory lock", replicas: 2, locker: true, wantBusy: true},
		{name: "three replicas contending for the advisory lock", replicas: 3, locker: true, wantBusy: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			store := newBindingStore()
			fake := newFakeLiteLLM(t)
			fake.gate = make(chan struct{})
			locks := &advisoryLocks{}

			servers := make([]*httptest.Server, tt.replicas)
			for i := range servers {
				var db dbclient.Interface = store.mockDB(ctrl)
				if tt.locker {
					db = &lockingDB{MockInterface: store.mockDB(ctrl), locks: locks}
				}
				servers[i] = newGatewayServer(t, newAutoHandler(db, fake), autoTestEmail)
			}

			const perReplica = 8
			var wg sync.WaitGroup
			statuses := make(chan int, perReplica*tt.replicas)
			for _, srv := range servers {
				for i := 0; i < perReplica; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						statuses <- chat(t, srv, "")
					}()
				}
			}

			// Hold the key generation open until every contender has had its
			// turn at the lock, so the creator is mid-flight when they arrive.
			if tt.wantBusy {
				require.Eventually(t, func() bool { return locks.busy.Load() >= int32(tt.replicas-1) },
					10*time.Second, 5*time.Millisecond, "no replica was refused the advisory lock")
			} else {
				time.Sleep(100 * time.Millisecond)
			}
			close(fake.gate)
			wg.Wait()
			close(statuses)

			for status := range statuses {
				assert.Equal(t, http.StatusOK, status)
			}
			userNew, generate, _, del, upstream := fake.snapshot()
			assert.Equal(t, 1, generate, "virtual keys generated")
			assert.Equal(t, 1, userNew)
			assert.Zero(t, del, "no key rolled back")
			rows, creates, _ := store.counts()
			assert.Equal(t, 1, rows)
			assert.Equal(t, 1, creates, "binding created once")
			require.Len(t, upstream, perReplica*tt.replicas)
			for _, call := range upstream {
				assert.Equal(t, "Bearer vkey-1", call.auth)
				_, present := call.apimHeader()
				assert.False(t, present)
			}
		})
	}
}

// ── Failures while creating the binding ───────────────────────────────────

func TestProxyLLMRequest_BindingCreationFailures(t *testing.T) {
	tests := []struct {
		name       string
		arrange    func(store *bindingStore, fake *fakeLiteLLM)
		wantStatus int
		wantRows   int
		wantDelete int
		wantUp     int
	}{
		{
			name: "LiteLLM cannot generate a key",
			arrange: func(_ *bindingStore, fake *fakeLiteLLM) {
				fake.failGenerate.Store(true)
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "the binding cannot be saved: the generated key is rolled back",
			arrange: func(store *bindingStore, _ *fakeLiteLLM) {
				store.createErr = errors.New("db unavailable")
			},
			wantStatus: http.StatusInternalServerError,
			wantDelete: 1,
		},
		{
			name: "the binding lookup fails",
			arrange: func(store *bindingStore, _ *fakeLiteLLM) {
				store.getErr = errors.New("db unavailable")
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "the alias is taken while a binding appears: that binding is used",
			arrange: func(store *bindingStore, fake *fakeLiteLLM) {
				fake.aliasTaken.Store(true)
				fake.onAliasTaken = func() { store.update(context.Background(), seededBinding("")) }
			},
			wantStatus: http.StatusOK,
			wantRows:   1,
			wantUp:     1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			store := newBindingStore()
			fake := newFakeLiteLLM(t)
			tt.arrange(store, fake)
			srv := newGatewayServer(t, newAutoHandler(store.mockDB(ctrl), fake), autoTestEmail)

			assert.Equal(t, tt.wantStatus, chat(t, srv, ""))

			_, _, _, del, upstream := fake.snapshot()
			assert.Equal(t, tt.wantDelete, del)
			assert.Len(t, upstream, tt.wantUp)
			store.mu.Lock()
			store.getErr = nil
			store.mu.Unlock()
			rows, _, _ := store.counts()
			assert.Equal(t, tt.wantRows, rows)
		})
	}
}

func TestProxyLLMRequest_CreationRetriedAfterFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := newBindingStore()
	fake := newFakeLiteLLM(t)
	srv := newGatewayServer(t, newAutoHandler(store.mockDB(ctrl), fake), autoTestEmail)

	fake.failGenerate.Store(true)
	assert.Equal(t, http.StatusInternalServerError, chat(t, srv, ""))
	rows, _, _ := store.counts()
	assert.Zero(t, rows, "a failed creation leaves no binding behind")

	fake.failGenerate.Store(false)
	assert.Equal(t, http.StatusOK, chat(t, srv, ""))
	assert.Equal(t, http.StatusOK, chat(t, srv, ""))
	_, generate, _, _, _ := fake.snapshot()
	assert.Equal(t, 2, generate, "one failed attempt, one that stuck")
	rows, creates, _ := store.counts()
	assert.Equal(t, 1, rows)
	assert.Equal(t, 1, creates)
}

// A caller that goes away does not abort the creation other callers wait for.
func TestEnsureBinding_CallerCancellationDoesNotAbortCreation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := newBindingStore()
	fake := newFakeLiteLLM(t)
	fake.gate = make(chan struct{})
	h := newAutoHandler(store.mockDB(ctrl), fake)

	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		_, err := h.ensureBinding(ctx, autoTestEmail)
		first <- err
	}()
	second := make(chan *dbclient.LLMGatewayUserBinding, 1)
	go func() {
		b, _ := h.ensureBinding(context.Background(), autoTestEmail)
		second <- b
	}()

	require.Eventually(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return fake.userNew >= 1
	}, 10*time.Second, 5*time.Millisecond)
	cancel()
	assert.ErrorIs(t, <-first, context.Canceled)

	close(fake.gate)
	binding := <-second
	require.NotNil(t, binding)
	assert.Equal(t, "hash-1", binding.LiteLLMKeyHash)
	rows, creates, _ := store.counts()
	assert.Equal(t, 1, rows)
	assert.Equal(t, 1, creates)
}

// ── Uploading an APIM key fills the binding the proxy created ─────────────

func TestAPIMKeyUpload_FillsTheProxyCreatedBinding(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "POST binding", method: http.MethodPost, path: "/binding"},
		{name: "PUT binding", method: http.MethodPut, path: "/binding"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			store := newBindingStore()
			fake := newFakeLiteLLM(t)
			srv := newGatewayServer(t, newAutoHandler(store.mockDB(ctrl), fake), autoTestEmail)

			// Before any key: the proxy gives the user a binding, which reports as unbound.
			require.Equal(t, http.StatusOK, chat(t, srv, ""))
			status, raw := send(t, srv, http.MethodGet, "/binding", "")
			require.Equal(t, http.StatusOK, status)
			var before BindingResponse
			require.NoError(t, json.Unmarshal(raw, &before))
			assert.False(t, before.HasAPIMKey)
			assert.Empty(t, before.ApimKeyHint)
			status, _ = send(t, srv, http.MethodGet, "/binding/apim-key", "")
			assert.Equal(t, http.StatusNotFound, status, "there is no APIM key to reveal")
			created, _ := store.row(autoTestEmail)

			// Upload one.
			status, raw = send(t, srv, tt.method, tt.path, `{"apim_key":"uploaded-apim-key"}`)
			require.Equal(t, map[string]int{http.MethodPost: http.StatusCreated, http.MethodPut: http.StatusOK}[tt.method], status, string(raw))
			var resp BindingResponse
			require.NoError(t, json.Unmarshal(raw, &resp))
			assert.True(t, resp.HasAPIMKey)
			if tt.method == http.MethodPost {
				assert.Equal(t, "vkey-1", resp.VirtualKey, "the user receives the key the proxy created")
			}

			// The same binding and the same LiteLLM key now carry the APIM key.
			rows, creates, updates := store.counts()
			assert.Equal(t, 1, rows)
			assert.Equal(t, 1, creates)
			assert.Equal(t, 1, updates)
			row, _ := store.row(autoTestEmail)
			assert.Equal(t, "uploaded-apim-key", row.ApimKey)
			assert.Equal(t, dbclient.ComputeApimKeyHash("uploaded-apim-key"), row.ApimKeyHash)
			assert.Equal(t, created.LiteLLMVirtualKey, row.LiteLLMVirtualKey)
			assert.Equal(t, created.LiteLLMKeyHash, row.LiteLLMKeyHash)
			assert.Equal(t, created.KeyAlias, row.KeyAlias)
			_, generate, update, _, _ := fake.snapshot()
			assert.Equal(t, 1, generate, "no second virtual key")
			assert.Equal(t, 1, update)
			assert.Equal(t, map[string]string{"apim_key": "uploaded-apim-key", "safe_user_id": autoTestEmail}, fake.updated[0])

			// Reported as bound, revealable, and used by the proxy.
			status, raw = send(t, srv, http.MethodGet, "/binding", "")
			require.Equal(t, http.StatusOK, status)
			var after BindingResponse
			require.NoError(t, json.Unmarshal(raw, &after))
			assert.True(t, after.HasAPIMKey)
			assert.Contains(t, after.ApimKeyHint, "****")
			status, _ = send(t, srv, http.MethodGet, "/binding/apim-key", "")
			assert.Equal(t, http.StatusOK, status)
			require.Equal(t, http.StatusOK, chat(t, srv, ""))
			_, _, _, _, upstream := fake.snapshot()
			require.Len(t, upstream, 2)
			_, present := upstream[0].apimHeader()
			assert.False(t, present)
			value, present := upstream[1].apimHeader()
			assert.True(t, present)
			assert.Equal(t, "uploaded-apim-key", value)
			assert.Equal(t, "Bearer vkey-1", upstream[1].auth)
		})
	}
}

func TestCreateBinding_ExistingAPIMKeyStillConflicts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := newBindingStore(seededBinding("seed-apim"))
	fake := newFakeLiteLLM(t)
	srv := newGatewayServer(t, newAutoHandler(store.mockDB(ctrl), fake), autoTestEmail)

	status, _ := send(t, srv, http.MethodPost, "/binding", `{"apim_key":"another"}`)
	assert.Equal(t, http.StatusConflict, status)

	_, generate, update, _, _ := fake.snapshot()
	assert.Zero(t, generate)
	assert.Zero(t, update)
	row, _ := store.row(autoTestEmail)
	assert.Equal(t, *seededBinding("seed-apim"), row)
}

func TestFillAPIMKey_FailureLeavesBindingKeyless(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := newBindingStore(seededBinding(""))
	fake := newFakeLiteLLM(t)
	h := newAutoHandler(store.mockDB(ctrl), fake)

	// LiteLLM metadata update works but the row cannot be saved.
	db := mock_client.NewMockInterface(ctrl)
	db.EXPECT().GetLLMBindingByEmail(gomock.Any(), gomock.Any()).DoAndReturn(store.get).AnyTimes()
	db.EXPECT().UpdateLLMBinding(gomock.Any(), gomock.Any()).Return(errors.New("db unavailable"))
	h.dbClient = db
	srv := newGatewayServer(t, h, autoTestEmail)

	status, _ := send(t, srv, http.MethodPost, "/binding", `{"apim_key":"uploaded-apim-key"}`)
	assert.Equal(t, http.StatusInternalServerError, status)

	_, _, update, _, _ := fake.snapshot()
	require.Equal(t, 2, update, "the metadata is written, then put back")
	assert.Equal(t, map[string]string{"apim_key": "uploaded-apim-key", "safe_user_id": autoTestEmail}, fake.updated[0])
	assert.Equal(t, map[string]string{"safe_user_id": autoTestEmail}, fake.updated[1], "back to a key without APIM metadata")
	row, _ := store.row(autoTestEmail)
	assert.Empty(t, row.ApimKey, "the stored binding is still key-less")
}

// ── CreateKey metadata ────────────────────────────────────────────────────

func TestCreateKey_EmptyAPIMKeyOmitsMetadata(t *testing.T) {
	tests := []struct {
		name     string
		apimKey  string
		wantMeta map[string]string
	}{
		{name: "empty", apimKey: "", wantMeta: map[string]string{"safe_user_id": autoTestEmail}},
		{name: "set", apimKey: "k", wantMeta: map[string]string{"safe_user_id": autoTestEmail, "apim_key": "k"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received CreateKeyRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&received)
				_ = json.NewEncoder(w).Encode(CreateKeyResponse{Key: "k", TokenID: "h"})
			}))
			defer server.Close()

			_, err := NewLiteLLMClient(server.URL, "admin", "team").CreateKey(context.Background(), autoTestEmail, tt.apimKey)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMeta, received.Metadata)
		})
	}
}

// A caller whose lookup missed before another request committed the binding
// must find it again once it holds the lock, not create a second one.
func TestEnsureBinding_BindingAppearedAfterStaleRead(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := newBindingStore(seededBinding(""))
	fake := newFakeLiteLLM(t)
	h := newAutoHandler(&lockingDB{MockInterface: store.mockDB(ctrl), locks: &advisoryLocks{}}, fake)

	binding, err := h.ensureBinding(context.Background(), autoTestEmail)

	require.NoError(t, err)
	assert.Equal(t, "seed-hash", binding.LiteLLMKeyHash)
	userNew, generate, _, _, _ := fake.snapshot()
	assert.Zero(t, userNew)
	assert.Zero(t, generate)
	_, creates, _ := store.counts()
	assert.Zero(t, creates)
}

// Replacing the APIM key of a binding that has one still undoes the LiteLLM
// metadata change when the row cannot be saved.
func TestUpdateBinding_FailureRestoresPreviousAPIMKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := newBindingStore(seededBinding("old-apim-key"))
	fake := newFakeLiteLLM(t)
	h := newAutoHandler(store.mockDB(ctrl), fake)
	db := mock_client.NewMockInterface(ctrl)
	db.EXPECT().GetLLMBindingByEmail(gomock.Any(), gomock.Any()).DoAndReturn(store.get).AnyTimes()
	db.EXPECT().UpdateLLMBinding(gomock.Any(), gomock.Any()).Return(errors.New("db unavailable"))
	h.dbClient = db
	srv := newGatewayServer(t, h, autoTestEmail)

	status, _ := send(t, srv, http.MethodPut, "/binding", `{"apim_key":"new-apim-key"}`)

	assert.Equal(t, http.StatusInternalServerError, status)
	_, _, update, _, _ := fake.snapshot()
	require.Equal(t, 2, update)
	assert.Equal(t, "new-apim-key", fake.updated[0]["apim_key"])
	assert.Equal(t, "old-apim-key", fake.updated[1]["apim_key"])
	row, _ := store.row(autoTestEmail)
	assert.Equal(t, "old-apim-key", row.ApimKey)
}
