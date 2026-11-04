/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package slime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

// chartRegistry parses the chart's default registry and validates its slime image for the test.
func chartRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "..", "..", "charts", "primus-safe", "values.yaml"))
	require.NoError(t, err)
	var values map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &values))
	out, err := yaml.Marshal(values["post_training"].(map[string]interface{})["registry"])
	require.NoError(t, err)
	reg, err := registry.Parse(out)
	require.NoError(t, err)
	reg.Images[0].Validated, reg.Images[0].Default = true, true
	reg.Images[0].Features = []string{registry.FeatureFull, registry.FeatureSeparate}
	return reg
}

// qwen3Input is a Qwen3-4B compilation input on one MI355X node.
func qwen3Input(t *testing.T, trainType string) *recipe.Input {
	t.Helper()
	ds := &recipe.DatasetInfo{ID: "d1", Type: recipe.DatasetTypeSFT, Path: "/wekafs/datasets/d1", Ready: true}
	if trainType == registry.TrainTypeRL {
		ds.Type = recipe.DatasetTypeRL
	}
	return &recipe.Input{
		Registry: chartRegistry(t),
		Request: &recipe.SubmitRequest{Workspace: "ws", TrainType: trainType, Method: registry.MethodFull,
			ModelID: "m1", DatasetID: "d1", Resources: recipe.Resources{Nodes: 1, GpusPerNode: 8}},
		RunID:  "pt-1",
		RunDir: "/wekafs/custom/models/pt-1",
		Model: recipe.ModelInfo{ID: "m1", Path: "/wekafs/models/Qwen3-4B", Ready: true, MaxTokens: 40960,
			Config: &v1.ModelConfig{Architectures: []string{"Qwen3ForCausalLM"}, NumHiddenLayers: 36,
				HiddenSize: 2560, NumAttentionHeads: 32, NumKeyValueHeads: 8, RopeTheta: "1000000",
				MaxPositionEmbeddings: 40960}},
		Dataset:   ds,
		Workspace: recipe.WorkspaceInfo{ID: "ws", GPUModel: "MI355X", Nodes: 4},
	}
}

// compile runs the platform compiler and fails the test on any error diagnostic.
func compile(t *testing.T, in *recipe.Input) *recipe.Result {
	t.Helper()
	res := recipe.Compile(in)
	for _, d := range res.Diagnostics {
		require.NotEqual(t, recipe.LevelError, d.Level, d.Message)
	}
	return res
}

// errorsOf returns the error messages of a compilation.
func errorsOf(in *recipe.Input) string {
	var msgs []string
	for _, d := range recipe.Compile(in).Diagnostics {
		if d.Level == recipe.LevelError {
			msgs = append(msgs, d.Message)
		}
	}
	return strings.Join(msgs, "; ")
}

func TestCompileSFT(t *testing.T) {
	res := compile(t, qwen3Input(t, registry.TrainTypeSFT))
	want := []string{
		"--actor-num-nodes", "1", "--actor-num-gpus-per-node", "8",
		"--hf-checkpoint", "/wekafs/models/Qwen3-4B", "--ref-load", "/wekafs/custom/models/post-training-cache/slime-qwen3-4B-m1-540ea91/torch_dist",
		"--save", "/wekafs/custom/models/pt-1/checkpoints", "--save-interval", "200",
		"--prompt-data", "/wekafs/custom/models/pt-1/data/train.jsonl", "--rollout-shuffle",
		"--input-key", "messages", "--rollout-function-path", "slime.rollout.sft_rollout.generate_rollout",
		"--rollout-batch-size", "128", "--global-batch-size", "128", "--loss-type", "sft_loss",
		"--calculate-per-token-loss", "--disable-compute-advantages-and-returns", "--debug-train-only",
		"--num-epoch", "1", "--use-tensorboard",
		"--tensor-model-parallel-size", "2", "--pipeline-model-parallel-size", "1", "--context-parallel-size", "2",
		"--expert-model-parallel-size", "1", "--expert-tensor-parallel-size", "1", "--sequence-parallel",
		"--recompute-granularity", "full", "--recompute-method", "uniform", "--recompute-num-layers", "1",
		"--use-dynamic-batch-size", "--max-tokens-per-gpu", "9216", "--log-probs-chunk-size", "1024",
		"--optimizer", "adam", "--lr", "1e-05", "--weight-decay", "0.1", "--adam-beta1", "0.9",
		"--lr-decay-style", "cosine", "--min-lr", "1e-06", "--adam-beta2", "0.95", "--lr-warmup-fraction", "0.1",
		"--attention-dropout", "0.0", "--hidden-dropout", "0.0", "--accumulate-allreduce-grads-in-fp32",
		"--attention-softmax-in-fp32", "--attention-backend", "flash", "--no-gradient-accumulation-fusion",
	}
	require.Equal(t, want, res.Recipe.FrameworkArgs)
	require.Equal(t, "scripts/models/qwen3-4B.sh", res.Recipe.Model.Script)
	require.Equal(t, "slime-qwen3-4B-m1-540ea91", res.Recipe.Convert.CacheKey)
	require.Equal(t, 2, res.Recipe.Choices.TP, "spec defaults are filled in")
	require.Equal(t, "1", res.Plan.Env["CUDA_DEVICE_MAX_CONNECTIONS"])
}

func TestCompileSFTValidation(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeSFT)
	in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeatureValLoss)
	in.Request.EvalDatasetID = "v1"
	in.EvalDataset = &recipe.DatasetInfo{ID: "v1", Type: recipe.DatasetTypeSFT, Path: "/wekafs/datasets/v1", Ready: true}
	res := compile(t, in)
	got := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, got, "--eval-interval 200 --eval-prompt-data val /wekafs/custom/models/pt-1/data/eval.jsonl")
	require.Equal(t, &recipe.RecipeVal{Interval: 200, MaxSamples: 256}, res.Recipe.Val)
	require.Equal(t, "/wekafs/datasets/v1", res.Recipe.Data.Eval)

	in.Request.Choices.ValSplit = 0.1
	require.Contains(t, errorsOf(in), "not both")
	in.Request.Choices.ValSplit, in.Request.EvalDatasetID, in.EvalDataset = 0.1, "", nil
	in.Request.Choices.ValMaxSamples = 64
	require.Contains(t, errorsOf(in), "Validation samples (64) must be at least the batch size (128)")
	in.Request.Choices.ValMaxSamples, in.Request.Choices.ValInterval = 256, 150
	res = compile(t, in)
	require.Contains(t, res.Diagnostics[0].Message, "not a multiple of the validation interval")

	plain := compile(t, qwen3Input(t, registry.TrainTypeSFT))
	require.Contains(t, plain.Diagnostics[0].Message, "No validation set")
	require.Nil(t, plain.Recipe.Val)
}

func TestCompileConvertCache(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeSFT)
	res := compile(t, in)
	require.Equal(t, "/wekafs/custom/models/post-training-cache/slime-qwen3-4B-m1-540ea91/torch_dist",
		res.Recipe.Convert.Dir, "runs of a model share its conversion")

	in.Request.Resources.Image = "docker.io/me/slime:dev"
	res = compile(t, in)
	require.Empty(t, res.Recipe.Convert.CacheKey, "a custom image neither reads nor writes the cache")
	require.Equal(t, "/wekafs/custom/models/pt-1/convert/torch_dist", res.Recipe.Convert.Dir)
}

func TestCompileKeepsEvaluationOutOfTheHash(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeSFT)
	plain := compile(t, in)
	require.Nil(t, plain.Recipe.Evaluation)
	in.Request.Evaluation = recipe.EvaluationRequest{Enabled: true, Benchmarks: []string{"gsm8k"}, KeepService: true}
	res := compile(t, in)
	require.Equal(t, &recipe.EvaluationRequest{Enabled: true, Benchmarks: []string{"gsm8k"}, KeepService: true},
		res.Recipe.Evaluation, "the controller reads the evaluation from the recipe snapshot")
	require.Equal(t, plain.Recipe.Provenance.RecipeHash, res.Recipe.Provenance.RecipeHash)
}

func TestCompileWandb(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeSFT)
	plain := compile(t, in)
	require.NotContains(t, plain.Recipe.FrameworkArgs, "--use-wandb")
	in.Request.Tracking.Wandb = recipe.Wandb{Enabled: true, SecretID: "wb", Project: "pt", Team: "lab"}
	in.Secrets = map[string]*recipe.SecretInfo{"wb": {Type: string(v1.SecretGeneral),
		Keys: []string{"WANDB_API_KEY", "WANDB_BASE_URL"}}}
	res := compile(t, in)
	args := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--use-wandb --wandb-project pt --wandb-group pt-1 --wandb-team lab")
	require.NotContains(t, args, "--wandb-key", "the key reaches wandb only as an environment variable")
	require.Equal(t, "off", res.Plan.Env["WANDB_CONSOLE"])
	require.Equal(t, "wb", res.Recipe.Tracking.Wandb.SecretID)
	require.Equal(t, plain.Recipe.Provenance.RecipeHash, res.Recipe.Provenance.RecipeHash)

	for secret, want := range map[*recipe.SecretInfo]string{
		nil: "doesn't exist or you can't read it",
		{Type: string(v1.SecretImage), Keys: []string{"WANDB_API_KEY"}}:               "must be a general secret",
		{Type: string(v1.SecretGeneral), Keys: []string{"WANDB_BASE_URL"}}:            "has no WANDB_API_KEY",
		{Type: string(v1.SecretGeneral), Keys: []string{"WANDB_API_KEY", "HF_TOKEN"}}: "only hold WANDB_ keys",
	} {
		in.Secrets = map[string]*recipe.SecretInfo{"wb": secret}
		require.Contains(t, errorsOf(in), want)
	}
	in.Secrets = nil
	in.Request.Tracking.Wandb = recipe.Wandb{Enabled: true}
	require.Contains(t, errorsOf(in), "Name the W&B project")
	require.Contains(t, errorsOf(in), "Select the secret")
}

func TestCompileGRPOColocated(t *testing.T) {
	res := compile(t, qwen3Input(t, registry.TrainTypeRL))
	got := strings.Join(res.Recipe.FrameworkArgs, " ")
	for _, want := range []string{
		"--actor-num-nodes 1 --actor-num-gpus-per-node 8 --colocate --rollout-num-gpus 8",
		"--input-key prompt --label-key label --apply-chat-template --rm-type deepscaler",
		"--num-rollout 3 --rollout-batch-size 32 --n-samples-per-prompt 8 --global-batch-size 256",
		"--rollout-max-response-len 8192 --rollout-temperature 1 --balance-data --advantage-estimator grpo",
		"--use-kl-loss --kl-loss-coef 0 --kl-loss-type low_var_kl --entropy-coef 0 --eps-clip 0.2 --eps-clip-high 0.28",
		"--rollout-num-gpus-per-engine 2 --sglang-mem-fraction-static 0.7",
		"--lr 1e-06", "--lr-decay-style constant --adam-beta2 0.98",
	} {
		require.Contains(t, got, want)
	}
	require.NotContains(t, got, "--use-tis")
	require.Equal(t, "grpo", res.Recipe.Algorithm)
}

func TestCompileGRPOSeparate(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	in.Request.Resources.Nodes = 2
	in.Request.Choices.Layout, in.Request.Choices.RolloutGpus, in.Request.Choices.Correction = "separate", 8, "tis"
	res := compile(t, in)
	got := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, got, "--actor-num-nodes 1 --actor-num-gpus-per-node 8 --rollout-num-gpus 8 --hf-checkpoint")
	require.Contains(t, got, "--use-tis --tis-clip-low 0 --tis-clip 2")
	require.NotContains(t, got, "--colocate")
}

func TestSlimeRules(t *testing.T) {
	cases := map[string]struct {
		trainType string
		mutate    func(*recipe.Input)
		want      string
	}{
		"divisibility": {registry.TrainTypeSFT, func(in *recipe.Input) { in.Request.Choices.TP = 3 },
			"must be divisible by TP × PP × CP (6)"},
		"engine multiple": {registry.TrainTypeRL, func(in *recipe.Input) { in.Request.Choices.GpusPerEngine = 3 },
			"multiple of GPUs per engine (3)"},
		"whole nodes": {registry.TrainTypeRL, func(in *recipe.Input) {
			in.Request.Resources.Nodes = 2
			in.Request.Choices.Layout, in.Request.Choices.RolloutGpus = "separate", 4
		}, "training must use whole nodes"},
		"async colocate": {registry.TrainTypeRL, func(in *recipe.Input) { in.Request.Choices.Schedule = "async" },
			"Fully async requires separate"},
		"context": {registry.TrainTypeRL, func(in *recipe.Input) { in.Request.Choices.MaxResponseLen = 40960 },
			"40960-token context"},
		"reward": {registry.TrainTypeRL, func(in *recipe.Input) { in.Request.Choices.RewardType = "f1" },
			`Reward type "f1" is not available`},
		"ppo": {registry.TrainTypeRL, func(in *recipe.Input) {
			in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeaturePPO)
			in.Request.Choices.AdvantageEstimator = "ppo"
		}, `Algorithm "ppo" is not available yet`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := qwen3Input(t, c.trainType)
			c.mutate(in)
			require.Contains(t, errorsOf(in), c.want)
		})
	}
}

func TestOptions(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	in.Request.Method = registry.MethodLora
	opts, reason := recipe.FrameworkOptions(in)
	require.Empty(t, reason)
	require.Equal(t, 1e-6, opts.Defaults.Lr)
	layout := opts.Fields["layout"].Values
	require.True(t, layout[0].Disabled)
	require.Equal(t, "RL with LoRA needs a separate rollout", layout[0].Reason)
	require.False(t, layout[1].Disabled)
}
