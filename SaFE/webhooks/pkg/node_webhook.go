/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	// DefaultNodePort Default node SSH port.
	DefaultNodePort = 22
)

var SupportedTaintEffect = []corev1.TaintEffect{
	corev1.TaintEffectNoSchedule,
	corev1.TaintEffectPreferNoSchedule, corev1.TaintEffectNoExecute,
}

// AddNodeWebhook registers the node validation and mutation webhooks.
func AddNodeWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.NodeKind), &webhook.Admission{Handler: &NodeMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.NodeKind), &webhook.Admission{Handler: &NodeValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

// NodeMutator handles mutation logic for Node resources.
type NodeMutator struct {
	client.Client
	decoder admission.Decoder
}

// Handle processes node creation requests and applies default values and normalizations.
func (m *NodeMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation == admissionv1.Delete {
		return admission.Allowed("")
	}

	node := &v1.Node{}
	if err := m.decoder.Decode(req, node); err != nil {
		return handleError(v1.NodeKind, err)
	}
	if !node.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("")
	}
	isChanged := false
	switch req.Operation {
	case admissionv1.Create:
		isChanged = m.mutateOnCreation(ctx, node)
	case admissionv1.Update:
		isChanged = m.mutateOnUpdate(ctx, node)
	}
	if !isChanged {
		return admission.Allowed("")
	}
	data, err := json.Marshal(node)
	if err != nil {
		return handleError(v1.NodeKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, data)
}

// mutateOnCreation applies default values and normalizations during creation.
func (m *NodeMutator) mutateOnCreation(ctx context.Context, node *v1.Node) bool {
	m.mutateSpec(ctx, node)
	m.mutateMeta(ctx, node)
	m.mutateCommon(ctx, node)
	return true
}

// mutateOnUpdate applies mutations during updates.
func (m *NodeMutator) mutateOnUpdate(ctx context.Context, node *v1.Node) bool {
	return m.mutateCommon(ctx, node)
}

// mutateSpec normalizes hostname, private IP and default SSH port.
func (m *NodeMutator) mutateSpec(_ context.Context, node *v1.Node) {
	node.Spec.PrivateIP = strings.Trim(node.Spec.PrivateIP, " ")
	if node.GetSpecHostName() == "" {
		node.Spec.Hostname = pointer.String(node.Spec.PrivateIP)
	} else {
		node.Spec.Hostname = pointer.String(strings.Trim(*node.Spec.Hostname, " "))
	}
	if node.Spec.Port == nil {
		node.Spec.Port = pointer.Int32(DefaultNodePort)
	}
}

// mutateMeta sets node name, default labels and finalizer.
func (m *NodeMutator) mutateMeta(_ context.Context, node *v1.Node) {
	node.Name = stringutil.NormalizeName(node.GetSpecHostName())
	if v1.GetDisplayName(node) == "" {
		v1.SetLabel(node, v1.DisplayNameLabel, node.GetSpecHostName())
	}
	v1.SetLabel(node, v1.NodeIdLabel, node.Name)
	controllerutil.AddFinalizer(node, v1.NodeFinalizer)
}

// mutateCommon syncs labels, flavor-derived fields and taints.
func (m *NodeMutator) mutateCommon(ctx context.Context, node *v1.Node) bool {
	isChanged := false
	if m.mutateLabels(node) {
		isChanged = true
	}
	if m.mutateByNodeFlavor(ctx, node) {
		isChanged = true
	}
	if m.mutateTaints(node) {
		isChanged = true
	}
	return isChanged
}

// mutateLabels updates flavor/cluster/workspace labels and clears reset flags.
func (m *NodeMutator) mutateLabels(node *v1.Node) bool {
	isChanged := false
	if node.Spec.NodeFlavor != nil {
		if v1.SetLabel(node, v1.NodeFlavorIdLabel, node.Spec.NodeFlavor.Name) {
			isChanged = true
		}
	}
	if v1.RemoveEmptyLabel(node, v1.WorkspaceIdLabel) {
		isChanged = true
	}
	if v1.RemoveEmptyLabel(node, v1.ClusterIdLabel) {
		isChanged = true
	}
	if node.GetSpecHostName() != "" && v1.SetLabel(node, v1.NodeHostnameLabel, node.GetSpecHostName()) {
		isChanged = true
	}
	return isChanged
}

// mutateByNodeFlavor syncs GPU annotations/labels from the node flavor.
func (m *NodeMutator) mutateByNodeFlavor(ctx context.Context, node *v1.Node) bool {
	nf, _ := getNodeFlavor(ctx, m.Client, v1.GetNodeFlavorId(node))
	if nf == nil {
		return false
	}
	isChanged := false
	if nf.HasGpu() {
		if v1.SetAnnotation(node, v1.GpuResourceNameAnnotation, nf.Spec.Gpu.ResourceName) {
			isChanged = true
		}
		if v1.SetLabel(node, v1.NodeGpuCountLabel, nf.Spec.Gpu.Quantity.String()) {
			isChanged = true
		}
	} else {
		if v1.RemoveAnnotation(node, v1.GpuResourceNameAnnotation) {
			isChanged = true
		}
		if v1.RemoveLabel(node, v1.NodeGpuCountLabel) {
			isChanged = true
		}
	}
	return isChanged
}

// mutateTaints clears taints for unmanaged nodes or timestamps taints when managed.
func (m *NodeMutator) mutateTaints(node *v1.Node) bool {
	isChanged := false
	if node.GetSpecCluster() == "" {
		// clear all taints when unmanaging node
		if len(node.Spec.Taints) > 0 {
			node.Spec.Taints = nil
			isChanged = true
		}
	} else {
		for i := range node.Spec.Taints {
			if node.Spec.Taints[i].TimeAdded == nil {
				node.Spec.Taints[i].TimeAdded = &metav1.Time{Time: time.Now().UTC()}
				isChanged = true
			}
		}
	}
	return isChanged
}

// NodeValidator validates Node resources on create and update operations.
type NodeValidator struct {
	client.Client
	decoder admission.Decoder
}

// Handle validates node resources on create, update, and delete operations.
func (v *NodeValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	node := &v1.Node{}
	var err error
	switch req.Operation {
	case admissionv1.Create:
		if err = v.decoder.Decode(req, node); err != nil {
			break
		}
		err = v.validateOnCreation(ctx, node)
	case admissionv1.Update:
		if err = v.decoder.Decode(req, node); err != nil {
			break
		}
		if !node.GetDeletionTimestamp().IsZero() {
			break
		}
		oldNode := &v1.Node{}
		if err = v.decoder.DecodeRaw(req.OldObject, oldNode); err == nil {
			err = v.validateOnUpdate(ctx, node, oldNode)
		}
	default:
	}
	if err != nil {
		return handleError(v1.NodeKind, err)
	}
	return admission.Allowed("")
}

// validateOnCreation validates node display name and spec on creation.
func (v *NodeValidator) validateOnCreation(ctx context.Context, node *v1.Node) error {
	if err := v.validateCommon(ctx, node); err != nil {
		return err
	}
	return nil
}

// validateOnUpdate validates immutable fields and common spec on update.
func (v *NodeValidator) validateOnUpdate(ctx context.Context, newNode, oldNode *v1.Node) error {
	if err := v.validateImmutableFields(newNode, oldNode); err != nil {
		return err
	}
	if err := v.validateCommon(ctx, newNode); err != nil {
		return err
	}
	return nil
}

// validateCommon validates display name and node spec.
func (v *NodeValidator) validateCommon(ctx context.Context, node *v1.Node) error {
	if err := validateDisplayName(v1.GetDisplayName(node), ""); err != nil {
		return err
	}
	if err := validateLabels(node.GetLabels()); err != nil {
		return err
	}
	if err := v.validateNodeSpec(ctx, node); err != nil {
		return err
	}
	return nil
}

// validateNodeSpec validates workspace, flavor, SSH, port, IP and taints configuration.
func (v *NodeValidator) validateNodeSpec(ctx context.Context, node *v1.Node) error {
	if err := v.validateNodeWorkspace(ctx, node); err != nil {
		return err
	}
	if err := v.validateNodeFlavor(ctx, node); err != nil {
		return err
	}
	if err := v.validateNodeSSH(ctx, node); err != nil {
		return err
	}
	if err := validatePort(v1.NodeKind, int(node.GetSpecPort())); err != nil {
		return err
	}
	if node.Spec.PrivateIP == "" {
		return commonerrors.NewBadRequest("privateIp is required")
	}
	if err := v.validateNodeTaints(node); err != nil {
		return err
	}
	return nil
}

// validateNodeWorkspace ensures the workspace exists.
func (v *NodeValidator) validateNodeWorkspace(ctx context.Context, node *v1.Node) error {
	workspaceId := node.GetSpecWorkspace()
	if _, err := getWorkspace(ctx, v.Client, workspaceId); err != nil {
		return err
	}
	return nil
}

// validateNodeFlavor ensures the node flavor exists.
func (v *NodeValidator) validateNodeFlavor(ctx context.Context, node *v1.Node) error {
	if node.Spec.NodeFlavor == nil {
		return commonerrors.NewBadRequest("the flavor of node is not found")
	}
	nf, _ := getNodeFlavor(ctx, v.Client, node.Spec.NodeFlavor.Name)
	if nf == nil {
		return commonerrors.NewBadRequest(fmt.Sprintf("the flavor(%s) is not found", node.Spec.NodeFlavor.Name))
	}
	return nil
}

// validateNodeSSH ensures SSH secret is configured.
func (v *NodeValidator) validateNodeSSH(_ context.Context, node *v1.Node) error {
	if node.Spec.SSHSecret == nil {
		return commonerrors.NewBadRequest("the ssh secret of node is not found")
	}
	return nil
}

// validateNodeTaints checks for duplicate taints and validates taint effects.
func (v *NodeValidator) validateNodeTaints(node *v1.Node) error {
	taintSet := sets.NewSet()
	for _, t := range node.Spec.Taints {
		if taintSet.Has(t.Key) {
			return commonerrors.NewBadRequest(fmt.Sprintf("repeat taint, key: %s", t.Key))
		}
		hasFound := false
		for _, effect := range SupportedTaintEffect {
			if t.Effect == effect {
				hasFound = true
				break
			}
		}
		if !hasFound {
			return commonerrors.NewBadRequest(
				fmt.Sprintf("invalid taint effect. key: %s, unsupported: %s, supported: %v",
					t.Key, t.Effect, SupportedTaintEffect))
		}
		if err := validateLabelKey(t.Key); err != nil {
			return err
		}
		taintSet.Insert(t.Key)
	}
	return nil
}

// validateImmutableFields ensures hostname, cluster and workspace cannot be modified.
func (v *NodeValidator) validateImmutableFields(newNode, oldNode *v1.Node) error {
	if oldNode.GetSpecHostName() != newNode.GetSpecHostName() {
		return field.Forbidden(field.NewPath("spec").Key("hostname"), "immutable")
	}
	if oldNode.GetSpecCluster() != "" && newNode.GetSpecCluster() != "" &&
		oldNode.GetSpecCluster() != newNode.GetSpecCluster() {
		return field.Forbidden(field.NewPath("spec").Key("cluster"), "immutable")
	}
	if oldNode.GetSpecWorkspace() != "" && newNode.GetSpecWorkspace() != "" &&
		oldNode.GetSpecWorkspace() != newNode.GetSpecWorkspace() {
		return field.Forbidden(field.NewPath("spec").Key("workspace"), "immutable")
	}
	if oldNode.Spec.PrivateIP != newNode.Spec.PrivateIP && v1.IsControlPlane(newNode) {
		return field.Forbidden(field.NewPath("spec").Key("privateIP"), "immutable")
	}
	return nil
}

// getNode retrieves the requested information.
func getNode(ctx context.Context, cli client.Client, nodeId string) (*v1.Node, error) {
	if nodeId == "" {
		return nil, nil
	}
	node := &v1.Node{}
	if err := cli.Get(ctx, client.ObjectKey{Name: nodeId}, node); err != nil {
		return nil, err
	}
	return node, nil
}
