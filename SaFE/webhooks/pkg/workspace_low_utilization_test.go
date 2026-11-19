/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"strings"
	"testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func lowUtilWorkspace(scope v1.WorkspaceScope, policy v1.LowUtilizationPolicy) *v1.Workspace {
	ws := &v1.Workspace{}
	ws.Spec.LowUtilization = map[v1.WorkspaceScope]v1.LowUtilizationPolicy{scope: policy}
	return ws
}

func TestValidateLowUtilization(t *testing.T) {
	valid := v1.LowUtilizationPolicy{
		Threshold: 5, Window: "30m", Mode: v1.LowUtilizationAverage, GracePeriod: "1h", NotifyBefore: "15m",
	}
	with := func(change func(p *v1.LowUtilizationPolicy)) v1.LowUtilizationPolicy {
		p := valid
		change(&p)
		return p
	}
	cases := []struct {
		name    string
		scope   v1.WorkspaceScope
		policy  v1.LowUtilizationPolicy
		wantErr string
	}{
		{"valid average", v1.TrainScope, valid, ""},
		{"valid min without durations", v1.InferScope, with(func(p *v1.LowUtilizationPolicy) {
			p.Mode, p.GracePeriod, p.NotifyBefore = v1.LowUtilizationMin, "", ""
		}), ""},
		{"threshold bounds", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.Threshold = 100 }), ""},
		{"window lower bound", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.Window = "5m" }), ""},
		{"window upper bound", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.Window = "24h" }), ""},
		{"zero durations", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.GracePeriod, p.NotifyBefore = "0s", "0s" }), ""},
		{"invalid scope", "Batch", valid, "invalid scope"},
		{"negative threshold", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.Threshold = -1 }), "threshold"},
		{"threshold above 100", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.Threshold = 101 }), "threshold"},
		{"missing window", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.Window = "" }), "invalid window"},
		{"unparsable window", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.Window = "half an hour" }), "invalid window"},
		{"window too short", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.Window = "4m59s" }), "between 5m and 24h"},
		{"window too long", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.Window = "25h" }), "between 5m and 24h"},
		{"invalid mode", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.Mode = "Max" }), "invalid mode"},
		{"empty mode", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.Mode = "" }), "invalid mode"},
		{"unparsable grace period", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.GracePeriod = "1d" }), "invalid gracePeriod"},
		{"negative grace period", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.GracePeriod = "-1m" }), "gracePeriod must not be negative"},
		{"negative notify before", v1.TrainScope, with(func(p *v1.LowUtilizationPolicy) { p.NotifyBefore = "-1m" }), "notifyBefore must not be negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLowUtilization(lowUtilWorkspace(tc.scope, tc.policy))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}

	if err := validateLowUtilization(&v1.Workspace{}); err != nil {
		t.Fatalf("a workspace without policies must pass: %v", err)
	}
}
