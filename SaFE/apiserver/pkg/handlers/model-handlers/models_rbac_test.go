/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// adminModelUserID is the user id used by existing write-op tests; it is seeded
// as a system administrator so those tests keep passing under fail-closed RBAC.
const adminModelUserID = "u1"

var (
	adminModelACOnce sync.Once
	adminModelACInst *authority.AccessController
)

// adminModelAC returns a shared AccessController whose backing store contains a
// wildcard system-admin role bound to adminModelUserID. Existing model write
// tests use this via newMockModelHandler so their happy paths keep working.
// It is built with a direct struct (not authority.NewAccessController) to avoid
// the process-wide singleton created elsewhere in the test binary.
func adminModelAC() *authority.AccessController {
	adminModelACOnce.Do(func() {
		scheme := runtime.NewScheme()
		_ = v1.AddToScheme(scheme)
		role := &v1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: string(v1.SystemAdminRole)},
			Rules: []v1.PolicyRule{{
				Resources:    []string{authority.AllResource},
				GrantedUsers: []string{authority.GrantedAllUser},
				Verbs:        []v1.RoleVerb{v1.AllVerb},
			}},
		}
		user := &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: adminModelUserID},
			Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
		}
		adminModelACInst = &authority.AccessController{
			Client: ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(role, user).Build(),
		}
	})
	return adminModelACInst
}

// newModelOwnerAC builds an AccessController that grants model owners
// get/list/update/delete and workspace members create, mirroring the default
// role rules. Users owner-1 and other-1 both carry this role but neither is an
// administrator, so ownership is what decides access.
func newModelOwnerAC(t *testing.T) *authority.AccessController {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "model-role"},
		Rules: []v1.PolicyRule{
			{
				Resources:    []string{"model"},
				GrantedUsers: []string{authority.GrantedOwner},
				Verbs:        []v1.RoleVerb{v1.GetVerb, v1.ListVerb, v1.UpdateVerb, v1.DeleteVerb},
			},
			{
				Resources:    []string{"model"},
				GrantedUsers: []string{authority.GrantedWorkspaceUser},
				Verbs:        []v1.RoleVerb{v1.CreateVerb},
			},
		},
	}
	owner := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "owner-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{"model-role"}},
	}
	other := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "other-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{"model-role"}},
	}
	// wsmember-1 is a member of workspace "ws-1" (workspace-user), so it may
	// create models in ws-1 but is not an owner of existing models.
	wsMember := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "wsmember-1"},
		Spec: v1.UserSpec{
			Type:      v1.DefaultUserType,
			Roles:     []v1.UserRole{"model-role"},
			Resources: map[string][]string{common.UserWorkspaces: {"ws-1"}},
		},
	}
	return &authority.AccessController{
		Client: ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(role, owner, other, wsMember).Build(),
	}
}

func ownedModel(name, owner string) *v1.Model {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: name}}
	m.Labels = map[string]string{v1.UserIdLabel: owner}
	m.Spec.Source.AccessMode = v1.AccessModeRemoteAPI
	m.Status.Phase = v1.ModelPhaseReady
	return m
}

func deleteCtx(userID, modelID string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: modelID}}
	if userID != "" {
		c.Set(common.UserId, userID)
	}
	return c, w
}

// TestDeleteModelDeniedForNonOwner verifies S3: a user who is neither the owner
// nor an admin cannot delete another user's model.
func TestDeleteModelDeniedForNonOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	model := ownedModel("m-owned", "owner-1")
	k8s := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}

	c, _ := deleteCtx("other-1", "m-owned")
	_, err := h.deleteModel(c)
	if err == nil {
		t.Fatal("expected forbidden error for non-owner delete, got nil")
	}
	if code := getHTTPStatusCode(err); code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%v)", code, err)
	}
}

// TestDeleteModelAllowedForOwner verifies the owner can delete their own model.
func TestDeleteModelAllowedForOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	model := ownedModel("m-owned", "owner-1")
	k8s := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}

	c, _ := deleteCtx("owner-1", "m-owned")
	if _, err := h.deleteModel(c); err != nil {
		t.Fatalf("expected owner delete to succeed, got %v", err)
	}
}

// TestDeleteModelDeniedWhenNoAccessController verifies fail-closed behavior.
func TestDeleteModelDeniedWhenNoAccessController(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	model := ownedModel("m-owned", "owner-1")
	k8s := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s} // no access controller

	c, _ := deleteCtx("owner-1", "m-owned")
	if _, err := h.deleteModel(c); err == nil {
		t.Fatal("expected fail-closed error when access controller is nil, got nil")
	}
}

// TestRetryModelDeniedForNonOwner verifies that retry (a re-download, i.e. a
// state-changing write) is not allowed for a non-owner, matching patch/delete.
func TestRetryModelDeniedForNonOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	model := ownedModel("m-owned", "owner-1")
	model.Status.Phase = v1.ModelPhaseFailed
	k8s := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}

	c, _ := deleteCtx("other-1", "m-owned")
	_, err := h.retryModel(c)
	if err == nil {
		t.Fatal("expected forbidden error for non-owner retry, got nil")
	}
	if code := getHTTPStatusCode(err); code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%v)", code, err)
	}
}

func modelScheme2(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return s
}

// TestCreateModelDeniedForNonAdminPublicModel: creating a public model (empty
// workspace) is admin-only; a normal user must be denied.
func TestCreateModelDeniedForNonAdminPublicModel(t *testing.T) {
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme2(t)).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}
	c := sessCtx(t, http.MethodPost, `{"displayName":"P","source":{"accessMode":"local","url":"https://huggingface.co/x/y"}}`, "other-1", nil)
	_, err := h.createModel(c)
	if err == nil || getHTTPStatusCode(err) != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin public model create, got %v", err)
	}
}

// TestCreateModelDeniedForNonWorkspaceMember: creating in a workspace the user
// does not belong to must be denied.
func TestCreateModelDeniedForNonWorkspaceMember(t *testing.T) {
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme2(t)).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}
	c := sessCtx(t, http.MethodPost, `{"displayName":"P","workspace":"ws-1","source":{"accessMode":"local","url":"https://huggingface.co/x/y"}}`, "other-1", nil)
	_, err := h.createModel(c)
	if err == nil || getHTTPStatusCode(err) != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member workspace create, got %v", err)
	}
}

// TestCreateModelAllowedForWorkspaceMember: a workspace member may create a
// model in that workspace (authorization must not block it).
func TestCreateModelAllowedForWorkspaceMember(t *testing.T) {
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme2(t)).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}
	c := sessCtx(t, http.MethodPost, `{"displayName":"P","workspace":"ws-1","source":{"accessMode":"local","url":"https://huggingface.co/x/y"}}`, "wsmember-1", nil)
	_, err := h.createModel(c)
	if err != nil && getHTTPStatusCode(err) == http.StatusForbidden {
		t.Fatalf("workspace member create must not be forbidden, got %v", err)
	}
}

// TestPatchModelDeniedForNonOwner: patching another user's model must be denied.
func TestPatchModelDeniedForNonOwner(t *testing.T) {
	model := ownedModel("m-owned", "owner-1")
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme2(t)).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}
	c := sessCtx(t, http.MethodPatch, `{"displayName":"new"}`, "other-1", gin.Params{{Key: "id", Value: "m-owned"}})
	_, err := h.patchModel(c)
	if err == nil || getHTTPStatusCode(err) != http.StatusForbidden {
		t.Fatalf("expected 403 for non-owner patch, got %v", err)
	}
}

// TestPatchModelAllowedForOwner: the model owner may patch their own model.
func TestPatchModelAllowedForOwner(t *testing.T) {
	model := ownedModel("m-owned", "owner-1")
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme2(t)).WithObjects(model).Build()
	h := &Handler{k8sClient: k8s, accessController: newModelOwnerAC(t)}
	c := sessCtx(t, http.MethodPatch, `{"displayName":"new"}`, "owner-1", gin.Params{{Key: "id", Value: "m-owned"}})
	if _, err := h.patchModel(c); err != nil {
		t.Fatalf("owner patch must succeed, got %v", err)
	}
}
