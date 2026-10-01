/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

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
	"strconv"
	"testing"
	"time"

	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

func useClaimServer(t *testing.T, body string, status int) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
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
	files := map[string][]byte{
		"ca.crt":  ca,
		"tls.crt": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		"tls.key": pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	}
	for name, contents := range files {
		if err = os.WriteFile(filepath.Join(dir, name), contents, 0o600); err != nil {
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

func claimWorkload() *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "train-1", UID: "11111111-1111-1111-1111-111111111111"},
		Status: v1.WorkloadStatus{ExternalExecution: &v1.WorkloadExternalExecution{
			ClaimId: "c1", DispatchGeneration: 1,
			Placements: []v1.WorkloadExternalPlacement{{
				UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-1", ImageRef: pinnedDispatchImage,
			}},
		}},
	}
}

const pinnedDispatchImage = "docker.io/team/app@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func claimBody(phase, uid string, gen int, expires string) string {
	return `{"api_version":"safe-exec/v1alpha1","request_id":"r","claim_id":"c1","revision":1,"phase":"` + phase +
		`","workload_uid":"` + uid + `","dispatch_generation":` + itoa(gen) +
		`,"cluster_id":"c","workspace_id":"w","expires_at":"` + expires + `","placements":[]}`
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestVerifyExternalClaim(t *testing.T) {
	r := &DispatcherReconciler{}
	if err := r.verifyExternalClaim(context.Background(), &v1.Workload{}); err == nil {
		t.Fatal("missing claim must fail before any call")
	}
	if err := r.verifyExternalClaim(context.Background(), claimWorkload()); err == nil {
		t.Fatal("unconfigured client must fail")
	}

	w := claimWorkload()
	useClaimServer(t, `{"code":"NotFound","message":"gone","request_id":"r"}`, http.StatusNotFound)
	err := r.verifyExternalClaim(context.Background(), w)
	if !isClaimGone(err) {
		t.Fatalf("not found: %v", err)
	}

	useClaimServer(t, claimBody("Active", "other", 1, "2099-01-01T00:00:00.000Z"), http.StatusOK)
	if err = r.verifyExternalClaim(context.Background(), w); !isClaimGone(err) {
		t.Fatalf("foreign claim: %v", err)
	}
	useClaimServer(t, claimBody("Active", string(w.UID), 9, "2099-01-01T00:00:00.000Z"), http.StatusOK)
	if err = r.verifyExternalClaim(context.Background(), w); !isClaimGone(err) {
		t.Fatalf("wrong generation: %v", err)
	}
	useClaimServer(t, claimBody("Revoking", string(w.UID), 1, "2099-01-01T00:00:00.000Z"), http.StatusOK)
	if err = r.verifyExternalClaim(context.Background(), w); !isClaimGone(err) {
		t.Fatalf("revoking: %v", err)
	}
	useClaimServer(t, claimBody("Active", string(w.UID), 1, "2000-01-01T00:00:00.000Z"), http.StatusOK)
	if err = r.verifyExternalClaim(context.Background(), w); !isClaimGone(err) {
		t.Fatalf("expired: %v", err)
	}

	bare := claimWorkload()
	bare.Status.ExternalExecution.Placements = nil
	useClaimServer(t, claimBody("Active", string(bare.UID), 1, "2099-01-01T00:00:00.000Z"), http.StatusOK)
	if err = r.verifyExternalClaim(context.Background(), bare); !isClaimGone(err) {
		t.Fatalf("no nodes: %v", err)
	}
	tagged := claimWorkload()
	tagged.Status.ExternalExecution.Placements[0].ImageRef = "docker.io/team/app:v1"
	useClaimServer(t, claimBody("Active", string(tagged.UID), 1, "2099-01-01T00:00:00.000Z"), http.StatusOK)
	if err = r.verifyExternalClaim(context.Background(), tagged); !isClaimGone(err) {
		t.Fatalf("tag image: %v", err)
	}
	useClaimServer(t, claimBody("Active", string(w.UID), 1, "2099-01-01T00:00:00.000Z"), http.StatusOK)
	if err = r.verifyExternalClaim(context.Background(), w); err != nil {
		t.Fatalf("usable claim: %v", err)
	}
}

func TestReturnToQueueDropsTheScheduledMark(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	w := claimWorkload()
	v1.SetAnnotation(w, v1.WorkloadScheduledAnnotation, v1.TrueStr)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(w).Build()
	r := &DispatcherReconciler{Client: cl}
	if err := r.returnToQueue(context.Background(), w, &claimGoneError{"gone"}); err != nil {
		t.Fatal(err)
	}
	if v1.IsWorkloadScheduled(w) {
		t.Fatal("scheduled mark remains")
	}
}

func TestExternalPodAnnotationsAndPlacementLookup(t *testing.T) {
	if externalPodAnnotations(&v1.Workload{}, v1.ExternalSingleUnitKey) != nil {
		t.Fatal("annotations without a claim")
	}
	w := claimWorkload()
	ann := externalPodAnnotations(w, v1.ExternalSingleUnitKey)
	if ann[v1.ExternalClaimIdAnnotation] != "c1" {
		t.Fatalf("annotations: %v", ann)
	}
	if findPlacement(w.Status.ExternalExecution, "missing") != nil {
		t.Fatal("unknown unit")
	}
	if externalApprovedImage(&v1.Workload{}, "x") != "" || len(externalApprovedNodes(&v1.Workload{})) != 0 {
		t.Fatal("empty workload")
	}
	if !isExternalWorkload(w) || isExternalWorkload(&v1.Workload{}) {
		t.Fatal("external predicate")
	}
	if !isClaimGone(&claimGoneError{"x"}) || isClaimGone(context.Canceled) {
		t.Fatal("claim gone predicate")
	}
}
