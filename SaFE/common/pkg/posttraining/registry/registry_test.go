/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// chartRegistry returns the default registry from the Helm chart values as YAML.
func chartRegistry(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "charts", "primus-safe", "values.yaml"))
	require.NoError(t, err)
	var values map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &values))
	pt, ok := values["post_training"].(map[string]interface{})
	require.True(t, ok, "values.yaml has no post_training section")
	out, err := yaml.Marshal(pt["registry"])
	require.NoError(t, err)
	return out
}

const minimal = `
frameworks:
  - name: slime
    default: true
    compiler: slime
    adapter: frameworks/slime
    metrics: {include: ["train/*"], maxPointsPerRun: 1000}
specs:
  - name: a
    match: {architectures: [X], num_hidden_layers: 2, rope_theta: 1000000}
    evalTp: 1
    frameworks:
      slime:
        methods: {sft: [full, lora], rl: [full, lora]}
        minNodes: {sft-full: 1, sft-lora: 1, rl: 1, rl-separate: 2}
images:
  - {image: img:1, framework: slime, arch: [gfx950], frameworkCommit: abc, validated: true, default: true, features: [full]}
gpuArch: {MI355X: gfx950}
`

// The chart's registry parses, and its specs match the models they are written for.
func TestChartRegistryIsValid(t *testing.T) {
	r, err := Parse(chartRegistry(t))
	require.NoError(t, err)
	require.NotNil(t, r.FrameworkByName(""))

	qwen4b := &v1.ModelConfig{Architectures: []string{"Qwen3ForCausalLM"}, NumHiddenLayers: 36, HiddenSize: 2560,
		NumAttentionHeads: 32, NumKeyValueHeads: 8, RopeTheta: "1000000"}
	require.Equal(t, "qwen3-4B", r.MatchSpec(qwen4b).Name)
	qwen4b.RopeTheta = "5000000.0"
	require.Equal(t, "qwen3-4B-Instruct-2507", r.MatchSpec(qwen4b).Name)
	qwen4b.HiddenSize, qwen4b.RopeTheta = 4096, "1000000"
	require.Equal(t, "qwen3-8B", r.MatchSpec(qwen4b).Name)
	qwen4b.NumHiddenLayers = 48
	require.Nil(t, r.MatchSpec(qwen4b))
}

// Parse refuses each kind of invalid registry with the reason.
func TestParseRejects(t *testing.T) {
	cases := map[string]struct{ from, to, want string }{
		"unknown field":      {"evalTp: 1", "evalTp: 1\n    bogus: 1", "unknown field"},
		"no default":         {"default: true", "default: false", "exactly one framework must be the default"},
		"minNodes gap":       {"rl: 1, rl-separate: 2", "rl: 1", "minNodes has no entry for rl-lora-separate"},
		"unknown feature":    {"features: [full]", "features: [fulll]", "unknown feature fulll"},
		"unknown arch":       {"arch: [gfx950]", "arch: [gfx942]", "architecture gfx942 that gpuArch does not map to"},
		"unknown framework":  {"framework: slime,", "framework: verl,", `unknown framework "verl"`},
		"bad method":         {"sft: [full, lora]", "sft: [full, qlora]", "unknown method qlora"},
		"missing max points": {"maxPointsPerRun: 1000", "maxPointsPerRun: 0", "positive metrics.maxPointsPerRun"},
		"freeze default":     {"minNodes: {", "freeze: {default: [experts]}\n        minNodes: {", "freeze.default needs freeze.allowed"},
		"repeated image": {"gpuArch:", "  - {image: img:1, framework: slime, arch: [gfx950], frameworkCommit: abc, " +
			"validated: true, features: [full]}\ngpuArch:", "image img:1 is listed twice"},
		"no commit":          {"frameworkCommit: abc, ", "", "validated image img:1 has no frameworkCommit"},
		"text default":       {"minNodes: {", "defaults: {tp: \"2\"}\n        minNodes: {", "default tp must be a positive whole number"},
		"zero default":       {"minNodes: {", "defaults: {gfx950: {tp: 0}}\n        minNodes: {", "default gfx950.tp must be a positive whole number"},
		"unknown unverified": {"minNodes: {", "unverified: [sft-qlora]\n        minNodes: {", "unverified names no combination sft-qlora"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, minimal, c.from)
			_, err := Parse([]byte(strings.Replace(minimal, c.from, c.to, 1)))
			require.Error(t, err)
			require.Contains(t, err.Error(), c.want)
		})
	}
}

// Two specs that can match the same model are refused.
func TestOverlappingSpecsAreRejected(t *testing.T) {
	second := `
  - name: b
    match: {architectures: [X], num_hidden_layers: 2}
    evalTp: 1
    frameworks:
      slime:
        methods: {sft: [full]}
        minNodes: {sft-full: 1}
images:`
	_, err := Parse([]byte(strings.Replace(minimal, "\nimages:", second, 1)))
	require.ErrorContains(t, err, "specs a and b can match the same model")

	distinct := strings.Replace(second, "num_hidden_layers: 2}", "num_hidden_layers: 2, rope_theta: 5000000}", 1)
	_, err = Parse([]byte(strings.Replace(minimal, "\nimages:", distinct, 1)))
	require.NoError(t, err)
}

// Each framework and architecture needs exactly one validated default image.
func TestDefaultImages(t *testing.T) {
	image := func(attrs string) string {
		return "  - {image: img:2, framework: slime, arch: [gfx950], frameworkCommit: def" + attrs + "}\ngpuArch:"
	}
	r, err := Parse([]byte(strings.Replace(minimal, "gpuArch:", image(", validated: true"), 1)))
	require.NoError(t, err, "a second validated image is fine while one image is the default")
	require.Equal(t, "img:1", r.DefaultImage("slime", "gfx950").Image)

	_, err = Parse([]byte(strings.Replace(minimal, "gpuArch:", image(", validated: true, default: true"), 1)))
	require.ErrorContains(t, err, "slime/gfx950 needs exactly one default image, found 2")
	_, err = Parse([]byte(strings.Replace(minimal, "gpuArch:", image(", default: true"), 1)))
	require.ErrorContains(t, err, "default image img:2 must be validated and not deprecated")
	_, err = Parse([]byte(strings.Replace(minimal, "default: true, features", "features", 1)))
	require.ErrorContains(t, err, "slime/gfx950 needs exactly one default image, found 0")

	r, err = Parse([]byte(strings.Replace(minimal, "validated: true, default: true", "validated: false", 1)))
	require.NoError(t, err, "no default is needed before any image is validated")
	require.Nil(t, r.DefaultImage("slime", "gfx950"))
}

// The combinations of a framework follow its methods and freeze, and a missing minNodes entry falls back.
func TestCombinationsAndMinNodes(t *testing.T) {
	fs := &FrameworkSpec{
		Methods:  map[string][]string{TrainTypeSFT: {MethodFull, MethodLora}, TrainTypeRL: {MethodFull, MethodLora}},
		Freeze:   &Freeze{Allowed: true},
		MinNodes: map[string]int{"sft-full": 8, "sft-freeze": 2, "sft-lora": 2, "rl-separate": 3, "rl-full-separate": 9},
		Rollout:  &Rollout{RequireSeparate: true},
	}
	require.Equal(t, []Combination{{Variant: VariantFull, Separate: true}, {Variant: VariantFreeze, Separate: true},
		{Variant: VariantLora, Separate: true}}, fs.Combinations(TrainTypeRL))
	n, ok := fs.MinNodesFor(TrainTypeRL, VariantFreeze, true)
	require.True(t, ok)
	require.Equal(t, 3, n, "rl-freeze-separate falls back to rl-separate")
	n, _ = fs.MinNodesFor(TrainTypeRL, VariantFull, true)
	require.Equal(t, 9, n)
	require.Empty(t, fs.validate())

	fs.Rollout = nil
	require.Contains(t, fs.validate()[0], "rl-full")
}

// A registry may not drop an image that runs use, also after a restart with no previous version.
func TestCheckRemovedImages(t *testing.T) {
	prev, err := Parse([]byte(minimal))
	require.NoError(t, err)
	next, err := Parse([]byte(strings.Replace(minimal, "img:1", "img:2", 1)))
	require.NoError(t, err)
	require.ErrorContains(t, CheckRemovedImages(nil, next, []string{"img:1"}), "still use the removed images img:1",
		"after a restart there is no previous version, and the images in use still have to stay listed")
	require.NoError(t, CheckRemovedImages(nil, next, []string{"img:2"}))
	require.NoError(t, CheckRemovedImages(prev, next, []string{"custom:1"}))
	require.ErrorContains(t, CheckRemovedImages(prev, next, []string{"img:1"}), "still use the removed images img:1")
}

// The store keeps the last valid registry when an invalid one is loaded.
func TestStoreKeepsLastValidVersion(t *testing.T) {
	s := &Store{}
	_, err := s.Get()
	require.Error(t, err)

	require.Error(t, s.Load([]byte("frameworks: [}"), nil))
	_, err = s.Get()
	require.ErrorContains(t, err, "invalid registry")

	require.NoError(t, s.Load([]byte(minimal), nil))
	require.Error(t, s.Load([]byte(strings.Replace(minimal, "default: true", "default: false", 1)), nil))
	r, err := s.Get()
	require.NoError(t, err)
	require.Equal(t, "a", r.Specs[0].Name)
	require.Error(t, s.LastError())
}
