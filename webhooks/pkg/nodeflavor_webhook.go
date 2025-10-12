/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

var (
	GpuResourceWhiteList = []string{common.NvidiaGpu, common.AmdGpu}
)

func AddNodeFlavorWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.NodeFlavorKind), &webhook.Admission{Handler: &NodeFlavorMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.NodeFlavorKind), &webhook.Admission{Handler: &NodeFlavorValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

type NodeFlavorMutator struct {
	client.Client
	decoder admission.Decoder
}

func (m *NodeFlavorMutator) Handle(_ context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}
	nf := &v1.NodeFlavor{}
	if err := m.decoder.Decode(req, nf); err != nil {
		return handleError(v1.NodeFlavorKind, err)
	}
	m.mutateOnCreation(nf)
	data, err := json.Marshal(nf)
	if err != nil {
		return handleError(v1.NodeFlavorKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, data)
}

func (m *NodeFlavorMutator) mutateOnCreation(nf *v1.NodeFlavor) {
	nf.Name = stringutil.NormalizeName(nf.Name)
	if nf.Spec.Gpu != nil && nf.Spec.Gpu.Quantity.IsZero() {
		nf.Spec.Gpu = nil
	}
}

func (m *NodeFlavorMutator) mutateExtendResources(nf *v1.NodeFlavor) {
	extendResources := make(corev1.ResourceList)
	for key, val := range nf.Spec.ExtendResources {
		strKey := string(key)
		strKey = strings.Trim(strKey, " ")
		if strKey == "" {
			continue
		}
		extendResources[corev1.ResourceName(strKey)] = val
	}
	nf.Spec.ExtendResources = extendResources
}

type NodeFlavorValidator struct {
	client.Client
	decoder admission.Decoder
}

func (v *NodeFlavorValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	newFlavor := &v1.NodeFlavor{}
	var err error
	if err = v.decoder.Decode(req, newFlavor); err != nil {
		return handleError(v1.NodeFlavorKind, err)
	}
	if !newFlavor.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("")
	}

	switch req.Operation {
	case admissionv1.Create, admissionv1.Update:
		err = v.validateCommon(newFlavor)
	default:
	}
	if err != nil {
		return handleError(v1.NodeFlavorKind, err)
	}
	return admission.Allowed("")
}

func (v *NodeFlavorValidator) validateCommon(nf *v1.NodeFlavor) error {
	if nf.Spec.Cpu.Quantity.Value() <= 0 {
		return fmt.Errorf("invalid cpu: %s", nf.Spec.Cpu.Quantity.String())
	}
	if nf.Spec.Memory.Value() <= 0 {
		return fmt.Errorf("invalid memory: %s", nf.Spec.Memory.String())
	}
	if nf.Spec.Gpu != nil {
		if !isValidGpuResource(nf.Spec.Gpu.ResourceName) {
			return fmt.Errorf("invalid gpu resourceName: %s", nf.Spec.Gpu.ResourceName)
		}
		if nf.Spec.Gpu.Quantity.Value() <= 0 {
			return fmt.Errorf("invalid gpu quantity: %s", nf.Spec.Gpu.Quantity.String())
		}
	}
	if nf.Spec.RootDisk != nil {
		if nf.Spec.RootDisk.Count <= 0 || nf.Spec.RootDisk.Quantity.Value() <= 0 {
			return fmt.Errorf("invalid root disk: %v", *nf.Spec.RootDisk)
		}
	}
	if nf.Spec.DataDisk != nil {
		if nf.Spec.DataDisk.Count <= 0 || nf.Spec.DataDisk.Quantity.Value() <= 0 {
			return fmt.Errorf("invalid root disk: %v", *nf.Spec.DataDisk)
		}
	}
	ephemeralStorage, ok := nf.Spec.ExtendResources[corev1.ResourceEphemeralStorage]
	if ok && ephemeralStorage.Value() <= 0 {
		return fmt.Errorf("invalid %s: %v", corev1.ResourceEphemeralStorage, ephemeralStorage.String())
	}
	rdma, ok := nf.Spec.ExtendResources[corev1.ResourceName(commonconfig.GetRdmaName())]
	if ok && rdma.Value() <= 0 {
		return fmt.Errorf("invalid %s: %v", commonconfig.GetRdmaName(), rdma.String())
	}
	return nil
}

func (v *NodeFlavorValidator) validateImmutableFields(oldFlavor, newFlavor *v1.NodeFlavor) error {
	if (newFlavor.Spec.Gpu == nil && oldFlavor.Spec.Gpu != nil) ||
		(newFlavor.Spec.Gpu != nil && oldFlavor.Spec.Gpu == nil) {
		return field.Forbidden(field.NewPath("spec").Key("gpu"), "immutable")
	}
	if newFlavor.Spec.Gpu != nil && oldFlavor.Spec.Gpu != nil {
		if newFlavor.Spec.Gpu.ResourceName != oldFlavor.Spec.Gpu.ResourceName {
			return field.Forbidden(field.NewPath("spec").Key("gpu").Key("resourceName"), "immutable")
		}
		if newFlavor.Spec.Gpu.Quantity.Value() != oldFlavor.Spec.Gpu.Quantity.Value() {
			return field.Forbidden(field.NewPath("spec").Key("gpu").Key("quantity"), "immutable")
		}
		if newFlavor.Spec.Gpu.Product != oldFlavor.Spec.Gpu.Product {
			return field.Forbidden(field.NewPath("spec").Key("gpu").Key("product"), "immutable")
		}
	}
	return nil
}

func isValidGpuResource(name string) bool {
	for _, n := range GpuResourceWhiteList {
		if name == n {
			return true
		}
	}
	return false
}

func getNodeFlavor(ctx context.Context, cli client.Client, flavorId string) (*v1.NodeFlavor, error) {
	if flavorId == "" {
		return nil, nil
	}
	nf := &v1.NodeFlavor{}
	if err := cli.Get(ctx, client.ObjectKey{Name: flavorId}, nf); err != nil {
		return nil, err
	}
	return nf, nil
}
