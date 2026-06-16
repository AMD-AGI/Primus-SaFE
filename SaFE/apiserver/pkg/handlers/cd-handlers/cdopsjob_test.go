/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cdhandlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func paramValue(inputs []v1.Parameter, name string) string {
	for _, p := range inputs {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}

// TestGenerateCDOpsJobLens verifies the Lens deployment path produces expected parameters.
func TestGenerateCDOpsJobLens(t *testing.T) {
	h := &Handler{}
	req := &dbclient.DeploymentRequest{
		Id:         7,
		DeployType: DeployTypeLens,
		EnvConfig:  `{"branch":"dev"}`,
	}
	opsJob, err := h.generateCDOpsJob(context.Background(), req, testUserId, testUserName)
	require.NoError(t, err)
	require.NotNil(t, opsJob)

	assert.Equal(t, v1.OpsJobCDType, opsJob.Spec.Type)
	assert.Equal(t, DeployTypeLens, paramValue(opsJob.Spec.Inputs, v1.ParameterDeployType))
	assert.Equal(t, "dev", paramValue(opsJob.Spec.Inputs, v1.ParameterDeployBranch))
	assert.Equal(t, "lens-cd-config-7", paramValue(opsJob.Spec.Inputs, v1.ParameterLensConfigMap))
	assert.Equal(t, testUserId, opsJob.Labels[v1.UserIdLabel])
}

// TestGenerateCDOpsJobLensDefaultBranch verifies the branch defaults to main when unset.
func TestGenerateCDOpsJobLensDefaultBranch(t *testing.T) {
	h := &Handler{}
	req := &dbclient.DeploymentRequest{
		Id:         8,
		DeployType: DeployTypeLens,
		EnvConfig:  `{}`,
	}
	opsJob, err := h.generateCDOpsJob(context.Background(), req, testUserId, testUserName)
	require.NoError(t, err)
	assert.Equal(t, "main", paramValue(opsJob.Spec.Inputs, v1.ParameterDeployBranch))
}

// TestGenerateCDOpsJobBadConfig verifies invalid JSON config returns an error.
func TestGenerateCDOpsJobBadConfig(t *testing.T) {
	h := &Handler{}
	req := &dbclient.DeploymentRequest{Id: 1, EnvConfig: "not-json"}
	_, err := h.generateCDOpsJob(context.Background(), req, testUserId, testUserName)
	assert.Error(t, err)
}

// TestCreateLensConfigMap verifies a ConfigMap is created with values data and owner ref.
func TestCreateLensConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()

	h := &Handler{Client: fakeClient}
	req := &dbclient.DeploymentRequest{
		Id:        9,
		EnvConfig: `{"control_plane_config":"cp-yaml","data_plane_config":"dp-yaml"}`,
	}
	opsJob := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "cd-job-9"}}

	err := h.createLensConfigMap(context.Background(), req, opsJob)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = fakeClient.Get(context.Background(), ctrlclient.ObjectKey{
		Name:      "lens-cd-config-9",
		Namespace: corev1.NamespaceDefault,
	}, cm)
	require.NoError(t, err)
	assert.Equal(t, "cp-yaml", cm.Data["cp-values.yaml"])
	assert.Equal(t, "dp-yaml", cm.Data["dp-values.yaml"])
}
