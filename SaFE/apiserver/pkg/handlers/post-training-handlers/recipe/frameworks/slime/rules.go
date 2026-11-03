/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package slime

import (
	"fmt"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

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

	if ctx.Request.TrainType != registry.TrainTypeRL {
		return out
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
