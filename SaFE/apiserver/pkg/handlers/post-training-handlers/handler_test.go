/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

const testRegistry = `
frameworks:
  - name: slime
    default: true
    compiler: slime
    adapter: frameworks/slime
    metrics: {include: ["train/*"], maxPointsPerRun: 1000}
specs:
  - name: qwen3-4B
    match: {architectures: [Qwen3ForCausalLM], hidden_size: 2560}
    evalTp: 1
    frameworks:
      slime:
        methods: {sft: [full], rl: [full]}
        minNodes: {sft-full: 1, rl: 1, rl-separate: 2}
images:
  - {image: "img:1", framework: slime, arch: [gfx950], frameworkCommit: abc, validated: true, default: true, features: [full],
     env: {RAY_EXPERIMENTAL_NOSET_HIP_VISIBLE_DEVICES: "1"}}
gpuArch: {MI355X: gfx950}
network:
  ainic: {env: {USING_AINIC: "1"}}
`

// testContext returns a gin context with a signed-in user.
func testContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	c.Set(common.UserId, "u1")
	c.Set(common.UserName, "alice")
	return c
}

// flavor is an MI355X node flavor.
func flavor() *v1.NodeFlavor {
	return &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
		Cpu:    v1.CpuChip{Quantity: resource.MustParse("256")},
		Memory: resource.MustParse("2048Gi"),
		Gpu:    &v1.GpuChip{Product: "MI355X", ResourceName: "amd.com/gpu", Quantity: resource.MustParse("8")},
	}}
}

// compiled compiles a valid SFT request for the given node count.
func compiled(t *testing.T, nodes int) (*registry.Registry, *recipe.SubmitRequest, *recipe.Result) {
	t.Helper()
	reg, err := registry.Parse([]byte(testRegistry))
	require.NoError(t, err)
	req := &recipe.SubmitRequest{Workspace: "ws", DisplayName: "Qwen3 SFT demo", TrainType: "sft", Method: "full",
		ModelID: "m1", DatasetID: "d1", Resources: recipe.Resources{Nodes: nodes, GpusPerNode: 8, Priority: 1}}
	res := recipe.Compile(&recipe.Input{Registry: reg, Request: req, RunID: "pt-x", RunDir: "/wekafs/custom/models/pt-x",
		Model: recipe.ModelInfo{ID: "m1", Path: "/wekafs/models/m1", Ready: true,
			Config: &v1.ModelConfig{Architectures: []string{"Qwen3ForCausalLM"}, HiddenSize: 2560}},
		Dataset:   &recipe.DatasetInfo{ID: "d1", Type: "sft", Path: "/wekafs/datasets/d1", Ready: true},
		Workspace: recipe.WorkspaceInfo{ID: "ws", GPUModel: "MI355X", Nodes: 4}})
	require.False(t, res.HasErrors(), "%v", res.Diagnostics)
	return reg, req, res
}

func TestNewRunID(t *testing.T) {
	id := newRunID("Qwen3 8B — LoRA SFT, very long display name that keeps going")
	require.True(t, strings.HasPrefix(id, "pt-qwen3-8b-lora-sft-very-long-"), id)
	require.LessOrEqual(t, len(id), 40)
	require.True(t, strings.HasPrefix(newRunID("中文"), "pt-run-"))
}

func TestBuildWorkload(t *testing.T) {
	reg, req, res := compiled(t, 2)
	ws := &workspaceInfo{WorkspaceInfo: recipe.WorkspaceInfo{ID: "ws"}, flavor: flavor()}
	recipeJSON, err := json.Marshal(res.Recipe)
	require.NoError(t, err)
	w, err := buildWorkload(testContext(), reg, req, res, ws, "pt-x", recipeJSON)
	require.NoError(t, err)

	require.Equal(t, "pt-x", w.Name)
	require.Equal(t, "pt-x", w.Labels[v1.PostTrainingRunIdLabel])
	require.Equal(t, v1.PostTrainingRoleTrain, w.Labels[v1.PostTrainingRoleLabel])
	require.Equal(t, "u1", w.Labels[v1.UserIdLabel])
	require.Equal(t, common.RayJobKind, w.Spec.Kind)
	require.Equal(t, 1, w.Spec.MaxRetry)
	require.Equal(t, 86400, *w.Spec.TTLSecondsAfterFinished)
	require.Len(t, w.Spec.Resources, 2)
	require.Equal(t, 1, w.Spec.Resources[0].Replica)
	require.Equal(t, 1, w.Spec.Resources[1].Replica)
	require.Equal(t, "8", w.Spec.Resources[0].GPU)
	require.Equal(t, rdmaResource, w.Spec.Resources[0].RdmaResource)
	require.Equal(t, []string{"img:1", "img:1"}, w.Spec.Images)

	env := w.Spec.Env
	entry, _ := base64.StdEncoding.DecodeString(env[common.RayJobEntrypoint])
	require.Equal(t, launcherEntrypoint, string(entry))
	decoded, _ := base64.StdEncoding.DecodeString(env[recipeEnv])
	require.JSONEq(t, string(recipeJSON), string(decoded))
	require.Equal(t, "0", env["RAY_DEDUP_LOGS"])
	require.Equal(t, "WARN", env["NCCL_DEBUG"])
	require.Equal(t, "1", env["PYTHONUNBUFFERED"])
	require.Equal(t, "1", env["RAY_EXPERIMENTAL_NOSET_HIP_VISIBLE_DEVICES"], "image env")
	require.Equal(t, "1", env["CUDA_DEVICE_MAX_CONNECTIONS"], "plan env")
	require.Equal(t, "1", env["USING_AINIC"], "network env on multi-node runs")
	require.NotContains(t, env, "NCCL_DEBUG_FILE")
}

func TestBuildWorkloadSingleNode(t *testing.T) {
	reg, req, res := compiled(t, 1)
	zero := 0
	req.Resources.MaxRetry = &zero
	res.Image.Env = map[string]string{"NCCL_DEBUG": "INFO"}
	w, err := buildWorkload(testContext(), reg, req, res, &workspaceInfo{flavor: flavor()}, "pt-x", []byte("{}"))
	require.NoError(t, err)
	require.Len(t, w.Spec.Resources, 1)
	require.Empty(t, w.Spec.Resources[0].RdmaResource)
	require.Equal(t, 0, w.Spec.MaxRetry)
	require.NotContains(t, w.Spec.Env, "USING_AINIC")
	require.Equal(t, "/wekafs/custom/models/pt-x/logs/nccl/%h-%p.log", w.Spec.Env["NCCL_DEBUG_FILE"])

	_, err = buildWorkload(testContext(), reg, req, res, &workspaceInfo{}, "pt-x", []byte("{}"))
	require.Error(t, err, "a workspace without a node flavor cannot size the run")
	require.Empty(t, w.Spec.Secrets)

	req.Tracking.Wandb = recipe.Wandb{Enabled: true, SecretID: "wb", Project: "pt"}
	w, err = buildWorkload(testContext(), reg, req, res, &workspaceInfo{flavor: flavor()}, "pt-x", []byte("{}"))
	require.NoError(t, err)
	require.Equal(t, []v1.SecretEntity{{Id: "wb", Type: v1.SecretGeneral, AsEnv: true}}, w.Spec.Secrets,
		"the W&B key reaches the containers as environment variables")

	req.Resources.EphemeralStorage = "70Gi"
	w, err = buildWorkload(testContext(), reg, req, res, &workspaceInfo{flavor: flavor()}, "pt-x", []byte("{}"))
	require.NoError(t, err)
	require.Equal(t, "70Gi", w.Spec.Resources[0].EphemeralStorage, "the request can set the ephemeral storage")
	req.Resources.EphemeralStorage = "-1Gi"
	_, err = buildWorkload(testContext(), reg, req, res, &workspaceInfo{flavor: flavor()}, "pt-x", []byte("{}"))
	require.Error(t, err)
}

func TestRewardSecret(t *testing.T) {
	req := &recipe.SubmitRequest{TrainType: registry.TrainTypeRL, Choices: recipe.Choices{RewardType: "remote",
		RewardRemote: &recipe.RewardRemote{URL: "https://rm.svc/s", SecretID: "rm-key"}}}
	require.Equal(t, "rm-key", rewardSecret(req))
	req.Choices.RewardType = "math"
	require.Empty(t, rewardSecret(req), "only a remote reward sends a key")
	req.Choices.RewardType, req.TrainType = "remote", registry.TrainTypeSFT
	require.Empty(t, rewardSecret(req))
	require.Contains(t, nodeInitScript, "safe_post_training.pth", "Ray workers can import the slime plugins")
}

func TestNodeResource(t *testing.T) {
	r := nodeResource(flavor(), 8)
	require.Equal(t, "8", r.GPU)
	require.Equal(t, "256", r.CPU)
	mem := resource.MustParse(r.Memory)
	shm := resource.MustParse(r.SharedMemory)
	require.Equal(t, mem.Value()/2, shm.Value())

	half := nodeResource(flavor(), 4)
	halfMem := resource.MustParse(half.Memory)
	require.Equal(t, "128", half.CPU, "a run on part of a node requests its GPUs' share")
	require.Equal(t, mem.Value()/2, halfMem.Value())

	viper.Set("workspace.ephemeral_store_reserve_percent", 0.05)
	viper.Set("workload.max_ephemeral_store_percent", 0.6)
	t.Cleanup(func() {
		viper.Set("workspace.ephemeral_store_reserve_percent", nil)
		viper.Set("workload.max_ephemeral_store_percent", nil)
	})
	nf := flavor()
	nf.Spec.ExtendResources = corev1.ResourceList{corev1.ResourceEphemeralStorage: resource.MustParse("1000Gi")}
	require.Equal(t, "600Gi", nodeResource(nf, 8).EphemeralStorage,
		"the ephemeral storage stays within the per-workload maximum, not the 950Gi left after the reserve")
	require.Equal(t, "75Gi", nodeResource(nf, 1).EphemeralStorage, "the maximum is shared by the GPUs")
}

func TestModelPathIn(t *testing.T) {
	m := &v1.Model{Spec: v1.ModelSpec{Source: v1.ModelSource{AccessMode: v1.AccessModeLocal}},
		Status: v1.ModelStatus{Phase: v1.ModelPhaseReady, LocalPaths: []v1.ModelLocalPath{
			{Workspace: "ws", Path: "/wekafs/models/m", Status: v1.LocalPathStatusReady},
			{Workspace: "other", Path: "/x", Status: v1.LocalPathStatusDownloading}}}}
	require.Equal(t, "/wekafs/models/m", modelPathIn(m, "ws"))
	require.Empty(t, modelPathIn(m, "other"))
	info := modelInfo(m, "other")
	require.False(t, info.Ready)
}

func TestRegistryLoading(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewHandler(cli, nil)
	_, err := h.registry(context.Background())
	require.True(t, commonerrors.IsNotFound(err), "no ConfigMap means Post Training is off")

	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: common.PrimusSafeNamespace,
		Name: registry.ConfigMapName}, Data: map[string]string{registry.DataKey: "frameworks: []"}}
	require.NoError(t, cli.Create(context.Background(), cm))
	_, err = h.registry(context.Background())
	require.ErrorContains(t, err, "invalid registry")

	cm.Data[registry.DataKey] = testRegistry
	require.NoError(t, cli.Update(context.Background(), cm))
	reg, err := h.registry(context.Background())
	require.NoError(t, err)
	require.Equal(t, "qwen3-4B", reg.Specs[0].Name)
	require.Empty(t, workspaceUnavailable(reg, "", "MI355X"))
	require.Equal(t, `The workspace GPU "MI325X" has no known architecture`, workspaceUnavailable(reg, "", "MI325X"))
}

func TestPausedError(t *testing.T) {
	reg, err := registry.Parse([]byte(testRegistry))
	require.NoError(t, err)
	require.NoError(t, pausedError(reg))
	reg.Paused = registry.Paused{Enabled: true, Reason: "cluster upgrade until 18:00"}
	err = pausedError(reg)
	require.ErrorContains(t, err, "cluster upgrade until 18:00")
}

func TestListItemJSON(t *testing.T) {
	data, err := json.Marshal(listItem{PostTrainingRun: &dbclient.PostTrainingRun{RunID: "pt-1"}, VerifiedRuns: 2,
		LatestVerifiedRun: "pt-0"})
	require.NoError(t, err)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, "pt-1", got["runId"], "the run's fields stay at the top level")
	require.Equal(t, float64(2), got["verifiedRuns"])
	require.Equal(t, "pt-0", got["latestVerifiedRun"])

	data, err = json.Marshal(listItem{PostTrainingRun: &dbclient.PostTrainingRun{RunID: "pt-2"}})
	require.NoError(t, err)
	require.NotContains(t, string(data), "verifiedRuns", "a recipe that never succeeded carries no label")
}

func TestGetModelNeedsAnID(t *testing.T) {
	_, _, err := evalHandler(t).getModel(context.Background(), "", "ws")
	require.Equal(t, 400, statusCode(err), "a request without a model is the caller's mistake, not a server error")
}
