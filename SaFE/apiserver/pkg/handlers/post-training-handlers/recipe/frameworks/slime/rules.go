/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package slime

import (
	"fmt"
	"net/netip"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/idna"

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

// metadataHosts are cloud metadata endpoints outside the link-local range.
var metadataHosts = map[string]bool{"metadata.google.internal": true, "fd00:ec2::254": true, "100.100.100.200": true}

// loopbackNames are the names of the pod itself, as the /etc/hosts kubelet writes maps them.
var loopbackNames = map[string]bool{"localhost": true, "ip6-localhost": true, "ip6-loopback": true}

// blockedRewardHost tells whether a reward address names the pod itself, a link-local or metadata endpoint, or an
// unspecified or multicast address. Only names and literals are checked; a name that resolves to one passes. A name
// with characters outside ASCII is first mapped to ASCII the way an IDNA lookup does, so full-width digits or
// ideographic full stops read as the address they spell; such a name that cannot be mapped is refused. A plain
// ASCII name is what it spells and is not mapped, as the mapping refuses names a resolver takes, such as one with
// an underscore.
func blockedRewardHost(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	notASCII := func(r rune) bool { return r > unicode.MaxASCII }
	if _, err := netip.ParseAddr(h); err != nil && strings.IndexFunc(h, notASCII) >= 0 {
		ascii, err := idna.Lookup.ToASCII(h)
		if err != nil {
			return true
		}
		h = strings.TrimSuffix(strings.ToLower(ascii), ".")
	}
	if loopbackNames[h] || strings.HasSuffix(h, ".localhost") || metadataHosts[h] {
		return true
	}
	ip, err := netip.ParseAddr(h)
	if err != nil {
		var ok bool
		if ip, ok = shorthandIPv4(h); !ok {
			return false
		}
	}
	ip = ip.Unmap()
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() ||
		metadataHosts[ip.WithZone("").String()]
}

// shorthandIPv4 parses the numeric IPv4 forms the C resolver accepts besides the dotted quad, such as 2130706433,
// 0x7f.1 or 0177.0.0.1, where the last part fills the remaining bytes.
func shorthandIPv4(h string) (netip.Addr, bool) {
	parts := strings.Split(h, ".")
	if len(parts) > 4 {
		return netip.Addr{}, false
	}
	last, v := len(parts)-1, uint64(0)
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 0, 32)
		if err != nil || strings.Contains(p, "_") || (i < last && n > 255) || (i == last && n >= 1<<(8*(4-last))) {
			return netip.Addr{}, false
		}
		if i < last {
			n <<= 8 * (3 - i)
		}
		v |= n
	}
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}), true
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
		case err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "":
			fail("choices.rewardRemote.url", "Give the http or https address of the reward service")
		case u.User != nil:
			fail("choices.rewardRemote.url", "Keep credentials out of the address; put the key in a secret")
		case blockedRewardHost(u.Hostname()):
			fail("choices.rewardRemote.url", "The reward service cannot be on a loopback, link-local, metadata, "+
				"unspecified or multicast address")
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

// rewardTypes are the RL reward types this version compiles.
var rewardTypes = map[string]bool{defaultRewardType: true, "dapo": true, "math": true, "f1": true,
	rewardCustom: true, rewardRemote: true}

// modelParallelRules checks the parallelism against the model's structure the way Megatron does at start-up: TP
// divides the attention heads, the KV heads and TP are multiples of one another, and EP divides the experts.
func modelParallelRules(cfg *v1.ModelConfig, c recipe.Choices) []recipe.Diagnostic {
	if cfg == nil {
		return nil
	}
	var out []recipe.Diagnostic
	fail := func(field, format string, args ...interface{}) {
		out = append(out, recipe.Diagnostic{Level: recipe.LevelError, Field: field, Message: fmt.Sprintf(format, args...)})
	}
	if heads := cfg.NumAttentionHeads; heads > 0 && heads%c.TP != 0 {
		fail("choices.tp", "TP (%d) must divide the %d attention heads", c.TP, heads)
	}
	if kv := cfg.NumKeyValueHeads; kv > 0 && kv%c.TP != 0 && c.TP%kv != 0 {
		fail("choices.tp", "The KV heads (%d) and TP (%d) must be multiples of one another", kv, c.TP)
	}
	switch {
	case cfg.NumExperts == 0 && c.EP > 1:
		fail("choices.ep", "EP must be 1 for a model without experts")
	case cfg.NumExperts > 0 && cfg.NumExperts%c.EP != 0:
		fail("choices.ep", "EP (%d) must divide the %d experts", c.EP, cfg.NumExperts)
	}
	return out
}

// Validate applies slime's rules on top of the platform rules.
func (p plugin) Validate(ctx *recipe.Context) []recipe.Diagnostic {
	c, l := p.normalize(ctx), ctx.Layout
	var out []recipe.Diagnostic
	fail := func(field, format string, args ...interface{}) {
		out = append(out, recipe.Diagnostic{Level: recipe.LevelError, Field: field, Message: fmt.Sprintf(format, args...)})
	}

	if ctx.FrameworkSpec.ModelScript == "" {
		fail("modelId", "Spec %s names no model script for slime", ctx.Spec.Name)
	}
	if c.TP < 1 || c.PP < 1 || c.CP < 1 || c.EP < 1 {
		fail("choices.tp", "TP, PP, CP and EP must be positive")
		return out
	}
	if train := l.ActorGpus; train <= 0 || train%(c.TP*c.PP*c.CP) != 0 || train%(c.EP*c.PP) != 0 {
		fail("choices.tp", "Training GPUs (%d) must be divisible by TP × PP × CP (%d) and EP × PP (%d)",
			l.ActorGpus, c.TP*c.PP*c.CP, c.EP*c.PP)
	} else {
		dp, global := train/(c.TP*c.PP*c.CP), c.BatchSize
		if ctx.Request.TrainType == registry.TrainTypeRL {
			global = c.BatchSize * c.NSamplesPerPrompt
		}
		if global > 0 && global%dp != 0 {
			fail("choices.batchSize", "The global batch (%d) must be divisible by the data-parallel size (%d)",
				global, dp)
		}
	}
	out = append(out, modelParallelRules(ctx.Input.Model.Config, c)...)
	if c.BatchSize < 1 || c.MaxTokensPerGpu < 1 || c.SaveInterval < 1 || c.Lr <= 0 {
		fail("choices.batchSize", "Batch size, max tokens per GPU, save interval and learning rate must be positive")
	}
	if ctx.Request.TrainType != registry.TrainTypeRL {
		if c.MinLr < 0 || c.MinLr > c.Lr {
			fail("choices.minLr", "The minimum learning rate must be between 0 and the learning rate")
		}
		if c.NumEpoch < 0 || c.NumRollout < 0 || (c.NumEpoch == 0 && c.NumRollout == 0) {
			fail("choices.numEpoch", "Train for at least one epoch or step")
		}
	}

	out = append(out, trackingRules(ctx)...)
	if ctx.Request.TrainType != registry.TrainTypeRL {
		return append(out, validationRules(ctx, c)...)
	}
	if c.NumRollout < 1 || c.NSamplesPerPrompt < 1 || c.MaxResponseLen < 1 {
		fail("choices.numRollout", "Rollouts, samples per prompt and response length must be positive")
	}
	if c.KlLossCoef < 0 {
		fail("choices.klLossCoef", "The KL loss coefficient cannot be negative")
	}
	if c.Correction != "" && c.Correction != "none" && c.Correction != correctionTIS {
		fail("choices.correction", `Correction must be "none" or "tis"`)
	}
	if ev := ctx.Input.EvalDataset; ev != nil && ev.Type != recipe.DatasetTypeRL {
		fail("evalDatasetId", "The evaluation set must be an %s dataset, not %s", recipe.DatasetTypeRL, ev.Type)
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
	if g, n := c.GpusPerEngine, l.GpusPerNode; g > 0 && n > 0 && n%g != 0 && g%n != 0 {
		fail("choices.gpusPerEngine", "GPUs per engine (%d) must fit within a node of %d GPUs or span whole nodes",
			g, n)
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
		warn("evalDatasetId", "Fully async does not evaluate during training")
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
	if n := ctx.Input.Model.ContextLen(); n > 0 {
		switch {
		case c.MaxResponseLen >= n:
			fail("choices.maxResponseLen", "The response limit must leave room for the prompt within the model's "+
				"%d-token context", n)
		case c.MaxPromptLen+c.MaxResponseLen > n:
			fail("choices.maxResponseLen", "The prompt and response limits (%d + %d tokens) exceed the model's "+
				"%d-token context", c.MaxPromptLen, c.MaxResponseLen, n)
		}
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
