/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

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

type ResourceTemplateMutator struct {
	client.Client
	decoder admission.Decoder
}

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

func (m *ResourceTemplateMutator) mutateOnCreation(rt *v1.ResourceTemplate) {
	rt.Name = stringutil.NormalizeName(rt.Name)
}

type ResourceTemplateValidator struct {
	client.Client
	decoder admission.Decoder
}

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

func (v *ResourceTemplateValidator) validate(rt *v1.ResourceTemplate) error {
	if err := v.validateTemplate(rt); err != nil {
		return err
	}
	return nil
}

func (v *ResourceTemplateValidator) validateTemplate(rt *v1.ResourceTemplate) error {
	if len(rt.Spec.ResourceSpecs) <= 1 {
		return nil
	}
	count := 0
	for _, template := range rt.Spec.ResourceSpecs {
		if template.Replica > 0 {
			count++
		}
	}
	if count < len(rt.Spec.ResourceSpecs)-1 {
		return commonerrors.NewInternalError("If more than one template is defined, only one can have a empty replica field")
	}
	return nil
}

func getResourceTemplate(ctx context.Context, cli client.Client, gvk v1.GroupVersionKind) (*v1.ResourceTemplate, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{
		v1.WorkloadKindLabel: gvk.Kind, v1.WorkloadVersionLabel: gvk.Version})
	rtl := &v1.ResourceTemplateList{}
	err := cli.List(ctx, rtl, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	if len(rtl.Items) == 0 {
		return nil, commonerrors.NewNotFound(v1.ResourceTemplateKind, gvk.VersionKind())
	}
	return &rtl.Items[0], nil
}
