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
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// AddRoleWebhook registers the role validation and mutation webhooks.
func AddRoleWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.RoleKind), &webhook.Admission{Handler: &RoleMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.RoleKind), &webhook.Admission{Handler: &RoleValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

// RoleMutator handles mutation logic for Role resources.
type RoleMutator struct {
	client.Client
	decoder admission.Decoder
}

// Handle processes role creation requests and applies normalizations.
func (m *RoleMutator) Handle(_ context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}
	role := &v1.Role{}
	if err := m.decoder.Decode(req, role); err != nil {
		return handleError(v1.RoleKind, err)
	}
	m.mutateOnCreation(role)
	data, err := json.Marshal(role)
	if err != nil {
		return handleError(v1.RoleKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, data)
}

// mutateOnCreation applies default values and normalizations during creation.
func (m *RoleMutator) mutateOnCreation(role *v1.Role) {
	role.Name = stringutil.NormalizeName(role.Name)
	for i := range role.Rules {
		for j := range role.Rules[i].Resources {
			role.Rules[i].Resources[j] = strings.ToLower(role.Rules[i].Resources[j])
		}
	}
}

// RoleValidator validates Role resources on create and update operations.
type RoleValidator struct {
	client.Client
	decoder admission.Decoder
}

// Handle validates role resources on create, update, and delete operations.
func (v *RoleValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	role := &v1.Role{}
	var err error
	switch req.Operation {
	case admissionv1.Create, admissionv1.Update:
		if err = v.decoder.Decode(req, role); err != nil {
			break
		}
		if !role.GetDeletionTimestamp().IsZero() {
			break
		}
		err = v.validate(role)
	default:
	}
	if err != nil {
		return handleError(v1.RoleKind, err)
	}
	return admission.Allowed("")
}

// validate ensures role has valid rules with non-empty resources and verbs.
func (v *RoleValidator) validate(role *v1.Role) error {
	if len(role.Rules) == 0 {
		return fmt.Errorf("invalid rules of role")
	}
	for i := range role.Rules {
		if len(role.Rules[i].Resources) == 0 {
			return fmt.Errorf("invalid resources of role")
		}
		if len(role.Rules[i].Verbs) == 0 {
			return fmt.Errorf("invalid verbs of role")
		}
	}
	return nil
}
