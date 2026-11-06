/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	authzWorkspace = "train-ws"
	authzOther     = "other-ws"
	// authzMember is a member of authzWorkspace.
	authzMember = "train-member"
	// authzOutsider is signed in and a member of authzOther only.
	authzOutsider = "train-outsider"
	// authzManager manages authzWorkspace.
	authzManager = "train-manager"
	// authzAdmin is a system administrator and a member of no workspace.
	authzAdmin = "train-admin"
)

// trainingRoute is one of the routes that build and create a training workload themselves.
type trainingRoute struct {
	name   string
	kind   string
	create func(h *Handler, c *gin.Context) (string, error)
	body   func(name, workspace string, priority int) any
}

var trainingRoutes = []trainingRoute{
	{
		name: "POST /sft/jobs",
		kind: common.PytorchJobKind,
		create: func(h *Handler, c *gin.Context) (string, error) {
			res, err := h.createSftJob(c)
			if err != nil {
				return "", err
			}
			return res.(*CreateSftJobResponse).WorkloadId, nil
		},
		body: func(name, workspace string, priority int) any {
			exportModel := false
			return CreateSftJobRequest{
				DisplayName: name, Workspace: workspace, ModelId: "model-1", DatasetId: "dataset-1",
				ExportModel: &exportModel, Image: "img", NodeCount: 1, GpuCount: 8, Cpu: "80",
				Memory: "1000Gi", SharedMemory: "500Gi", EphemeralStorage: "1000Gi",
				TrainConfig: SftTrainConfig{Peft: "none"}, Priority: priority,
			}
		},
	},
	{
		name: "POST /rl/jobs",
		kind: common.RayJobKind,
		create: func(h *Handler, c *gin.Context) (string, error) {
			res, err := h.createRlJob(c)
			if err != nil {
				return "", err
			}
			return res.(*CreateRlJobResponse).WorkloadId, nil
		},
		body: func(name, workspace string, priority int) any {
			exportModel := false
			return CreateRlJobRequest{
				DisplayName: name, Workspace: workspace, ModelId: "model-1", DatasetId: "dataset-1",
				ExportModel: &exportModel, Image: "img", NodeCount: 1, GpuCount: 8, Cpu: "80",
				Memory: "1000Gi", SharedMemory: "500Gi", EphemeralStorage: "1000Gi", Priority: priority,
			}
		},
	},
}

// newTrainingAuthzHandler is the model handler over a cluster with the built-in roles as the chart
// ships them, two workspaces that both hold a ready model and a dataset, and users in different
// relations to the first workspace.
func newTrainingAuthzHandler(t *testing.T) (*Handler, client.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mockDB := mock_client.NewMockInterface(gomock.NewController(t))
	localPaths, err := json.Marshal([]dbclient.DatasetLocalPathDB{
		{Workspace: authzWorkspace, Path: "/shared_nfs/datasets/alpaca", Status: dbclient.DatasetStatusReady},
		{Workspace: authzOther, Path: "/shared_nfs/datasets/alpaca", Status: dbclient.DatasetStatusReady},
	})
	require.NoError(t, err)
	mockDB.EXPECT().GetDataset(gomock.Any(), "dataset-1").
		Return(&dbclient.Dataset{DatasetId: "dataset-1", LocalPaths: string(localPaths)}, nil).AnyTimes()
	mockDB.EXPECT().GetModelByID(gomock.Any(), "model-1").
		Return(&dbclient.Model{ID: "model-1", ModelName: "Qwen/Qwen3-8B"}, nil).AnyTimes()

	model := genMockLocalK8sModel("model-1", authzWorkspace)
	model.Spec.Source.ModelName = "Qwen/Qwen3-8B"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{Workspace: authzWorkspace, Path: "/shared_nfs/models/Qwen/Qwen3-8B", Status: v1.LocalPathStatusReady},
		{Workspace: authzOther, Path: "/shared_nfs/models/Qwen/Qwen3-8B", Status: v1.LocalPathStatusReady},
	}
	user := func(name string, workspaces, managed []string, roles ...v1.UserRole) *v1.User {
		u := &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}},
			Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Resources: map[string][]string{}},
		}
		if len(workspaces) > 0 {
			u.Spec.Resources[common.UserWorkspaces] = workspaces
		}
		if len(managed) > 0 {
			u.Spec.Resources[common.UserManagedWorkspaces] = managed
		}
		return u
	}
	ws := []string{authzWorkspace}
	objs := append(authtest.ChartRoles(t), model,
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: authzWorkspace}},
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: authzOther}},
		user(authzMember, ws, nil, v1.DefaultRole),
		user(authzOutsider, []string{authzOther}, nil, v1.DefaultRole),
		user(authzManager, ws, ws, v1.DefaultRole),
		user(authzAdmin, nil, nil, v1.SystemAdminRole),
	)
	cli := fake.NewClientBuilder().WithObjects(objs...).WithScheme(scheme.Scheme).Build()
	return &Handler{k8sClient: cli, dbClient: mockDB, accessController: &authority.AccessController{Client: cli}}, cli
}

// postTrainingJob calls the route's handler as the router does, as userId.
func postTrainingJob(t *testing.T, h *Handler, route trainingRoute, userId, workspace string, priority int) (string, error) {
	t.Helper()
	// The routes derive the workload name from the display name and the clock's milliseconds,
	// so each call gets its own display name.
	name := fmt.Sprintf("%s-%s-%d", userId, workspace, priority)
	body, err := json.Marshal(route.body(name, workspace, priority))
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, userId)
	c.Set(common.UserName, userId)
	return route.create(h, c)
}

func countWorkloads(t *testing.T, cli client.Client) int {
	t.Helper()
	list := &v1.WorkloadList{}
	require.NoError(t, cli.List(t.Context(), list))
	return len(list.Items)
}

// The SFT and RL routes build their workload and create it themselves, not through the workload
// create path, so they hold the caller to that path's rules: a signed-in user who is not a member
// of the workspace is refused (403, nothing created), a member creates at the default and the
// medium priority but not at high priority, and the workspace's manager and a system
// administrator create at high priority.
func TestTrainingJobsAuthorizeLikeTheWorkloadCreatePath(t *testing.T) {
	for _, route := range trainingRoutes {
		t.Run(route.name, func(t *testing.T) {
			h, cli := newTrainingAuthzHandler(t)

			for _, refused := range []struct {
				user, workspace string
				priority        int
				want            string
			}{
				{authzOutsider, authzWorkspace, 0, "not allowed to create workload"},
				{authzOutsider, authzWorkspace, common.MedPriorityInt, "not allowed to create workload"},
				{authzManager, authzOther, 0, "not allowed to create workload"},
				{authzMember, authzWorkspace, common.HighPriorityInt, "not allowed to create workload/high-priority"},
			} {
				_, err := postTrainingJob(t, h, route, refused.user, refused.workspace, refused.priority)
				require.True(t, apierrors.IsForbidden(err), "%+v: want 403, got %v", refused, err)
				require.Contains(t, err.Error(), refused.want, "%+v", refused)
			}
			require.Equal(t, 0, countWorkloads(t, cli), "a refused workload was created")

			for _, allowed := range []struct {
				user     string
				priority int
				stored   int
			}{
				{authzMember, 0, common.MedPriorityInt},
				{authzMember, common.MedPriorityInt, common.MedPriorityInt},
				{authzManager, common.HighPriorityInt, common.HighPriorityInt},
				{authzAdmin, common.HighPriorityInt, common.HighPriorityInt},
			} {
				id, err := postTrainingJob(t, h, route, allowed.user, authzWorkspace, allowed.priority)
				require.NoError(t, err, "%+v", allowed)
				stored := &v1.Workload{}
				require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: id}, stored))
				require.Equal(t, route.kind, stored.SpecKind())
				require.Equal(t, authzWorkspace, stored.Spec.Workspace)
				require.Equal(t, allowed.stored, stored.Spec.Priority, "%+v", allowed)
				require.Equal(t, allowed.user, v1.GetUserId(stored))
			}
		})
	}
}

// Without an access controller the routes create nothing.
func TestTrainingJobsFailClosedWithoutAccessController(t *testing.T) {
	for _, route := range trainingRoutes {
		t.Run(route.name, func(t *testing.T) {
			h, cli := newTrainingAuthzHandler(t)
			h.accessController = nil
			_, err := postTrainingJob(t, h, route, authzAdmin, authzWorkspace, 0)
			require.Error(t, err)
			require.Contains(t, err.Error(), "access controller is not initialized")
			require.Equal(t, 0, countWorkloads(t, cli))
		})
	}
}
