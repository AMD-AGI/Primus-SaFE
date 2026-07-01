/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestPersistWorkloadStatus_DBDisabled(t *testing.T) {
	viper.Reset()
	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w1"},
		Status: v1.WorkloadStatus{
			Pods: []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodRunning, AdminNodeName: "n1"}},
		},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w.DeepCopy()).WithStatusSubresource(w).Build()
	r := &SyncerReconciler{Client: cl}

	fresh := &v1.Workload{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w1"}, fresh))
	fresh.Status = w.Status
	err := r.persistWorkloadStatus(context.Background(), fresh)
	require.NoError(t, err)

	got := &v1.Workload{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w1"}, got))
	assert.Len(t, got.Status.Pods, 1)
	assert.Empty(t, got.Status.NodeUsage)
}

func TestPersistWorkloadStatus_Offload(t *testing.T) {
	viper.Reset()
	viper.Set("db.enable", true)

	ctrl := gomock.NewController(t)
	mockDB := mockclient.NewMockInterface(ctrl)
	mockDB.EXPECT().BatchUpsertWorkloadPods(gomock.Any(), gomock.Any()).Return(nil)
	mockDB.EXPECT().DeleteWorkloadPodsNotIn(gomock.Any(), "w1", []string{"p1"}).Return(nil)
	mockDB.EXPECT().UpsertWorkloadDispatchNode(gomock.Any(), gomock.Any()).Return(nil)

	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w1", Labels: map[string]string{v1.WorkloadDispatchCntLabel: "1"}},
		Spec: v1.WorkloadSpec{
			Resources: []v1.WorkloadResource{{Replica: 1, CPU: "1", GPU: "1", Memory: "1Gi"}},
		},
		Status: v1.WorkloadStatus{
			Pods:  []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodRunning, AdminNodeName: "n1", ResourceId: 0}},
			Nodes: [][]string{{"n1"}},
			Ranks: [][]string{{"0"}},
		},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w.DeepCopy()).WithStatusSubresource(w).Build()
	r := &SyncerReconciler{Client: cl, dbClient: mockDB}

	fresh := &v1.Workload{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w1"}, fresh))
	fresh.Status = w.Status
	fresh.Labels = w.Labels
	err := r.persistWorkloadStatus(context.Background(), fresh)
	require.NoError(t, err)

	got := &v1.Workload{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w1"}, got))
	assert.Empty(t, got.Status.Pods)
	assert.NotEmpty(t, got.Status.NodeUsage)
}

func TestHydrateWorkloadStatusFromDB(t *testing.T) {
	viper.Reset()
	viper.Set("db.enable", true)

	ctrl := gomock.NewController(t)
	mockDB := mockclient.NewMockInterface(ctrl)
	mockDB.EXPECT().ListWorkloadPods(gomock.Any(), "w1").Return([]*dbclient.WorkloadPod{
		{WorkloadId: "w1", PodId: "p1", ResourceId: 0},
	}, nil)
	mockDB.EXPECT().ListWorkloadDispatchNodes(gomock.Any(), "w1").Return([]*dbclient.WorkloadDispatchNode{
		{WorkloadId: "w1", DispatchIndex: 0},
	}, nil)

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl, dbClient: mockDB}

	got, err := r.getAdminWorkload(context.Background(), "w1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Len(t, got.Status.Pods, 1)
	assert.Equal(t, "p1", got.Status.Pods[0].PodId)
}
