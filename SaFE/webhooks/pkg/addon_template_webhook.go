/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// AddAddOnTemplateWebhook registers the addon template validation and mutation webhooks.
func AddAddOnTemplateWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.AddOnTemplateKind), &webhook.Admission{Handler: &AddOnTemplateMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.AddOnTemplateKind), &webhook.Admission{Handler: &AddOnTemplateValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

// AddOnTemplateMutator handles mutation logic for AddonTemplate resources on creation.
type AddOnTemplateMutator struct {
	client.Client
	decoder admission.Decoder
}

// Handle processes addon template creation requests and applies default values and normalizations.
func (m *AddOnTemplateMutator) Handle(_ context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}
	addon := &v1.AddonTemplate{}
	if err := m.decoder.Decode(req, addon); err != nil {
		return handleError(v1.AddOnTemplateKind, err)
	}
	m.mutateOnCreation(addon)
	data, err := json.Marshal(addon)
	if err != nil {
		return handleError(v1.AddOnTemplateKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, data)
}

// mutateOnCreation applies default values and normalizations to the addon template during creation.
func (m *AddOnTemplateMutator) mutateOnCreation(addon *v1.AddonTemplate) {
	addon.Name = stringutil.NormalizeName(addon.Name)
	if addon.Spec.Type == "" {
		addon.Spec.Type = v1.AddonTemplateDefault
	}
	addon.Spec.Action = strings.Trim(addon.Spec.Action, " ")
}

// AddOnTemplateValidator validates AddonTemplate resources on create and update operations.
type AddOnTemplateValidator struct {
	client.Client
	decoder admission.Decoder
}

// Handle validates addon template resources on create and update operations.
func (v *AddOnTemplateValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	addon := &v1.AddonTemplate{}
	var err error
	switch req.Operation {
	case admissionv1.Create, admissionv1.Update:
		if err = v.decoder.Decode(req, addon); err != nil {
			break
		}
		if !addon.GetDeletionTimestamp().IsZero() {
			break
		}
		err = v.validate(addon)
	default:
	}
	if err != nil {
		return handleError(v1.AddOnTemplateKind, err)
	}
	return admission.Allowed("")
}

// validate validates the addon template's required fields and display name.
func (v *AddOnTemplateValidator) validate(addon *v1.AddonTemplate) error {
	if err := v.validateRequiredParams(addon); err != nil {
		return err
	}
	if err := validateDisplayName(v1.GetDisplayName(addon), ""); err != nil {
		return err
	}
	return nil
}

// validateRequiredParams validates that required parameters are set based on the template type.
func (v *AddOnTemplateValidator) validateRequiredParams(addon *v1.AddonTemplate) error {
	switch addon.Spec.Type {
	case v1.AddonTemplateDefault:
		if addon.Spec.Action == "" {
			return commonerrors.NewBadRequest("the action of spec is empty")
		}
	case v1.AddonTemplateHelm:
	default:
		return commonerrors.NewBadRequest("invalid addon-template type")
	}
	return nil
}
