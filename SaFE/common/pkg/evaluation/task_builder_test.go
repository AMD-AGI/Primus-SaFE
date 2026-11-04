/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package evaluation

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// noSecret fails every Secret read.
func noSecret(context.Context, string) (*corev1.Secret, error) {
	return nil, errors.New("no secret")
}

func TestBuildTaskValidation(t *testing.T) {
	cli := fake.NewClientBuilder().Build()
	for name, req := range map[string]TaskRequest{
		"missing serviceId":   {ServiceType: ServiceTypeRemoteAPI},
		"missing serviceType": {ServiceID: "svc-1"},
		"missing benchmarks":  {ServiceID: "svc-1", ServiceType: ServiceTypeRemoteAPI},
		"invalid benchmarks":  {ServiceID: "svc-1", ServiceType: ServiceTypeRemoteAPI, Benchmarks: "not-json"},
		"missing datasetId":   {ServiceID: "svc-1", ServiceType: ServiceTypeRemoteAPI, Benchmarks: `[{}]`},
	} {
		_, err := BuildTask(context.Background(), cli, nil, noSecret, req)
		assert.Error(t, err, name)
	}
}

func TestBuildTaskLocalWorkload(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Volumes = []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	db := mockclient.NewMockInterface(gomock.NewController(t))
	db.EXPECT().GetDataset(gomock.Any(), "gsm8k").Return(
		&dbclient.Dataset{DatasetType: "evaluation", DisplayName: "gsm8k-test"}, nil)
	cmd := base64.StdEncoding.EncodeToString([]byte("python3 -m sglang.launch_server --served-model-name m-1 --port 8000"))
	db.EXPECT().GetWorkload(gomock.Any(), "pt-eval-1").Return(&dbclient.Workload{
		WorkloadId: "pt-eval-1", DisplayName: "eval service", Cluster: "c1", Workspace: "ws1", EntryPoint: cmd}, nil)
	var stored *dbclient.EvaluationTask
	db.EXPECT().UpsertEvaluationTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, task *dbclient.EvaluationTask) error {
			stored = task
			return nil
		})

	task, err := BuildTask(context.Background(), cli, db, noSecret, TaskRequest{
		Name: "pt-qwen-eval", ServiceID: "pt-eval-1", ServiceType: ServiceTypeLocalWorkload,
		Benchmarks: `[{"datasetId":"gsm8k"}]`, UserID: "u1", PostTrainingRunID: "pt-qwen-abc12", ModelID: "m-1"})
	require.NoError(t, err)
	assert.Equal(t, "c1", task.Cluster)
	assert.Equal(t, "ws1", task.Workspace)
	names := make([]string, 0, len(task.Inputs))
	values := map[string]string{}
	for _, in := range task.Inputs {
		names = append(names, in.Name)
		values[in.Name] = in.Value
	}
	assert.Equal(t, []string{v1.ParameterEvalTaskId, v1.ParameterEvalServiceId, v1.ParameterEvalServiceType,
		v1.ParameterEvalBenchmarks, v1.ParameterModelEndpoint, v1.ParameterModelName, v1.ParameterCluster,
		v1.ParameterWorkspace, v1.ParameterEvalConcurrency}, names)
	assert.Equal(t, "http://pt-eval-1:8000/v1", values[v1.ParameterModelEndpoint])
	assert.Equal(t, "m-1", values[v1.ParameterModelName], "the served model name comes from the entry point")
	assert.Equal(t, "32", values[v1.ParameterEvalConcurrency])
	assert.JSONEq(t, `[{"datasetId":"gsm8k","datasetName":"gsm8k-test","datasetLocalDir":"/wekafs/datasets/gsm8k-test"}]`,
		values[v1.ParameterEvalBenchmarks])
	require.NotNil(t, stored)
	assert.Equal(t, task.TaskID, stored.TaskId)
	assert.Equal(t, sql.NullString{String: "pt-qwen-abc12", Valid: true}, stored.PostTrainingRunId)
	assert.Equal(t, sql.NullString{String: "m-1", Valid: true}, stored.ModelId)
	assert.Equal(t, dbclient.EvaluationTaskStatusPending, stored.Status)
}

func TestServedModelNameFromCmd(t *testing.T) {
	assert.Equal(t, "my-model", ServedModelNameFromCmd("vllm serve --served-model-name my-model --port 8000"))
	assert.Equal(t, "m2", ServedModelNameFromCmd("cmd --served-model-name=m2"))
	assert.Equal(t, "", ServedModelNameFromCmd("vllm serve --port 8000"))
}

func TestServedModelName(t *testing.T) {
	ep := base64.StdEncoding.EncodeToString([]byte("vllm serve --served-model-name my-model"))
	assert.Equal(t, "my-model", servedModelName(ep, sql.NullString{}))

	// From entryPoints array.
	ep2 := base64.StdEncoding.EncodeToString([]byte("cmd --served-model-name arr-model"))
	arr := `["` + ep2 + `"]`
	assert.Equal(t, "arr-model", servedModelName("", sql.NullString{String: arr, Valid: true}))

	// None.
	assert.Equal(t, "", servedModelName("", sql.NullString{}))
}

func TestModelNameFromEnv(t *testing.T) {
	assert.Equal(t, "", modelNameFromEnv(sql.NullString{}))
	assert.Equal(t, "", modelNameFromEnv(sql.NullString{String: "not-json", Valid: true}))
	assert.Equal(t, "Qwen/Q", modelNameFromEnv(sql.NullString{String: `{"PRIMUS_SOURCE_MODEL":"Qwen/Q"}`, Valid: true}))
	assert.Equal(t, "m2", modelNameFromEnv(sql.NullString{String: `{"MODEL_NAME":"m2"}`, Valid: true}))
}
