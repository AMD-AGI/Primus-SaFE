/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package recipe

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

// fakeFramework is a plugin that accepts everything and echoes the choices.
type fakeFramework struct{}

func (fakeFramework) Name() string { return "fake" }

func (fakeFramework) Options(*Context) *Options { return &Options{} }

func (fakeFramework) Validate(*Context) []Diagnostic { return nil }

func (fakeFramework) Compile(ctx *Context) (*FrameworkPlan, error) {
	c := ctx.Request.Choices
	if c.Lr == 0 {
		c.Lr = 1e-5
	}
	return &FrameworkPlan{Choices: c, Args: []string{"--lr"}}, nil
}

func init() { Register(fakeFramework{}) }

const testRegistry = `
frameworks:
  - name: slime
    default: true
    compiler: fake
    adapter: frameworks/slime
    metrics: {include: ["train/*"], maxPointsPerRun: 1000}
specs:
  - name: qwen3-8B
    match: {architectures: [Qwen3ForCausalLM], hidden_size: 4096}
    evalTp: 1
    frameworks:
      slime:
        methods: {sft: [full], rl: [full, lora]}
        minNodes: {sft-full: 1, rl: 1, rl-separate: 2}
        unverified: [rl-separate]
images:
  - {image: img:1, framework: slime, arch: [gfx950], frameworkCommit: abc, validated: true, default: true,
     features: [full, separate, lora, loraRl]}
  - {image: img:old, framework: slime, arch: [gfx950], frameworkCommit: old, validated: true, deprecated: true,
     features: [full]}
  - {image: img:new, framework: slime, arch: [gfx950], frameworkCommit: new, validated: false}
gpuArch: {MI355X: gfx950, MI325X: gfx942}
`

// input builds a valid SFT compilation input.
func input(t *testing.T) *Input {
	t.Helper()
	reg, err := registry.Parse([]byte(testRegistry))
	require.NoError(t, err)
	return &Input{
		Registry: reg,
		Request: &SubmitRequest{Workspace: "ws", DisplayName: "demo", TrainType: "sft", Method: "full",
			ModelID: "m1", DatasetID: "d1", Resources: Resources{Nodes: 1, GpusPerNode: 8}},
		RunID:  "pt-1",
		RunDir: "/wekafs/custom/models/pt-1",
		Model: ModelInfo{ID: "m1", Path: "/wekafs/models/m1", Ready: true, MaxTokens: 40960,
			Config: &v1.ModelConfig{Architectures: []string{"Qwen3ForCausalLM"}, HiddenSize: 4096}},
		Dataset:   &DatasetInfo{ID: "d1", Type: "sft", Path: "/wekafs/datasets/d1", Ready: true},
		Workspace: WorkspaceInfo{ID: "ws", GPUModel: "MI355X", Nodes: 4},
	}
}

// errorsOf returns the error messages of a result.
func errorsOf(r *Result) []string {
	var out []string
	for _, d := range r.Diagnostics {
		if d.Level == LevelError {
			out = append(out, d.Message)
		}
	}
	return out
}

func TestCompileBuildsRecipe(t *testing.T) {
	res := Compile(input(t))
	require.Empty(t, errorsOf(res))
	require.Equal(t, "qwen3-8B", res.Spec.Name)
	require.Equal(t, "img:1", res.Image.Image)
	r := res.Recipe
	require.Equal(t, RecipeSchema, r.Schema)
	require.Equal(t, "slime", r.Framework)
	require.Equal(t, "abc", r.FrameworkVersion)
	require.Equal(t, "qwen3-8B", r.Model.Spec)
	require.Equal(t, Layout{Nodes: 1, GpusPerNode: 8, Colocate: true, ActorGpus: 8}, r.Layout)
	require.Equal(t, 1e-5, r.Choices.Lr)
	require.True(t, r.Output.ExportHF)
	require.Len(t, r.Provenance.RecipeHash, 64)
}

func TestRecipeHashCoversTheConfiguration(t *testing.T) {
	base := Compile(input(t)).Recipe.Provenance.RecipeHash

	in := input(t)
	in.RunID, in.Request.DisplayName, in.RunDir = "pt-2", "other", "/elsewhere"
	in.Request.Resources.Priority, in.Request.Tracking.Wandb.Enabled = 3, true
	in.Request.Ext = map[string]interface{}{"note": "x"}
	require.Equal(t, base, Compile(in).Recipe.Provenance.RecipeHash, "excluded fields must not change the hash")

	in = input(t)
	in.Request.Choices.Lr = 1e-5
	require.Equal(t, base, Compile(in).Recipe.Provenance.RecipeHash, "an explicit default equals the default")

	in = input(t)
	in.Request.Choices.Lr = 2e-5
	require.NotEqual(t, base, Compile(in).Recipe.Provenance.RecipeHash)

	in = input(t)
	in.Dataset.ID = "d2"
	require.NotEqual(t, base, Compile(in).Recipe.Provenance.RecipeHash)
}

func TestCompileRejects(t *testing.T) {
	cases := map[string]struct {
		mutate func(*Input)
		want   string
	}{
		"model not ready":   {func(in *Input) { in.Model.Ready = false }, "not ready"},
		"reading structure": {func(in *Input) { in.Model.Config = nil }, "Reading the model structure"},
		"config error": {func(in *Input) { in.Model.Config, in.Model.ConfigError = nil, "no such file" },
			"Cannot read config.json: no such file"},
		"unknown structure": {func(in *Input) { in.Model.Config.HiddenSize = 2560 }, "not supported"},
		"no image for arch": {func(in *Input) { in.Workspace.GPUModel = "MI325X" },
			"No validated slime image for workspace GPU architecture gfx942"},
		"unknown gpu":    {func(in *Input) { in.Workspace.GPUModel = "" }, "no known architecture"},
		"method":         {func(in *Input) { in.Request.Method = "lora" }, "does not support sft with lora"},
		"too many nodes": {func(in *Input) { in.Request.Resources.Nodes = 5 }, "only 4 nodes"},
		"dataset type":   {func(in *Input) { in.Dataset.Type = "rlhf" }, "needs a sft dataset"},
		"no dataset":     {func(in *Input) { in.Dataset = nil }, "Select a training dataset"},
		"unvalidated lora": {func(in *Input) {
			in.Request.Resources.Image = "img:new"
			in.Request.Export.PeftAdapter = true
		}, "allows only full SFT and GRPO without other features, not peftExport"},
		"feature":          {func(in *Input) { in.Request.Export.PeftAdapter = true }, "validated for peftExport"},
		"rl lora colocate": {rlLora(""), "RL with LoRA needs a separate rollout"},
		"rl min nodes":     {rlLora(LayoutSeparate), "at least 2 nodes"},
		"missing field": {func(in *Input) {
			in.Dataset.Fields = []string{"instruction"}
			in.Request.Choices.FieldMap = map[string]string{"output": "answer"}
		}, "Field answer is not in the first rows"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := input(t)
			c.mutate(in)
			res := Compile(in)
			require.Nil(t, res.Recipe)
			require.NotEmpty(t, errorsOf(res))
			require.Contains(t, errorsOf(res)[0], c.want)
		})
	}
}

// rlLora turns the input into an RL LoRA request with the given layout.
func rlLora(layout string) func(*Input) {
	return func(in *Input) {
		in.Request.TrainType, in.Request.Method, in.Dataset.Type = "rl", "lora", "rlhf"
		in.Request.Choices.Layout, in.Request.Choices.RolloutGpus = layout, 4
	}
}

func TestImageSelection(t *testing.T) {
	warnings := func(in *Input) []string {
		var out []string
		for _, d := range Compile(in).Diagnostics {
			if d.Level == LevelWarning {
				out = append(out, d.Message)
			}
		}
		return out
	}
	in := input(t)
	res := Compile(in)
	require.Equal(t, "img:1", res.Image.Image, "the default image")

	in.Request.Resources.Image = "img:old"
	require.Contains(t, warnings(in)[0], "deprecated")
	in.Request.Resources.Image = "img:new"
	require.Contains(t, warnings(in)[0], "has not been validated")

	in.Request.Resources.Image = "registry.local/slime:mine"
	res = Compile(in)
	require.Empty(t, errorsOf(res))
	require.Equal(t, "abc", res.Recipe.FrameworkVersion, "a custom image compiles with the default image's commit")
	require.Contains(t, warnings(in)[0], "not in the image table")
}

func TestUnverifiedConfigurationWarns(t *testing.T) {
	in := input(t)
	rlLora(LayoutSeparate)(in)
	in.Request.Resources.Nodes = 2
	res := Compile(in)
	require.Empty(t, errorsOf(res))
	require.Equal(t, LevelWarning, res.Diagnostics[0].Level)
	require.Equal(t, Layout{Nodes: 2, GpusPerNode: 8, ActorGpus: 12, RolloutGpus: 4}, res.Recipe.Layout)
}

func TestSupport(t *testing.T) {
	in := input(t)
	ok, reason := Support(in)
	require.True(t, ok, reason)

	in.Request.TrainType, in.Request.Method = "sft", "lora"
	ok, reason = Support(in)
	require.False(t, ok)
	require.Contains(t, reason, "does not support sft with lora")

	in.Request.TrainType, in.Request.Method = "", ""
	in.Model.Config = nil
	ok, reason = Support(in)
	require.False(t, ok)
	require.Equal(t, "Reading the model structure", reason)
}

func TestCheckRegistry(t *testing.T) {
	reg, err := registry.Parse([]byte(testRegistry))
	require.NoError(t, err)
	require.NoError(t, CheckRegistry(reg))
	reg.Frameworks[0].Compiler = "missing"
	require.ErrorContains(t, CheckRegistry(reg), "compiler missing, which is not built in")
}
