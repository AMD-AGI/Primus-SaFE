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
)

func TestSetBool(t *testing.T) {
	SetBool(LastPushTimestamp, true)
	if got := testGaugeValue(t, "safe_health_last_push_timestamp_seconds", nil); got != 1 {
		t.Fatalf("SetBool true: want 1, got %v", got)
	}
	SetBool(LastPushTimestamp, false)
	if got := testGaugeValue(t, "safe_health_last_push_timestamp_seconds", nil); got != 0 {
		t.Fatalf("SetBool false: want 0, got %v", got)
	}
}

func TestEscapeLabelValue(t *testing.T) {
	cases := map[string]string{
		`plain`:        `plain`,
		`a"b`:          `a\"b`,
		"line\nbreak":  `line\nbreak`,
		`back\slash`:   `back\\slash`,
	}
	for in, want := range cases {
		if got := escapeLabelValue(in); got != want {
			t.Errorf("escapeLabelValue(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestEncodeExtraLabels(t *testing.T) {
	if got := encodeExtraLabels(nil); got != "" {
		t.Errorf("empty labels should encode to empty, got %q", got)
	}
	got := encodeExtraLabels(map[string]string{"job": "primus-safe", "cluster": "crusoe"})
	// keys are sorted, so cluster comes before job
	want := "extra_label=cluster=crusoe&extra_label=job=primus-safe"
	if got != want {
		t.Errorf("encodeExtraLabels=%q, want %q", got, want)
	}
}

func TestGatherText(t *testing.T) {
	ResetScanned()
	ComponentUp.WithLabelValues("apiserver", "Deployment").Set(1)
	SubsystemUp.WithLabelValues(SubsystemDatabase).Set(0)

	body, err := gatherText()
	if err != nil {
		t.Fatalf("gatherText: %v", err)
	}
	out := string(body)
	if !strings.Contains(out, `safe_component_up{component="apiserver",kind="Deployment"} 1`) {
		t.Errorf("missing component_up line in:\n%s", out)
	}
	if !strings.Contains(out, `safe_subsystem_up{subsystem="database"} 0`) {
		t.Errorf("missing subsystem_up line in:\n%s", out)
	}
}

func TestPushSuccessAppliesExtraLabels(t *testing.T) {
	ResetScanned()
	ComponentUp.WithLabelValues("rm", "Deployment").Set(1)

	var gotPath, gotQuery, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	err := Push(context.Background(), srv.Client(), PushConfig{
		URL:   srv.URL + "/",
		Job:   "primus-safe",
		Token: "tok123",
		Extra: map[string]string{"cluster": "crusoe"},
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if gotPath != importPath {
		t.Errorf("path=%q, want %q", gotPath, importPath)
	}
	if !strings.Contains(gotQuery, "extra_label=cluster=crusoe") || !strings.Contains(gotQuery, "extra_label=job=primus-safe") {
		t.Errorf("query missing extra labels: %q", gotQuery)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth=%q, want Bearer tok123", gotAuth)
	}
	if !strings.Contains(gotBody, "safe_component_up") {
		t.Errorf("body missing metrics: %q", gotBody)
	}
}

func TestPushRemoteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := Push(context.Background(), srv.Client(), PushConfig{URL: srv.URL})
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status: %v", err)
	}
}

// testGaugeValue reads a single gauge sample value from the health Registry.
func testGaugeValue(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			ok := true
			for _, lp := range m.GetLabel() {
				if labels[lp.GetName()] != lp.GetValue() {
					ok = false
					break
				}
			}
			if ok && m.Gauge != nil {
				return m.Gauge.GetValue()
			}
		}
	}
	t.Fatalf("metric %s not found", name)
	return 0
}
