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

// AddUserWebhook registers the user validation and mutation webhooks.
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

// UserMutator handles mutation logic for User resources.
type UserMutator struct {
	client.Client
	decoder admission.Decoder
}

// Handle processes user creation requests and applies default values and normalizations.
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

// mutateOnCreation applies default values and normalizations during creation.
func (m *UserMutator) mutateOnCreation(ctx context.Context, user *v1.User) {
	m.mutateMetadata(user)
	m.mutateCommon(ctx, user)
	m.mutateDefaultWorkspace(ctx, user)
	m.mutateManagedWorkspaces(ctx, user, true)
}

// mutateOnUpdate applies mutations during updates.
func (m *UserMutator) mutateOnUpdate(ctx context.Context, user *v1.User) {
	m.mutateCommon(ctx, user)
	m.mutateManagedWorkspaces(ctx, user, false)
}

// mutateCommon applies mutations to the resource.
func (m *UserMutator) mutateCommon(ctx context.Context, user *v1.User) {
	m.mutateRoles(user)
	m.mutateWorkspace(ctx, user)
	m.mutateLabels(user)
}

// mutateMetadata applies mutations to the resource.
func (m *UserMutator) mutateMetadata(user *v1.User) {
	if user.Spec.Type == "" {
		user.Spec.Type = v1.DefaultUserType
	}
	metav1.SetMetaDataLabel(&user.ObjectMeta, v1.UserIdLabel, user.Name)
}

// mutateLabels updates user label hashes and the user type label.
func (m *UserMutator) mutateLabels(user *v1.User) {
	if val := v1.GetUserEmail(user); val != "" {
		v1.SetLabel(user, v1.UserEmailMd5Label, stringutil.MD5(val))
	} else {
		v1.RemoveLabel(user, v1.UserEmailMd5Label)
	}
	if val := v1.GetUserName(user); val != "" {
		v1.SetLabel(user, v1.UserNameMd5Label, stringutil.MD5(val))
	} else {
		v1.RemoveLabel(user, v1.UserNameMd5Label)
	}
	v1.SetLabel(user, v1.UserTypeLabel, string(user.Spec.Type))
}

// mutateRoles handles user role mutations including:
// 1. System admin users only keep SystemAdminRole
// 2. Remove duplicate roles
// 3. Filter out WorkspaceAdminRole (not allowed)
// 4. Add DefaultRole for non-system admins if missing
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

// mutateWorkspace removes duplicate and non-existent workspaces from the user's workspace list.
// It ensures that each workspace appears only once and that all workspaces actually exist.
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

// mutateDefaultWorkspace adds default workspaces to users' workspace lists.
// It ensures all users have access to workspaces marked as default (IsDefault=true).
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

// mutateManagedWorkspaces filters and validates managed workspaces for a user.
// It removes duplicates, checks access rights if required, and ensures workspaces exist.
func (m *UserMutator) mutateManagedWorkspaces(ctx context.Context, user *v1.User, isCheckAccessRight bool) {
	workspaceSet := sets.NewSet()
	allWorkspaces := commonuser.GetManagedWorkspace(user)
	validWorkspaces := make([]string, 0, len(allWorkspaces))
	for _, w := range allWorkspaces {
		if workspaceSet.Has(w) {
			continue
		}
		workspaceSet.Insert(w)
		if isCheckAccessRight && !commonuser.HasWorkspaceRight(user, w) {
			continue
		}
		if _, err := getWorkspace(ctx, m.Client, w); err == nil {
			validWorkspaces = append(validWorkspaces, w)
		}
	}
	commonuser.AssignManagedWorkspace(user, validWorkspaces...)
}

// UserValidator validates User resources on create and update operations.
type UserValidator struct {
	client.Client
	decoder admission.Decoder
}

// Handle validates user resources on create, update, and delete operations.
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

// validateOnCreation validates user metadata and spec on creation.
func (v *UserValidator) validateOnCreation(ctx context.Context, user *v1.User) error {
	if err := v.validateMetadata(user); err != nil {
		return err
	}
	if err := v.validateCommon(ctx, user); err != nil {
		return err
	}
	return nil
}

// validateOnUpdate validates immutable fields and common spec on update.
func (v *UserValidator) validateOnUpdate(ctx context.Context, newUser, oldUser *v1.User) error {
	if err := v.validateImmutableFields(newUser, oldUser); err != nil {
		return err
	}
	if err := v.validateCommon(ctx, newUser); err != nil {
		return err
	}
	if err := v.validateAccessRemoved(newUser, oldUser); err != nil {
		return err
	}
	return nil
}

// validateCommon validates required parameters and role references.
func (v *UserValidator) validateCommon(ctx context.Context, user *v1.User) error {
	if err := v.validateRequiredParams(user); err != nil {
		return err
	}
	if err := v.validateRoles(ctx, user); err != nil {
		return err
	}
	return nil
}

// validateMetadata ensures username is not a reserved word like "self" or "system".
func (v *UserValidator) validateMetadata(user *v1.User) error {
	// "self"/"system" is reserved word
	if user.Name == common.UserSelf || user.Name == common.UserSystem {
		return commonerrors.NewForbidden(
			fmt.Sprintf("%s is a system reserved word that cannot be used", user.Name))
	}
	return nil
}

// validateImmutableFields ensures user type and username cannot be modified.
func (v *UserValidator) validateImmutableFields(newUser, oldUser *v1.User) error {
	if newUser.Spec.Type != oldUser.Spec.Type {
		return field.Forbidden(field.NewPath("spec").Key("type"), "immutable")
	}
	if newUser.Spec.Type == v1.DefaultUserType && v1.GetUserName(newUser) != v1.GetUserName(oldUser) {
		return field.Forbidden(field.NewPath("user").Key("name"), "immutable")
	}
	return nil
}

// validateRequiredParams ensures user type and roles are not empty.
func (v *UserValidator) validateRequiredParams(user *v1.User) error {
	var errs []error
	if user.Spec.Type != v1.DefaultUserType && user.Spec.Type != v1.SSOUserType {
		errs = append(errs, fmt.Errorf("the user's type is not supported"))
	}
	if len(user.Spec.Roles) == 0 {
		errs = append(errs, fmt.Errorf("the user's roles is empty"))
	}
	if user.Spec.Type == v1.DefaultUserType && user.Spec.Password == "" {
		errs = append(errs, fmt.Errorf("the user's password is empty"))
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

// validateRoles ensures all referenced roles exist.
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

// validateAccessRemoved ensures that a user's management permissions are removed before revoking their access to a workspace.
// Returns an error if the user still manages a workspace that they no longer have access to.
func (v *UserValidator) validateAccessRemoved(newUser, oldUser *v1.User) error {
	newUserAccessibleSet := sets.NewSetByKeys(commonuser.GetWorkspace(newUser)...)
	newUserManagedSet := sets.NewSetByKeys(commonuser.GetManagedWorkspace(newUser)...)
	oldUserAccessibleList := commonuser.GetWorkspace(oldUser)
	for _, w := range oldUserAccessibleList {
		if !newUserAccessibleSet.Has(w) && newUserManagedSet.Has(w) {
			return commonerrors.NewForbidden(fmt.Sprintf("Please remove the user's workspace(%s) management first.", w))
		}
	}
	return nil
}

// getUser retrieves the requested information.
func getUser(ctx context.Context, cli client.Client, userId string) (*v1.User, error) {
	if userId == "" {
		return nil, fmt.Errorf("userId is empty")
	}
	user := &v1.User{}
	if err := cli.Get(ctx, client.ObjectKey{Name: userId}, user); err != nil {
		return nil, err
	}
	return user, nil
}
