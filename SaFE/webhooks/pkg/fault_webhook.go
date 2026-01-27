/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// AddFaultWebhook registers the fault validation and mutation webhooks.
func AddFaultWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.FaultKind), &webhook.Admission{Handler: &FaultMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.FaultKind), &webhook.Admission{Handler: &FaultValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

// FaultMutator handles mutation logic for Fault resources.
type FaultMutator struct {
	client.Client
	decoder admission.Decoder
}

// Handle processes fault creation requests and applies default values and normalizations.
func (m *FaultMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}

	fault := &v1.Fault{}
	if err := m.decoder.Decode(req, fault); err != nil {
		return handleError(v1.FaultKind, err)
	}
	m.mutateOnCreation(ctx, fault)
	data, err := json.Marshal(fault)
	if err != nil {
		return handleError(v1.FaultKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, data)
}

// mutateOnCreation applies default values and normalizations during creation.
func (m *FaultMutator) mutateOnCreation(ctx context.Context, fault *v1.Fault) {
	fault.Name = stringutil.NormalizeName(fault.Name)
	v1.SetLabel(fault, v1.ClusterIdLabel, fault.Spec.Node.ClusterName)
	v1.SetLabel(fault, v1.FaultMonitorId, fault.Spec.MonitorId)
	controllerutil.AddFinalizer(fault, v1.FaultFinalizer)

	if fault.Spec.Node != nil {
		if v1.GetNodeId(fault) == "" {
			v1.SetLabel(fault, v1.NodeIdLabel, fault.Spec.Node.AdminName)
		}
		if len(fault.OwnerReferences) == 0 {
			adminNodeName := fault.Spec.Node.AdminName
			if node, _ := getNode(ctx, m.Client, adminNodeName); node != nil {
				if err := controllerutil.SetControllerReference(node, fault, m.Client.Scheme()); err != nil {
					klog.ErrorS(err, "failed to SetControllerReference")
				}
			}
		}
	}
}

// FaultValidator validates Fault resources on create and update operations.
type FaultValidator struct {
	client.Client
	decoder admission.Decoder
}

// Handle validates fault resources on create, update, and delete operations.
func (v *FaultValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	fault := &v1.Fault{}
	var err error
	switch req.Operation {
	case admissionv1.Create:
		if err = v.decoder.Decode(req, fault); err != nil {
			break
		}
		err = v.validateOnCreation(fault)
	case admissionv1.Update:
		if err = v.decoder.Decode(req, fault); err != nil {
			break
		}
		if !fault.GetDeletionTimestamp().IsZero() {
			break
		}
		oldFault := &v1.Fault{}
		if err = v.decoder.DecodeRaw(req.OldObject, oldFault); err == nil {
			err = v.validateOnUpdate(fault, oldFault)
		}
	default:
	}
	if err != nil {
		return handleError(v1.FaultKind, err)
	}
	return admission.Allowed("")
}

// validateOnCreation validates fault spec and display name on creation.
func (v *FaultValidator) validateOnCreation(fault *v1.Fault) error {
	if err := v.validateFaultSpec(fault); err != nil {
		return err
	}
	if err := validateDisplayName(v1.GetDisplayName(fault), ""); err != nil {
		return err
	}
	return nil
}

// validateOnUpdate validates fault spec on update.
func (v *FaultValidator) validateOnUpdate(newFault, _ *v1.Fault) error {
	if err := v.validateFaultSpec(newFault); err != nil {
		return err
	}
	return nil
}

// validateFaultSpec validates monitor ID and node configuration in fault spec.
func (v *FaultValidator) validateFaultSpec(fault *v1.Fault) error {
	if fault.Spec.MonitorId == "" {
		return fmt.Errorf("the id of spec is empty")
	}
	if fault.Spec.Node != nil {
		if fault.Spec.Node.ClusterName == "" || v1.GetClusterId(fault) == "" {
			return fmt.Errorf("the cluster of spec is empty")
		}
		if fault.Spec.Node.AdminName == "" {
			return fmt.Errorf("the admin node of spec is empty")
		}
		if fault.Spec.Node.K8sName == "" {
			return fmt.Errorf("the k8s node of spec is empty")
		}
	}
	return nil
}
