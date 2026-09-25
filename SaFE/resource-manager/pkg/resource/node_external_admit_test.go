/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestIsVirtualKubeletNode(t *testing.T) {
	if isVirtualKubeletNode(nil) {
		t.Fatal("nil node is not a virtual kubelet")
	}
	native := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	if isVirtualKubeletNode(native) {
		t.Fatal("unlabelled node is not a virtual kubelet")
	}
	vk := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "vk-1",
		Labels: map[string]string{v1.VirtualKubeletTypeLabelKey: v1.VirtualKubeletTypeLabelValue},
	}}
	if !isVirtualKubeletNode(vk) {
		t.Fatal("expected type=virtual-kubelet to match")
	}
}

func TestAdminNodeNameForK8sNode(t *testing.T) {
	labelled := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "host-a",
		Labels: map[string]string{v1.NodeIdLabel: "admin-a"},
	}}
	if got := adminNodeNameForK8sNode(labelled); got != "admin-a" {
		t.Fatalf("got %q, want admin-a", got)
	}
	vk := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "vk-xyz",
		Labels: map[string]string{v1.VirtualKubeletTypeLabelKey: v1.VirtualKubeletTypeLabelValue},
	}}
	if got := adminNodeNameForK8sNode(vk); got != "vk-xyz" {
		t.Fatalf("got %q, want vk-xyz", got)
	}
	if got := adminNodeNameForK8sNode(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "plain"}}); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
