/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package health

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestNewReporter(t *testing.T) {
	r := NewReporter(nil)
	if r == nil {
		t.Fatal("NewReporter returned nil")
	}
	if !r.NeedLeaderElection() {
		t.Fatal("reporter must run leader-only")
	}
}

func TestCollectRefreshesGauges(t *testing.T) {
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
	r.collect(context.Background())

	if got := gaugeValue(t, "safe_component_up", map[string]string{"component": "apiserver", "kind": "Deployment"}); got != 1 {
		t.Errorf("healthy apiserver: want safe_component_up=1, got %v", got)
	}
	if got := gaugeValue(t, "safe_component_up", map[string]string{"component": "node-agent", "kind": "DaemonSet"}); got != 0 {
		t.Errorf("unhealthy node-agent: want safe_component_up=0, got %v", got)
	}
}

func ptrInt32(v int32) *int32 { return &v }

// gaugeValue reads a single gauge sample from the controller-runtime registry,
// where the self-health metrics are registered.
func gaugeValue(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := ctrlmetrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			match := true
			seen := map[string]string{}
			for _, lp := range m.GetLabel() {
				seen[lp.GetName()] = lp.GetValue()
			}
			for k, v := range labels {
				if seen[k] != v {
					match = false
					break
				}
			}
			if match && m.Gauge != nil {
				return m.Gauge.GetValue()
			}
		}
	}
	t.Fatalf("metric %s with labels %v not found", name, labels)
	return 0
}
