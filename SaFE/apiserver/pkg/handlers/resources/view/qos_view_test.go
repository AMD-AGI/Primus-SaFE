/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	"testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestValidateUserQosAcceptsClustersOnly(t *testing.T) {
	cluster := map[string]v1.UserQosScope{"c1": {MaxResources: &v1.ResourceCap{GPU: "8"}}}
	if err := ValidateUserQos(&v1.UserQos{Clusters: cluster}); err != nil {
		t.Fatalf("per-cluster values are accepted: %v", err)
	}
	if err := ValidateUserQos(&v1.UserQos{MaxResources: &v1.ResourceCap{GPU: "8"}, Clusters: cluster}); err == nil {
		t.Fatal("top-level maxResources must be rejected")
	}
	if err := ValidateUserQos(&v1.UserQos{MaxRuntime: map[string]int{string(v1.TrainScope): 3600}}); err == nil {
		t.Fatal("top-level maxRuntime must be rejected")
	}
}
