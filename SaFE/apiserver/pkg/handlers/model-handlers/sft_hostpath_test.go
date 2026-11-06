/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"bytes"
	"encoding/json"
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

// newSftHostPathHandler is the SFT handler over a cluster with the built-in roles as the chart
// ships them, a workspace that declares /shared_nfs as a hostpath volume, a ready model and a
// dataset in it, a member of the workspace, its manager and a system administrator.
func newSftHostPathHandler(t *testing.T) (*Handler, client.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mockDB := mock_client.NewMockInterface(gomock.NewController(t))
	localPaths, err := json.Marshal([]dbclient.DatasetLocalPathDB{
		{Workspace: "ws1", Path: "/shared_nfs/datasets/alpaca", Status: dbclient.DatasetStatusReady},
	})
	require.NoError(t, err)
	mockDB.EXPECT().GetDataset(gomock.Any(), "dataset-1").
		Return(&dbclient.Dataset{DatasetId: "dataset-1", LocalPaths: string(localPaths)}, nil).AnyTimes()

	model := genMockLocalK8sModel("model-1", "ws1")
	model.Spec.Source.ModelName = "Qwen/Qwen3-8B"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{Workspace: "ws1", Path: "/shared_nfs/models/Qwen/Qwen3-8B", Status: v1.LocalPathStatusReady},
	}
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
			{Id: 1, Type: v1.HOSTPATH, HostPath: "/shared_nfs", MountPath: "/shared_nfs"},
		}},
	}
	user := func(name string, managed bool, roles ...v1.UserRole) *v1.User {
		u := &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}},
			Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Resources: map[string][]string{
				common.UserWorkspaces: {"ws1"},
			}},
		}
		if managed {
			u.Spec.Resources[common.UserManagedWorkspaces] = []string{"ws1"}
		}
		return u
	}
	objs := append(authtest.ChartRoles(t), model, workspace,
		user("sft-member", false, v1.DefaultRole),
		user("sft-manager", true, v1.DefaultRole),
		user("sft-admin", false, v1.SystemAdminRole),
	)
	cli := fake.NewClientBuilder().WithObjects(objs...).WithScheme(scheme.Scheme).Build()
	return &Handler{k8sClient: cli, dbClient: mockDB, accessController: &authority.AccessController{Client: cli}}, cli
}

// createSft posts an SFT job named name that asks for hostpath, as the given user.
func createSft(t *testing.T, h *Handler, userId, name string, hostpath []string) (*CreateSftJobResponse, error) {
	t.Helper()
	exportModel := false
	body, err := json.Marshal(CreateSftJobRequest{
		DisplayName: name, Workspace: "ws1", ModelId: "model-1", DatasetId: "dataset-1",
		ExportModel: &exportModel, Image: "img", NodeCount: 1, GpuCount: 8, Cpu: "80",
		Memory: "1000Gi", SharedMemory: "500Gi", EphemeralStorage: "1000Gi",
		TrainConfig: SftTrainConfig{Peft: "none"}, Hostpath: hostpath,
	})
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/sft/jobs", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, userId)
	c.Set(common.UserName, userId)
	res, err := h.createSftJob(c)
	if err != nil {
		return nil, err
	}
	return res.(*CreateSftJobResponse), nil
}

// The SFT route creates its workload itself, not through the workload create path, so it holds
// the same rule: a member cannot mount a node path the workspace does not declare (403, nothing
// created), may mount a declared one, and the manager and a system administrator, who hold
// workload/privileged in the workspace, may mount any, which the workload records for the webhook.
func TestSftJobHostPathNeedsTheGrant(t *testing.T) {
	h, cli := newSftHostPathHandler(t)
	count := func() int {
		list := &v1.WorkloadList{}
		require.NoError(t, cli.List(t.Context(), list))
		return len(list.Items)
	}

	for _, hostpath := range [][]string{{"/"}, {"/etc"}, {"/shared_nfs/a", "/var/run/docker.sock"}} {
		_, err := createSft(t, h, "sft-member", "refused", hostpath)
		require.True(t, apierrors.IsForbidden(err), "hostpath %v: want 403, got %v", hostpath, err)
		require.Contains(t, err.Error(), "requires the workload/privileged grant")
	}
	require.Equal(t, 0, count(), "a refused SFT workload was created")

	resp, err := createSft(t, h, "sft-member", "declared", []string{"/shared_nfs/sft"})
	require.NoError(t, err)
	stored := &v1.Workload{}
	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: resp.WorkloadId}, stored))
	require.Equal(t, []string{"/shared_nfs/sft"}, stored.Spec.Hostpath)
	require.False(t, v1.HasAnnotation(stored, v1.WorkloadHostPathAuthzAnnotation))

	for _, who := range []string{"sft-manager", "sft-admin"} {
		resp, err = createSft(t, h, who, who, []string{"/"})
		require.NoError(t, err, who)
		require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: resp.WorkloadId}, stored))
		require.Equal(t, v1.TrueStr, v1.GetAnnotation(stored, v1.WorkloadHostPathAuthzAnnotation), who)
	}
}

// The SFT route marks its workload for the volumes that require workload/privileged only when the
// creator holds that grant in the workspace, and refuses a member who names such a volume.
func TestSftJobPrivilegedVolumeNeedsTheGrant(t *testing.T) {
	h, cli := newSftHostPathHandler(t)
	ws := &v1.Workspace{}
	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: "ws1"}, ws))
	ws.Spec.Volumes = append(ws.Spec.Volumes, v1.WorkspaceVolume{
		Id: 2, Type: v1.HOSTPATH, HostPath: "/secret", MountPath: "/secret", RequiresPrivilegedGrant: true,
	})
	require.NoError(t, cli.Update(t.Context(), ws))

	stored := &v1.Workload{}
	resp, err := createSft(t, h, "sft-member", "member", nil)
	require.NoError(t, err)
	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: resp.WorkloadId}, stored))
	require.False(t, v1.HasAnnotation(stored, v1.WorkloadPrivilegedVolumesAuthzAnnotation))

	_, err = createSft(t, h, "sft-member", "member-asks", []string{"/secret"})
	require.True(t, apierrors.IsForbidden(err), "want 403, got %v", err)
	require.Contains(t, err.Error(), "requiresPrivilegedGrant")

	for _, who := range []string{"sft-manager", "sft-admin"} {
		resp, err = createSft(t, h, who, who, nil)
		require.NoError(t, err, who)
		require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: resp.WorkloadId}, stored))
		require.Equal(t, v1.TrueStr, v1.GetAnnotation(stored, v1.WorkloadPrivilegedVolumesAuthzAnnotation), who)
	}
}
