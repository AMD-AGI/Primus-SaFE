/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
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

func TestK8sNodeFromInformerObjTombstone(t *testing.T) {
	vk := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "vk-gone",
		Labels: map[string]string{v1.VirtualKubeletTypeLabelKey: v1.VirtualKubeletTypeLabelValue},
	}}
	got, ok := k8sNodeFromInformerObj(cache.DeletedFinalStateUnknown{Key: "vk-gone", Obj: vk})
	if !ok || got.Name != "vk-gone" {
		t.Fatalf("tombstone unwrap failed: ok=%v name=%v", ok, got)
	}
	if _, ok = k8sNodeFromInformerObj("not-a-node"); ok {
		t.Fatal("non-node payload must fail")
	}
}

func TestHandleNodeUnmanagedDeletesExternalOnVKDelete(t *testing.T) {
	admin := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "vk-ff9f"},
		Spec: v1.NodeSpec{
			Cluster:       pointer.String("crusoe"),
			Workspace:     pointer.String("ws-ext"),
			LifecycleMode: v1.NodeLifecycleExternal,
			ExternalRef: &v1.NodeExternalRef{
				Provider: "spur", AllocationId: "a1", Generation: 1,
			},
		},
	}
	r := newNodeK8sReconciler(t, admin)
	err := r.handleNodeUnmanaged(context.Background(), &nodeQueueMessage{
		clusterName: "crusoe", k8sNodeName: "vk-ff9f", action: NodeDelete,
	}, admin)
	if err != nil {
		t.Fatalf("handle delete: %v", err)
	}
	missing := &v1.Node{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "vk-ff9f"}, missing); err == nil {
		t.Fatal("expected admin Node deleted after virtual kubelet removal")
	} else if !apierrors.IsNotFound(err) {
		t.Fatalf("unexpected get error: %v", err)
	}
}

func TestGcOrphanedExternalAdminNodes(t *testing.T) {
	viper.Set("external_execution.enabled", true)
	t.Cleanup(func() { viper.Set("external_execution.enabled", false) })

	orphan := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "vk-orphan"},
		Spec: v1.NodeSpec{
			Cluster:       pointer.String("crusoe"),
			Workspace:     pointer.String("ws-ext"),
			LifecycleMode: v1.NodeLifecycleExternal,
			ExternalRef: &v1.NodeExternalRef{
				Provider: "spur", AllocationId: "a1", Generation: 1,
			},
		},
	}
	kept := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "vk-live"},
		Spec: v1.NodeSpec{
			Cluster:       pointer.String("crusoe"),
			Workspace:     pointer.String("ws-ext"),
			LifecycleMode: v1.NodeLifecycleExternal,
			ExternalRef: &v1.NodeExternalRef{
				Provider: "spur", AllocationId: "a2", Generation: 1,
			},
		},
	}
	native := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Spec:       v1.NodeSpec{Cluster: pointer.String("crusoe")},
	}
	r := newNodeK8sReconciler(t, orphan, kept, native)
	cs := k8sfake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "vk-live",
		Labels: map[string]string{
			v1.VirtualKubeletTypeLabelKey: v1.VirtualKubeletTypeLabelValue,
		},
	}})
	factory := commonclient.NewClientFactoryForTestWithInformer("crusoe", cs)
	t.Cleanup(func() { _ = factory.Release() })
	if err := r.clientManager.Add("crusoe", factory); err != nil {
		t.Fatalf("add factory: %v", err)
	}

	if err := r.gcOrphanedExternalAdminNodes(context.Background(), "crusoe"); err != nil {
		t.Fatalf("gc: %v", err)
	}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "vk-orphan"}, &v1.Node{}); err == nil {
		t.Fatal("orphaned external admin Node should be deleted")
	}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "vk-live"}, &v1.Node{}); err != nil {
		t.Fatalf("live external admin Node should remain: %v", err)
	}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "worker-1"}, &v1.Node{}); err != nil {
		t.Fatalf("native admin Node must not be touched: %v", err)
	}
}

func TestAdmitVirtualKubeletCreatesExternalOnly(t *testing.T) {
	viper.Set("external_execution.enabled", true)
	t.Cleanup(func() { viper.Set("external_execution.enabled", false) })

	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ws-ext",
			Labels: map[string]string{v1.WorkspaceExternalLabel: "true"},
		},
		Spec: v1.WorkspaceSpec{Cluster: "crusoe", NodeFlavor: "vk-mi355x"},
	}
	nativeWS := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-native"},
		Spec:       v1.WorkspaceSpec{Cluster: "crusoe", NodeFlavor: "mi355x"},
	}
	r := newNodeK8sReconciler(t, ws, nativeWS)

	vk := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "vk-ff9f",
		Labels: map[string]string{
			v1.VirtualKubeletTypeLabelKey: v1.VirtualKubeletTypeLabelValue,
			v1.ExternalWorkspaceLabel:     "ws-ext",
			v1.ExternalProviderLabel:      "spur",
			v1.ExternalAllocationIdLabel:  "ff9fb6d2-5c1a-4f56-b70a-d368ad81cefc",
			v1.ExternalGenerationLabel:    "1",
		},
		Annotations: map[string]string{v1.ExternalHostKeyAnnotation: "host-1"},
	}}
	name, err := r.admitVirtualKubelet(context.Background(), "crusoe", vk)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if name != "vk-ff9f" {
		t.Fatalf("name = %q, want vk-ff9f", name)
	}
	created := &v1.Node{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "vk-ff9f"}, created); err != nil {
		t.Fatalf("get admitted node: %v", err)
	}
	if !created.IsExternal() {
		t.Fatal("admitted node must be lifecycleMode external with externalRef")
	}
	if created.GetSpecWorkspace() != "ws-ext" {
		t.Fatalf("workspace = %q, want ws-ext", created.GetSpecWorkspace())
	}
	if created.Spec.NodeTemplate == nil || created.Spec.SSHSecret == nil {
		t.Fatal("CRD-required nodeTemplate and secret stubs must be present")
	}

	// Native workspace: never create an admin Node from a virtual kubelet.
	vkNative := vk.DeepCopy()
	vkNative.Name = "vk-native-ws"
	vkNative.Labels[v1.ExternalWorkspaceLabel] = "ws-native"
	name, err = r.admitVirtualKubelet(context.Background(), "crusoe", vkNative)
	if err != nil {
		t.Fatalf("admit native workspace: %v", err)
	}
	if name != "" {
		t.Fatalf("non-external workspace must not admit, got %q", name)
	}
	missing := &v1.Node{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "vk-native-ws"}, missing); err == nil {
		t.Fatal("expected no admin Node for non-external workspace")
	}
}

func TestAdmitVirtualKubeletSkipsNonVirtualKubelet(t *testing.T) {
	r := newNodeK8sReconciler(t)
	name, err := r.admitVirtualKubelet(context.Background(), "crusoe", &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
	})
	if err != nil || name != "" {
		t.Fatalf("native node must be skipped, name=%q err=%v", name, err)
	}
}
