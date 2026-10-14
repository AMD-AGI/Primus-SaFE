/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
	testifyassert "github.com/stretchr/testify/assert"
	testifyrequire "github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonsecret "github.com/AMD-AIG-AIMA/SAFE/common/pkg/secret"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const (
	sandboxEnvSecret = commonsecret.SandboxEnvSecretName
	// sandboxEnvRefusal is the phrase only authorizeSandboxEnvSecret uses, so a refusal by the
	// ordinary role check cannot pass for one by the guard.
	sandboxEnvRefusal = "read by sandboxes as environment variables"
)

// chartRoles reads the roles the chart installs, so the members below have exactly the
// permissions a deployment gives them rather than ones written for this test.
func chartRoles(t *testing.T) []client.Object {
	t.Helper()
	raw, err := os.ReadFile("../../../../charts/primus-safe-cr/templates/user/role.yaml")
	testifyrequire.NoError(t, err)
	raw = regexp.MustCompile(`\{\{[^}]*\}\}`).ReplaceAll(raw, []byte("release"))
	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(raw), 4096)
	var roles []client.Object
	for {
		role := &v1.Role{}
		if err := decoder.Decode(role); err != nil {
			break
		}
		if role.Name != "" {
			roles = append(roles, role)
		}
	}
	names := map[string]bool{}
	for _, r := range roles {
		names[r.GetName()] = true
	}
	for _, want := range []v1.UserRole{v1.SystemAdminRole, v1.DefaultRole, v1.WorkspaceAdminRole} {
		testifyrequire.True(t, names[string(want)], "role %s not found in the chart", want)
	}
	return roles
}

func sandboxEnvUser(name string, roles []v1.UserRole, workspaces, managed []string) *v1.User {
	user := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Resources: map[string][]string{}},
	}
	if len(workspaces) > 0 {
		user.Spec.Resources[common.UserWorkspaces] = workspaces
	}
	if len(managed) > 0 {
		user.Spec.Resources[common.UserManagedWorkspaces] = managed
	}
	return user
}

type sandboxEnvUsers struct {
	member, wsAdmin, sysAdmin *v1.User
}

func newSandboxEnvHandler(t *testing.T, secrets ...*corev1.Secret) (*Handler, sandboxEnvUsers) {
	t.Helper()
	return newSandboxEnvHandlerWith(t, interceptor.Funcs{}, secrets...)
}

func newSandboxEnvHandlerWith(t *testing.T, funcs interceptor.Funcs, secrets ...*corev1.Secret) (*Handler, sandboxEnvUsers) {
	t.Helper()
	users := sandboxEnvUsers{
		member:   sandboxEnvUser("member", []v1.UserRole{v1.DefaultRole}, []string{"ws-a", "ws-b"}, nil),
		wsAdmin:  sandboxEnvUser("ws-admin", []v1.UserRole{v1.DefaultRole}, []string{"ws-a", "ws-b"}, []string{"ws-a"}),
		sysAdmin: sandboxEnvUser("sys-admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil),
	}
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	objs := append(chartRoles(t), users.member, users.wsAdmin, users.sysAdmin)
	var k8sObjs []runtime.Object
	for _, s := range secrets {
		objs = append(objs, s.DeepCopy())
		k8sObjs = append(k8sObjs, s.DeepCopy())
	}
	cli := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(funcs).Build()
	// Not authority.NewAccessController: it is a process-wide singleton bound to the first client.
	return &Handler{Client: cli, clientSet: k8sfake.NewSimpleClientset(k8sObjs...),
		accessController: &authority.AccessController{Client: cli}}, users
}

func sandboxEnvCreate(name string, workspaces ...string) *view.CreateSecretRequest {
	return &view.CreateSecretRequest{Name: name, Type: v1.SecretGeneral, WorkspaceIds: workspaces,
		Params: []map[view.SecretParam]string{{view.SecretParam("LD_PRELOAD"): b64("/tmp/x.so")}}}
}

// ownedSecret is a general Secret the member created and owns, bound to ws-a: the shape a
// member's Secret of that name would have if it predates the guard.
func ownedSecret(name, owner string, workspaces ...string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{v1.SecretTypeLabel: string(v1.SecretGeneral), v1.UserIdLabel: owner},
			Annotations: map[string]string{
				v1.WorkspaceIdsAnnotation: string(jsonutils.MarshalSilently(workspaces))}},
		Data: map[string][]byte{"HF_TOKEN": []byte("admin")},
	}
}

func assertGuardRefused(t *testing.T, err error) {
	t.Helper()
	testifyrequire.Error(t, err)
	testifyassert.True(t, commonerrors.IsForbidden(err), "not a 403: %v", err)
	testifyassert.Contains(t, err.Error(), sandboxEnvRefusal)
}

func TestMemberCannotCreateTheSandboxEnvSecret(t *testing.T) {
	h, users := newSandboxEnvHandler(t)
	ctx := context.Background()

	// The role lets a member create a Secret bound to their workspace...
	_, err := h.createSecretImpl(ctx, sandboxEnvCreate("other-name", "ws-a"), users.member)
	testifyrequire.NoError(t, err)
	// ...but not one with the reserved name, and nothing is created.
	_, err = h.createSecretImpl(ctx, sandboxEnvCreate(sandboxEnvSecret, "ws-a"), users.member)
	assertGuardRefused(t, err)
	_, err = h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, sandboxEnvSecret, metav1.GetOptions{})
	testifyassert.True(t, apierrors.IsNotFound(err), "the refused Secret exists: %v", err)

	// An admin of some but not all of the workspaces is refused too.
	_, err = h.createSecretImpl(ctx, sandboxEnvCreate(sandboxEnvSecret, "ws-a", "ws-b"), users.wsAdmin)
	assertGuardRefused(t, err)
	// An admin of every workspace it is bound to may create it.
	_, err = h.createSecretImpl(ctx, sandboxEnvCreate(sandboxEnvSecret, "ws-a"), users.wsAdmin)
	testifyrequire.NoError(t, err)
}

func TestSystemAdminCanCreateTheSandboxEnvSecret(t *testing.T) {
	h, users := newSandboxEnvHandler(t)
	_, err := h.createSecretImpl(context.Background(), sandboxEnvCreate(sandboxEnvSecret, "ws-a", "ws-b"), users.sysAdmin)
	testifyrequire.NoError(t, err)
}

// The name is reserved with no configuration at all: that is the state of an apiserver pod
// still running while the chart turns the sandbox template on, so it must already refuse.
func TestSandboxEnvSecretIsReservedWithoutAnyConfig(t *testing.T) {
	testifyrequire.Equal(t, "sandbox-env", sandboxEnvSecret)
	h, users := newSandboxEnvHandler(t)
	ctx := context.Background()
	_, err := h.createSecretImpl(ctx, sandboxEnvCreate("site-sandbox-vars", "ws-a"), users.member)
	testifyrequire.NoError(t, err)
	_, err = h.createSecretImpl(ctx, sandboxEnvCreate(sandboxEnvSecret, "ws-a"), users.member)
	assertGuardRefused(t, err)
}

// An internal caller with no user is not an admin.
func TestSandboxEnvSecretGuardRefusesANilUser(t *testing.T) {
	assertGuardRefused(t, authorizeSandboxEnvSecret(nil, sandboxEnvSecret, []string{"ws-a"}, nil))
	testifyassert.NoError(t, authorizeSandboxEnvSecret(nil, "other-name", []string{"ws-a"}, nil))
	h, _ := newSandboxEnvHandler(t, func() *corev1.Secret {
		s := ownedSecret(sandboxEnvSecret, "sys-admin", "ws-a")
		s.Labels[v1.SecretTypeLabel] = string(v1.SecretImage)
		return s
	}())
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-b"}}
	assertGuardRefused(t, h.updateWorkspaceImageSecrets(context.Background(), ws, nil, []string{sandboxEnvSecret}))
}

func TestMemberCannotUpdateTheSandboxEnvSecret(t *testing.T) {
	h, users := newSandboxEnvHandler(t,
		ownedSecret(sandboxEnvSecret, "member", "ws-a"), ownedSecret("other-name", "member", "ws-a"))
	params := []map[view.SecretParam]string{{view.SecretParam("LD_PRELOAD"): b64("/tmp/x.so")}}
	patch := func(h *Handler, user *v1.User, name string, req view.PatchSecretRequest) error {
		c := newPostCtx(jsonutils.MarshalSilently(req))
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, name)
		_, err := h.patchSecret(c)
		return err
	}

	// The owner may change a Secret of any other name...
	testifyrequire.NoError(t, patch(h, users.member, "other-name", view.PatchSecretRequest{Params: &params}))
	// ...but not the reserved one, even their own.
	assertGuardRefused(t, patch(h, users.member, sandboxEnvSecret, view.PatchSecretRequest{Params: &params}))
	got, err := h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(
		context.Background(), sandboxEnvSecret, metav1.GetOptions{})
	testifyrequire.NoError(t, err)
	testifyassert.Equal(t, map[string][]byte{"HF_TOKEN": []byte("admin")}, got.Data)

	// An admin of ws-a may change the one they own, but not bind it to ws-b as well, though the
	// role alone lets them bind a Secret to any workspace they are a member of.
	h, users = newSandboxEnvHandler(t,
		ownedSecret(sandboxEnvSecret, "ws-admin", "ws-a"), ownedSecret("other-name", "ws-admin", "ws-a"))
	wsB := []string{"ws-a", "ws-b"}
	testifyrequire.NoError(t, patch(h, users.wsAdmin, "other-name", view.PatchSecretRequest{WorkspaceIds: &wsB}))
	assertGuardRefused(t, patch(h, users.wsAdmin, sandboxEnvSecret, view.PatchSecretRequest{WorkspaceIds: &wsB}))
	testifyrequire.NoError(t, patch(h, users.wsAdmin, sandboxEnvSecret, view.PatchSecretRequest{Params: &params}))
}

func TestMemberCannotDeleteTheSandboxEnvSecret(t *testing.T) {
	h, users := newSandboxEnvHandler(t,
		ownedSecret(sandboxEnvSecret, "member", "ws-a"), ownedSecret("other-name", "member", "ws-a"))
	ctx := context.Background()

	testifyrequire.NoError(t, h.deleteSecretImpl(ctx, "other-name", users.member))
	assertGuardRefused(t, h.deleteSecretImpl(ctx, sandboxEnvSecret, users.member))
	_, err := h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, sandboxEnvSecret, metav1.GetOptions{})
	testifyrequire.NoError(t, err)

	testifyrequire.NoError(t, h.deleteSecretImpl(ctx, sandboxEnvSecret, users.sysAdmin))
}

// A patch that loses a conflict retries on a re-read Secret; if that Secret was meanwhile bound
// to a workspace the caller does not administer, the retry is refused rather than written.
func TestSandboxEnvSecretPatchRechecksAfterAConflict(t *testing.T) {
	var h *Handler
	conflicted := false
	h, users := newSandboxEnvHandlerWith(t, interceptor.Funcs{
		Update: func(ctx context.Context, cli client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if !conflicted {
				conflicted = true
				// Someone else binds it to ws-b between the check and the write.
				rebound := ownedSecret(sandboxEnvSecret, "ws-admin", "ws-a", "ws-b")
				_, err := h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Update(ctx, rebound, metav1.UpdateOptions{})
				testifyrequire.NoError(t, err)
				return apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, obj.GetName(), nil)
			}
			return cli.Update(ctx, obj, opts...)
		},
	}, ownedSecret(sandboxEnvSecret, "ws-admin", "ws-a"))
	params := []map[view.SecretParam]string{{view.SecretParam("LD_PRELOAD"): b64("/tmp/x.so")}}
	c := newPostCtx(jsonutils.MarshalSilently(view.PatchSecretRequest{Params: &params}))
	c.Set(common.UserId, users.wsAdmin.Name)
	c.Set(common.Name, sandboxEnvSecret)
	_, err := h.patchSecret(c)
	testifyassert.True(t, conflicted)
	assertGuardRefused(t, err)
}

// Binding an image Secret through the workspace API also changes which workspaces receive it.
func TestWorkspaceImageSecretBindingHonoursTheSandboxEnvSecret(t *testing.T) {
	image := func(name string) *corev1.Secret {
		s := ownedSecret(name, "sys-admin", "ws-a")
		s.Labels[v1.SecretTypeLabel] = string(v1.SecretImage)
		return s
	}
	reserved := image(sandboxEnvSecret)
	reserved.ResourceVersion = "5"
	h, _ := newSandboxEnvHandler(t, reserved, image("other-name"))
	bAdmin := sandboxEnvUser("b-admin", []v1.UserRole{v1.DefaultRole}, []string{"ws-b"}, []string{"ws-b"})
	ctx := context.Background()

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-b"}}
	testifyrequire.NoError(t, h.updateWorkspaceImageSecrets(ctx, ws, bAdmin, []string{"other-name"}))
	assertGuardRefused(t, h.updateWorkspaceImageSecrets(ctx, ws, bAdmin, []string{sandboxEnvSecret}))
	sysAdmin := sandboxEnvUser("root", []v1.UserRole{v1.SystemAdminRole}, nil, nil)
	testifyrequire.NoError(t, h.updateWorkspaceImageSecrets(ctx, ws, sysAdmin, []string{sandboxEnvSecret}))
}

// The delete of the reserved Secret is pinned to the version whose bindings were checked.
func TestSandboxEnvSecretDeleteIsPinnedToTheCheckedVersion(t *testing.T) {
	reserved := ownedSecret(sandboxEnvSecret, "member", "ws-a")
	reserved.ResourceVersion = "7"
	other := ownedSecret("other-name", "member", "ws-a")
	other.ResourceVersion = "8"
	h, users := newSandboxEnvHandler(t, reserved, other)
	ctx := context.Background()
	testifyrequire.NoError(t, h.deleteSecretImpl(ctx, sandboxEnvSecret, users.sysAdmin))
	testifyrequire.NoError(t, h.deleteSecretImpl(ctx, "other-name", users.member))

	preconditions := map[string]*metav1.Preconditions{}
	for _, action := range h.clientSet.(*k8sfake.Clientset).Actions() {
		if del, ok := action.(k8stesting.DeleteAction); ok {
			preconditions[del.GetName()] = del.GetDeleteOptions().Preconditions
		}
	}
	testifyrequire.Contains(t, preconditions, sandboxEnvSecret)
	testifyrequire.NotNil(t, preconditions[sandboxEnvSecret])
	testifyassert.Equal(t, "7", *preconditions[sandboxEnvSecret].ResourceVersion)
	testifyassert.Nil(t, preconditions["other-name"])
}

// A delete that loses the resourceVersion precondition is retried on the re-read Secret, and
// checked again: here it was rebound to ws-b, which the caller does not administer.
func TestSandboxEnvSecretDeleteRetriesAndRechecksAfterAConflict(t *testing.T) {
	reserved := ownedSecret(sandboxEnvSecret, "ws-admin", "ws-a")
	reserved.ResourceVersion = "7"
	h, users := newSandboxEnvHandler(t, reserved)
	cs := h.clientSet.(*k8sfake.Clientset)
	conflicts := 0
	cs.PrependReactor("delete", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if conflicts > 0 {
			return false, nil, nil
		}
		conflicts++
		// Someone rebinds it between the check and the delete.
		rebound := ownedSecret(sandboxEnvSecret, "ws-admin", "ws-a", "ws-b")
		rebound.ResourceVersion = "8"
		testifyrequire.NoError(t, cs.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"),
			rebound, common.PrimusSafeNamespace))
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, sandboxEnvSecret, nil)
	})
	assertGuardRefused(t, h.deleteSecretImpl(context.Background(), sandboxEnvSecret, users.wsAdmin))
	testifyassert.Equal(t, 1, conflicts)
	_, err := cs.CoreV1().Secrets(common.PrimusSafeNamespace).Get(context.Background(), sandboxEnvSecret, metav1.GetOptions{})
	testifyassert.NoError(t, err)
}

// A delete that keeps losing the precondition ends in a message that says so, not a bare 409.
func TestSandboxEnvSecretDeleteConflictIsExplained(t *testing.T) {
	h, users := newSandboxEnvHandler(t, ownedSecret(sandboxEnvSecret, "ws-admin", "ws-a"))
	h.clientSet.(*k8sfake.Clientset).PrependReactor("delete", "secrets",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, sandboxEnvSecret, nil)
		})
	err := h.deleteSecretImpl(context.Background(), sandboxEnvSecret, users.wsAdmin)
	testifyrequire.Error(t, err)
	testifyassert.True(t, apierrors.IsConflict(err), "%v", err)
	testifyassert.Contains(t, err.Error(), "kept changing while it was being deleted")
}

// A patch whose Secret is deleted after a lost conflict ends in a 404, not a nil dereference
// while logging the failure.
func TestSecretPatchOfASecretDeletedDuringAConflict(t *testing.T) {
	var h *Handler
	h, users := newSandboxEnvHandlerWith(t, interceptor.Funcs{
		Update: func(ctx context.Context, cli client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			testifyrequire.NoError(t, h.clientSet.(*k8sfake.Clientset).Tracker().Delete(
				corev1.SchemeGroupVersion.WithResource("secrets"), common.PrimusSafeNamespace, obj.GetName()))
			return apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, obj.GetName(), nil)
		},
	}, ownedSecret("other-name", "member", "ws-a"))
	params := []map[view.SecretParam]string{{view.SecretParam("K"): b64("v")}}
	c := newPostCtx(jsonutils.MarshalSilently(view.PatchSecretRequest{Params: &params}))
	c.Set(common.UserId, users.member.Name)
	c.Set(common.Name, "other-name")
	_, err := h.patchSecret(c)
	testifyrequire.Error(t, err)
	testifyassert.True(t, apierrors.IsNotFound(err), "%v", err)
}

func patchSecretAs(h *Handler, user *v1.User, name string, req view.PatchSecretRequest) error {
	c := newPostCtx(jsonutils.MarshalSilently(req))
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, name)
	_, err := h.patchSecret(c)
	return err
}

// What docs/apis/secret.md says: an API PATCH with params replaces every key of a general Secret,
// so adding one variable that way deletes the others.
func TestSandboxEnvApiPatchReplacesEveryParam(t *testing.T) {
	h, users := newSandboxEnvHandler(t, ownedSecret(sandboxEnvSecret, "sys-admin", "ws-a"))
	params := []map[view.SecretParam]string{{view.SecretParam("NEW_VAR"): b64("new")}}
	testifyrequire.NoError(t, patchSecretAs(h, users.sysAdmin, sandboxEnvSecret, view.PatchSecretRequest{Params: &params}))
	got := &corev1.Secret{}
	testifyrequire.NoError(t, h.Get(context.Background(),
		client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: sandboxEnvSecret}, got))
	testifyassert.Equal(t, map[string][]byte{"NEW_VAR": []byte("new")}, got.Data)
}

// selectorSandboxEnv is sandbox-env bound to ws-a by id, and also mirrored by a selector (or a
// record of copies a selector made), which reaches workspaces the ids list does not name.
func selectorSandboxEnv(key, value string) *corev1.Secret {
	s := ownedSecret(sandboxEnvSecret, "ws-admin", "ws-a")
	s.Annotations[key] = value
	return s
}

// An admin of every workspace in the ids list may not change, delete or rebind a sandbox-env
// that a selector mirrors: the selector reaches workspaces they do not administer.
func TestOnlySystemAdminMayChangeASelectorMirroredSandboxEnv(t *testing.T) {
	params := []map[view.SecretParam]string{{view.SecretParam("LD_PRELOAD"): b64("/tmp/x.so")}}
	ctx := context.Background()
	for _, tc := range []struct{ key, value string }{
		{v1.WorkspaceSelectorAnnotation, "primus-safe.workspace.scope.sandbox=true"},
		{v1.WorkspaceSelectorAnnotation, " "},
		{v1.WorkspaceSelectorAppliedAnnotation, `["ws-c"]`},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			h, users := newSandboxEnvHandler(t, selectorSandboxEnv(tc.key, tc.value))
			err := patchSecretAs(h, users.wsAdmin, sandboxEnvSecret, view.PatchSecretRequest{Params: &params})
			assertGuardRefused(t, err)
			testifyassert.Contains(t, err.Error(), "workspace selector")
			assertGuardRefused(t, h.deleteSecretImpl(ctx, sandboxEnvSecret, users.wsAdmin))

			image := selectorSandboxEnv(tc.key, tc.value)
			image.Labels[v1.SecretTypeLabel] = string(v1.SecretImage)
			hi, _ := newSandboxEnvHandler(t, image)
			ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-a2"}}
			wsAdmin2 := sandboxEnvUser("ws-admin2", []v1.UserRole{v1.DefaultRole},
				[]string{"ws-a", "ws-a2"}, []string{"ws-a", "ws-a2"})
			assertGuardRefused(t, hi.updateWorkspaceImageSecrets(ctx, ws, wsAdmin2, []string{sandboxEnvSecret}))

			// A system admin may.
			testifyrequire.NoError(t, patchSecretAs(h, users.sysAdmin, sandboxEnvSecret, view.PatchSecretRequest{Params: &params}))
			testifyrequire.NoError(t, h.deleteSecretImpl(ctx, sandboxEnvSecret, users.sysAdmin))
		})
	}
	// Reverse proof: without a selector the same admin is let through, and the selector check
	// leaves every other name alone.
	h, users := newSandboxEnvHandler(t, ownedSecret(sandboxEnvSecret, "ws-admin", "ws-a"))
	testifyrequire.NoError(t, patchSecretAs(h, users.wsAdmin, sandboxEnvSecret, view.PatchSecretRequest{Params: &params}))
	testifyassert.NoError(t, authorizeSandboxEnvSecret(users.wsAdmin, "other-name", []string{"ws-a"},
		selectorSandboxEnv(v1.WorkspaceSelectorAnnotation, "primus-safe.workspace.scope.sandbox=true")))
	// An empty record of selector copies reaches nothing.
	testifyassert.NoError(t, authorizeSandboxEnvSecret(users.wsAdmin, sandboxEnvSecret, []string{"ws-a"},
		selectorSandboxEnv(v1.WorkspaceSelectorAppliedAnnotation, "[]")))
}

// The refusal does not tell the caller which workspaces the Secret is bound to.
func TestSandboxEnvRefusalDoesNotNameTheWorkspaces(t *testing.T) {
	// Through the API: the owner passes the ordinary role check whatever the bindings, so the
	// refusal is the guard's, over a Secret bound to a workspace the caller is not in.
	h, users := newSandboxEnvHandler(t, ownedSecret(sandboxEnvSecret, "ws-admin", "ws-a", "ws-hidden"))
	params := []map[view.SecretParam]string{{view.SecretParam("K"): b64("v")}}
	err := patchSecretAs(h, users.wsAdmin, sandboxEnvSecret, view.PatchSecretRequest{Params: &params})
	assertGuardRefused(t, err)
	testifyassert.NotContains(t, err.Error(), "ws-hidden")
	err = h.deleteSecretImpl(context.Background(), sandboxEnvSecret, users.wsAdmin)
	assertGuardRefused(t, err)
	testifyassert.NotContains(t, err.Error(), "ws-hidden")

	err = authorizeSandboxEnvSecret(users.wsAdmin, sandboxEnvSecret, []string{"ws-a", "ws-hidden"}, nil)
	assertGuardRefused(t, err)
	testifyassert.NotContains(t, err.Error(), "ws-hidden")
	testifyassert.NotContains(t, err.Error(), "ws-a")
}

// A delete of the reserved Secret whose retry finds it already deleted (by someone else, after
// the precondition was lost) succeeds, as a delete of a missing Secret always has.
func TestSandboxEnvSecretDeletedConcurrentlyDuringARetryIsSuccess(t *testing.T) {
	reserved := ownedSecret(sandboxEnvSecret, "ws-admin", "ws-a")
	reserved.ResourceVersion = "7"
	h, users := newSandboxEnvHandler(t, reserved)
	cs := h.clientSet.(*k8sfake.Clientset)
	deletes := 0
	cs.PrependReactor("delete", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		deletes++
		// Someone else deletes it between the check and our delete.
		testifyrequire.NoError(t, cs.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("secrets"),
			common.PrimusSafeNamespace, sandboxEnvSecret))
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, sandboxEnvSecret, nil)
	})
	testifyassert.NoError(t, h.deleteSecretImpl(context.Background(), sandboxEnvSecret, users.wsAdmin))
	testifyassert.Equal(t, 1, deletes)

	// Reverse proof: missing on the first read is still a 404, as on main.
	err := h.deleteSecretImpl(context.Background(), sandboxEnvSecret, users.wsAdmin)
	testifyassert.True(t, apierrors.IsNotFound(err), "%v", err)
}

// Any other name is deleted exactly as before the reserved name existed: one read, one delete
// with no precondition, no retry, a conflict returned as it came, a missing Secret on delete
// ignored.
func TestNonReservedSecretDeleteIsUnchanged(t *testing.T) {
	ctx := context.Background()
	h, users := newSandboxEnvHandler(t, ownedSecret("other-name", "member", "ws-a"))
	cs := h.clientSet.(*k8sfake.Clientset)
	cs.PrependReactor("delete", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, "other-name", nil)
	})
	err := h.deleteSecretImpl(ctx, "other-name", users.member)
	testifyrequire.Error(t, err)
	testifyassert.True(t, apierrors.IsConflict(err), "%v", err)
	testifyassert.NotContains(t, err.Error(), "kept changing")
	gets, deletes := 0, 0
	for _, action := range cs.Actions() {
		switch action.GetVerb() {
		case "get":
			gets++
		case "delete":
			deletes++
			testifyassert.Nil(t, action.(k8stesting.DeleteAction).GetDeleteOptions().Preconditions)
		}
	}
	testifyassert.Equal(t, 1, gets)
	testifyassert.Equal(t, 1, deletes)

	h, users = newSandboxEnvHandler(t, ownedSecret("other-name", "member", "ws-a"))
	h.clientSet.(*k8sfake.Clientset).PrependReactor("delete", "secrets",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, "other-name")
		})
	testifyassert.NoError(t, h.deleteSecretImpl(ctx, "other-name", users.member))
}

// Binding the reserved Secret to a workspace through the image-secret API is patched at the
// version whose bindings were checked. On a conflict it is read and checked again: here someone
// bound it to ws-c meanwhile, which the caller does not administer, so the retry is refused.
func TestSandboxEnvImageBindingRechecksAfterAConflict(t *testing.T) {
	image := ownedSecret(sandboxEnvSecret, "sys-admin", "ws-a")
	image.Labels[v1.SecretTypeLabel] = string(v1.SecretImage)
	image.ResourceVersion = "5"
	var h *Handler
	var sent []string
	h, _ = newSandboxEnvHandlerWith(t, interceptor.Funcs{
		Patch: func(ctx context.Context, cli client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			data, err := patch.Data(obj)
			testifyrequire.NoError(t, err)
			sent = append(sent, string(data))
			if len(sent) == 1 {
				rebound := image.DeepCopy()
				rebound.ResourceVersion = "6"
				v1.SetAnnotation(rebound, v1.WorkspaceIdsAnnotation, `["ws-a","ws-c"]`)
				testifyrequire.NoError(t, h.clientSet.(*k8sfake.Clientset).Tracker().Update(
					corev1.SchemeGroupVersion.WithResource("secrets"), rebound, common.PrimusSafeNamespace))
				return apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, obj.GetName(), nil)
			}
			return nil
		},
	}, image)
	abAdmin := sandboxEnvUser("ab-admin", []v1.UserRole{v1.DefaultRole}, []string{"ws-a", "ws-b"}, []string{"ws-a", "ws-b"})
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-b"}}
	assertGuardRefused(t, h.updateWorkspaceImageSecrets(context.Background(), ws, abAdmin, []string{sandboxEnvSecret}))
	testifyrequire.Len(t, sent, 1, "the retry was written without a check")
	testifyassert.Contains(t, sent[0], `"resourceVersion":"5"`)
}

// A conflict on a re-read Secret still bound only to workspaces the caller administers is
// retried, at the new version, and keeps the binding added meanwhile.
func TestSandboxEnvImageBindingRetriesKeepTheConcurrentBinding(t *testing.T) {
	image := ownedSecret(sandboxEnvSecret, "sys-admin", "ws-a")
	image.Labels[v1.SecretTypeLabel] = string(v1.SecretImage)
	image.ResourceVersion = "5"
	var h *Handler
	var sent []string
	h, _ = newSandboxEnvHandlerWith(t, interceptor.Funcs{
		Patch: func(ctx context.Context, cli client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			data, err := patch.Data(obj)
			testifyrequire.NoError(t, err)
			sent = append(sent, string(data))
			if len(sent) == 1 {
				rebound := image.DeepCopy()
				rebound.ResourceVersion = "6"
				v1.SetAnnotation(rebound, v1.WorkspaceIdsAnnotation, `["ws-a","ws-c"]`)
				testifyrequire.NoError(t, h.clientSet.(*k8sfake.Clientset).Tracker().Update(
					corev1.SchemeGroupVersion.WithResource("secrets"), rebound, common.PrimusSafeNamespace))
				return apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, obj.GetName(), nil)
			}
			return nil
		},
	}, image)
	admin := sandboxEnvUser("abc-admin", []v1.UserRole{v1.DefaultRole},
		[]string{"ws-a", "ws-b", "ws-c"}, []string{"ws-a", "ws-b", "ws-c"})
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-b"}}
	testifyrequire.NoError(t, h.updateWorkspaceImageSecrets(context.Background(), ws, admin, []string{sandboxEnvSecret}))
	testifyrequire.Len(t, sent, 2)
	testifyassert.Contains(t, sent[1], `"resourceVersion":"6"`)
	testifyassert.Contains(t, sent[1], `[\"ws-a\",\"ws-c\",\"ws-b\"]`)
}

// Reverse proof: any other image Secret is patched as before, with no resourceVersion.
func TestOtherImageBindingIsPatchedAsBefore(t *testing.T) {
	image := ownedSecret("other-name", "sys-admin", "ws-a")
	image.Labels[v1.SecretTypeLabel] = string(v1.SecretImage)
	image.ResourceVersion = "5"
	var sent []string
	h, _ := newSandboxEnvHandlerWith(t, interceptor.Funcs{
		Patch: func(ctx context.Context, cli client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			data, err := patch.Data(obj)
			testifyrequire.NoError(t, err)
			sent = append(sent, string(data))
			return nil
		},
	}, image)
	bAdmin := sandboxEnvUser("b-admin", []v1.UserRole{v1.DefaultRole}, []string{"ws-b"}, []string{"ws-b"})
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-b"}}
	testifyrequire.NoError(t, h.updateWorkspaceImageSecrets(context.Background(), ws, bAdmin, []string{"other-name"}))
	testifyrequire.Len(t, sent, 1)
	testifyassert.NotContains(t, sent[0], "resourceVersion")
}

// readSecretsFromClient makes the handler's clientset read Secrets from its controller-runtime
// client, so both see one cluster: what a Patch wrote is what a re-read returns.
func readSecretsFromClient(h *Handler) {
	h.clientSet.(*k8sfake.Clientset).PrependReactor("get", "secrets",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			got := &corev1.Secret{}
			err := h.Get(context.Background(), client.ObjectKey{
				Namespace: action.GetNamespace(), Name: action.(k8stesting.GetAction).GetName()}, got)
			return true, got, err
		})
}

// patchWorkspaceAs sends PATCH /workspaces/{workspace} through the HTTP handler.
func patchWorkspaceAs(h *Handler, user *v1.User, workspace string, req view.PatchWorkspaceRequest) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(jsonutils.MarshalSilently(req)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, workspace)
	h.PatchWorkspace(c)
	return rsp
}

// Binding an image Secret of any name to a workspace, when the first write of its workspace ids
// loses a conflict, retries on the Secret as stored: the retry writes the binding, keeping one
// added meanwhile, and the stored Secret names the workspace, so it is mirrored there. Before,
// the retry found the workspace in its own unwritten copy, wrote nothing, and the API returned
// 200 with the workspace listing a pull Secret that would never reach its namespace.
func TestImageBindingAfterAConflictIsWritten(t *testing.T) {
	ctx := context.Background()
	image := ownedSecret("image-pull", "sys-admin", "ws-a")
	image.Labels[v1.SecretTypeLabel] = string(v1.SecretImage)
	var h *Handler
	patches := 0
	h, users := newSandboxEnvHandlerWith(t, interceptor.Funcs{
		Patch: func(ctx context.Context, cli client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if _, ok := obj.(*corev1.Secret); !ok {
				return cli.Patch(ctx, obj, patch, opts...)
			}
			patches++
			if patches == 1 {
				// Someone binds it to ws-c between the read and the write, and ours is refused.
				stored := &corev1.Secret{}
				testifyrequire.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(obj), stored))
				v1.SetAnnotation(stored, v1.WorkspaceIdsAnnotation, `["ws-a","ws-c"]`)
				testifyrequire.NoError(t, cli.Update(ctx, stored))
				return apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, obj.GetName(), nil)
			}
			return cli.Patch(ctx, obj, patch, opts...)
		},
	}, image)
	readSecretsFromClient(h)
	testifyrequire.NoError(t, h.Create(ctx, &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-b"}}))

	ids := []string{"image-pull"}
	rsp := patchWorkspaceAs(h, users.sysAdmin, "ws-b", view.PatchWorkspaceRequest{ImageSecretIds: &ids})
	testifyrequire.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	testifyassert.Equal(t, 2, patches, "the binding was not written again after the conflict")
	stored := &corev1.Secret{}
	testifyrequire.NoError(t, h.Get(ctx, client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: "image-pull"}, stored))
	testifyassert.Equal(t, []string{"ws-a", "ws-c", "ws-b"}, commonsecret.GetSecretWorkspaces(stored))
	ws := &v1.Workspace{}
	testifyrequire.NoError(t, h.Get(ctx, client.ObjectKey{Name: "ws-b"}, ws))
	testifyrequire.Len(t, ws.Spec.ImageSecrets, 1)
	testifyassert.Equal(t, "image-pull", ws.Spec.ImageSecrets[0].Name)
}

// A write that fails for any other reason leaves the caller's Secret as it was read: it does not
// claim a binding that was never stored.
func TestImageBindingFailureLeavesTheSecretUnbound(t *testing.T) {
	image := ownedSecret("image-pull", "sys-admin", "ws-a")
	image.Labels[v1.SecretTypeLabel] = string(v1.SecretImage)
	h, users := newSandboxEnvHandlerWith(t, interceptor.Funcs{
		Patch: func(ctx context.Context, cli client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			return apierrors.NewInternalError(errors.New("etcd is down"))
		},
	}, image)
	secret := image.DeepCopy()
	err := h.bindSecretToWorkspace(context.Background(), secret, "ws-b", users.sysAdmin)
	testifyrequire.Error(t, err)
	testifyassert.Equal(t, []string{"ws-a"}, commonsecret.GetSecretWorkspaces(secret))
}

// A binding that keeps conflicting ends in a 409 that says to try again, for the reserved name as
// for any other, the same as a delete of the reserved Secret that keeps conflicting.
func TestImageBindingConflictIsExplained(t *testing.T) {
	for _, name := range []string{sandboxEnvSecret, "image-pull"} {
		t.Run(name, func(t *testing.T) {
			image := ownedSecret(name, "sys-admin", "ws-a")
			image.Labels[v1.SecretTypeLabel] = string(v1.SecretImage)
			patches := 0
			h, users := newSandboxEnvHandlerWith(t, interceptor.Funcs{
				Patch: func(ctx context.Context, cli client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					patches++
					return apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, obj.GetName(), nil)
				},
			}, image)
			ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-b"}}
			err := h.updateWorkspaceImageSecrets(context.Background(), ws, users.sysAdmin, []string{name})
			testifyrequire.Error(t, err)
			testifyassert.Equal(t, defaultRetryCount, patches)
			testifyassert.True(t, apierrors.IsConflict(err), "%v", err)
			testifyassert.Contains(t, err.Error(), "kept changing while it was being bound")

			// What the HTTP layer answers with it.
			gin.SetMode(gin.TestMode)
			rsp := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rsp)
			apiutils.AbortWithApiError(c, err)
			testifyassert.Equal(t, http.StatusConflict, rsp.Code)
			testifyassert.Contains(t, rsp.Body.String(), commonerrors.StatusConflict)
			testifyassert.Contains(t, rsp.Body.String(), "try again")
		})
	}
}

// The ordinary role check's 403 names the workspaces it checked. For sandbox-env, whose name
// everyone knows, it is replaced so that no caller learns which workspaces it is bound to.
func TestSandboxEnvRoleRefusalDoesNotNameTheWorkspaces(t *testing.T) {
	ctx := context.Background()
	outsider := sandboxEnvUser("outsider", []v1.UserRole{v1.DefaultRole}, []string{"ws-a"}, nil)
	params := []map[view.SecretParam]string{{view.SecretParam("K"): b64("v")}}
	refusals := func(name string) []error {
		h, _ := newSandboxEnvHandler(t, ownedSecret(name, "sys-admin", "ws-a", "ws-hidden"))
		testifyrequire.NoError(t, h.Create(ctx, outsider.DeepCopy()))
		getCtx := newPostCtx(nil)
		getCtx.Set(common.UserId, outsider.Name)
		getCtx.Set(common.Name, name)
		_, getErr := h.getSecret(getCtx)
		return []error{
			patchSecretAs(h, outsider, name, view.PatchSecretRequest{Params: &params}),
			h.deleteSecretImpl(ctx, name, outsider),
			getErr,
		}
	}
	for i, err := range refusals(sandboxEnvSecret) {
		testifyrequire.Error(t, err, "call %d", i)
		testifyassert.True(t, commonerrors.IsForbidden(err), "call %d: %v", i, err)
		testifyassert.NotContains(t, err.Error(), "ws-hidden", "call %d", i)
	}
	// Reverse proof: any other name keeps the ordinary message, which names them.
	for i, err := range refusals("other-name") {
		testifyrequire.Error(t, err, "call %d", i)
		testifyassert.Contains(t, err.Error(), "ws-hidden", "call %d", i)
	}
}
