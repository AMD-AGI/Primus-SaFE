/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package slime

import (
	"fmt"
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

// wandbKeyPrefix is the only key prefix a W&B secret may hold, since all of its keys become environment variables.
const wandbKeyPrefix = "WANDB_"

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
	secret := ctx.Input.Secrets[w.SecretID]
	switch {
	case w.SecretID == "":
		fail("tracking.wandb.secretId", "Select the secret that holds WANDB_API_KEY")
	case secret == nil:
		fail("tracking.wandb.secretId", "Secret %s doesn't exist or you can't read it", w.SecretID)
	case secret.Type != string(v1.SecretGeneral):
		fail("tracking.wandb.secretId", "Secret %s must be a general secret", w.SecretID)
	case !slices.Contains(secret.Keys, "WANDB_API_KEY"):
		fail("tracking.wandb.secretId", "Secret %s has no WANDB_API_KEY", w.SecretID)
	default:
		for _, k := range secret.Keys {
			if !strings.HasPrefix(k, wandbKeyPrefix) {
				fail("tracking.wandb.secretId", "Secret %s may only hold WANDB_ keys, not %s", w.SecretID, k)
				break
			}
		}
	}
	return out
}

// rewardTypes are the RL reward types this version compiles.
var rewardTypes = map[string]bool{defaultRewardType: true}

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
	if ctx.Request.TrainType != registry.TrainTypeRL {
		return append(out, validationRules(ctx, c)...)
	}
	if c.NumRollout < 1 || c.NSamplesPerPrompt < 1 || c.MaxResponseLen < 1 {
		fail("choices.numRollout", "Rollouts, samples per prompt and response length must be positive")
	}
	if !rewardTypes[c.RewardType] {
		fail("choices.rewardType", "Reward type %q is not available", c.RewardType)
	}
	if c.AdvantageEstimator != recipe.AlgorithmGRPO {
		fail("choices.advantageEstimator", "Algorithm %q is not available yet", c.AdvantageEstimator)
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
