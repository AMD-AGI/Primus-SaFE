/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// isVirtualKubeletNode reports whether the execution-cluster node is a provider virtual node.
func isVirtualKubeletNode(node *corev1.Node) bool {
	return node != nil && node.Labels[v1.VirtualKubeletTypeLabelKey] == v1.VirtualKubeletTypeLabelValue
}

// adminNodeNameForK8sNode resolves the SaFE Node name for an execution-cluster node.
// Virtual kubelets are named after the k8s node; managed nodes carry the SaFE id label.
func adminNodeNameForK8sNode(node *corev1.Node) string {
	if node == nil {
		return ""
	}
	if id := v1.GetNodeId(node); id != "" {
		return id
	}
	if isVirtualKubeletNode(node) {
		return node.Name
	}
	return ""
}

// admitVirtualKubelet ensures a SaFE Node exists for a provider virtual node whose workspace
// is marked external. The provider never writes the SaFE CR; SaFE owns create and update.
//
// Returns the admin node name when admission applied or the object already exists, and an
// empty name when the node is not eligible (wrong type, missing identity, or non-external
// workspace). Errors are transport or API failures that should be retried.
func (r *NodeK8sReconciler) admitVirtualKubelet(ctx context.Context, clusterName string,
	k8sNode *corev1.Node) (string, error) {
	if !isVirtualKubeletNode(k8sNode) {
		return "", nil
	}
	workspaceID := k8sNode.Labels[v1.ExternalWorkspaceLabel]
	if workspaceID == "" {
		klog.V(4).Infof("skip virtual kubelet %s: missing %s", k8sNode.Name, v1.ExternalWorkspaceLabel)
		return "", nil
	}
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, apitypes.NamespacedName{Name: workspaceID}, workspace); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("skip virtual kubelet %s: workspace %s not found", k8sNode.Name, workspaceID)
			return "", nil
		}
		return "", err
	}
	if !v1.IsExternalWorkspace(workspace) {
		klog.V(4).Infof("skip virtual kubelet %s: workspace %s is not external", k8sNode.Name, workspaceID)
		return "", nil
	}
	if workspace.Spec.NodeFlavor == "" {
		return "", fmt.Errorf("external workspace %s has no nodeFlavor", workspaceID)
	}

	provider := k8sNode.Labels[v1.ExternalProviderLabel]
	allocationID := k8sNode.Labels[v1.ExternalAllocationIdLabel]
	generationRaw := k8sNode.Labels[v1.ExternalGenerationLabel]
	if provider == "" || allocationID == "" || generationRaw == "" {
		klog.V(4).Infof("skip virtual kubelet %s: incomplete allocation identity", k8sNode.Name)
		return "", nil
	}
	generation, err := strconv.ParseInt(generationRaw, 10, 64)
	if err != nil || generation <= 0 {
		return "", fmt.Errorf("virtual kubelet %s has invalid generation %q", k8sNode.Name, generationRaw)
	}
	hostKey := k8sNode.Annotations[v1.ExternalHostKeyAnnotation]

	desired := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8sNode.Name,
			Labels: map[string]string{
				v1.DisplayNameLabel:  k8sNode.Name,
				v1.ClusterIdLabel:    clusterName,
				v1.NodeIdLabel:       k8sNode.Name,
				v1.WorkspaceIdLabel:  workspaceID,
				v1.NodeFlavorIdLabel: workspace.Spec.NodeFlavor,
			},
		},
		Spec: v1.NodeSpec{
			Cluster:       pointer.String(clusterName),
			Workspace:     pointer.String(workspaceID),
			Hostname:      pointer.String(k8sNode.Name),
			LifecycleMode: v1.NodeLifecycleExternal,
			NodeFlavor: &corev1.ObjectReference{
				APIVersion: v1.SchemeGroupVersion.String(),
				Kind:       v1.NodeFlavorKind,
				Name:       workspace.Spec.NodeFlavor,
			},
			ExternalRef: &v1.NodeExternalRef{
				Provider:     provider,
				AllocationId: allocationID,
				Generation:   generation,
				HostKey:      hostKey,
			},
		},
	}

	existing := &v1.Node{}
	err = r.Get(ctx, apitypes.NamespacedName{Name: k8sNode.Name}, existing)
	if apierrors.IsNotFound(err) {
		if err = r.Create(ctx, desired); err != nil {
			return "", err
		}
		klog.Infof("admitted external node %s for virtual kubelet in cluster %s workspace %s",
			k8sNode.Name, clusterName, workspaceID)
		return k8sNode.Name, nil
	}
	if err != nil {
		return "", err
	}
	if !existing.IsExternal() {
		return "", fmt.Errorf("node %s exists but is not lifecycleMode external", existing.Name)
	}
	if err = r.patchAdmittedExternalNode(ctx, existing, desired); err != nil {
		return "", err
	}
	return existing.Name, nil
}

// patchAdmittedExternalNode updates mutable admission fields after create. lifecycleMode and
// the allocation identity keys stay immutable; hostKey may appear after the first observation
// freeze. Spec cluster/workspace/flavor are not rewritten here -- the validating webhook
// treats those as immutable once set.
func (r *NodeK8sReconciler) patchAdmittedExternalNode(ctx context.Context, existing, desired *v1.Node) error {
	patch := client.MergeFrom(existing.DeepCopy())
	changed := false
	if existing.Spec.ExternalRef != nil && desired.Spec.ExternalRef != nil {
		if existing.Spec.ExternalRef.HostKey == "" && desired.Spec.ExternalRef.HostKey != "" {
			existing.Spec.ExternalRef.HostKey = desired.Spec.ExternalRef.HostKey
			changed = true
		}
	}
	for _, k := range []string{v1.ClusterIdLabel, v1.NodeIdLabel, v1.WorkspaceIdLabel, v1.NodeFlavorIdLabel, v1.DisplayNameLabel} {
		if v, ok := desired.Labels[k]; ok {
			if v1.SetLabel(existing, k, v) {
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	return r.Patch(ctx, existing, patch)
}
