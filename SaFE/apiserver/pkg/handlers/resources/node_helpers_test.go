/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestValidateCreateNodeRequest(t *testing.T) {
	// Missing flavorId.
	assert.Error(t, validateCreateNodeRequest(&view.CreateNodeRequest{}))
	// Missing privateIP.
	assert.Error(t, validateCreateNodeRequest(&view.CreateNodeRequest{FlavorId: "f1"}))
	// Missing sshSecretId.
	assert.Error(t, validateCreateNodeRequest(&view.CreateNodeRequest{FlavorId: "f1", PrivateIP: "1.2.3.4"}))
	// Valid.
	assert.NoError(t, validateCreateNodeRequest(&view.CreateNodeRequest{
		FlavorId: "f1", PrivateIP: "1.2.3.4", SSHSecretId: "s1",
	}))
}

func TestGetPrimusTaints(t *testing.T) {
	taints := []corev1.Taint{
		{Key: v1.PrimusSafePrefix + "gpu-fault", Value: "true", Effect: corev1.TaintEffectNoSchedule},
		{Key: "user-taint", Value: "x", Effect: corev1.TaintEffectNoSchedule},
	}
	result := getPrimusTaints(taints)
	assert.Len(t, result, 1)
	assert.Equal(t, "gpu-fault", result[0].Key)
}

func TestGetNodeCustomerLabels(t *testing.T) {
	in := map[string]string{
		v1.PrimusSafePrefix + "internal": "x",
		v1.KubernetesControlPlane:        "",
		"team":                           "infra",
	}
	out := getNodeCustomerLabels(in)
	assert.Equal(t, map[string]string{"team": "infra"}, out)
}

func TestConvertToNodeBriefResponse(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Spec:       v1.NodeSpec{PrivateIP: "10.0.0.1"},
	}
	resp := convertToNodeBriefResponse(node)
	assert.Equal(t, "node-1", resp.NodeId)
	assert.Equal(t, "10.0.0.1", resp.InternalIP)
}

func TestCvtToListNodeRebootSql(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	query := &view.ListNodeRebootLogRequest{
		SinceTime: time.Now().Add(-time.Hour),
		UntilTime: time.Now(),
		SortBy:    "creation_time",
		Order:     dbclient.DESC,
	}
	sql, orderBy := cvtToListNodeRebootSql(query, node)
	assert.NotNil(t, sql)
	assert.NotEmpty(t, orderBy)

	// Without time range filters.
	sql2, _ := cvtToListNodeRebootSql(&view.ListNodeRebootLogRequest{SortBy: "creation_time", Order: dbclient.DESC}, node)
	assert.NotNil(t, sql2)
}

func TestGenerateWorkloadInfo(t *testing.T) {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-1"},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: "PyTorchJob"},
			Workspace:        "ws-1",
		},
	}
	info := generateWorkloadInfo(wl)
	assert.Equal(t, "wl-1", info.Id)
	assert.Equal(t, "PyTorchJob", info.Kind)
	assert.Equal(t, "ws-1", info.WorkspaceId)
}
