/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

const (
	DefaultPort = 22
)

var (
	SupportedTaintEffect = []corev1.TaintEffect{corev1.TaintEffectNoSchedule,
		corev1.TaintEffectPreferNoSchedule, corev1.TaintEffectNoExecute}
)

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

type NodeMutator struct {
	client.Client
	decoder admission.Decoder
}

func (m *NodeMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
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
	if data, err := json.Marshal(node); err != nil {
		return handleError(v1.NodeKind, err)
	} else {
		return admission.PatchResponseFromRaw(req.Object.Raw, data)
	}
}

func (m *NodeMutator) mutateOnCreation(ctx context.Context, n *v1.Node) bool {
	m.mutateSpec(ctx, n)
	m.mutateMeta(ctx, n)
	m.mutateCommon(ctx, n)
	return true
}

func (m *NodeMutator) mutateOnUpdate(ctx context.Context, n *v1.Node) bool {
	return m.mutateCommon(ctx, n)
}

func (m *NodeMutator) mutateSpec(_ context.Context, n *v1.Node) {
	if n.GetSpecHostName() == "" {
		n.Spec.Hostname = ptr.To(n.Spec.PrivateIP)
	}
	if n.Spec.Port == nil {
		n.Spec.Port = pointer.Int32(DefaultPort)
	}
}

func (m *NodeMutator) mutateMeta(_ context.Context, n *v1.Node) {
	n.Name = stringutil.NormalizeName(n.GetSpecHostName())
	if v1.GetDisplayName(n) == "" {
		metav1.SetMetaDataLabel(&n.ObjectMeta, v1.DisplayNameLabel, n.GetSpecHostName())
	}
	controllerutil.AddFinalizer(n, v1.NodeFinalizer)
}

func (m *NodeMutator) mutateCommon(ctx context.Context, n *v1.Node) bool {
	isChanged := false
	if m.mutateLabels(n) {
		isChanged = true
	}
	if m.mutateByNodeFlavor(ctx, n) {
		isChanged = true
	}
	if m.mutateTaints(n) {
		isChanged = true
	}
	return isChanged
}

func (m *NodeMutator) mutateLabels(n *v1.Node) bool {
	isChanged := false
	if n.Spec.NodeFlavor != nil {
		if v1.SetLabel(n, v1.NodeFlavorIdLabel, n.Spec.NodeFlavor.Name) {
			isChanged = true
		}
	}
	if v1.RemoveEmptyLabel(n, v1.WorkspaceIdLabel) {
		isChanged = true
	}
	if v1.RemoveEmptyLabel(n, v1.ClusterIdLabel) {
		isChanged = true
	}
	return isChanged
}

func (m *NodeMutator) mutateByNodeFlavor(ctx context.Context, n *v1.Node) bool {
	nf, _ := getNodeFlavor(ctx, m.Client, v1.GetNodeFlavorId(n))
	if nf == nil {
		return false
	}
	isChanged := false
	if nf.HasGpu() {
		if v1.SetAnnotation(n, v1.GpuProductNameAnnotation, nf.Spec.Gpu.Product) {
			isChanged = true
		}
		if v1.SetAnnotation(n, v1.GpuResourceNameAnnotation, nf.Spec.Gpu.ResourceName) {
			isChanged = true
		}
		if v1.SetLabel(n, v1.NodeGpuCountLabel, nf.Spec.Gpu.Quantity.String()) {
			isChanged = true
		}
	} else {
		if v1.RemoveAnnotation(n, v1.GpuProductNameAnnotation) {
			isChanged = true
		}
		if v1.RemoveAnnotation(n, v1.GpuResourceNameAnnotation) {
			isChanged = true
		}
		if v1.RemoveLabel(n, v1.NodeGpuCountLabel) {
			isChanged = true
		}
	}
	return isChanged
}

func (m *NodeMutator) mutateTaints(n *v1.Node) bool {
	isChanged := false
	if n.GetSpecCluster() == "" {
		// clear all taints when unmanaging node
		if len(n.Spec.Taints) > 0 {
			n.Spec.Taints = nil
			isChanged = true
		}
	} else {
		for i := range n.Spec.Taints {
			if n.Spec.Taints[i].TimeAdded == nil {
				n.Spec.Taints[i].TimeAdded = &metav1.Time{Time: time.Now().UTC()}
				isChanged = true
			}
		}
	}
	return isChanged
}

type NodeValidator struct {
	client.Client
	decoder admission.Decoder
}

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

func (v *NodeValidator) validateOnCreation(ctx context.Context, node *v1.Node) error {
	if err := v.validateCommon(ctx, node); err != nil {
		return err
	}
	return nil
}

func (v *NodeValidator) validateOnUpdate(ctx context.Context, newNode, oldNode *v1.Node) error {
	if err := v.validateImmutableFields(newNode, oldNode); err != nil {
		return err
	}
	if err := v.validateCommon(ctx, newNode); err != nil {
		return err
	}
	if err := v.validateReadyWhenManaging(ctx, newNode, oldNode); err != nil {
		return err
	}
	if err := v.validateRelatedFaults(ctx, newNode, oldNode); err != nil {
		return err
	}
	return nil
}

func (v *NodeValidator) validateCommon(ctx context.Context, node *v1.Node) error {
	if err := validateDisplayName(v1.GetDisplayName(node)); err != nil {
		return err
	}
	if err := v.validateNodeSpec(ctx, node); err != nil {
		return err
	}
	return nil
}

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
	if node.Spec.Port != nil {
		if err := validatePort(v1.NodeKind, int(*node.Spec.Port)); err != nil {
			return err
		}
	}
	if node.Spec.PrivateIP == "" {
		return commonerrors.NewBadRequest("privateIp is required")
	}
	if err := v.validateNodeTaints(node); err != nil {
		return err
	}
	return nil
}

func (v *NodeValidator) validateNodeWorkspace(ctx context.Context, node *v1.Node) error {
	workspaceName := node.GetSpecWorkspace()
	if workspaceName == "" {
		return nil
	}
	if _, err := getWorkspace(ctx, v.Client, workspaceName); err != nil {
		return err
	}
	return nil
}

func (v *NodeValidator) validateNodeFlavor(ctx context.Context, node *v1.Node) error {
	if node.Spec.NodeFlavor == nil {
		return commonerrors.NewBadRequest("the flavor of node is not found")
	}
	nf, _ := getNodeFlavor(ctx, v.Client, node.Spec.NodeFlavor.Name)
	if nf == nil {
		return commonerrors.NewBadRequest(fmt.Sprintf("the flavo(%s) is not found", node.Spec.NodeFlavor.Name))
	}
	return nil
}

func (v *NodeValidator) validateNodeSSH(_ context.Context, node *v1.Node) error {
	if node.Spec.SSHSecret == nil {
		return commonerrors.NewBadRequest("the ssh secret of node is not found")
	}
	return nil
}

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
		taintSet.Insert(t.Key)
	}
	return nil
}

func (v *NodeValidator) validateImmutableFields(newNode, oldNode *v1.Node) error {
	if oldNode.GetSpecHostName() != newNode.GetSpecHostName() {
		return field.Forbidden(field.NewPath("spec").Key("hostname"), "immutable")
	}
	if newNode.Spec.Port == nil || *oldNode.Spec.Port != *newNode.Spec.Port {
		return field.Forbidden(field.NewPath("spec").Key("port"), "immutable")
	}
	if oldNode.GetSpecCluster() != "" && newNode.GetSpecCluster() != "" &&
		oldNode.GetSpecCluster() != newNode.GetSpecCluster() {
		return field.Forbidden(field.NewPath("spec").Key("cluster"), "immutable")
	}
	if oldNode.GetSpecWorkspace() != "" && newNode.GetSpecWorkspace() != "" &&
		oldNode.GetSpecWorkspace() != newNode.GetSpecWorkspace() {
		return field.Forbidden(field.NewPath("spec").Key("workspace"), "immutable")
	}
	if newNode.Spec.SSHSecret == nil ||
		oldNode.Spec.SSHSecret.Name != newNode.Spec.SSHSecret.Name ||
		oldNode.Spec.SSHSecret.Namespace != newNode.Spec.SSHSecret.Namespace {
		return field.Forbidden(field.NewPath("spec").Key("sshSecret"), "immutable")
	}
	return nil
}

// When a node is being managed by the cluster, it must be in the Ready state.
func (v *NodeValidator) validateReadyWhenManaging(_ context.Context, newNode, oldNode *v1.Node) error {
	if oldNode.GetSpecCluster() == "" && newNode.GetSpecCluster() != "" &&
		oldNode.Status.MachineStatus.Phase != v1.NodeReady {
		return commonerrors.NewNodeNotReady(
			fmt.Sprintf("node %s is not ready. current state: %s", oldNode.Name, oldNode.Status.MachineStatus.Phase))
	}
	return nil
}

func (v *NodeValidator) validateRelatedFaults(ctx context.Context, newNode, oldNode *v1.Node) error {
	if newNode.GetSpecCluster() == "" {
		return nil
	}
	newTaintKeys := sets.NewSet()
	for _, t := range newNode.Spec.Taints {
		newTaintKeys.Insert(t.Key)
	}
	for _, t := range oldNode.Spec.Taints {
		if newTaintKeys.Has(t.Key) {
			continue
		}
		id := commonfaults.GetIdByTaintKey(t.Key)
		faultName := commonfaults.GenerateFaultName(newNode.Name, id)
		fault := &v1.Fault{}
		if v.Get(ctx, client.ObjectKey{Name: faultName}, fault) == nil && fault.GetDeletionTimestamp().IsZero() {
			return commonerrors.NewForbidden(
				fmt.Sprintf("the taint %s is controlled by fault %s. Please delete the fault first",
					t.Key, faultName))
		}
	}
	return nil
}

func getNode(ctx context.Context, cli client.Client, name string) (*v1.Node, error) {
	if name == "" {
		return nil, nil
	}
	node := &v1.Node{}
	if err := cli.Get(ctx, client.ObjectKey{Name: name}, node); err != nil {
		return nil, err
	}
	return node, nil
}
