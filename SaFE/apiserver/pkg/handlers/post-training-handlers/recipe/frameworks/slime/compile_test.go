/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package slime

import (
	"encoding/json"
	"fmt"
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

// An SFT request compiles into slime's arguments with the spec's defaults filled in.
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

// An SFT validation set comes from a split or an evaluation dataset, not both.
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

// A table image reads and writes the shared conversion cache, and a custom image neither.
func TestCompileConvertCache(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeSFT)
	res := compile(t, in)
	require.Equal(t, "/wekafs/custom/models/post-training-cache/slime-qwen3-4B-m1-540ea91/torch_dist",
		res.Recipe.Convert.Dir, "runs of a model share its conversion")
	require.True(t, res.Recipe.Provenance.TableImage)

	in.Request.Resources.Image = "docker.io/me/slime:dev"
	res = compile(t, in)
	require.Empty(t, res.Recipe.Convert.CacheKey, "a custom image neither reads nor writes the cache")
	require.Equal(t, "/wekafs/custom/models/pt-1/convert/torch_dist", res.Recipe.Convert.Dir)
	require.False(t, res.Recipe.Provenance.TableImage)
}

// Tracking turns on wandb with its project, group and team, the key only in the environment.
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

// Each reward type compiles into its slime arguments, a custom reward with its path.
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
	for _, u := range []string{"http://localhost:8000/s", "http://RM.localhost./s", "http://127.0.0.1/s",
		"http://[::1]:8000/s", "http://0.0.0.0:8000/s", "http://169.254.169.254/latest/meta-data",
		"http://[fe80::1%25eth0]/s", "http://metadata.google.internal/computeMetadata/v1", "http://[fd00:ec2::254]/s",
		"http://100.100.100.200/s", "http://[::ffff:169.254.169.254]/s", "http://2130706433/s", "http://0x7f.1/s",
		"http://0177.0.0.1/s", "http://224.0.0.1/s", "http://ip6-localhost/s", "http://ip6-loopback:8000/s",
		"http://127\u30020\u30020\u30021:8000/s", "http://169\u3002254\u3002169\u3002254/latest/meta-data",
		"http://\uff11\uff12\uff17.0.0.1/s", "http://\uff4c\uff4f\uff43\uff41\uff4c\uff48\uff4f\uff53\uff54/s"} {
		in.Request.Choices = recipe.Choices{RewardType: "remote", RewardRemote: &recipe.RewardRemote{URL: u}}
		require.Contains(t, errorsOf(in), "cannot be on a loopback, link-local, metadata", u)
	}
	in.Request.Choices = recipe.Choices{RewardType: "remote", RewardRemote: &recipe.RewardRemote{URL: "http://:8000/s"}}
	require.Contains(t, errorsOf(in), "http or https address", "an address needs a host")
	for _, u := range []string{"http://10.0.0.5:8000/score", "https://cafe.babe/score", "http://0x7f.svc/s",
		"http://rm.ws.svc.cluster.local:8000/score", "http://reward_svc.internal:8000/s",
		"http://ab--cd.example.com/s"} {
		args, _ = argsOf(recipe.Choices{RewardType: "remote", RewardRemote: &recipe.RewardRemote{URL: u}})
		require.Contains(t, args, "--rm-url "+u, "private addresses and cluster names stay allowed")
	}
	in.Request.Choices = recipe.Choices{}
	in.Dataset.Fields = []string{"prompt", "answer"}
	require.Contains(t, errorsOf(in), "the dataset has no label field")
}

// A seed is passed to training and rollout, and none means none.
func TestCompileSeed(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	plain := compile(t, in)
	require.NotContains(t, plain.Recipe.FrameworkArgs, "--seed")
	in.Request.Seed = 1234
	res := compile(t, in)
	require.Contains(t, strings.Join(res.Recipe.FrameworkArgs, " "), "--seed 1234 --rollout-seed 1234")
	require.Equal(t, plain.Recipe.Provenance.RecipeHash, res.Recipe.Provenance.RecipeHash,
		"the seed is not part of the recipe hash")

	sft := qwen3Input(t, registry.TrainTypeSFT)
	sft.Request.Seed = 7
	args := strings.Join(compile(t, sft).Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--seed 7")
	require.NotContains(t, args, "--rollout-seed")
}

// Pass rates are logged with eight samples per prompt by default.
func TestCompilePassrate(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	require.Contains(t, compile(t, in).Recipe.FrameworkArgs, "--log-passrate", "eight samples per prompt by default")
	in.Request.Choices.NSamplesPerPrompt = 1
	require.NotContains(t, compile(t, in).Recipe.FrameworkArgs, "--log-passrate")
	reg := in.Registry.Frameworks[0].Metrics
	require.Equal(t, "lower", reg.Better["eval/*-truncated_ratio"])
}

// A colocated GRPO run compiles into slime's colocated arguments.
func TestCompileGRPOColocated(t *testing.T) {
	res := compile(t, qwen3Input(t, registry.TrainTypeRL))
	got := strings.Join(res.Recipe.FrameworkArgs, " ")
	for _, want := range []string{
		"--actor-num-nodes 1 --actor-num-gpus-per-node 8 --colocate --rollout-num-gpus 8",
		"--input-key prompt --label-key label --apply-chat-template --rm-type deepscaler",
		"--num-rollout 3 --rollout-batch-size 32 --n-samples-per-prompt 8 --global-batch-size 256",
		"--rollout-max-response-len 8192 --rollout-temperature 1 --balance-data --rollout-max-prompt-len 32768 " +
			"--log-passrate",
		"--use-kl-loss --kl-loss-coef 0 --kl-loss-type low_var_kl --entropy-coef 0 --eps-clip 0.2 --eps-clip-high 0.28",
		"--rollout-num-gpus-per-engine 2 --sglang-mem-fraction-static 0.7",
		"--lr 1e-06", "--lr-decay-style constant --adam-beta2 0.98",
	} {
		require.Contains(t, got, want)
	}
	require.NotContains(t, got, "--use-tis")
	require.Equal(t, "grpo", res.Recipe.Algorithm)
}

// A separate GRPO rollout compiles without colocation and with TIS when asked.
func TestCompileGRPOSeparate(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	in.Request.Resources.Nodes = 2
	in.Request.Choices.Layout, in.Request.Choices.RolloutGpus, in.Request.Choices.Correction = "separate", 8, "tis"
	res := compile(t, in)
	got := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, got,
		"--actor-num-nodes 1 --actor-num-gpus-per-node 8 --rollout-num-gpus 8 --num-gpus-per-node 8 --hf-checkpoint")
	require.Contains(t, got, "--use-tis --tis-clip-low 0 --tis-clip 2")
	require.NotContains(t, got, "--colocate")
}

// The rollout prompt limit leaves room for a full response and stays out of the choices.
func TestRolloutPromptLimitFollowsTheContext(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	in.Request.Choices.MaxResponseLen = 4096
	in.Request.EvalDatasetID = "e1"
	in.EvalDataset = &recipe.DatasetInfo{ID: "e1", Type: recipe.DatasetTypeRL, Path: "/wekafs/datasets/e1", Ready: true}
	res := compile(t, in)
	args := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--rollout-max-prompt-len 36864", "a prompt must fit the context with a full response")
	require.Contains(t, args, "--eval-prompt-data eval /wekafs/custom/models/pt-1/data/eval.jsonl "+
		"--eval-max-prompt-len 36864")
	require.Zero(t, res.Recipe.Choices.MaxPromptLen, "the derived limit stays out of the choices a copy starts from")

	in.Request.Choices.MaxPromptLen = 1024
	args = strings.Join(compile(t, in).Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--rollout-max-prompt-len 1024")
	require.Contains(t, args, "--eval-max-prompt-len 36864", "the eval set is only cut to what fits the context")

	in = qwen3Input(t, registry.TrainTypeRL)
	in.Model.MaxTokens, in.Model.Config.MaxPositionEmbeddings = 0, 0
	require.NotContains(t, strings.Join(compile(t, in).Recipe.FrameworkArgs, " "), "max-prompt-len",
		"an unknown context keeps every prompt")
}

// The slime rules refuse each invalid choice with the reason.
func TestSlimeRules(t *testing.T) {
	cases := map[string]struct {
		trainType string
		mutate    func(*recipe.Input)
		want      string
	}{
		"divisibility": {registry.TrainTypeSFT, func(in *recipe.Input) { in.Request.Choices.TP = 3 },
			"must be divisible by TP × PP × CP (6)"},
		"model script": {registry.TrainTypeSFT, func(in *recipe.Input) {
			in.Registry.Specs[0].Frameworks[Name].ModelScript = ""
		}, "names no model script"},
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
		"declared context": {registry.TrainTypeRL, func(in *recipe.Input) { in.Model.MaxTokens = 8192 },
			"leave room for the prompt within the model's 8192-token context"},
		"prompt limit": {registry.TrainTypeRL, func(in *recipe.Input) { in.Request.Choices.MaxPromptLen = 40000 },
			"The prompt and response limits (40000 + 8192 tokens) exceed the model's 40960-token context"},
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
		"global batch": {registry.TrainTypeSFT, func(in *recipe.Input) {
			in.Request.Choices.TP, in.Request.Choices.CP, in.Request.Choices.BatchSize = 1, 1, 100
		}, "global batch (100) must be divisible by the data-parallel size (8)"},
		"rl global batch": {registry.TrainTypeRL, func(in *recipe.Input) {
			in.Request.Choices.TP, in.Request.Choices.CP = 1, 1
			in.Request.Choices.BatchSize, in.Request.Choices.NSamplesPerPrompt = 3, 3
		}, "global batch (9) must be divisible by the data-parallel size (8)"},
		"dense ep": {registry.TrainTypeSFT, func(in *recipe.Input) { in.Request.Choices.EP = 2 },
			"EP must be 1 for a model without experts"},
		"min lr above lr": {registry.TrainTypeSFT, func(in *recipe.Input) {
			in.Request.Choices.Lr, in.Request.Choices.MinLr = 1e-5, 1e-4
		}, "The minimum learning rate must be between 0 and the learning rate"},
		"negative min lr": {registry.TrainTypeSFT, func(in *recipe.Input) { in.Request.Choices.MinLr = -1e-6 },
			"The minimum learning rate must be between 0 and the learning rate"},
		"negative epochs": {registry.TrainTypeSFT, func(in *recipe.Input) { in.Request.Choices.NumEpoch = -1 },
			"Train for at least one epoch or step"},
		"negative steps": {registry.TrainTypeSFT, func(in *recipe.Input) {
			in.Request.Choices.NumRollout, in.Request.Choices.NumEpoch = -1, 1
		}, "Train for at least one epoch or step"},
		"negative kl": {registry.TrainTypeRL, func(in *recipe.Input) { in.Request.Choices.KlLossCoef = -0.1 },
			"The KL loss coefficient cannot be negative"},
		"correction": {registry.TrainTypeRL, func(in *recipe.Input) { in.Request.Choices.Correction = "foo" },
			`Correction must be "none" or "tis"`},
		"rl eval dataset": {registry.TrainTypeRL, func(in *recipe.Input) {
			in.Request.EvalDatasetID = "e1"
			in.EvalDataset = &recipe.DatasetInfo{ID: "e1", Type: recipe.DatasetTypeSFT, Path: "/wekafs/datasets/e1",
				Ready: true}
		}, "The evaluation set must be an rlhf dataset, not sft"},
		"framework options": {registry.TrainTypeSFT, func(in *recipe.Input) {
			in.Request.FrameworkOptions = map[string]interface{}{"x": strings.Repeat("a", 20000)}
		}, "Framework options must not exceed 16384 bytes"},
		"freeze patterns": {registry.TrainTypeSFT, func(in *recipe.Input) {
			in.Request.Choices.Freeze = &recipe.FreezeChoice{TrainOnly: make([]string, 70)}
			for i := range in.Request.Choices.Freeze.TrainOnly {
				in.Request.Choices.Freeze.TrainOnly[i] = fmt.Sprintf("layers.%d", i)
			}
		}, "at most 64 patterns to train"},
		"field map": {registry.TrainTypeSFT, func(in *recipe.Input) {
			in.Request.Choices.FieldMap = map[string]string{}
			for i := 0; i < 20; i++ {
				in.Request.Choices.FieldMap[fmt.Sprintf("k%d", i)] = "v"
			}
		}, "at most 16 field mappings"},
		"engine straddles nodes": {registry.TrainTypeRL, func(in *recipe.Input) {
			in.Request.Resources.Nodes, in.Request.Resources.GpusPerNode = 2, 6
			in.Request.Choices.GpusPerEngine = 4
		}, "GPUs per engine (4) must fit within a node of 6 GPUs or span whole nodes"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := qwen3Input(t, c.trainType)
			c.mutate(in)
			require.Contains(t, errorsOf(in), c.want)
		})
	}
}

// Sequence parallel is passed only when chosen.
func TestSequenceParallelFollowsTheChoice(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeSFT)
	require.Contains(t, strings.Join(compile(t, in).Recipe.FrameworkArgs, " "), "--sequence-parallel",
		"on by default with TP above 1")
	off := false
	in.Request.Choices.SequenceParallel = &off
	require.NotContains(t, strings.Join(compile(t, in).Recipe.FrameworkArgs, " "), "--sequence-parallel")
}

// Choices of the other train type leave the recipe hash alone.
func TestTheRecipeHashIgnoresChoicesThatDoNotApply(t *testing.T) {
	sft := qwen3Input(t, registry.TrainTypeSFT)
	want := compile(t, sft).Recipe.Provenance.RecipeHash
	sft.Request.Choices.NSamplesPerPrompt, sft.Request.Choices.MaxResponseLen = 16, 4096
	require.Equal(t, want, compile(t, sft).Recipe.Provenance.RecipeHash, "RL options leave an SFT recipe alone")

	rl := qwen3Input(t, registry.TrainTypeRL)
	want = compile(t, rl).Recipe.Provenance.RecipeHash
	rl.Request.Choices.MinLr, rl.Request.Choices.ValSplit = 1e-7, 0.1
	require.Equal(t, want, compile(t, rl).Recipe.Provenance.RecipeHash, "SFT options leave an RL recipe alone")
}

// With the HF export off the run writes no model.
func TestTurningTheHFExportOffWritesNoModel(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeSFT)
	in.Model.Path = "/wekafs/models/GLM-5.3-BF16"
	in.Model.MaxTokens = 202752
	in.Model.Config = &v1.ModelConfig{Architectures: []string{"GlmMoeDsaForCausalLM"}, ModelType: "glm_moe_dsa",
		NumHiddenLayers: 78, HiddenSize: 6144, NumAttentionHeads: 64, NumKeyValueHeads: 64, NumExperts: 256,
		MaxPositionEmbeddings: 1048576}
	in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeatureFreeze)
	in.Workspace.Nodes, in.Workspace.HostMemoryBytes = 8, 3_000_000_000_000
	in.Request.Resources.Nodes = 2
	off := false
	in.Request.Export.HF = &off
	res := compile(t, in)
	args := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--trainable-only-ckpt-base "+in.Model.Path, "training still loads the frozen parameters")
	require.NotContains(t, args, "--save-hf ", "the training process writes no model")
	require.False(t, res.Recipe.Output.SaveHF)
	require.False(t, res.Recipe.Output.ExportHF)
}

// The minimum learning rate stays at or below a learning rate set lower.
func TestTheMinimumLearningRateStaysBelowALowRate(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeSFT)
	in.Request.Choices.Lr = 5e-7
	args := strings.Join(compile(t, in).Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--lr 5e-07", "the rate the request asks for")
	require.Contains(t, args, "--min-lr 5e-07", "Megatron refuses a minimum above the rate")
}

// Model parallel sizes must fit the model, checked once its structure is known.
func TestModelParallelRules(t *testing.T) {
	messages := func(cfg *v1.ModelConfig, tp, ep int) string {
		var msgs []string
		for _, d := range modelParallelRules(cfg, recipe.Choices{TP: tp, EP: ep}) {
			msgs = append(msgs, d.Message)
		}
		return strings.Join(msgs, "; ")
	}
	require.Empty(t, messages(nil, 3, 3), "a model still being read is checked once its structure is known")
	require.Empty(t, messages(&v1.ModelConfig{NumAttentionHeads: 32, NumKeyValueHeads: 4}, 8, 1),
		"TP may be a multiple of the KV heads")
	require.Contains(t, messages(&v1.ModelConfig{NumAttentionHeads: 20, NumKeyValueHeads: 4}, 8, 1),
		"TP (8) must divide the 20 attention heads")
	require.Contains(t, messages(&v1.ModelConfig{NumAttentionHeads: 32, NumKeyValueHeads: 3}, 2, 1),
		"The KV heads (3) and TP (2) must be multiples of one another")
	require.Contains(t, messages(&v1.ModelConfig{NumAttentionHeads: 64, NumExperts: 256}, 1, 3),
		"EP (3) must divide the 256 experts")
	require.Empty(t, messages(&v1.ModelConfig{NumAttentionHeads: 64, NumExperts: 256}, 1, 8))
}

// RL tells slime the GPUs of a node for placing rollout engines.
func TestRLNamesTheGPUsOfANode(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	in.Request.Resources.GpusPerNode = 4
	args := strings.Join(compile(t, in).Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--num-gpus-per-node 4", "slime places rollout engines by the GPUs of a node")
}

// The options of a model list its defaults and layouts.
func TestOptions(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	opts, reason := recipe.FrameworkOptions(in)
	require.Empty(t, reason)
	require.Equal(t, 1e-6, opts.Defaults.Lr)
	layout := opts.Fields["layout"].Values
	require.False(t, layout[0].Disabled, "a spec that needs no separate rollout offers the colocated one")
	require.False(t, layout[1].Disabled)
}

// PPO compiles a critic that saves apart from the actor.
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
	require.Nil(t, overrides["only_train_params_name_list"], "the critic is always fully trained")

	warmup := 0
	in.Request.Choices.CriticWarmupRollouts = &warmup
	require.NotContains(t, strings.Join(compile(t, in).Recipe.FrameworkArgs, " "), "--num-critic-only-steps")

	grpo := compile(t, qwen3Input(t, registry.TrainTypeRL))
	require.Nil(t, grpo.Recipe.Critic)
	require.NotContains(t, strings.Join(grpo.Recipe.FrameworkArgs, " "), "megatron-config-path")
	require.Zero(t, grpo.Recipe.Choices.CriticLr, "critic choices of GRPO stay out of the recipe hash")
}

// PPO is offered with the critic defaults, and only on an image validated for it.
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

// The larger models' specs match their structures and images, GLM loading HF weights.
func TestSpecsOfTheLargerModels(t *testing.T) {
	reg := chartRegistry(t)
	qwen3 := func(layers, hidden, heads int) *v1.ModelConfig {
		return &v1.ModelConfig{Architectures: []string{"Qwen3ForCausalLM"}, NumHiddenLayers: layers, HiddenSize: hidden,
			NumAttentionHeads: heads, NumKeyValueHeads: 8, RopeTheta: "1000000", MaxPositionEmbeddings: 40960}
	}
	llama := func(layers, hidden, heads, context int) *v1.ModelConfig {
		return &v1.ModelConfig{Architectures: []string{"LlamaForCausalLM"}, NumHiddenLayers: layers, HiddenSize: hidden,
			NumAttentionHeads: heads, NumKeyValueHeads: 8, RopeTheta: "500000", MaxPositionEmbeddings: context}
	}
	glm := &v1.ModelConfig{Architectures: []string{"GlmMoeDsaForCausalLM"}, ModelType: "glm_moe_dsa",
		NumHiddenLayers: 78, HiddenSize: 6144, NumAttentionHeads: 64, NumKeyValueHeads: 64, NumExperts: 256,
		MaxPositionEmbeddings: 1048576}
	for name, cfg := range map[string]*v1.ModelConfig{
		"qwen3-14B": qwen3(40, 5120, 40), "qwen3-32B": qwen3(64, 5120, 64),
		"llama3.1-8B": llama(32, 4096, 32, 131072), "llama3.1-70B": llama(80, 8192, 64, 131072),
		"glm5.3-744B-A40B": glm,
	} {
		spec := reg.MatchSpec(cfg)
		require.NotNil(t, spec, name)
		require.Equal(t, name, spec.Name)
	}
	require.Nil(t, reg.MatchSpec(llama(32, 4096, 32, 8192)), "Llama 3 8B has the 3.1 structure with an 8K context")
	require.Equal(t, registry.Checkpoint{KeepLast: 2, KeepBest: true}, reg.MatchSpec(qwen3(64, 5120, 64)).Checkpoint)
	g := reg.MatchSpec(glm)
	require.Equal(t, []string{recipe.AlgorithmGRPO}, g.Frameworks[Name].Algorithms)
	require.True(t, g.Frameworks[Name].HFDirect, "GLM loads HF weights; Spur never converted them")
	n, _ := g.Frameworks[Name].MinNodesFor(registry.TrainTypeRL, registry.VariantFreeze, true)
	require.Equal(t, 3, n, "frozen-layer GRPO runs on 2 training nodes and 1 rollout node, as J3 did")
}

// A request without an algorithm takes the spec's default.
func TestAnOmittedAlgorithmIsTheSpecsDefault(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	in.Registry.Specs[0].Frameworks[Name].Algorithms = []string{recipe.AlgorithmGRPO}
	require.Equal(t, recipe.AlgorithmGRPO, compile(t, in).Recipe.Choices.AdvantageEstimator)

	in.Registry.Specs[0].Frameworks[Name].Algorithms = []string{recipe.AlgorithmPPO}
	require.Equal(t, recipe.AlgorithmPPO, compile(t, in).Recipe.Choices.AdvantageEstimator,
		"the default keeps to the spec's list")
	in.Request.Choices.AdvantageEstimator = recipe.AlgorithmGRPO
	require.Contains(t, errorsOf(in), "supports only [ppo]")
}

// GLM needs its host memory on every node, against the pod's share.
func TestGLMNeedsItsHostMemoryOnEveryNode(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeSFT)
	in.Model.Path = "/wekafs/models/GLM-5.3-BF16"
	in.Model.MaxTokens = 202752
	in.Model.Config = &v1.ModelConfig{Architectures: []string{"GlmMoeDsaForCausalLM"}, ModelType: "glm_moe_dsa",
		NumHiddenLayers: 78, HiddenSize: 6144, NumAttentionHeads: 64, NumKeyValueHeads: 64, NumExperts: 256,
		MaxPositionEmbeddings: 1048576}
	in.Request.Resources.Nodes = 8
	in.Request.Choices.Freeze = &recipe.FreezeChoice{}
	in.Workspace.Nodes, in.Workspace.HostMemoryBytes = 8, 2_000_000_000_000
	res := recipe.Compile(in)
	var errs []string
	for _, d := range res.Diagnostics {
		if d.Level == recipe.LevelError {
			errs = append(errs, d.Field+": "+d.Message)
		}
	}
	require.Equal(t, []string{"resources.nodes: glm5.3-744B-A40B needs 2.1 TB of host memory per node, and a run " +
		"with 8 GPUs per node gets 2.00 TB in this workspace"}, errs)

	in.Workspace.HostMemoryBytes, in.Workspace.NodeGPUs = 3_000_000_000_000, 8
	in.Request.Resources.GpusPerNode = 4
	half := recipe.Compile(in)
	require.Contains(t, fmt.Sprint(half.Diagnostics), "a run with 4 GPUs per node gets 1.50 TB",
		"a run on half of each node gets half of its memory")

	in.Request.Resources.GpusPerNode = 8
	res = compile(t, in)
	require.Equal(t, 2.1, res.Recipe.MinHostMemoryTB, "the check stage measures every node against it")
	require.Contains(t, warningsOf(res), "has not been verified", "full training without frozen layers")
	require.Nil(t, res.Recipe.Convert, "GLM loads the BF16 HF directory; SFT has no conversion")
	require.Contains(t, strings.Join(res.Recipe.FrameworkArgs, " "),
		"--hf-checkpoint /wekafs/models/GLM-5.3-BF16 --ref-load /wekafs/models/GLM-5.3-BF16")

	in.Workspace.HostMemoryBytes = 0
	compile(t, in)
}

// A GLM FP8 rollout keeps the BF16 HF checkpoint as the reference and for saving.
func TestGLMFP8RolloutKeepsTheHFReference(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	in.Model.Path = "/wekafs/models/GLM-5.3-BF16"
	in.Model.MaxTokens = 202752
	in.Model.Config = &v1.ModelConfig{Architectures: []string{"GlmMoeDsaForCausalLM"}, ModelType: "glm_moe_dsa",
		NumHiddenLayers: 78, HiddenSize: 6144, NumAttentionHeads: 64, NumKeyValueHeads: 64, NumExperts: 256,
		MaxPositionEmbeddings: 1048576}
	in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeatureFP8Rollout,
		registry.FeatureFreeze)
	in.Workspace.Nodes, in.Workspace.HostMemoryBytes = 4, 3_000_000_000_000
	in.Request.Resources.Nodes = 3
	in.Request.Choices.Freeze = &recipe.FreezeChoice{
		TrainOnly: []string{`^(?!.*decoder\.layers\.([0-9]|[1-6][0-9])\.mlp\.experts\.)`}}
	in.Request.Choices.Layout, in.Request.Choices.RolloutGpus = recipe.LayoutSeparate, 8
	in.Request.Choices.RolloutPrecision = recipe.RolloutPrecisionFP8
	in.Request.Choices.Correction = correctionTIS
	res := compile(t, in)
	conv := res.Recipe.Convert
	require.NotNil(t, conv)
	require.True(t, conv.HFDirect && conv.FP8)
	require.Equal(t, 1, conv.Nodes)
	args := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--hf-checkpoint "+filepath.Join(filepath.Dir(conv.Dir), "fp8_hf")+
		" --ref-load /wekafs/models/GLM-5.3-BF16")
	require.Contains(t, args, "--save-hf "+in.RunDir+"/final_model --save-hf-origin-dir /wekafs/models/GLM-5.3-BF16"+
		" --ref-trainable-only", "the export stays BF16 and the reference keeps only the trainable weights")
}

// GLM freezes its early experts by default, and training everything needs more nodes.
func TestGLMFreezesTheEarlyExpertsByDefault(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeSFT)
	in.Model.Path = "/wekafs/models/GLM-5.3-BF16"
	in.Model.MaxTokens = 202752
	in.Model.Config = &v1.ModelConfig{Architectures: []string{"GlmMoeDsaForCausalLM"}, ModelType: "glm_moe_dsa",
		NumHiddenLayers: 78, HiddenSize: 6144, NumAttentionHeads: 64, NumKeyValueHeads: 64, NumExperts: 256,
		MaxPositionEmbeddings: 1048576}
	in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeatureFreeze)
	in.Workspace.Nodes, in.Workspace.HostMemoryBytes = 8, 3_000_000_000_000
	in.Request.Resources.Nodes = 2
	def := `^(?!.*decoder\.layers\.([0-9]|[1-6][0-9])\.mlp\.experts\.)`

	opts, reason := recipe.FrameworkOptions(in)
	require.Empty(t, reason)
	require.Equal(t, []string{def}, opts.Defaults.Freeze.TrainOnly, "the wizard starts from the spec's freezing")
	require.Equal(t, "experts in layers 0–69", opts.Freeze.Label)
	require.Equal(t, 95.91, opts.Freeze.TrainableB)
	require.Equal(t, 8, opts.Freeze.FullNodes, "training every parameter needs 8 nodes")
	require.True(t, opts.Freeze.FullUnverified)

	in.Request.Choices.Freeze = nil
	res := compile(t, in)
	require.Equal(t, []string{def}, res.Recipe.Choices.Freeze.TrainOnly, "an omitted freeze takes the default")
	args := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--only-train-params-name-list "+def+" --trainable-only-ckpt-base "+in.Model.Path+
		" --save-hf "+in.RunDir+"/final_model")
	require.NotContains(t, args, "--ref-trainable-only", "SFT has no reference model")
	require.True(t, res.Recipe.Output.SaveHF)
	require.Equal(t, 1.34, res.Recipe.CheckpointTB)
	require.NotContains(t, warningsOf(res), "has not been verified")

	in.Request.Choices.Freeze = &recipe.FreezeChoice{TrainOnly: []string{"decoder.layers.7[0-7]."}}
	res = compile(t, in)
	require.Zero(t, res.Recipe.CheckpointTB, "the spec measured its default patterns only")
	require.True(t, res.Recipe.Output.SaveHF)

	in.Request.Choices.Freeze = &recipe.FreezeChoice{TrainOnly: []string{"--lr"}}
	require.Contains(t, errorsOf(in), "does not start with -")

	in.Request.Choices.Freeze = &recipe.FreezeChoice{}
	require.Contains(t, errorsOf(in), "This configuration needs at least 8 nodes", "an empty freeze trains everything")
	in.Request.Resources.Nodes = 8
	res = compile(t, in)
	require.NotContains(t, strings.Join(res.Recipe.FrameworkArgs, " "), "--only-train-params-name-list")
	require.False(t, res.Recipe.Output.SaveHF)
	require.Contains(t, warningsOf(res), "has not been verified")
}

// Specs without a node floor keep the recipe hashes of earlier runs.
func TestSpecsWithoutAFloorKeepTheirRecipe(t *testing.T) {
	res := compile(t, qwen3Input(t, registry.TrainTypeSFT))
	data, err := json.Marshal(res.Recipe)
	require.NoError(t, err)
	require.NotContains(t, string(data), "minHostMemoryTB", "the recipe hashes of earlier runs stay the same")
}

// An FP8 rollout serves the weights the convert stage quantizes, and exports start from BF16.
func TestFP8RolloutServesTheQuantizedWeights(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	in.Registry.Specs[0].Frameworks[Name].Rollout = &registry.Rollout{FP8: true}
	in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeatureFP8Rollout)
	in.Request.Choices.RolloutPrecision = recipe.RolloutPrecisionFP8
	res := compile(t, in)
	args := strings.Join(res.Recipe.FrameworkArgs, " ")
	conv := res.Recipe.Convert
	require.Contains(t, args, "--hf-checkpoint "+filepath.Join(filepath.Dir(conv.Dir), "fp8_hf")+" --ref-load "+conv.Dir)
	require.True(t, conv.FP8, "the convert stage makes the FP8 weights next to the conversion")
	require.Equal(t, recipe.RolloutPrecisionFP8, res.Recipe.Rollout["precision"])
	require.Equal(t, "/wekafs/models/Qwen3-4B", res.Recipe.Model.Path, "exports keep starting from the BF16 base")
	require.Contains(t, warningsOf(res), "FP8 rollout without a train-inference correction")

	in.Request.Choices.Correction, in.Request.Choices.KvCacheDtype = correctionTIS, kvCacheFP8
	res = compile(t, in)
	require.NotContains(t, warningsOf(res), "train-inference correction")
	require.Contains(t, strings.Join(res.Recipe.FrameworkArgs, " "), "--sglang-kv-cache-dtype fp8_e4m3")

	in.Request.Choices.KvCacheDtype = "int8"
	var errs []string
	for _, d := range recipe.Compile(in).Diagnostics {
		if d.Level == recipe.LevelError {
			errs = append(errs, d.Field)
		}
	}
	require.Equal(t, []string{"choices.kvCacheDtype"}, errs)

	res = compile(t, qwen3Input(t, registry.TrainTypeRL))
	require.False(t, res.Recipe.Convert.FP8)
	require.Contains(t, strings.Join(res.Recipe.FrameworkArgs, " "), "--hf-checkpoint /wekafs/models/Qwen3-4B ")
	require.Equal(t, "bf16", res.Recipe.Rollout["precision"])
}

// A fully async rollout keeps generating with its concurrency, and is not evaluated during training.
func TestFullyAsyncKeepsGenerating(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeatureAsync)
	in.Request.Resources.Nodes = 2
	in.Request.Choices.Layout, in.Request.Choices.RolloutGpus = recipe.LayoutSeparate, 8
	in.Request.Choices.Schedule = recipe.ScheduleAsync
	in.Request.EvalDatasetID = "e1"
	in.EvalDataset = &recipe.DatasetInfo{ID: "e1", Type: recipe.DatasetTypeRL, Path: "/wekafs/datasets/e1", Ready: true}
	res := compile(t, in)
	args := strings.Join(res.Recipe.FrameworkArgs, " ")
	require.Contains(t, args, "--rollout-function-path "+asyncRolloutFunction+" --skip-eval-before-train")
	require.Contains(t, args, "--sglang-server-concurrency 128")
	require.NotContains(t, args, "--eval-interval")
	require.Equal(t, recipe.ScheduleAsync, res.Recipe.Rollout["schedule"])
	require.Contains(t, warningsOf(res), "Fully async does not evaluate during training")
	require.Contains(t, warningsOf(res), "Fully async without a train-inference correction")
	opts, _ := recipe.FrameworkOptions(qwen3Input(t, registry.TrainTypeRL))
	require.Equal(t, asyncConcurrency, opts.Defaults.Concurrency, "the wizard shows what an async run gets")

	in.Request.Choices.Concurrency, in.Request.Choices.NSamplesPerPrompt = 1, 16
	require.Contains(t, errorsOf(in), "Fully async needs concurrency × rollout engines (1 × 4) of at least the "+
		"samples per prompt (16)")
	in.Request.Choices.Concurrency, in.Request.Choices.Layout = 0, recipe.LayoutColocate
	require.Contains(t, errorsOf(in), "Fully async requires separate training and rollout GPUs")
	in.Request.Choices.Layout, in.Request.Choices.Schedule = recipe.LayoutSeparate, "later"
	require.Contains(t, errorsOf(in), "Schedule must be sync or async")
}

// Mooncake PD splits the rollout into prefill and decode engines over RDMA.
func TestMooncakePDSplitsTheRollout(t *testing.T) {
	in := qwen3Input(t, registry.TrainTypeRL)
	in.Registry.Specs[0].Frameworks[Name].Rollout = &registry.Rollout{PD: true}
	in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeaturePD)
	in.Registry.Network.RdmaDevices = map[string][]string{"c1": {"rdma0", "rdma1"}}
	in.Workspace.Cluster = "c1"
	in.Request.Resources.Nodes = 2
	in.Request.Choices.Layout, in.Request.Choices.RolloutGpus = recipe.LayoutSeparate, 8
	in.Request.Choices.PD = &recipe.PDChoice{Enabled: true}
	res := compile(t, in)
	require.Contains(t, strings.Join(res.Recipe.FrameworkArgs, " "), "--sglang-config /wekafs/custom/models/pt-1/"+
		"sglang.yaml --sglang-disaggregation-transfer-backend mooncake --sglang-disaggregation-ib-device rdma0,rdma1")
	require.Equal(t, "sglang:\n  - name: default\n    update_weights: true\n    server_groups:\n"+
		"      - worker_type: prefill\n        num_gpus: 2\n        num_gpus_per_engine: 2\n"+
		"      - worker_type: decode\n        num_gpus: 6\n        num_gpus_per_engine: 2\n",
		res.Recipe.Files[sglangConfigFile], "one prefill engine by default, the other rollout GPUs decode")
	require.Equal(t, map[string]interface{}{"prefillGpus": 2, "rdmaDevices": []string{"rdma0", "rdma1"}},
		res.Recipe.Rollout["pd"], "the check stage looks for these devices on every node")
	opts, _ := recipe.FrameworkOptions(in)
	require.False(t, opts.Fields["pd"].Values[1].Disabled)
	require.Equal(t, "FP8 rollout isn't enabled for spec qwen3-4B", opts.Fields["rolloutPrecision"].Values[1].Reason)

	in.Request.Choices.PD.PrefillGpus = 3
	require.Contains(t, errorsOf(in), "Mooncake PD needs whole prefill and decode engines of 2 GPUs within the 8")
	in.Request.Choices.PD.PrefillGpus = 0
	in.Registry.Images[0].Features = append(in.Registry.Images[0].Features, registry.FeatureAsync)
	in.Request.Choices.Schedule, in.Request.Choices.Concurrency = recipe.ScheduleAsync, 4
	require.Contains(t, errorsOf(in), "(4 × 1) of at least the samples per prompt (8)",
		"a request needs a prefill and a decode engine, and there is one prefill engine")

	in.Request.Choices.Schedule, in.Request.Choices.Concurrency = "", 0
	in.Request.Choices.Layout = recipe.LayoutColocate
	require.Contains(t, errorsOf(in), "Mooncake PD needs separate training and rollout GPUs")
	in.Request.Choices.Layout = recipe.LayoutSeparate
	in.Registry.Network.RdmaDevices = nil
	require.Contains(t, errorsOf(in), "Mooncake PD needs the RDMA devices of cluster c1 in the registry")
	opts, _ = recipe.FrameworkOptions(in)
	require.Equal(t, "No RDMA devices are registered for cluster c1", opts.Fields["pd"].Values[1].Reason)
}
