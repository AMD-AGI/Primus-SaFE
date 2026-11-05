/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package slime is the slime compiler plugin. Its defaults and checks follow slime's experiment configurator
// (docs/_static/js/experiment-engine.js) and the upstream Qwen3 example scripts, adjusted for ROCm as in
// scripts/run-qwen3-4B-amd.sh.
package slime

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

const (
	// Name is the compiler name the registry uses for slime.
	Name = "slime"

	sftRolloutFunction = "slime.rollout.sft_rollout.generate_rollout"
	defaultRewardType  = "deepscaler"
	rewardCustom       = "custom"
	rewardRemote       = "remote"
	// rewardKeyEnv is the reward service key a remote reward's secret holds; the adapter's plugin sends it as a
	// bearer token.
	rewardKeyEnv = "PT_RM_API_KEY"
	// remoteRewardPlugin and customRewardPlugin are the slime adapter's reward functions: the upstream remote
	// reward with an Authorization header, and a loader for a user's reward file named by customRewardEnv.
	remoteRewardPlugin = "safe_post_training.frameworks.slime.plugins.remote_rm.remote_rm"
	customRewardPlugin = "safe_post_training.frameworks.slime.plugins.custom_rm.custom_rm"
	customRewardEnv    = "PT_CUSTOM_RM"
	// sampleRolloutPlugin and sampleEvalPlugin write the sampled responses for the launcher to report.
	sampleRolloutPlugin = "safe_post_training.frameworks.slime.plugins.sample_log.log_rollout"
	sampleEvalPlugin    = "safe_post_training.frameworks.slime.plugins.sample_log.log_eval"
	defaultMaxTokens    = 9216
	defaultResponseLen  = 8192
	defaultValSamples   = 256
	correctionTIS       = "tis"
	// fp8Dir holds the blockwise FP8 HF weights an FP8 rollout serves, next to the conversion's torch_dist.
	fp8Dir     = "fp8_hf"
	kvCacheFP8 = "fp8_e4m3"
	// asyncRolloutFunction keeps rollouts generating across weight updates; asyncConcurrency is slime's
	// configurator default for it.
	asyncRolloutFunction = "slime.rollout.fully_async_rollout.generate_rollout_fully_async"
	asyncConcurrency     = 128
	maxConcurrency       = 8192
	// sglangConfigFile splits the rollout GPUs into Mooncake prefill and decode engines.
	sglangConfigFile = "sglang.yaml"
	// valDatasetName names the SFT validation set, so its loss is reported as eval/val/loss.
	valDatasetName = "val"

	// megatronConfigFile holds the PPO critic's own settings, which the adapter writes into the run directory.
	megatronConfigFile  = "megatron.yaml"
	criticCheckpoints   = "critic_checkpoints"
	defaultCriticLr     = 1e-5
	defaultCriticWarmup = 1

	// loraPlugin builds slime's model with Megatron-Bridge LoRA on the target modules; pluginsConfigFile hands it
	// the LoRA settings as the pt_lora argument, and loraTrainable leaves only the adapters trainable.
	loraPlugin        = "safe_post_training.frameworks.slime.plugins.lora.model_provider"
	pluginsConfigFile = "plugins.yaml"
	loraTrainable     = "adapter"
	defaultLoraRank   = 16
	defaultLoraAlpha  = 32
	expertModeShared  = "shared"
)

func init() {
	recipe.Register(plugin{})
}

// plugin implements recipe.Framework for slime.
type plugin struct{}

// Name returns the compiler name.
func (plugin) Name() string { return Name }

// Options returns the wizard defaults and the reward, layout and algorithm values for the request's train type.
func (p plugin) Options(ctx *recipe.Context) *recipe.Options {
	opts := &recipe.Options{Defaults: p.normalize(ctx), Fields: map[string]recipe.FieldOptions{}}
	if ctx.Request.TrainType != registry.TrainTypeRL {
		return opts
	}
	opts.Fields["rewardType"] = recipe.FieldOptions{Values: []recipe.OptionValue{
		{Value: defaultRewardType, Label: "DeepScaleR (math answer)"},
		{Value: "dapo", Label: "DAPO (math answer)"},
		{Value: "math", Label: "Math (equivalent answer)"},
		{Value: "f1", Label: "F1 (reference text)"},
		{Value: rewardCustom, Label: "Custom function"},
		{Value: rewardRemote, Label: "Remote service"},
	}}
	sep := recipe.OptionValue{Value: recipe.LayoutSeparate, Label: "Disaggregated"}
	if !ctx.Image.HasFeature(registry.FeatureSeparate) {
		sep.Disabled, sep.Reason = true, "The image has not been validated for a separate rollout"
	}
	colocate := recipe.OptionValue{Value: recipe.LayoutColocate, Label: "Colocated"}
	if ctx.Request.Method == registry.MethodLora {
		colocate.Disabled, colocate.Reason = true, "RL with LoRA needs a separate rollout"
	} else if fs := ctx.FrameworkSpec; fs.Rollout != nil && fs.Rollout.RequireSeparate {
		colocate.Disabled, colocate.Reason = true, ctx.Spec.Name+" needs a separate rollout"
	}
	opts.Fields["layout"] = recipe.FieldOptions{Values: []recipe.OptionValue{colocate, sep}}
	ppo := recipe.OptionValue{Value: recipe.AlgorithmPPO, Label: "PPO"}
	if fs := ctx.FrameworkSpec; len(fs.Algorithms) > 0 && !slices.Contains(fs.Algorithms, recipe.AlgorithmPPO) {
		ppo.Disabled, ppo.Reason = true, ctx.Spec.Name+" supports GRPO only"
	} else if !ctx.Image.HasFeature(registry.FeaturePPO) {
		ppo.Disabled, ppo.Reason = true, "The image has not been validated for PPO"
	}
	opts.Fields["advantageEstimator"] = recipe.FieldOptions{Values: []recipe.OptionValue{
		{Value: recipe.AlgorithmGRPO, Label: "GRPO"}, ppo,
	}}
	rolloutOptions(ctx, opts)
	if opts.Defaults.CriticLr == 0 {
		opts.Defaults.CriticLr = defaultCriticLr
	}
	if opts.Defaults.Concurrency == 0 {
		opts.Defaults.Concurrency = asyncConcurrency
	}
	if opts.Defaults.CriticWarmupRollouts == nil {
		warmup := defaultCriticWarmup
		opts.Defaults.CriticWarmupRollouts = &warmup
	}
	return opts
}

// rolloutOptions lists the rollout precision, schedule and Mooncake PD values with the reasons for the disabled ones:
// what the spec, the image and the cluster's registered RDMA devices allow.
func rolloutOptions(ctx *recipe.Context, opts *recipe.Options) {
	fs, img := ctx.FrameworkSpec, ctx.Image
	fp8 := recipe.OptionValue{Value: recipe.RolloutPrecisionFP8, Label: "FP8"}
	switch {
	case fs.Rollout == nil || !fs.Rollout.FP8:
		fp8.Disabled, fp8.Reason = true, "FP8 rollout isn't enabled for spec "+ctx.Spec.Name
	case !img.HasFeature(registry.FeatureFP8Rollout):
		fp8.Disabled, fp8.Reason = true, "The image has not been validated for FP8 rollout"
	}
	opts.Fields["rolloutPrecision"] = recipe.FieldOptions{Values: []recipe.OptionValue{
		{Value: "bf16", Label: "BF16"}, fp8,
	}}
	async := recipe.OptionValue{Value: recipe.ScheduleAsync, Label: "Fully async"}
	if !img.HasFeature(registry.FeatureAsync) {
		async.Disabled, async.Reason = true, "The image has not been validated for fully async"
	}
	opts.Fields["schedule"] = recipe.FieldOptions{Values: []recipe.OptionValue{{Value: "sync", Label: "Sync"}, async}}
	pd := recipe.OptionValue{Value: "on", Label: "On"}
	switch cluster := ctx.Input.Workspace.Cluster; {
	case fs.Rollout == nil || !fs.Rollout.PD:
		pd.Disabled, pd.Reason = true, "Mooncake PD isn't enabled for spec "+ctx.Spec.Name
	case !img.HasFeature(registry.FeaturePD):
		pd.Disabled, pd.Reason = true, "The image has not been validated for Mooncake PD"
	case len(ctx.Input.Registry.Network.RdmaDevices[cluster]) == 0:
		pd.Disabled, pd.Reason = true, "No RDMA devices are registered for cluster "+cluster
	}
	opts.Fields["pd"] = recipe.FieldOptions{Values: []recipe.OptionValue{{Value: "off", Label: "Off"}, pd}}
}

// normalize returns the request choices with slime's defaults filled in; the result is what the recipe stores and
// the recipe hash covers.
func (plugin) normalize(ctx *recipe.Context) recipe.Choices {
	c := ctx.Request.Choices
	d := parallelDefaults(ctx.FrameworkSpec, ctx.Arch)
	fill := func(v *int, def int) {
		if *v == 0 {
			*v = def
		}
	}
	fill(&c.TP, d["tp"])
	fill(&c.PP, d["pp"])
	fill(&c.CP, d["cp"])
	fill(&c.EP, d["ep"])
	if d["maxTokensPerGpu"] > 0 {
		fill(&c.MaxTokensPerGpu, d["maxTokensPerGpu"])
	}
	fill(&c.MaxTokensPerGpu, defaultMaxTokens)
	c.Lora = loraDefaults(ctx)
	if ctx.Request.TrainType != registry.TrainTypeRL {
		if c.NumRollout == 0 && c.NumEpoch == 0 {
			c.NumEpoch = 1
		}
		fill(&c.BatchSize, 128)
		fill(&c.SaveInterval, 200)
		if c.Lr == 0 {
			c.Lr = 1e-5
		}
		if c.MinLr == 0 {
			c.MinLr = 1e-6
		}
		if len(c.FieldMap) == 0 {
			c.FieldMap = map[string]string{"instruction": "instruction", "input": "input", "output": "output"}
		}
		if hasValidation(ctx.Request) {
			fill(&c.ValInterval, c.SaveInterval)
			fill(&c.ValMaxSamples, max(defaultValSamples, c.BatchSize))
		}
		return c
	}
	fill(&c.NumRollout, 3)
	fill(&c.BatchSize, 32)
	fill(&c.NSamplesPerPrompt, 8)
	fill(&c.SaveInterval, 20)
	if c.Schedule == recipe.ScheduleAsync {
		fill(&c.Concurrency, asyncConcurrency)
	} else if ctx.Request.EvalDatasetID != "" {
		fill(&c.EvalInterval, c.SaveInterval)
	}
	fill(&c.MaxResponseLen, defaultResponseLen)
	fill(&c.GpusPerEngine, d["gpusPerEngine"])
	fill(&c.GpusPerEngine, 1)
	if c.PD != nil && c.PD.Enabled {
		pd := *c.PD
		fill(&pd.PrefillGpus, c.GpusPerEngine)
		c.PD = &pd
	}
	if c.Lr == 0 {
		c.Lr = 1e-6
	}
	if c.AdvantageEstimator == "" {
		c.AdvantageEstimator = recipe.AlgorithmGRPO
		if algs := ctx.FrameworkSpec.Algorithms; len(algs) > 0 && !slices.Contains(algs, c.AdvantageEstimator) {
			c.AdvantageEstimator = algs[0]
		}
	}
	if c.AdvantageEstimator == recipe.AlgorithmPPO {
		if c.CriticLr == 0 {
			c.CriticLr = defaultCriticLr
		}
		if c.CriticWarmupRollouts == nil {
			warmup := defaultCriticWarmup
			c.CriticWarmupRollouts = &warmup
		}
	} else {
		c.CriticLr, c.CriticWarmupRollouts = 0, nil
	}
	if c.RewardType == "" {
		c.RewardType = defaultRewardType
	}
	if c.Layout == "" {
		c.Layout = recipe.LayoutColocate
	}
	if c.UseKlLoss == nil {
		on := true
		c.UseKlLoss = &on
	}
	if len(c.FieldMap) == 0 {
		c.FieldMap = map[string]string{"prompt": "prompt", "label": "label"}
	}
	return c
}

// loraDefaults returns the LoRA choices of a LoRA request with the defaults filled in: rank 16, alpha 32, every
// target module of the spec and, with expert layers, one adapter shared by the experts of a layer. Other methods
// have none.
func loraDefaults(ctx *recipe.Context) *recipe.LoraChoice {
	if ctx.Request.Method != registry.MethodLora {
		return nil
	}
	l := recipe.LoraChoice{}
	if ctx.Request.Choices.Lora != nil {
		l = *ctx.Request.Choices.Lora
	}
	if l.Rank == 0 {
		l.Rank = defaultLoraRank
	}
	if l.Alpha == 0 {
		l.Alpha = defaultLoraAlpha
	}
	if len(l.Targets) == 0 && ctx.FrameworkSpec.Lora != nil {
		l.Targets = append([]string(nil), ctx.FrameworkSpec.Lora.Targets...)
	}
	if l.Experts && l.ExpertMode == "" {
		l.ExpertMode = expertModeShared
	}
	return &l
}

// loraConfig renders plugins.yaml, written as JSON, which YAML reads; slime sets each entry as an argument. With
// expert layers it names the spec's expert targets.
func loraConfig(l *recipe.LoraChoice, spec *registry.Lora) string {
	cfg := map[string]interface{}{
		"rank": l.Rank, "alpha": l.Alpha, "dropout": l.Dropout, "targets": l.Targets,
		"experts": l.Experts, "expertMode": l.ExpertMode,
	}
	if l.Experts && spec != nil {
		cfg["expertTargets"] = spec.ExpertTargets
	}
	data, _ := json.MarshalIndent(map[string]interface{}{"pt_lora": cfg}, "", "  ")
	return string(data) + "\n"
}

// parallelDefaults reads the spec's integer defaults, with the architecture's nested map overriding them.
func parallelDefaults(fs *registry.FrameworkSpec, arch string) map[string]int {
	out := map[string]int{"tp": 1, "pp": 1, "cp": 1, "ep": 1}
	read := func(m map[string]interface{}) {
		for k, v := range m {
			if f, ok := v.(float64); ok {
				out[k] = int(f)
			}
		}
	}
	read(fs.Defaults)
	if over, ok := fs.Defaults[arch].(map[string]interface{}); ok {
		read(over)
	}
	return out
}

// Compile generates the train.py arguments. The launcher prepends the model script's arguments and adds --load when
// a checkpoint exists.
func (p plugin) Compile(ctx *recipe.Context) (*recipe.FrameworkPlan, error) {
	c := p.normalize(ctx)
	in, l := ctx.Input, ctx.Layout
	env := map[string]string{"CUDA_DEVICE_MAX_CONNECTIONS": "1"}
	rl := ctx.Request.TrainType == registry.TrainTypeRL
	fp8 := rl && c.RolloutPrecision == recipe.RolloutPrecisionFP8
	hfDirect := ctx.FrameworkSpec.HFDirect
	convert, hf, refLoad := convertPlan(ctx, in, fp8, hfDirect)

	var a args
	actorNodes, actorGpusPerNode := l.Nodes, l.GpusPerNode
	if rl && !l.Colocate {
		actorNodes, actorGpusPerNode = actorPlacement(l)
	}
	a.add("--actor-num-nodes", itoa(actorNodes), "--actor-num-gpus-per-node", itoa(actorGpusPerNode))
	if rl {
		if l.Colocate {
			a.add("--colocate")
		}
		a.add("--rollout-num-gpus", itoa(l.RolloutGpus))
	}
	a.add("--hf-checkpoint", hf, "--ref-load", refLoad, "--save", in.RunDir+"/checkpoints",
		"--save-interval", itoa(c.SaveInterval))
	a.add("--prompt-data", in.RunDir+"/data/train.jsonl", "--rollout-shuffle")
	if rl {
		a.add("--input-key", "prompt", "--label-key", "label", "--apply-chat-template")
		a.add(rewardArgs(c, env)...)
		a.add("--num-rollout", itoa(c.NumRollout), "--rollout-batch-size", itoa(c.BatchSize),
			"--n-samples-per-prompt", itoa(c.NSamplesPerPrompt),
			"--global-batch-size", itoa(c.BatchSize*c.NSamplesPerPrompt),
			"--rollout-max-response-len", itoa(c.MaxResponseLen), "--rollout-temperature", "1", "--balance-data")
		if c.MaxPromptLen > 0 {
			a.add("--rollout-max-prompt-len", itoa(c.MaxPromptLen))
		}
		if c.NSamplesPerPrompt > 1 {
			// Pass@k needs several samples per prompt.
			a.add("--log-passrate")
		}
		if fw := in.Registry.FrameworkByName(Name); fw != nil && fw.Metrics.Samples != nil &&
			(fw.Metrics.Samples.RolloutGroups > 0 || fw.Metrics.Samples.EvalPerSet > 0) {
			a.add("--custom-rollout-log-function-path", sampleRolloutPlugin,
				"--custom-eval-rollout-log-function-path", sampleEvalPlugin)
		}
		a.add("--advantage-estimator", c.AdvantageEstimator)
		if c.AdvantageEstimator == recipe.AlgorithmPPO {
			a.add("--megatron-config-path", in.RunDir+"/"+megatronConfigFile)
			if *c.CriticWarmupRollouts > 0 {
				a.add("--num-critic-only-steps", itoa(*c.CriticWarmupRollouts))
			}
		}
		if *c.UseKlLoss {
			a.add("--use-kl-loss", "--kl-loss-coef", ftoa(c.KlLossCoef), "--kl-loss-type", "low_var_kl")
		}
		a.add("--entropy-coef", "0", "--eps-clip", "0.2", "--eps-clip-high", "0.28")
		if c.Correction == correctionTIS {
			a.add("--use-tis", "--tis-clip-low", "0", "--tis-clip", "2")
		}
		a.add("--rollout-num-gpus-per-engine", itoa(c.GpusPerEngine), "--sglang-mem-fraction-static", "0.7")
		if c.KvCacheDtype != "" {
			a.add("--sglang-kv-cache-dtype", c.KvCacheDtype)
		}
		if c.Schedule == recipe.ScheduleAsync {
			a.add("--rollout-function-path", asyncRolloutFunction, "--skip-eval-before-train")
		}
		if c.PD != nil && c.PD.Enabled {
			a.add("--sglang-config", in.RunDir+"/"+sglangConfigFile,
				"--sglang-disaggregation-transfer-backend", "mooncake",
				"--sglang-disaggregation-ib-device", strings.Join(rdmaDevices(in), ","))
		}
		if c.Concurrency > 0 {
			a.add("--sglang-server-concurrency", itoa(c.Concurrency))
		}
	} else {
		a.add("--input-key", "messages", "--rollout-function-path", sftRolloutFunction,
			"--rollout-batch-size", itoa(c.BatchSize), "--global-batch-size", itoa(c.BatchSize),
			"--loss-type", "sft_loss", "--calculate-per-token-loss", "--disable-compute-advantages-and-returns",
			"--debug-train-only")
		if c.NumRollout > 0 {
			a.add("--num-rollout", itoa(c.NumRollout))
		} else {
			a.add("--num-epoch", itoa(c.NumEpoch))
		}
		if hasValidation(ctx.Request) {
			a.add("--eval-interval", itoa(c.ValInterval),
				"--eval-prompt-data", valDatasetName, in.RunDir+"/data/eval.jsonl")
		}
	}
	if rl && in.EvalDataset != nil && c.EvalInterval > 0 && c.Schedule != recipe.ScheduleAsync {
		a.add("--eval-interval", itoa(c.EvalInterval), "--eval-prompt-data", "eval", in.RunDir+"/data/eval.jsonl")
	}
	a.add("--use-tensorboard")
	if c.Lora != nil {
		a.add("--custom-model-provider-path", loraPlugin, "--only-train-params-name-list", loraTrainable,
			"--custom-config-path", in.RunDir+"/"+pluginsConfigFile)
		if rl {
			// The reference copy is taken from the actor loaded from the base, whose adapters start at zero.
			a.add("--ref-trainable-only")
		}
		if c.Lora.AdapterOnlyCheckpoint {
			// Checkpoints hold the adapters only; loading reads the base from its HF directory first.
			a.add("--trainable-only-ckpt-base", in.Model.Path)
		}
	}

	a.add("--tensor-model-parallel-size", itoa(c.TP), "--pipeline-model-parallel-size", itoa(c.PP),
		"--context-parallel-size", itoa(c.CP), "--expert-model-parallel-size", itoa(c.EP),
		"--expert-tensor-parallel-size", "1")
	if c.TP > 1 {
		a.add("--sequence-parallel")
	}
	a.add("--recompute-granularity", "full", "--recompute-method", "uniform", "--recompute-num-layers", "1",
		"--use-dynamic-batch-size", "--max-tokens-per-gpu", itoa(c.MaxTokensPerGpu), "--log-probs-chunk-size", "1024")

	a.add("--optimizer", "adam", "--lr", ftoa(c.Lr), "--weight-decay", "0.1", "--adam-beta1", "0.9")
	if rl {
		a.add("--lr-decay-style", "constant", "--adam-beta2", "0.98")
	} else {
		a.add("--lr-decay-style", "cosine", "--min-lr", ftoa(c.MinLr), "--adam-beta2", "0.95")
		if c.WarmupIters > 0 {
			a.add("--lr-warmup-iters", itoa(c.WarmupIters))
		} else {
			a.add("--lr-warmup-fraction", "0.1")
		}
	}
	if in.Model.Config != nil && in.Model.Config.NumExperts > 0 {
		a.add("--optimizer-cpu-offload", "--overlap-cpu-optimizer-d2h-h2d", "--use-precision-aware-optimizer")
	}
	a.add("--attention-dropout", "0.0", "--hidden-dropout", "0.0", "--accumulate-allreduce-grads-in-fp32",
		"--attention-softmax-in-fp32", "--attention-backend", "flash", "--no-gradient-accumulation-fusion")

	if seed := ctx.Request.Seed; seed > 0 {
		a.add("--seed", itoa(seed))
		if rl {
			a.add("--rollout-seed", itoa(seed))
		}
	}
	if w := ctx.Request.Tracking.Wandb; w.Enabled {
		a.add("--use-wandb", "--wandb-project", w.Project, "--wandb-group", in.RunID)
		if w.Team != "" {
			a.add("--wandb-team", w.Team)
		}
		// wandb would otherwise upload the console output, which can hold dataset and model text.
		env["WANDB_CONSOLE"] = "off"
	}
	plan := &recipe.FrameworkPlan{
		Choices: c,
		Args:    []string(a),
		Env:     env,
		Convert: convert,
		Lora:    c.Lora,
	}
	if c.Lora != nil {
		plan.Files = map[string]string{pluginsConfigFile: loraConfig(c.Lora, ctx.FrameworkSpec.Lora)}
	}
	if rl {
		precision, schedule := "bf16", "sync"
		if fp8 {
			precision = recipe.RolloutPrecisionFP8
		}
		if c.Schedule == recipe.ScheduleAsync {
			schedule = recipe.ScheduleAsync
		}
		plan.Rollout = map[string]interface{}{"precision": precision, "schedule": schedule, "layout": c.Layout}
		if pd := c.PD; pd != nil && pd.Enabled {
			if plan.Files == nil {
				plan.Files = map[string]string{}
			}
			plan.Files[sglangConfigFile] = sglangConfig(pd.PrefillGpus, l.RolloutGpus-pd.PrefillGpus, c.GpusPerEngine)
			plan.Rollout["pd"] = map[string]interface{}{"prefillGpus": pd.PrefillGpus, "rdmaDevices": rdmaDevices(in)}
		}
		if c.AdvantageEstimator == recipe.AlgorithmPPO {
			plan.Critic = criticPlan(c, in.RunDir)
		}
	} else if hasValidation(ctx.Request) {
		plan.Val = &recipe.RecipeVal{Interval: c.ValInterval, MaxSamples: c.ValMaxSamples, Split: c.ValSplit}
	}
	return plan, nil
}

// rdmaDevices returns the RDMA NICs the registry lists for the workspace's cluster, which Mooncake moves the KV
// cache over.
func rdmaDevices(in *recipe.Input) []string {
	return in.Registry.Network.RdmaDevices[in.Workspace.Cluster]
}

// sglangConfig renders sglang.yaml for Mooncake PD the way slime's configurator does: one model whose weights the
// rollout updates, served by a prefill and a decode group of engines.
func sglangConfig(prefill, decode, perEngine int) string {
	var b strings.Builder
	b.WriteString("sglang:\n  - name: default\n    update_weights: true\n    server_groups:\n")
	for _, g := range []struct {
		role string
		gpus int
	}{{"prefill", prefill}, {"decode", decode}} {
		fmt.Fprintf(&b, "      - worker_type: %s\n        num_gpus: %d\n        num_gpus_per_engine: %d\n",
			g.role, g.gpus, perEngine)
	}
	return b.String()
}

// criticPlan describes the PPO critic for the adapter: its megatron.yaml entry overrides the actor's arguments with
// its own checkpoint directory and learning rate and keeps it fully trained, without LoRA or frozen parameters. The
// adapter drops load while the directory holds no checkpoint, so a new critic starts from the base model.
func criticPlan(c recipe.Choices, runDir string) map[string]interface{} {
	dir := runDir + "/" + criticCheckpoints
	return map[string]interface{}{
		"lr":             c.CriticLr,
		"warmupRollouts": *c.CriticWarmupRollouts,
		"megatronConfig": megatronConfigFile,
		"checkpoints":    criticCheckpoints,
		"overrides": map[string]interface{}{
			"load": dir, "save": dir, "lr": c.CriticLr,
			"custom_model_provider_path": nil, "only_train_params_name_list": nil, "freeze_params_name_list": nil,
		},
	}
}

// convertPlan is the weight conversion and the --hf-checkpoint / --ref-load paths. HFDirect specs load the BF16
// HF directory as the reference; convert is then only present when an FP8 rollout needs quantized weights.
func convertPlan(ctx *recipe.Context, in *recipe.Input, fp8, hfDirect bool) (*recipe.RecipeConvert, string, string) {
	hf, refLoad := in.Model.Path, in.Model.Path
	if hfDirect && !fp8 {
		return nil, hf, refLoad
	}
	convert := &recipe.RecipeConvert{
		Dir: in.RunDir + "/convert/torch_dist", Nodes: max(ctx.FrameworkSpec.ConvertNodes, 1), HFDirect: hfDirect,
	}
	if hfDirect {
		convert.Nodes = 1
	}
	if !ctx.CustomImage {
		convert.CacheKey = fmt.Sprintf("%s-%s-%s-%s", Name, ctx.Spec.Name, in.Model.ID, ctx.Image.FrameworkCommit)
		convert.Dir = path.Join(path.Dir(in.RunDir), recipe.ConvertCacheDir, convert.CacheKey, "torch_dist")
	}
	if !hfDirect {
		refLoad = convert.Dir
	}
	if fp8 {
		// Training stays BF16 and quantizes each weight sync with the FP8 weights' quantization config.
		convert.FP8 = true
		hf = path.Join(path.Dir(convert.Dir), fp8Dir)
	}
	return convert, hf, refLoad
}

// rewardArgs returns slime's reward arguments for an RL request and sets the environment a reward plugin reads.
func rewardArgs(c recipe.Choices, env map[string]string) []string {
	switch c.RewardType {
	case rewardCustom:
		env[customRewardEnv] = c.RewardHook
		return []string{"--custom-rm-path", customRewardPlugin}
	case rewardRemote:
		remote := c.RewardRemote
		if remote == nil {
			remote = &recipe.RewardRemote{}
		}
		out := []string{"--rm-type", "remote_rm"}
		if remote.SecretID != "" {
			out = []string{"--custom-rm-path", remoteRewardPlugin}
		}
		out = append(out, "--rm-url", remote.URL)
		if remote.RewardKey != "" {
			out = append(out, "--reward-key", remote.RewardKey)
		}
		return out
	default:
		return []string{"--rm-type", c.RewardType}
	}
}

// hasValidation tells whether an SFT request has a validation set, given or split from the training data.
func hasValidation(req *recipe.SubmitRequest) bool {
	return req.TrainType != registry.TrainTypeRL && (req.EvalDatasetID != "" || req.Choices.ValSplit > 0)
}

// actorPlacement splits a separate RL layout: whole nodes for training, or the first GPUs of a single node.
func actorPlacement(l recipe.Layout) (int, int) {
	if l.Nodes == 1 || l.GpusPerNode == 0 {
		return 1, l.ActorGpus
	}
	return l.ActorGpus / l.GpusPerNode, l.GpusPerNode
}

// args collects command-line arguments.
type args []string

// add appends arguments.
func (a *args) add(v ...string) { *a = append(*a, v...) }

// itoa formats an integer argument.
func itoa(v int) string { return strconv.Itoa(v) }

// ftoa formats a float argument in the shortest form that parses back to the same value.
func ftoa(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
