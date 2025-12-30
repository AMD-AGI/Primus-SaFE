/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// AddResourceTemplateWebhook registers the resource template validation and mutation webhooks.
func AddResourceTemplateWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.ResourceTemplateKind), &webhook.Admission{Handler: &ResourceTemplateMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.ResourceTemplateKind), &webhook.Admission{Handler: &ResourceTemplateValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

// ResourceTemplateMutator handles mutation logic for ResourceTemplate resources.
type ResourceTemplateMutator struct {
	client.Client
	decoder admission.Decoder
}

// Handle processes resource template creation requests and applies normalizations.
func (m *ResourceTemplateMutator) Handle(_ context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}
	rt := &v1.ResourceTemplate{}
	if err := m.decoder.Decode(req, rt); err != nil {
		return handleError(v1.ResourceTemplateKind, err)
	}
	m.mutateOnCreation(rt)
	data, err := json.Marshal(rt)
	if err != nil {
		return handleError(v1.ResourceTemplateKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, data)
}

// mutateOnCreation applies default values and normalizations during creation.
func (m *ResourceTemplateMutator) mutateOnCreation(rt *v1.ResourceTemplate) {
	rt.Name = stringutil.NormalizeName(rt.Name)
}

// ResourceTemplateValidator validates ResourceTemplate resources on create and update operations.
type ResourceTemplateValidator struct {
	client.Client
	decoder admission.Decoder
}

// Handle validates resource template resources on create, update, and delete operations.
func (v *ResourceTemplateValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	rt := &v1.ResourceTemplate{}
	var err error
	switch req.Operation {
	case admissionv1.Create, admissionv1.Update:
		if err = v.decoder.Decode(req, rt); err != nil {
			break
		}
		if !rt.GetDeletionTimestamp().IsZero() {
			break
		}
		err = v.validate(rt)
	default:
	}
	if err != nil {
		return handleError(v1.ResourceTemplateKind, err)
	}
	return admission.Allowed("")
}

// validate ensures the resource template has valid spec configuration.
func (v *ResourceTemplateValidator) validate(rt *v1.ResourceTemplate) error {
	if rt.Spec.GroupVersionKind.Kind == "" {
		return fmt.Errorf("invalid kind")
	}
	if rt.Spec.GroupVersionKind.Version == "" {
		return fmt.Errorf("invalid version")
	}
	return nil
}
