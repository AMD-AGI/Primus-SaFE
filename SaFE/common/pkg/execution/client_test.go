/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package execution

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func selfSignedPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "execution-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"127.0.0.1", "localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

func testClient(t *testing.T, srv *httptest.Server, allowMock bool) *Client {
	t.Helper()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	certPEM, keyPEM := selfSignedPEM(t)
	client, err := NewClient(Config{
		BaseURL:         srv.URL,
		CACert:          ca,
		ClientCert:      certPEM,
		ClientKey:       keyPEM,
		AllowMockServer: allowMock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestNewClientRejectsBadConfig(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("empty url must fail")
	}
	if _, err := NewClient(Config{BaseURL: "://bad"}); err == nil {
		t.Fatal("unparseable url must fail")
	}
	if _, err := NewClient(Config{BaseURL: "http://capacity.internal"}); err == nil {
		t.Fatal("http must fail unless the mock server is allowed")
	}
	if _, err := NewClient(Config{BaseURL: "https://capacity.internal", CACert: []byte("nope")}); err == nil {
		t.Fatal("invalid ca pem must fail")
	}
	certPEM, _ := selfSignedPEM(t)
	if _, err := NewClient(Config{
		BaseURL: "https://capacity.internal", ClientCert: certPEM, ClientKey: []byte("nope"),
	}); err == nil {
		t.Fatal("invalid client key must fail")
	}
	if _, err := NewClient(Config{BaseURL: "http://capacity.internal", AllowMockServer: true}); err != nil {
		t.Fatal(err)
	}
}

func TestClientRoundTrip(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1alpha1/capacity-demands/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("demand method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"api_version":"safe-exec/v1alpha1","request_id":"r","demand_id":"d","revision":1,"eligible":true,"reason":"InsufficientCapacity","units":[],"observed_at":"2026-09-29T12:00:00.000Z","expires_at":"2026-09-29T12:05:00.000Z"}`))
	})
	mux.HandleFunc("/v1alpha1/placement-plans", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"api_version":"safe-exec/v1alpha1","request_id":"r","demand_id":"d","demand_revision":1,"placements":[]}`))
	})
	mux.HandleFunc("/v1alpha1/claims/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"api_version":"safe-exec/v1alpha1","request_id":"r","claim_id":"c","revision":2,"phase":"Released","workload_uid":"u","dispatch_generation":1,"cluster_id":"c","workspace_id":"w","expires_at":"2099-01-01T00:00:00.000Z","placements":[]}`))
	})
	mux.HandleFunc("/v1alpha1/claims", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"api_version":"safe-exec/v1alpha1","request_id":"r","claim_id":"c","revision":1,"phase":"Active","workload_uid":"u","dispatch_generation":1,"cluster_id":"c","workspace_id":"w","expires_at":"2099-01-01T00:00:00.000Z","placements":[]}`))
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := testClient(t, srv, false)

	demand, err := client.PublishDemand(context.Background(), &CapacityDemand{DemandID: "d/1", RequestID: "r"})
	if err != nil || demand.Revision != 1 {
		t.Fatalf("publish: %v %+v", err, demand)
	}
	plan, err := client.PlanPlacements(context.Background(), &PlacementPlanRequest{RequestID: "r"})
	if err != nil || plan.DemandID != "d" {
		t.Fatalf("plan: %v %+v", err, plan)
	}
	claim, err := client.CreateClaim(context.Background(), &ClaimRequest{RequestID: "r", ClaimID: "c"})
	if err != nil || !claim.IsActive() {
		t.Fatalf("create: %v %+v", err, claim)
	}
	got, err := client.GetClaim(context.Background(), "c/1")
	if err != nil || got.Phase != ClaimPhaseReleased {
		t.Fatalf("get: %v %+v", err, got)
	}
	released, err := client.ReleaseClaim(context.Background(), "c/1", &ReleaseRequest{RequestID: "r"})
	if err != nil || released.Phase != ClaimPhaseReleased {
		t.Fatalf("release: %v %+v", err, released)
	}
}

func TestClientRejectsMissingArguments(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	client := testClient(t, srv, false)
	if _, err := client.PublishDemand(context.Background(), nil); err == nil {
		t.Fatal("nil demand")
	}
	if _, err := client.PlanPlacements(context.Background(), nil); err == nil {
		t.Fatal("nil plan")
	}
	if _, err := client.CreateClaim(context.Background(), nil); err == nil {
		t.Fatal("nil claim")
	}
	if _, err := client.GetClaim(context.Background(), " "); err == nil {
		t.Fatal("blank claim id")
	}
	if _, err := client.ReleaseClaim(context.Background(), "", &ReleaseRequest{}); err == nil {
		t.Fatal("blank release id")
	}
	if _, err := client.ReleaseClaim(context.Background(), "c", nil); err == nil {
		t.Fatal("nil release")
	}
}

func TestClientDecodesRefusalsAndRefusesTheMock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/conflict", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"Conflict","message":"stale","request_id":"req-1","retryable":true,"retry_after_s":3}`))
	})
	mux.HandleFunc("/plain", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream"))
	})
	mux.HandleFunc("/empty", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/mock", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(mockHeader, "true")
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/unknown", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"not_a_field":true}`))
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := testClient(t, srv, false)

	err := client.do(context.Background(), http.MethodGet, "/conflict", nil, &ClaimResponse{})
	if !IsCode(err, CodeConflict) || !IsRetryable(err) || RetryAfterOf(err) != 3*time.Second {
		t.Fatalf("conflict: %v", err)
	}
	err = client.do(context.Background(), http.MethodGet, "/plain", nil, nil)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != CodeUnavailable || apiErr.Message != "upstream" {
		t.Fatalf("plain body: %v", err)
	}
	err = client.do(context.Background(), http.MethodGet, "/empty", nil, nil)
	apiErr, ok = err.(*APIError)
	if !ok || apiErr.Message != http.StatusText(http.StatusServiceUnavailable) {
		t.Fatalf("empty body: %v", err)
	}
	if err = client.do(context.Background(), http.MethodGet, "/mock", nil, &ClaimResponse{}); err == nil || !strings.Contains(err.Error(), "contract mock") {
		t.Fatalf("mock: %v", err)
	}
	if err = client.do(context.Background(), http.MethodGet, "/unknown", nil, &ClaimResponse{}); err == nil {
		t.Fatal("unknown field must fail")
	}
	if err = client.do(context.Background(), http.MethodGet, "/redirect", nil, nil); err == nil {
		t.Fatal("redirect must not be followed as success")
	}

	allowed := testClient(t, srv, true)
	if err = allowed.do(context.Background(), http.MethodGet, "/mock", nil, &ClaimResponse{}); err != nil {
		t.Fatalf("allowed mock: %v", err)
	}
}

func TestClientTransportFailures(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := testClient(t, srv, false)
	srv.Close()
	if _, err := client.GetClaim(context.Background(), "c"); err == nil {
		t.Fatal("closed server must fail")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	live := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(live.Close)
	if _, err := testClient(t, live, false).GetClaim(ctx, "c"); err == nil {
		t.Fatal("canceled context must fail")
	}

	if err := client.do(context.Background(), http.MethodPost, "/x", make(chan int), nil); err == nil {
		t.Fatal("unmarshalable body must fail")
	}
}
