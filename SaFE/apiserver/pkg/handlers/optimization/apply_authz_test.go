/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

const (
	applyWorkspace = "opt-ws"
	applyOther     = "opt-other-ws"
	// applyOwner submitted the task and is a member of applyWorkspace.
	applyOwner = "opt-owner"
	// applyMember is another member of applyWorkspace.
	applyMember = "opt-member"
	// applyOutsider is signed in and a member of applyOther only.
	applyOutsider = "opt-outsider"
)

// newApplyAuthzHandler is the optimization handler over a cluster with the built-in roles as the
// chart ships them, two workspaces that both hold the task's model, a finished task in the first,
// and a Claw that has no report for it (so the launch command is derived from the task).
func newApplyAuthzHandler(t *testing.T) (*Handler, client.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	claw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(claw.Close)

	localPaths, err := json.Marshal([]dbclient.ModelLocalPathDB{
		{Workspace: applyWorkspace, Path: "/shared/models/m", Status: "Ready"},
		{Workspace: applyOther, Path: "/shared/models/m", Status: "Ready"},
	})
	require.NoError(t, err)
	mockDB := mock_client.NewMockInterface(gomock.NewController(t))
	mockDB.EXPECT().GetOptimizationTask(gomock.Any(), "task-1").Return(&dbclient.OptimizationTask{
		ID: "task-1", DisplayName: "opt", Workspace: applyWorkspace, UserID: applyOwner, UserName: applyOwner,
		ModelID: "model-1", ClawSessionID: "session-1", Image: "img", Framework: "vllm", TP: 1,
	}, nil).AnyTimes()
	mockDB.EXPECT().GetModelByID(gomock.Any(), "model-1").Return(&dbclient.Model{
		ID: "model-1", DisplayName: "m", AccessMode: "local", Phase: "Ready", ModelName: "m",
		LocalPaths: string(localPaths),
	}, nil).AnyTimes()

	user := func(name string, workspaces ...string) *v1.User {
		return &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}},
			Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.DefaultRole},
				Resources: map[string][]string{common.UserWorkspaces: workspaces}},
		}
	}
	controlPlane := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "cp-1", Labels: map[string]string{common.KubernetesControlPlane: ""}},
		Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.1"}}},
	}
	objs := append(authtest.ChartRoles(t), controlPlane,
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: applyWorkspace}},
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: applyOther}},
		user(applyOwner, applyWorkspace), user(applyMember, applyWorkspace), user(applyOutsider, applyOther),
	)
	sch := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(sch))
	require.NoError(t, corev1.AddToScheme(sch))
	cli := fake.NewClientBuilder().WithScheme(sch).WithObjects(objs...).Build()
	return &Handler{
		dbClient:   mockDB,
		k8sClient:  cli,
		accessCtrl: &authority.AccessController{Client: cli},
		clawClient: NewClawClient(claw.URL, ""),
		hubs:       newHubRegistry(),
	}, cli
}

// apply calls POST /optimization/tasks/task-1/apply as userId, asking for the given workspace.
func apply(t *testing.T, h *Handler, userId, workspace string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(ApplyTaskRequest{Workspace: workspace})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "task-1"}}
	c.Set(common.UserId, userId)
	c.Set(common.UserName, userId)
	h.ApplyTask(c)
	return rec
}

func applyWorkloads(t *testing.T, cli client.Client) []v1.Workload {
	t.Helper()
	list := &v1.WorkloadList{}
	require.NoError(t, cli.List(t.Context(), list))
	return list.Items
}

// Applying an optimization task creates a Deployment with the apiserver's own client, so the route
// holds the caller to the workload create path's checks and makes the caller its user: a signed-in
// user who is not a member of the workspace they name is refused (403, nothing created); that
// user can apply the task into a workspace they are a member of, and a member other than the
// task's owner can apply it into the task's workspace, each creating a workload that is theirs.
func TestApplyTaskAuthorizesLikeTheWorkloadCreatePath(t *testing.T) {
	h, cli := newApplyAuthzHandler(t)

	for _, tc := range []struct{ user, workspace, refusal string }{
		{applyMember, applyOther, "not allowed to create workload"},
		{applyOutsider, applyWorkspace, "not allowed to list workload"},
		{applyOutsider, "", "not allowed to list workload"},
		{applyOutsider, applyOther, "not allowed to list workload"},
	} {
		rec := apply(t, h, tc.user, tc.workspace)
		require.Equal(t, http.StatusForbidden, rec.Code, "%+v: %s", tc, rec.Body.String())
		require.Contains(t, rec.Body.String(), tc.refusal, "%+v", tc)
	}
	require.Empty(t, applyWorkloads(t, cli), "a refused workload was created")

	for _, tc := range []struct{ user, workspace string }{
		{applyMember, applyWorkspace},
		{applyOwner, ""},
	} {
		rec := apply(t, h, tc.user, tc.workspace)
		require.Equal(t, http.StatusCreated, rec.Code, "%+v: %s", tc, rec.Body.String())
		var resp ApplyTaskResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		stored := &v1.Workload{}
		require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: resp.WorkloadID}, stored))
		wantWorkspace := tc.workspace
		if wantWorkspace == "" {
			wantWorkspace = applyWorkspace
		}
		require.Equal(t, wantWorkspace, stored.Spec.Workspace, "%+v", tc)
		require.Equal(t, tc.user, v1.GetUserId(stored), "%+v", tc)
		require.Equal(t, tc.user, v1.GetUserName(stored), "%+v", tc)
	}
}

// Without an access controller the route creates nothing.
func TestApplyTaskFailsClosedWithoutAccessController(t *testing.T) {
	h, cli := newApplyAuthzHandler(t)
	h.accessCtrl = nil
	rec := apply(t, h, applyOwner, applyWorkspace)
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	require.Empty(t, applyWorkloads(t, cli))
}
