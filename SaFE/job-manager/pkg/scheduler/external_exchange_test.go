/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/execution"
)

const activeClaimBody = `{"api_version":"safe-exec/v1alpha1","request_id":"r","claim_id":"c","revision":3,"phase":"Active","workload_uid":"11111111-1111-1111-1111-111111111111","dispatch_generation":1,"cluster_id":"crusoe","workspace_id":"ws-external","expires_at":"2099-01-01T00:00:00.000Z","placements":[{"unit_key":"master/0","allocation_id":"a","allocation_generation":1,"expected_allocation_revision":1,"node_name":"vk-1","image_ref":"` + pinnedImage + `","image_digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","resources":{"cpu_millis":8000,"memory_bytes":1,"scratch_bytes":0,"gpu_resource":"amd.com/gpu","gpu_model":"MI355X","gpu_count":1},"pid_limit":256,"device_ids":[],"ports":[]}]}`

func useExecutionServer(t *testing.T, handler http.Handler) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	certPEM, keyPEM := executionClientPEM(t)
	for name, body := range map[string][]byte{"ca.crt": ca, "tls.crt": certPEM, "tls.key": keyPEM} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	viper.Set("external_execution.enabled", true)
	commonconfig.SetValue("external_execution.controller_url", srv.URL)
	commonconfig.SetValue("external_execution.controller_secret_path", dir)
	t.Cleanup(func() {
		viper.Set("external_execution.enabled", false)
		viper.Set("external_execution.controller_url", "")
		viper.Set("external_execution.controller_secret_path", "")
	})
}

func executionClientPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: t.Name()},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
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

func exchangeFixture(t *testing.T, w *v1.Workload) (*SchedulerReconciler, *v1.Workspace) {
	t.Helper()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: w.Spec.Workspace},
		Spec: v1.WorkspaceSpec{Cluster: "crusoe", NodeFlavor: "nf-1"}}
	nf := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "nf-1"},
		Spec: v1.NodeFlavorSpec{Gpu: &v1.GpuChip{Product: "MI355X"}}}
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).
		WithObjects(w, ws, nf).WithStatusSubresource(w).Build()
	return &SchedulerReconciler{Client: cl}, ws
}

func okJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func TestRequestExternalCapacityPublishesDemand(t *testing.T) {
	useExecutionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		okJSON(`{"api_version":"safe-exec/v1alpha1","request_id":"r","demand_id":"d","revision":1,"eligible":true,"reason":"InsufficientCapacity","units":[],"observed_at":"2026-09-29T12:00:00.000Z","expires_at":"2026-09-29T12:05:00.000Z"}`)(w, r)
	}))
	w := gpuWorkload()
	w.Status.Phase = v1.WorkloadPending
	r, ws := exchangeFixture(t, w)
	ok, reason, err := r.requestExternalCapacity(context.Background(), w, ws)
	if ok || err != nil || reason != ExternalCapacityReason {
		t.Fatalf("publish: ok=%v reason=%q err=%v", ok, reason, err)
	}
	stored := &v1.Workload{}
	if err = r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.ExternalExecution == nil || stored.Status.ExternalExecution.DemandExpiresAt == nil {
		t.Fatalf("demand was not accepted: %+v", stored.Status.ExternalExecution)
	}
}

func TestPublishConflictAbandonsTheRevision(t *testing.T) {
	useExecutionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"Conflict","message":"stale","request_id":"r"}`))
	}))
	w := gpuWorkload()
	w.Status.Phase = v1.WorkloadPending
	r, ws := exchangeFixture(t, w)
	if err := r.ensureExternalDemand(context.Background(), w, ws); err == nil || !execution.IsCode(err, execution.CodeConflict) {
		t.Fatalf("conflict: %v", err)
	}
	stored := &v1.Workload{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.ExternalExecution.DemandRequestId != "" {
		t.Fatal("refused revision was not abandoned")
	}
}

func TestReserveExternalCapacityClaimsASeat(t *testing.T) {
	useExecutionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"NotFound","message":"missing","request_id":"r"}`))
		case r.URL.Path == "/v1alpha1/placement-plans":
			okJSON(`{"api_version":"safe-exec/v1alpha1","request_id":"r","demand_id":"d","demand_revision":1,"placements":[{"unit_key":"master/0","allocation_id":"a","allocation_generation":1,"expected_allocation_revision":1,"node_name":"vk-1","image_ref":"`+pinnedImage+`","image_digest":"x","resources":{"cpu_millis":1,"memory_bytes":1,"scratch_bytes":0,"gpu_resource":"amd.com/gpu","gpu_model":"MI355X","gpu_count":1},"pid_limit":1,"device_ids":[],"ports":[]}]}`)(w, r)
		case r.URL.Path == "/v1alpha1/claims":
			okJSON(activeClaimBody)(w, r)
		default:
			okJSON(`{"api_version":"safe-exec/v1alpha1","request_id":"r","demand_id":"d","revision":1,"eligible":true,"reason":"InsufficientCapacity","units":[],"observed_at":"2026-09-29T12:00:00.000Z","expires_at":"2026-09-29T12:05:00.000Z"}`)(w, r)
		}
	}))
	w := gpuWorkload()
	w.Status.Phase = v1.WorkloadPending
	r, ws := exchangeFixture(t, w)
	ok, reason, err := r.reserveExternalCapacity(context.Background(), w, ws)
	if !ok || err != nil || reason != "" {
		t.Fatalf("reserve: ok=%v reason=%q err=%v", ok, reason, err)
	}
}

func TestWithdrawExternalDemand(t *testing.T) {
	useExecutionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		okJSON(`{"api_version":"safe-exec/v1alpha1","request_id":"r","demand_id":"d1","revision":2,"eligible":false,"reason":"Withdrawn","units":[],"observed_at":"2026-09-29T12:00:00.000Z","expires_at":"2026-09-29T12:05:00.000Z"}`)(w, r)
	}))
	w, state := recoveryWorkload()
	state.DemandRevision = 1
	observed := metav1.NewTime(time.Now())
	state.DemandObservedAt = &observed
	state.DemandRequestId = "req"
	w.Status.ExternalExecution = state
	r, ws := exchangeFixture(t, w)
	stored := &v1.Workload{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	if err := r.withdrawExternalDemand(context.Background(), stored, ws); err != nil {
		t.Fatal(err)
	}
	after := &v1.Workload{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: w.Name}, after); err != nil {
		t.Fatal(err)
	}
	if !after.Status.ExternalExecution.DemandWithdrawn {
		t.Fatal("demand was not withdrawn")
	}
}

func TestReconcileExternalReleaseOnConflictReadsReleased(t *testing.T) {
	useExecutionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			body := activeClaimBody
			body = replacePhase(body, "Released")
			okJSON(body)(w, r)
			return
		}
		if r.Method == http.MethodPut {
			okJSON(`{"api_version":"safe-exec/v1alpha1","request_id":"r","demand_id":"d1","revision":2,"eligible":false,"reason":"Withdrawn","units":[],"observed_at":"2026-09-29T12:00:00.000Z","expires_at":"2026-09-29T12:05:00.000Z"}`)(w, r)
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"Conflict","message":"stale","request_id":"r"}`))
	}))
	w, state := recoveryWorkload()
	w.Status.Phase = v1.WorkloadSucceeded
	state.ClaimPhase = execution.ClaimPhaseActive
	state.ClaimRevision = 1
	state.DemandRevision = 1
	observed := metav1.NewTime(time.Now())
	state.DemandObservedAt = &observed
	state.DemandRequestId = "req"
	w.Status.ExternalExecution = state
	r, _ := exchangeFixture(t, w)
	stored := &v1.Workload{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	holding, err := r.reconcileExternalRelease(context.Background(), stored)
	if holding || err != nil {
		t.Fatalf("release: holding=%v err=%v", holding, err)
	}
}

func replacePhase(body, phase string) string {
	return replaceOnce(body, `"phase":"Active"`, `"phase":"`+phase+`"`)
}

func replaceOnce(s, old, new string) string {
	i := indexOf(s, old)
	if i < 0 {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestHandleReservationRefusalRestatesCapacityAndWaitsOnImage(t *testing.T) {
	mode := "capacity"
	useExecutionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1alpha1/placement-plans" && mode == "capacity" {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"CapacityUnavailable","message":"none","request_id":"r"}`))
			return
		}
		if r.URL.Path == "/v1alpha1/placement-plans" {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"ImagePreparing","message":"pulling","request_id":"r"}`))
			return
		}
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"NotFound","message":"missing","request_id":"r"}`))
			return
		}
		okJSON(`{"api_version":"safe-exec/v1alpha1","request_id":"r","demand_id":"d","revision":1,"eligible":true,"reason":"InsufficientCapacity","units":[],"observed_at":"2026-09-29T12:00:00.000Z","expires_at":"2026-09-29T12:05:00.000Z"}`)(w, r)
	}))
	w := gpuWorkload()
	w.Status.Phase = v1.WorkloadPending
	r, ws := exchangeFixture(t, w)
	ok, reason, err := r.reserveExternalCapacity(context.Background(), w, ws)
	if ok || err != nil || reason != ExternalCapacityReason {
		t.Fatalf("capacity refusal: ok=%v reason=%q err=%v", ok, reason, err)
	}
	mode = "image"
	stored := &v1.Workload{}
	if err = r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	ok, reason, err = r.reserveExternalCapacity(context.Background(), stored, ws)
	if ok || err != nil || !strings.HasPrefix(reason, ExternalImageReason) {
		t.Fatalf("image refusal: ok=%v reason=%q err=%v", ok, reason, err)
	}
}

func TestReleaseSupersededClaimOnNewGeneration(t *testing.T) {
	useExecutionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		okJSON(activeClaimBody)(w, r)
	}))
	w, state := recoveryWorkload()
	v1.SetLabel(w, v1.WorkloadDispatchCntLabel, "1")
	w.Status.ExternalExecution = state
	r, _ := exchangeFixture(t, w)
	stored := &v1.Workload{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	next, err := r.ensureExternalState(context.Background(), stored)
	if err != nil {
		t.Fatal(err)
	}
	if next.DispatchGeneration != 2 || next.ClaimId == "c-old" {
		t.Fatalf("generation was not replaced: %+v", next)
	}
}

func TestReconcileStaleReleaseRetriesANewerActiveClaim(t *testing.T) {
	var releases int
	useExecutionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			okJSON(activeClaimBody)(w, r)
			return
		}
		if r.Method == http.MethodPut {
			okJSON(`{"api_version":"safe-exec/v1alpha1","request_id":"r","demand_id":"d1","revision":2,"eligible":false,"reason":"Withdrawn","units":[],"observed_at":"2026-09-29T12:00:00.000Z","expires_at":"2026-09-29T12:05:00.000Z"}`)(w, r)
			return
		}
		releases++
		if releases == 1 {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"Conflict","message":"stale","request_id":"r"}`))
			return
		}
		body := replacePhase(activeClaimBody, "Revoking")
		okJSON(body)(w, r)
	}))
	w, state := recoveryWorkload()
	w.Status.Phase = v1.WorkloadSucceeded
	state.ClaimPhase = execution.ClaimPhaseActive
	state.ClaimRevision = 1
	state.DemandRevision = 1
	observed := metav1.NewTime(time.Now())
	state.DemandObservedAt = &observed
	state.DemandRequestId = "req"
	w.Status.ExternalExecution = state
	r, _ := exchangeFixture(t, w)
	stored := &v1.Workload{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	holding, err := r.reconcileExternalRelease(context.Background(), stored)
	if !holding || err != nil {
		t.Fatalf("revoking hold: holding=%v err=%v", holding, err)
	}
}

func TestRequestExternalCapacityRejectsAnUnexpressibleShape(t *testing.T) {
	w := gpuWorkload()
	w.Spec.Images = nil
	w.Status.Phase = v1.WorkloadPending
	r, ws := exchangeFixture(t, w)
	ok, reason, err := r.requestExternalCapacity(context.Background(), w, ws)
	if ok || err != nil || reason != ExternalUnsupportedReason {
		t.Fatalf("shape: ok=%v reason=%q err=%v", ok, reason, err)
	}
}

func TestExternalOutcomeRequeuesAWaitAndSkipsATerminalReason(t *testing.T) {
	r := &SchedulerReconciler{}
	r.KeyedController = controller.NewKeyedController[*SchedulerMessage](r, schedulerMessageKey, nil, 1)
	t.Cleanup(r.ShutDown)
	w := gpuWorkload()
	ok, reason, err := r.externalOutcome(w, false, ExternalCapacityReason, nil)
	if ok || err != nil || reason != ExternalCapacityReason {
		t.Fatalf("wait: ok=%v reason=%q err=%v", ok, reason, err)
	}
	ok, reason, err = r.externalOutcome(w, true, "", nil)
	if !ok || err != nil {
		t.Fatalf("admitted: ok=%v err=%v", ok, err)
	}
	ok, _, err = r.externalOutcome(w, false, ExternalUnsupportedReason, nil)
	if ok || err != nil {
		t.Fatalf("terminal reason: ok=%v err=%v", ok, err)
	}
	ok, reason, err = r.externalOutcome(w, false, "", errNoConnection{})
	if ok || reason != ExternalUnavailableReason || err != nil {
		t.Fatalf("error wait: ok=%v reason=%q err=%v", ok, reason, err)
	}
}

func TestEncodeConstraintsEscapesControls(t *testing.T) {
	got := string(encodeConstraints(execution.PlacementConstraints{
		AllowedNodeNames: []string{"a\"b", "c\\d"},
		NodeSelector:     map[string]string{"k\n": "v\t", "x": "y\u0001\b\f\r"},
	}))
	for _, want := range []string{`\"`, `\\`, `\n`, `\t`, `\u0001`, `\b`, `\f`, `\r`} {
		if indexOf(got, want) < 0 {
			t.Fatalf("encoded %s missing %s", got, want)
		}
	}
}
