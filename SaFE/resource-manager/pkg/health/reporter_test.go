/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package health

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const (
	keyEnable       = "metrics.remote_write.enable"
	keyURL          = "metrics.remote_write.url"
	keyClusterName  = "metrics.remote_write.cluster_name"
	keySkipVerify   = "metrics.remote_write.insecure_skip_verify"
	keyIntervalSecs = "metrics.remote_write.interval_seconds"
)

func TestNewReporterInsecureSkipVerify(t *testing.T) {
	viper.Reset()
	viper.Set(keySkipVerify, true)
	r := NewReporter(nil)
	tr, ok := r.httpClient.Transport.(*http.Transport)
	if !ok || tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify transport, got %#v", r.httpClient.Transport)
	}

	viper.Reset()
	r2 := NewReporter(nil)
	if r2.httpClient.Transport != nil {
		t.Fatalf("expected default transport (nil) when skip-verify disabled, got %#v", r2.httpClient.Transport)
	}
}

func TestStartDisabledReturnsNil(t *testing.T) {
	viper.Reset() // remote_write disabled
	r := NewReporter(nil)
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start with remote_write disabled should return nil, got %v", err)
	}

	// enabled but empty url also short-circuits without error
	viper.Reset()
	viper.Set(keyEnable, true)
	r2 := NewReporter(nil)
	if err := r2.Start(context.Background()); err != nil {
		t.Fatalf("Start with empty url should return nil, got %v", err)
	}
}

func TestCollectAndPush(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	viper.Reset()
	viper.Set(keyEnable, true)
	viper.Set(keyURL, srv.URL)
	viper.Set(keyClusterName, "crusoe")
	viper.Set(keyIntervalSecs, 30)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "apiserver", Namespace: common.PrimusSafeNamespace},
		Spec:       appsv1.DeploymentSpec{Replicas: ptrInt32(2)},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 2},
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "node-agent", Namespace: common.PrimusSafeNamespace},
		Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 1},
	}

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep, ds).Build()

	r := NewReporter(cli)
	r.httpClient = srv.Client()
	r.collectAndPush(context.Background())

	if !strings.Contains(gotBody, `safe_component_up{component="apiserver",kind="Deployment"} 1`) {
		t.Errorf("expected healthy apiserver component in push body:\n%s", gotBody)
	}
	if !strings.Contains(gotBody, `safe_component_up{component="node-agent",kind="DaemonSet"} 0`) {
		t.Errorf("expected unhealthy node-agent daemonset in push body:\n%s", gotBody)
	}
}

func ptrInt32(v int32) *int32 { return &v }
