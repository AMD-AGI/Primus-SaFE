/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import "testing"

func TestParseKubeVersion(t *testing.T) {
	major, minor, patch, ok := ParseKubeVersion("v1.33.7")
	if !ok || major != 1 || minor != 33 || patch != 7 {
		t.Fatalf("unexpected parsed version: %d.%d.%d, ok=%t", major, minor, patch, ok)
	}
	for _, version := range []string{"", "1.33", " 1.33.7", "1.33.7 ", "1.33.7;cmd", "1.33.x"} {
		if _, _, _, valid := ParseKubeVersion(version); valid {
			t.Fatalf("expected invalid version %q", version)
		}
	}
}

func TestIsAllowedKubeVersionUpgrade(t *testing.T) {
	if !IsAllowedKubeVersionUpgrade("1.32.5", "1.32.6") {
		t.Fatal("expected patch upgrade to be allowed")
	}
	if !IsAllowedKubeVersionUpgrade("1.32.5", "1.33.7") {
		t.Fatal("expected one minor step to be allowed")
	}
	for _, target := range []string{"1.31.9", "1.34.1", "2.0.0", "invalid"} {
		if IsAllowedKubeVersionUpgrade("1.32.5", target) {
			t.Fatalf("expected target %q to be rejected", target)
		}
	}
	if IsAllowedKubeVersionUpgrade("", "1.33.7") {
		t.Fatal("expected an empty baseline to be rejected")
	}
}

func TestKubeSprayK8sVersionsReturnsCopy(t *testing.T) {
	versions := KubeSprayK8sVersions()
	versions["primussafe/kubespray:v2.31.0"] = "changed"
	version, ok := KubeVersionForKubeSprayImage("primussafe/kubespray:v2.31.0")
	if !ok || version != "1.35.4" {
		t.Fatalf("unexpected stored mapping %q", version)
	}
}

func TestClusterIsReadyDuringUpgradeLifecycle(t *testing.T) {
	cluster := new(Cluster)
	for _, phase := range []ClusterPhase{ReadyPhase, UpgradingPhase, UpgradeFailedPhase} {
		cluster.Status.ControlPlaneStatus.Phase = phase
		if !cluster.IsReady() {
			t.Fatalf("expected phase %q to keep data-plane clients ready", phase)
		}
	}
	cluster.Status.ControlPlaneStatus.Phase = CreatingPhase
	if cluster.IsReady() {
		t.Fatal("expected creating cluster not to be ready")
	}
}
