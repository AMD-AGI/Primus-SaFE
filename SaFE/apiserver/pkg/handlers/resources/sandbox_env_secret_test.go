/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"testing"

	testifyassert "github.com/stretchr/testify/assert"
	testifyrequire "github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const (
	sandboxEnvSecret = "sandbox-env"
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

func enableSandboxEnvSecret(t *testing.T, name string) {
	commonconfig.SetValue("sandbox.env_secret", name)
	t.Cleanup(func() { commonconfig.SetValue("sandbox.env_secret", "") })
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
	enableSandboxEnvSecret(t, sandboxEnvSecret)
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
	enableSandboxEnvSecret(t, sandboxEnvSecret)
	h, users := newSandboxEnvHandler(t)
	_, err := h.createSecretImpl(context.Background(), sandboxEnvCreate(sandboxEnvSecret, "ws-a", "ws-b"), users.sysAdmin)
	testifyrequire.NoError(t, err)
}

// The name is whatever the chart configures, and nothing is reserved while it is empty.
func TestSandboxEnvSecretNameFollowsTheConfig(t *testing.T) {
	h, users := newSandboxEnvHandler(t)
	ctx := context.Background()
	_, err := h.createSecretImpl(ctx, sandboxEnvCreate(sandboxEnvSecret, "ws-a"), users.member)
	testifyrequire.NoError(t, err)

	enableSandboxEnvSecret(t, "site-sandbox-vars")
	_, err = h.createSecretImpl(ctx, sandboxEnvCreate("site-sandbox-vars", "ws-a"), users.member)
	assertGuardRefused(t, err)
}

func TestMemberCannotUpdateTheSandboxEnvSecret(t *testing.T) {
	enableSandboxEnvSecret(t, sandboxEnvSecret)
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
	enableSandboxEnvSecret(t, sandboxEnvSecret)
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
	enableSandboxEnvSecret(t, sandboxEnvSecret)
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
	enableSandboxEnvSecret(t, sandboxEnvSecret)
	image := func(name string) *corev1.Secret {
		s := ownedSecret(name, "sys-admin", "ws-a")
		s.Labels[v1.SecretTypeLabel] = string(v1.SecretImage)
		return s
	}
	h, _ := newSandboxEnvHandler(t, image(sandboxEnvSecret), image("other-name"))
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
	enableSandboxEnvSecret(t, sandboxEnvSecret)
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
