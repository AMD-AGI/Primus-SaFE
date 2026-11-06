/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const (
	ptWorkspace = "ws-cicd"
	ptOther     = "ws-cicd-other"
	// ptMember is a member of ptWorkspace.
	ptMember = "pt-member"
	// ptManager manages ptWorkspace.
	ptManager = "pt-manager"
	// ptOtherManager is a member of ptWorkspace and manages another workspace.
	ptOtherManager = "pt-other-manager"
	ptAdmin        = "pt-admin"
)

// privilegedTemplateKinds are the kinds whose stock templates run a privileged container; the
// common package's test holds that list to the chart.
var ptPrivilegedTemplateKinds = []string{
	common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind, common.CICDGithubRunnerKind, common.UnifiedJobKind,
}

type privilegedTemplateFixture struct {
	h   *Handler
	cli client.Client
}

// newPrivilegedTemplateFixture is a cluster with the built-in roles as the chart ships them, a
// workspace, the control-plane node the create path reads, and users in different relations to
// the workspace.
func newPrivilegedTemplateFixture(t *testing.T) *privilegedTemplateFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	user := func(name string, workspaces, managed []string, roles ...v1.UserRole) *v1.User {
		u := &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name},
				Annotations: map[string]string{v1.UserNameAnnotation: name}},
			Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Resources: map[string][]string{}},
		}
		if len(workspaces) > 0 {
			u.Spec.Resources[common.UserWorkspaces] = workspaces
		}
		if len(managed) > 0 {
			u.Spec.Resources[common.UserManagedWorkspaces] = managed
		}
		return u
	}
	controlPlane := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "cp-1", Labels: map[string]string{common.KubernetesControlPlane: ""}},
		Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{
			{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
		}},
	}
	ws := []string{ptWorkspace}
	objs := append(authtest.ChartRoles(t), controlPlane,
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: ptWorkspace}},
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: ptOther}},
		user(ptMember, ws, nil, v1.DefaultRole),
		user(ptManager, ws, ws, v1.DefaultRole),
		user(ptOtherManager, []string{ptWorkspace, ptOther}, []string{ptOther}, v1.DefaultRole),
		user(ptAdmin, nil, nil, v1.SystemAdminRole),
	)
	cli := fake.NewClientBuilder().WithScheme(getTestScheme()).WithObjects(objs...).
		WithStatusSubresource(&v1.Workload{}).Build()
	return &privilegedTemplateFixture{
		h:   &Handler{Client: cli, accessController: &authority.AccessController{Client: cli}},
		cli: cli,
	}
}

func (f *privilegedTemplateFixture) ginContext(t *testing.T, userId, method, body string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(method, "/api/v1/workloads", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, userId)
	return c
}

// post creates a workload of the given kind through the create route, as the runner and
// unified-job proxies do for the workloads they start.
func (f *privilegedTemplateFixture) post(t *testing.T, userId, kind string) (*v1.Workload, error) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"displayName":      "pt",
		"workspaceId":      ptWorkspace,
		"images":           []string{"img"},
		"entryPoints":      []string{"sleep"},
		"groupVersionKind": map[string]string{"version": "v1", "kind": kind},
		"resources":        []map[string]any{{"replica": 1, "cpu": "1", "memory": "1Gi"}},
	})
	require.NoError(t, err)
	res, err := f.h.createWorkload(f.ginContext(t, userId, http.MethodPost, string(body)))
	if err != nil {
		return nil, err
	}
	stored := &v1.Workload{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: res.(*view.CreateWorkloadResponse).WorkloadId}, stored))
	return stored, nil
}

// createBuilt hands a workload of the given kind, already generated, to the create path's checks,
// for the kinds whose generation needs GitHub credentials.
func (f *privilegedTemplateFixture) createBuilt(t *testing.T, userId, name, kind string) error {
	t.Helper()
	user := &v1.User{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: userId}, user))
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.WorkloadSpec{
			Workspace:        ptWorkspace,
			GroupVersionKind: v1.GroupVersionKind{Version: "v1", Kind: kind},
		},
	}
	c := f.ginContext(t, userId, http.MethodPost, "")
	_, err := f.h.createWorkloadImpl(c, workload, user, f.h.accessController.GetRoles(t.Context(), user))
	return err
}

// stored creates a workload directly in the cluster, owned by owner, as one created before this
// rule existed would be.
func (f *privilegedTemplateFixture) stored(t *testing.T, name, kind, owner string, annotations map[string]string) {
	t.Helper()
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: owner},
			Annotations: annotations},
		Spec: v1.WorkloadSpec{
			Workspace:        ptWorkspace,
			GroupVersionKind: v1.GroupVersionKind{Version: "v1", Kind: kind},
			Images:           []string{"img"},
			EntryPoints:      []string{"sleep"},
		},
	}
	require.NoError(t, f.cli.Create(t.Context(), workload))
}

// patch changes the description of the named workload through the patch route.
func (f *privilegedTemplateFixture) patch(t *testing.T, userId, name string) error {
	t.Helper()
	c := f.ginContext(t, userId, http.MethodPatch, `{"description":"changed"}`)
	c.Set(common.Name, name)
	_, err := f.h.patchWorkload(c)
	return err
}

func (f *privilegedTemplateFixture) count(t *testing.T) int {
	t.Helper()
	list := &v1.WorkloadList{}
	require.NoError(t, f.cli.List(t.Context(), list))
	return len(list.Items)
}

func requirePrivilegedGrantRefusal(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	require.True(t, apierrors.IsForbidden(err), "want 403, got %v %v", err, msgAndArgs)
	require.Contains(t, err.Error(), authority.WorkloadPrivilegedKind, msgAndArgs...)
}

// A GitHub Actions runner or a unified job does not ask for privileged mode, but its template runs
// privileged containers, so creating one takes the workload/privileged grant a privileged workload
// takes: a member of the workspace, and a member who manages another workspace, are refused (403
// naming the grant, nothing created) on the create route the proxies use and for every such kind;
// the workspace's manager and a system administrator create them; a member still creates a
// PyTorchJob.
func TestPrivilegedTemplateKindsNeedThePrivilegedGrantToCreate(t *testing.T) {
	f := newPrivilegedTemplateFixture(t)

	for _, kind := range []string{common.CICDEphemeralRunnerKind, common.UnifiedJobKind} {
		for _, who := range []string{ptMember, ptOtherManager} {
			_, err := f.post(t, who, kind)
			requirePrivilegedGrantRefusal(t, err, kind, who)
			require.Contains(t, err.Error(), kind+" workloads run privileged containers")
		}
	}
	for _, kind := range ptPrivilegedTemplateKinds {
		for _, who := range []string{ptMember, ptOtherManager} {
			requirePrivilegedGrantRefusal(t, f.createBuilt(t, who, strings.ToLower("built-"+who+"-"+kind), kind), kind, who)
		}
	}
	require.Equal(t, 0, f.count(t), "a refused workload was created")

	for _, kind := range []string{common.CICDEphemeralRunnerKind, common.UnifiedJobKind} {
		for _, who := range []string{ptManager, ptAdmin} {
			stored, err := f.post(t, who, kind)
			require.NoError(t, err, "%s %s", kind, who)
			require.Equal(t, kind, stored.SpecKind())
			require.Equal(t, who, v1.GetUserId(stored))
		}
	}
	for _, kind := range ptPrivilegedTemplateKinds {
		for _, who := range []string{ptManager, ptAdmin} {
			require.NoError(t, f.createBuilt(t, who, strings.ToLower("built-"+who+"-"+kind), kind), "%s %s", kind, who)
		}
	}

	stored, err := f.post(t, ptMember, common.PytorchJobKind)
	require.NoError(t, err)
	require.Equal(t, common.PytorchJobKind, stored.SpecKind())
}

// A change to a workload that runs privileged containers takes the same grant: the owner of a
// runner, a unified job or a privileged workload created before the rule, who holds no grant, can
// no longer change it; the workspace's manager can. A member still changes their own PyTorchJob.
func TestWorkloadsThatRunPrivilegedNeedThePrivilegedGrantToChange(t *testing.T) {
	f := newPrivilegedTemplateFixture(t)
	privileged := map[string]string{v1.WorkloadPrivilegedAnnotation: v1.TrueStr}
	cases := map[string]struct {
		kind        string
		annotations map[string]string
	}{
		"legacy-scaleset": {common.CICDScaleRunnerSetKind, nil},
		"legacy-runner":   {common.CICDEphemeralRunnerKind, nil},
		"legacy-github":   {common.CICDGithubRunnerKind, nil},
		"legacy-unified":  {common.UnifiedJobKind, nil},
		"legacy-priv":     {common.PytorchJobKind, privileged},
	}
	for name, tc := range cases {
		f.stored(t, name, tc.kind, ptMember, tc.annotations)
	}
	f.stored(t, "member-job", common.PytorchJobKind, ptMember, nil)

	for name := range cases {
		requirePrivilegedGrantRefusal(t, f.patch(t, ptMember, name), name)
		changed := &v1.Workload{}
		require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: name}, changed))
		require.NotEqual(t, "changed", v1.GetDescription(changed), name)
	}
	for name := range cases {
		require.NoError(t, f.patch(t, ptManager, name), name)
		changed := &v1.Workload{}
		require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: name}, changed))
		require.Equal(t, "changed", v1.GetDescription(changed), name)
	}
	require.NoError(t, f.patch(t, ptMember, "member-job"))
}
