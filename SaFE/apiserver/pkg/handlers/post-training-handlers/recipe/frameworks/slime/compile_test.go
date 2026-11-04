/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package slime

import (
	"encoding/json"
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

// warningsOf returns the warning messages of a compilation result.
func warningsOf(res *recipe.Result) string {
	var msgs []string
	for _, d := range res.Diagnostics {
		if d.Level == recipe.LevelWarning {
			msgs = append(msgs, d.Message)
		}
	}
	return strings.Join(msgs, "; ")
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

func TestCompileRewards(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	argsOf := func(c recipe.Choices) (string, map[string]string) {
		in.Request.Choices = c
		res := compile(t, in)
		return strings.Join(res.Recipe.FrameworkArgs, " "), res.Plan.Env
	}
	args, _ := argsOf(recipe.Choices{})
	require.Contains(t, args, "--rm-type deepscaler")
	args, _ = argsOf(recipe.Choices{RewardType: "f1"})
	require.Contains(t, args, "--rm-type f1")

	args, env := argsOf(recipe.Choices{RewardType: "custom", RewardHook: "/wekafs/ws/rm/judge.py:score"})
	require.Contains(t, args, "--custom-rm-path "+customRewardPlugin)
	require.NotContains(t, args, "--rm-type")
	require.Equal(t, "/wekafs/ws/rm/judge.py:score", env[customRewardEnv])

	remote := &recipe.RewardRemote{URL: "http://rm.svc:8000/score", RewardKey: "score"}
	args, _ = argsOf(recipe.Choices{RewardType: "remote", RewardRemote: remote})
	require.Contains(t, args, "--rm-type remote_rm --rm-url http://rm.svc:8000/score --reward-key score")

	remote.SecretID = "rm-key"
	in.Secrets = map[string]*recipe.SecretInfo{"rm-key": {Type: string(v1.SecretGeneral), Keys: []string{rewardKeyEnv}}}
	args, _ = argsOf(recipe.Choices{RewardType: "remote", RewardRemote: remote})
	require.Contains(t, args, "--custom-rm-path "+remoteRewardPlugin+" --rm-url http://rm.svc:8000/score")
	require.NotContains(t, args, "PT_RM_API_KEY")

	for c, want := range map[*recipe.Choices]string{
		{RewardType: "custom", RewardHook: "rm/judge.py"}:                                       "absolute path of the Python file",
		{RewardType: "remote", RewardRemote: &recipe.RewardRemote{URL: "rm.svc:8000"}}:          "http or https address",
		{RewardType: "remote", RewardRemote: &recipe.RewardRemote{URL: "https://u:p@rm.svc/s"}}: "Keep credentials out",
		{RewardType: "remote"}: "http or https address",
		{RewardType: "remote", RewardRemote: &recipe.RewardRemote{URL: "https://rm.svc/s", SecretID: "other"}}: "doesn't exist",
		{RewardType: "math", FieldMap: map[string]string{"prompt": "question"}}:                                "map the dataset's answer or label field",
	} {
		in.Request.Choices = *c
		require.Contains(t, errorsOf(in), want)
	}
	in.Request.Choices = recipe.Choices{}
	in.Dataset.Fields = []string{"prompt", "answer"}
	require.Contains(t, errorsOf(in), "the dataset has no label field")
}

func TestCompileSeed(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	plain := compile(t, in)
	require.NotContains(t, plain.Recipe.FrameworkArgs, "--seed")
	in.Request.Seed = 1234
	res := compile(t, in)
	require.Contains(t, strings.Join(res.Recipe.FrameworkArgs, " "), "--seed 1234 --rollout-seed 1234")
	require.Equal(t, plain.Recipe.Provenance.RecipeHash, res.Recipe.Provenance.RecipeHash,
		"the variants of an experiment share a seed, which is not part of the recipe hash")

	sft := qwen3Input(t, registry.TrainTypeSFT)
	sft.Request.Seed = 7
	args := strings.Join(compile(t, sft).Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--seed 7")
	require.NotContains(t, args, "--rollout-seed")
}

func TestCompilePassrate(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	require.Contains(t, compile(t, in).Recipe.FrameworkArgs, "--log-passrate", "eight samples per prompt by default")
	in.Request.Choices.NSamplesPerPrompt = 1
	require.NotContains(t, compile(t, in).Recipe.FrameworkArgs, "--log-passrate")
	reg := in.Registry.Frameworks[0].Metrics
	require.Equal(t, "lower", reg.Better["eval/*-truncated_ratio"])
	require.Equal(t, 4, reg.Samples.RolloutGroups)

	res := compile(t, in)
	args := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--custom-rollout-log-function-path "+sampleRolloutPlugin)
	require.Contains(t, args, "--custom-eval-rollout-log-function-path "+sampleEvalPlugin)
	require.Equal(t, 8, res.Recipe.Metrics.Samples.EvalPerSet, "the launcher and the plugin read the limits from the recipe")
	off := *reg.Samples
	off.RolloutGroups, off.EvalPerSet = 0, 0
	in.Registry.Frameworks[0].Metrics.Samples = &off
	require.NotContains(t, strings.Join(compile(t, in).Recipe.FrameworkArgs, " "), "sample_log",
		"a cluster that turns samples off gets no hooks")
	require.Nil(t, compile(t, qwen3Input(t, registry.TrainTypeSFT)).Recipe.Metrics.Samples)
}

func TestCompileGRPOColocated(t *testing.T) {
	res := compile(t, qwen3Input(t, registry.TrainTypeRL))
	got := strings.Join(res.Recipe.FrameworkArgs, " ")
	for _, want := range []string{
		"--actor-num-nodes 1 --actor-num-gpus-per-node 8 --colocate --rollout-num-gpus 8",
		"--input-key prompt --label-key label --apply-chat-template --rm-type deepscaler",
		"--num-rollout 3 --rollout-batch-size 32 --n-samples-per-prompt 8 --global-batch-size 256",
		"--rollout-max-response-len 8192 --rollout-temperature 1 --balance-data --log-passrate",
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
		"reward": {registry.TrainTypeRL, func(in *recipe.Input) { in.Request.Choices.RewardType = "gpqa" },
			`Reward type "gpqa" is not available`},
		"ppo async": {registry.TrainTypeRL, func(in *recipe.Input) {
			in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeaturePPO)
			in.Request.Choices.AdvantageEstimator, in.Request.Choices.Schedule = "ppo", "async"
		}, "Fully async supports GRPO only"},
		"critic lr": {registry.TrainTypeRL, func(in *recipe.Input) {
			in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeaturePPO)
			in.Request.Choices.AdvantageEstimator, in.Request.Choices.CriticLr = "ppo", -1
		}, "The critic learning rate must be positive"},
		"algorithm": {registry.TrainTypeRL, func(in *recipe.Input) { in.Request.Choices.AdvantageEstimator = "dpo" },
			`Algorithm "dpo" is not available`},
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

func TestCompilePPOCritic(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeaturePPO)
	in.Request.Choices.AdvantageEstimator = recipe.AlgorithmPPO
	res := compile(t, in)
	args := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--advantage-estimator ppo --megatron-config-path "+in.RunDir+"/megatron.yaml "+
		"--num-critic-only-steps 1")
	critic := res.Recipe.Critic
	require.Equal(t, defaultCriticLr, critic["lr"])
	overrides := critic["overrides"].(map[string]interface{})
	require.Equal(t, in.RunDir+"/critic_checkpoints", overrides["save"], "the critic must not overwrite the actor")
	require.Contains(t, overrides, "only_train_params_name_list")
	require.Nil(t, overrides["custom_model_provider_path"], "the critic is always fully trained")

	warmup := 0
	in.Request.Choices.CriticWarmupRollouts = &warmup
	require.NotContains(t, strings.Join(compile(t, in).Recipe.FrameworkArgs, " "), "--num-critic-only-steps")

	grpo := compile(t, qwen3Input(t, registry.TrainTypeRL))
	require.Nil(t, grpo.Recipe.Critic)
	require.NotContains(t, strings.Join(grpo.Recipe.FrameworkArgs, " "), "megatron-config-path")
	require.Zero(t, grpo.Recipe.Choices.CriticLr, "critic choices of GRPO stay out of the recipe hash")
}

func TestPPOOption(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	options := func(in *recipe.Input) *recipe.Options {
		opts, reason := recipe.FrameworkOptions(in)
		require.Empty(t, reason)
		return opts
	}
	ppoOf := func(in *recipe.Input) recipe.OptionValue { return options(in).Fields["advantageEstimator"].Values[1] }
	require.Equal(t, "The image has not been validated for PPO", ppoOf(in).Reason)
	in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeaturePPO)
	require.False(t, ppoOf(in).Disabled)
	defaults := options(in).Defaults
	require.Equal(t, defaultCriticLr, defaults.CriticLr, "the wizard shows the critic defaults before PPO is chosen")
	require.Equal(t, defaultCriticWarmup, *defaults.CriticWarmupRollouts)
}

func TestCompileLoraSFT(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeSFT)
	in.Registry.Images[0].Validated = true
	in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeatureLora,
		registry.FeatureFull)
	in.Request.Method = registry.MethodLora
	res := compile(t, in)
	args := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--custom-model-provider-path "+loraPlugin+" --only-train-params-name-list adapter "+
		"--custom-config-path "+in.RunDir+"/plugins.yaml")
	var cfg struct {
		Lora recipe.LoraChoice `json:"pt_lora"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Recipe.Files["plugins.yaml"]), &cfg))
	require.Equal(t, 16, cfg.Lora.Rank)
	require.EqualValues(t, 32, cfg.Lora.Alpha)
	require.Equal(t, []string{"linear_qkv", "linear_proj", "linear_fc1", "linear_fc2"}, cfg.Lora.Targets,
		"every target module of the spec by default")
	require.Equal(t, cfg.Lora, *res.Recipe.Lora, "the export reads rank and alpha from the recipe")
	require.Contains(t, warningsOf(res), "has not been verified")

	full := compile(t, qwen3Input(t, registry.TrainTypeSFT))
	require.Nil(t, full.Recipe.Lora)
	require.NotContains(t, strings.Join(full.Recipe.FrameworkArgs, " "), "custom-model-provider-path")
}

func TestLoraRules(t *testing.T) {
	cases := map[string]struct {
		lora recipe.LoraChoice
		want string
	}{
		"rank":   {recipe.LoraChoice{Rank: 300}, "The LoRA rank must be between 1 and 256"},
		"target": {recipe.LoraChoice{Targets: []string{"linear_q_down_proj"}}, "does not allow LoRA on linear_q_down_proj"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := qwen3Input(t, registry.TrainTypeSFT)
			in.Registry.Images[0].Validated = true
			in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeatureLora)
			in.Request.Method = registry.MethodLora
			lora := c.lora
			in.Request.Choices.Lora = &lora
			require.Contains(t, errorsOf(in), c.want)
		})
	}
}
