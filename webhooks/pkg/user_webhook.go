/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	sliceutil "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func AddUserWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.UserKind), &webhook.Admission{Handler: &UserMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.UserKind), &webhook.Admission{Handler: &UserValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

// UserMutator works when create/update
type UserMutator struct {
	client.Client
	decoder admission.Decoder
}

func (m *UserMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation == admissionv1.Delete {
		return admission.Allowed("")
	}
	obj := &v1.User{}
	if err := m.decoder.Decode(req, obj); err != nil {
		return handleError(v1.UserKind, err)
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("")
	}
	switch req.Operation {
	case admissionv1.Create:
		m.mutateOnCreation(ctx, obj)
	case admissionv1.Update:
		m.mutateOnUpdate(ctx, obj)
	}
	marshaledResult, err := json.Marshal(obj)
	if err != nil {
		return handleError(v1.UserKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledResult)
}

func (m *UserMutator) mutateOnCreation(ctx context.Context, user *v1.User) {
	m.mutateMetadata(user)
	m.mutateCommon(ctx, user)
	m.mutateDefaultWorkspace(ctx, user)
}

func (m *UserMutator) mutateOnUpdate(ctx context.Context, user *v1.User) {
	m.mutateCommon(ctx, user)
}

func (m *UserMutator) mutateCommon(ctx context.Context, user *v1.User) {
	m.mutateRoles(user)
	m.mutateWorkspace(ctx, user)
	m.mutateManagedWorkspace(ctx, user)
}

func (m *UserMutator) mutateMetadata(user *v1.User) {
	if val := v1.GetUserEmail(user); val != "" {
		v1.SetLabel(user, v1.UserEmailMd5Label, stringutil.MD5(val))
	}
	if user.Spec.Type == "" {
		user.Spec.Type = v1.DefaultUser
	}
	metav1.SetMetaDataLabel(&user.ObjectMeta, v1.UserIdLabel, user.Name)
}

func (m *UserMutator) mutateRoles(user *v1.User) {
	switch {
	case user.IsSystemAdmin() && len(user.Spec.Roles) > 1:
		user.Spec.Roles = []v1.UserRole{v1.SystemAdminRole}
	default:
		roleSet := sets.NewSet()
		newRoles := make([]v1.UserRole, 0, len(user.Spec.Roles))
		for i, r := range user.Spec.Roles {
			if roleSet.Has(string(r)) || r == v1.WorkspaceAdminRole {
				continue
			}
			newRoles = append(newRoles, user.Spec.Roles[i])
			roleSet.Insert(string(r))
		}
		if len(newRoles) != len(user.Spec.Roles) {
			user.Spec.Roles = newRoles
		}
	}
	if !user.IsSystemAdmin() && !v1.IsContainRole(user.Spec.Roles, v1.DefaultRole) {
		user.Spec.Roles = append(user.Spec.Roles, v1.DefaultRole)
	}
}

func (m *UserMutator) mutateWorkspace(ctx context.Context, user *v1.User) {
	workspaceSet := sets.NewSet()
	allWorkspaces := commonuser.GetWorkspace(user)
	var workspaces []string
	for _, w := range allWorkspaces {
		if workspaceSet.Has(w) {
			continue
		}
		workspaceSet.Insert(w)
		if _, err := getWorkspace(ctx, m.Client, w); err == nil {
			workspaces = append(workspaces, w)
		}
	}
	if len(allWorkspaces) != len(workspaces) {
		commonuser.AssignWorkspace(user, workspaces...)
	}
}

func (m *UserMutator) mutateDefaultWorkspace(ctx context.Context, user *v1.User) {
	workspaceList := &v1.WorkspaceList{}
	err := m.List(ctx, workspaceList)
	if err != nil {
		return
	}
	userWorkspaces := commonuser.GetWorkspace(user)
	for _, w := range workspaceList.Items {
		if !w.Spec.IsDefault {
			continue
		}
		if sliceutil.Contains(userWorkspaces, w.Name) {
			continue
		}
		userWorkspaces = append(userWorkspaces, w.Name)
	}
	commonuser.AssignWorkspace(user, userWorkspaces...)
}

func (m *UserMutator) mutateManagedWorkspace(ctx context.Context, user *v1.User) {
	workspaceSet := sets.NewSet()
	allWorkspaces := commonuser.GetManagedWorkspace(user)
	var workspaces []string
	for _, w := range allWorkspaces {
		if workspaceSet.Has(w) {
			continue
		}
		workspaceSet.Insert(w)
		if !commonuser.HasWorkspaceRight(user, w) {
			continue
		}
		if _, err := getWorkspace(ctx, m.Client, w); err == nil {
			workspaces = append(workspaces, w)
		}
	}
	commonuser.AssignManagedWorkspace(user, workspaces...)
}

// UserValidator works when create/update
type UserValidator struct {
	client.Client
	decoder admission.Decoder
}

func (v *UserValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	var err error
	obj := &v1.User{}
	if err = v.decoder.Decode(req, obj); err != nil {
		return handleError(v1.UserKind, err)
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("")
	}

	switch req.Operation {
	case admissionv1.Create:
		err = v.validateOnCreation(ctx, obj)
	case admissionv1.Update:
		oldObj := &v1.User{}
		if err = v.decoder.DecodeRaw(req.OldObject, oldObj); err == nil {
			err = v.validateOnUpdate(ctx, obj, oldObj)
		}
	default:
	}
	if err != nil {
		return handleError(v1.UserKind, err)
	}
	return admission.Allowed("")
}

func (v *UserValidator) validateOnCreation(ctx context.Context, user *v1.User) error {
	if err := v.validateMetadata(user); err != nil {
		return err
	}
	if err := v.validateCommon(ctx, user); err != nil {
		return err
	}
	return nil
}

func (v *UserValidator) validateOnUpdate(ctx context.Context, newUser, oldUser *v1.User) error {
	if err := v.validateImmutableFields(newUser, oldUser); err != nil {
		return err
	}
	if err := v.validateCommon(ctx, newUser); err != nil {
		return err
	}
	return nil
}

func (v *UserValidator) validateCommon(ctx context.Context, user *v1.User) error {
	if err := v.validateRequiredParams(user); err != nil {
		return err
	}
	if err := v.validateRoles(ctx, user); err != nil {
		return err
	}
	return nil
}

func (v *UserValidator) validateMetadata(user *v1.User) error {
	// "self"/"system" is reserved word
	if user.Name == common.UserSelf || user.Name == common.UserSystem {
		return commonerrors.NewForbidden(
			fmt.Sprintf("%s is a system reserved word that cannot be used", user.Name))
	}
	return nil
}

func (v *UserValidator) validateImmutableFields(newUser, oldUser *v1.User) error {
	if newUser.Spec.Type != oldUser.Spec.Type {
		return field.Forbidden(field.NewPath("spec").Key("type"), "immutable")
	}
	if v1.GetUserName(newUser) != v1.GetUserName(oldUser) {
		return field.Forbidden(field.NewPath("user").Key("name"), "immutable")
	}
	return nil
}

func (v *UserValidator) validateRequiredParams(user *v1.User) error {
	var errs []error
	if user.Spec.Type == "" {
		errs = append(errs, fmt.Errorf("the user's type is empty"))
	}
	if len(user.Spec.Roles) == 0 {
		errs = append(errs, fmt.Errorf("the user's roles is empty"))
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

func (v *UserValidator) validateRoles(ctx context.Context, user *v1.User) error {
	for _, r := range user.Spec.Roles {
		role := &v1.Role{}
		err := v.Get(ctx, client.ObjectKey{Name: string(r)}, role)
		if err != nil {
			return err
		}
	}
	return nil
}

func getUser(ctx context.Context, cli client.Client, name string) (*v1.User, error) {
	if name == "" {
		return nil, fmt.Errorf("userId is empty")
	}
	user := &v1.User{}
	if err := cli.Get(ctx, client.ObjectKey{Name: name}, user); err != nil {
		return nil, err
	}
	return user, nil
}
