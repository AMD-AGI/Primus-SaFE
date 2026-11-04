/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package slime

import (
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

// validationRules checks the SFT validation settings and warns when the curves will lack validation loss.
func validationRules(ctx *recipe.Context, c recipe.Choices) []recipe.Diagnostic {
	var out []recipe.Diagnostic
	add := func(level, field, format string, args ...interface{}) {
		out = append(out, recipe.Diagnostic{Level: level, Field: field, Message: fmt.Sprintf(format, args...)})
	}
	req := ctx.Request
	if c.ValSplit < 0 || c.ValSplit > 0.2 {
		add(recipe.LevelError, "choices.valSplit", "The validation split must be between 0 and 0.2")
	}
	if c.ValSplit > 0 && req.EvalDatasetID != "" {
		add(recipe.LevelError, "choices.valSplit", "Split a validation set from the training data or select one, not both")
	}
	if ev := ctx.Input.EvalDataset; ev != nil && ev.Type != recipe.DatasetTypeSFT {
		add(recipe.LevelError, "evalDatasetId", "The validation set must be an sft dataset, not %s", ev.Type)
	}
	if !hasValidation(req) {
		add(recipe.LevelWarning, "evalDatasetId", "No validation set: the curves will have no validation loss")
		return out
	}
	if c.ValInterval < 1 || (c.NumRollout > 0 && c.ValInterval > c.NumRollout) {
		add(recipe.LevelError, "choices.valInterval", "The validation interval must be between 1 and the number of steps")
	}
	if c.ValMaxSamples < c.BatchSize {
		add(recipe.LevelError, "choices.valMaxSamples", "Validation samples (%d) must be at least the batch size (%d)",
			c.ValMaxSamples, c.BatchSize)
	}
	if c.ValInterval > 0 && c.SaveInterval%c.ValInterval != 0 {
		add(recipe.LevelWarning, "choices.saveInterval",
			"The save interval is not a multiple of the validation interval: some checkpoints have no validation loss")
	}
	return out
}

// wandbKeyPrefix and rewardKeyPrefix are the only key prefixes a W&B or reward service secret may hold, since all
// of a secret's keys become environment variables.
const (
	wandbKeyPrefix  = "WANDB_"
	rewardKeyPrefix = "PT_RM_"
)

// secretRules checks that a secret the request names exists, is readable and general, holds key and only keys
// starting with prefix.
func secretRules(field, id string, secret *recipe.SecretInfo, key, prefix string) []recipe.Diagnostic {
	var out []recipe.Diagnostic
	fail := func(format string, args ...interface{}) {
		out = append(out, recipe.Diagnostic{Level: recipe.LevelError, Field: field, Message: fmt.Sprintf(format, args...)})
	}
	switch {
	case id == "":
		fail("Select the secret that holds %s", key)
	case secret == nil:
		fail("Secret %s doesn't exist or you can't read it", id)
	case secret.Type != string(v1.SecretGeneral):
		fail("Secret %s must be a general secret", id)
	case !slices.Contains(secret.Keys, key):
		fail("Secret %s has no %s", id, key)
	default:
		for _, k := range secret.Keys {
			if !strings.HasPrefix(k, prefix) {
				fail("Secret %s may only hold %s keys, not %s", id, prefix, k)
				break
			}
		}
	}
	return out
}

// trackingRules checks the W&B settings: a project, and a readable general secret that holds WANDB_API_KEY and only
// WANDB_ keys.
func trackingRules(ctx *recipe.Context) []recipe.Diagnostic {
	w := ctx.Request.Tracking.Wandb
	if !w.Enabled {
		return nil
	}
	var out []recipe.Diagnostic
	fail := func(field, format string, args ...interface{}) {
		out = append(out, recipe.Diagnostic{Level: recipe.LevelError, Field: field, Message: fmt.Sprintf(format, args...)})
	}
	if strings.TrimSpace(w.Project) == "" {
		fail("tracking.wandb.project", "Name the W&B project")
	}
	return append(out, secretRules("tracking.wandb.secretId", w.SecretID, ctx.Input.Secrets[w.SecretID],
		"WANDB_API_KEY", wandbKeyPrefix)...)
}

// rewardRules checks the reward settings of an RL request: a label for the built-in rewards, a Python file for a
// custom reward function, and an http(s) address without credentials and an optional secret for a remote service.
func rewardRules(ctx *recipe.Context, c recipe.Choices) []recipe.Diagnostic {
	var out []recipe.Diagnostic
	fail := func(field, format string, args ...interface{}) {
		out = append(out, recipe.Diagnostic{Level: recipe.LevelError, Field: field, Message: fmt.Sprintf(format, args...)})
	}
	switch c.RewardType {
	case rewardCustom:
		file, _, _ := strings.Cut(c.RewardHook, ":")
		if !path.IsAbs(file) || !strings.HasSuffix(file, ".py") || path.Clean(file) != file {
			fail("choices.rewardHook", "Give the absolute path of the Python file that defines the reward function, "+
				"optionally followed by :function_name")
		}
	case rewardRemote:
		remote := c.RewardRemote
		if remote == nil {
			remote = &recipe.RewardRemote{}
		}
		u, err := url.Parse(remote.URL)
		switch {
		case err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "":
			fail("choices.rewardRemote.url", "Give the http or https address of the reward service")
		case u.User != nil:
			fail("choices.rewardRemote.url", "Keep credentials out of the address; put the key in a secret")
		}
		if remote.SecretID != "" {
			out = append(out, secretRules("choices.rewardRemote.secretId", remote.SecretID,
				ctx.Input.Secrets[remote.SecretID], rewardKeyEnv, rewardKeyPrefix)...)
		}
	default:
		label := c.FieldMap["label"]
		ds := ctx.Input.Dataset
		switch {
		case label == "":
			fail("choices.fieldMap.label", "The %s reward compares the response with a label: map the dataset's "+
				"answer or label field", c.RewardType)
		case len(ctx.Request.Choices.FieldMap) == 0 && ds != nil && ds.Fields != nil && !slices.Contains(ds.Fields, label):
			fail("choices.fieldMap.label", "The %s reward needs a label, and the dataset has no %s field: map its "+
				"answer field", c.RewardType, label)
		}
	}
	return out
}

// loraRules checks the LoRA choices of a LoRA request: rank, alpha and dropout in range, target modules the spec
// allows and a known expert mode.
func loraRules(ctx *recipe.Context, l *recipe.LoraChoice) []recipe.Diagnostic {
	if l == nil {
		return nil
	}
	var out []recipe.Diagnostic
	fail := func(field, format string, args ...interface{}) {
		out = append(out, recipe.Diagnostic{Level: recipe.LevelError, Field: field, Message: fmt.Sprintf(format, args...)})
	}
	if l.Rank < 1 || l.Rank > 256 {
		fail("choices.lora.rank", "The LoRA rank must be between 1 and 256")
	}
	if l.Alpha <= 0 {
		fail("choices.lora.alpha", "The LoRA alpha must be positive")
	}
	if l.Dropout < 0 || l.Dropout >= 1 {
		fail("choices.lora.dropout", "The LoRA dropout must be at least 0 and below 1")
	}
	var allowed []string
	if ctx.FrameworkSpec.Lora != nil {
		allowed = ctx.FrameworkSpec.Lora.Targets
	}
	if len(l.Targets) == 0 {
		fail("choices.lora.targets", "Choose at least one module for LoRA")
	}
	for _, t := range l.Targets {
		if !slices.Contains(allowed, t) {
			fail("choices.lora.targets", "%s does not allow LoRA on %s; the modules are %v", ctx.Spec.Name, t, allowed)
		}
	}
	if l.Experts && l.ExpertMode != expertModeShared && l.ExpertMode != recipe.ExpertModePerExpert {
		fail("choices.lora.expertMode", "The expert mode must be shared or perExpert")
	}
	if l.Experts && ctx.Request.Export.PeftAdapter {
		fail("export.peftAdapter", "A PEFT adapter cannot hold the experts' LoRA; export the merged model")
	}
	return out
}

// rewardTypes are the RL reward types this version compiles.
var rewardTypes = map[string]bool{defaultRewardType: true, "dapo": true, "math": true, "f1": true,
	rewardCustom: true, rewardRemote: true}

// Validate applies slime's rules on top of the platform rules.
func (p plugin) Validate(ctx *recipe.Context) []recipe.Diagnostic {
	c, l := p.normalize(ctx), ctx.Layout
	var out []recipe.Diagnostic
	fail := func(field, format string, args ...interface{}) {
		out = append(out, recipe.Diagnostic{Level: recipe.LevelError, Field: field, Message: fmt.Sprintf(format, args...)})
	}

	if c.TP < 1 || c.PP < 1 || c.CP < 1 || c.EP < 1 {
		fail("choices.tp", "TP, PP, CP and EP must be positive")
		return out
	}
	if train := l.ActorGpus; train <= 0 || train%(c.TP*c.PP*c.CP) != 0 || train%(c.EP*c.PP) != 0 {
		fail("choices.tp", "Training GPUs (%d) must be divisible by TP × PP × CP (%d) and EP × PP (%d)",
			l.ActorGpus, c.TP*c.PP*c.CP, c.EP*c.PP)
	}
	if c.BatchSize < 1 || c.MaxTokensPerGpu < 1 || c.SaveInterval < 1 || c.Lr <= 0 {
		fail("choices.batchSize", "Batch size, max tokens per GPU, save interval and learning rate must be positive")
	}

	out = append(out, trackingRules(ctx)...)
	out = append(out, loraRules(ctx, c.Lora)...)
	if ctx.Request.TrainType != registry.TrainTypeRL {
		return append(out, validationRules(ctx, c)...)
	}
	if c.NumRollout < 1 || c.NSamplesPerPrompt < 1 || c.MaxResponseLen < 1 {
		fail("choices.numRollout", "Rollouts, samples per prompt and response length must be positive")
	}
	if !rewardTypes[c.RewardType] {
		fail("choices.rewardType", "Reward type %q is not available", c.RewardType)
	} else {
		out = append(out, rewardRules(ctx, c)...)
	}
	switch c.AdvantageEstimator {
	case recipe.AlgorithmGRPO:
	case recipe.AlgorithmPPO:
		if c.Schedule == recipe.ScheduleAsync {
			fail("choices.schedule", "Fully async supports GRPO only")
		}
		if c.CriticLr <= 0 {
			fail("choices.criticLr", "The critic learning rate must be positive")
		}
		if c.CriticWarmupRollouts != nil && *c.CriticWarmupRollouts < 0 {
			fail("choices.criticWarmupRollouts", "Critic-only rollouts cannot be negative")
		}
	default:
		fail("choices.advantageEstimator", "Algorithm %q is not available", c.AdvantageEstimator)
	}
	if c.RolloutPrecision != "" && c.RolloutPrecision != "bf16" && c.RolloutPrecision != recipe.RolloutPrecisionFP8 {
		fail("choices.rolloutPrecision", "Rollout precision must be bf16 or fp8")
	}
	if c.KvCacheDtype != "" && c.KvCacheDtype != kvCacheFP8 {
		fail("choices.kvCacheDtype", "The KV cache dtype must be %s, or empty for the model's own", kvCacheFP8)
	}
	async := c.Schedule == recipe.ScheduleAsync
	if c.Schedule != "" && c.Schedule != "sync" && !async {
		fail("choices.schedule", "Schedule must be sync or async")
	}
	if c.Concurrency < 0 || c.Concurrency > maxConcurrency {
		fail("choices.concurrency", "Concurrency must be between 1 and %d", maxConcurrency)
	}
	warn := func(field, message string) {
		out = append(out, recipe.Diagnostic{Level: recipe.LevelWarning, Field: field, Message: message})
	}
	engines := 0
	if c.GpusPerEngine > 0 {
		engines = l.RolloutGpus / c.GpusPerEngine
	}
	if pd := c.PD; pd != nil && pd.Enabled && c.GpusPerEngine > 0 {
		prefill, decode := pd.PrefillGpus, l.RolloutGpus-pd.PrefillGpus
		if prefill < c.GpusPerEngine || prefill%c.GpusPerEngine != 0 || decode < c.GpusPerEngine ||
			decode%c.GpusPerEngine != 0 {
			fail("choices.pd.prefillGpus", "Mooncake PD needs whole prefill and decode engines of %d GPUs within "+
				"the %d rollout GPUs", c.GpusPerEngine, l.RolloutGpus)
		}
		if cluster := ctx.Input.Workspace.Cluster; len(rdmaDevices(ctx.Input)) == 0 {
			fail("choices.pd", "Mooncake PD needs the RDMA devices of cluster %s in the registry", cluster)
		}
		if l.Colocate {
			fail("choices.pd", "Mooncake PD needs separate training and rollout GPUs")
		}
		// A request takes a prefill and then a decode engine, so the smaller group bounds the requests in flight.
		engines = min(prefill/c.GpusPerEngine, decode/c.GpusPerEngine)
	}
	if async && c.GpusPerEngine > 0 {
		// The object store has a single generation worker, so these requests are all a rollout runs at once.
		if c.Concurrency*engines < c.NSamplesPerPrompt {
			fail("choices.concurrency", "Fully async needs concurrency × rollout engines (%d × %d) of at least "+
				"the samples per prompt (%d)", c.Concurrency, engines, c.NSamplesPerPrompt)
		}
	}
	if async && ctx.Request.EvalDatasetID != "" {
		warn("evalDatasetId", "Fully async does not evaluate during training; the automatic evaluation after "+
			"training scores the model")
	}
	switch {
	case c.Correction != "":
	case c.RolloutPrecision == recipe.RolloutPrecisionFP8:
		warn("choices.correction", "FP8 rollout without a train-inference correction such as TIS: training uses "+
			"log-probs of the quantized rollout")
	case async:
		warn("choices.correction", "Fully async without a train-inference correction such as TIS: samples come "+
			"from older weights")
	}
	if c.Layout != recipe.LayoutColocate && c.Layout != recipe.LayoutSeparate {
		fail("choices.layout", "Layout must be colocate or separate")
	}
	if c.Schedule == recipe.ScheduleAsync && c.Layout == recipe.LayoutColocate {
		fail("choices.schedule", "Fully async requires separate training and rollout GPUs")
	}
	if cfg := ctx.Input.Model.Config; cfg != nil && cfg.MaxPositionEmbeddings > 0 &&
		c.MaxResponseLen >= cfg.MaxPositionEmbeddings {
		fail("choices.maxResponseLen", "The response limit must leave room for the prompt within the model's "+
			"%d-token context", cfg.MaxPositionEmbeddings)
	}
	if c.GpusPerEngine < 1 || l.RolloutGpus%c.GpusPerEngine != 0 {
		fail("choices.gpusPerEngine", "Rollout GPUs (%d) must be a multiple of GPUs per engine (%d)",
			l.RolloutGpus, c.GpusPerEngine)
	}
	if !l.Colocate {
		total := l.Nodes * l.GpusPerNode
		switch {
		case l.RolloutGpus <= 0 || l.RolloutGpus >= total:
			fail("choices.rolloutGpus", "A separate rollout needs between 1 and %d rollout GPUs", total-1)
		case l.Nodes > 1 && l.ActorGpus%l.GpusPerNode != 0:
			fail("choices.rolloutGpus", "On several nodes, training must use whole nodes (multiples of %d GPUs)",
				l.GpusPerNode)
		}
	}
	return out
}
