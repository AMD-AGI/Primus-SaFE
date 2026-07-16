/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package health

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestSetBool(t *testing.T) {
	SetBool(SubsystemUp.WithLabelValues(SubsystemDatabase), true)
	if got := gaugeValue(t, "safe_subsystem_up", map[string]string{"subsystem": SubsystemDatabase}); got != 1 {
		t.Fatalf("SetBool true: want 1, got %v", got)
	}
	SetBool(SubsystemUp.WithLabelValues(SubsystemDatabase), false)
	if got := gaugeValue(t, "safe_subsystem_up", map[string]string{"subsystem": SubsystemDatabase}); got != 0 {
		t.Fatalf("SetBool false: want 0, got %v", got)
	}
}

func TestMetricsRegisteredOnControllerRuntimeRegistry(t *testing.T) {
	ComponentUp.WithLabelValues("apiserver", "Deployment").Set(1)
	if got := gaugeValue(t, "safe_component_up", map[string]string{"component": "apiserver", "kind": "Deployment"}); got != 1 {
		t.Fatalf("safe_component_up should be exposed on the controller-runtime registry, got %v", got)
	}
}

func TestResetScanned(t *testing.T) {
	ComponentUp.WithLabelValues("gone", "Deployment").Set(1)
	ResetScanned()
	if metricPresent(t, "safe_component_up", map[string]string{"component": "gone", "kind": "Deployment"}) {
		t.Fatal("ResetScanned should clear scanned component series")
	}
}

// gaugeValue reads a single gauge sample from the controller-runtime registry.
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
			if labelsMatch(m.GetLabel(), labels) && m.Gauge != nil {
				return m.Gauge.GetValue()
			}
		}
	}
	t.Fatalf("metric %s with labels %v not found", name, labels)
	return 0
}

func metricPresent(t *testing.T, name string, labels map[string]string) bool {
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
			if labelsMatch(m.GetLabel(), labels) {
				return true
			}
		}
	}
	return false
}

func labelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	got := map[string]string{}
	for _, lp := range pairs {
		got[lp.GetName()] = lp.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}
