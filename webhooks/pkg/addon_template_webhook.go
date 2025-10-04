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
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

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

type AddOnTemplateMutator struct {
	client.Client
	decoder admission.Decoder
}

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

func (m *AddOnTemplateMutator) mutateOnCreation(addon *v1.AddonTemplate) {
	addon.Name = stringutil.NormalizeName(addon.Name)
	if addon.Spec.Type == "" {
		addon.Spec.Type = v1.AddonTemplateDefault
	}
	addon.Spec.Action = strings.Trim(addon.Spec.Action, " ")
}

type AddOnTemplateValidator struct {
	client.Client
	decoder admission.Decoder
}

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

func (v *AddOnTemplateValidator) validate(addon *v1.AddonTemplate) error {
	if err := v.validateRequiredParams(addon); err != nil {
		return err
	}
	if err := validateDisplayName(v1.GetDisplayName(addon)); err != nil {
		return err
	}
	return nil
}

func (v *AddOnTemplateValidator) validateRequiredParams(addon *v1.AddonTemplate) error {
	switch addon.Spec.Type {
	case v1.AddonTemplateDefault:
		if addon.Spec.Action == "" {
			return commonerrors.NewBadRequest(fmt.Sprintf("the action of spec is empty"))
		}
	case v1.AddonTemplateHelm:
	default:
		return commonerrors.NewBadRequest("invalid addon-template type")
	}
	return nil
}
