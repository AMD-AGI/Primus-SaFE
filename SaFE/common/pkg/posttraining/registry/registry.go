/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package registry parses and validates the Post Training registry, the Helm-managed ConfigMap that lists the
// training frameworks, model specs, images and cluster settings. The apiserver and the Post Training controller
// both load it, so it lives in common.
package registry

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"sigs.k8s.io/yaml"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const (
	// ConfigMapName is the ConfigMap that holds the registry; it exists only when Post Training is enabled.
	ConfigMapName = "post-training-registry"
	// DataKey is the ConfigMap key that holds the registry YAML.
	DataKey = "registry.yaml"

	TrainTypeSFT = "sft"
	TrainTypeRL  = "rl"

	MethodFull = "full"
	MethodLora = "lora"

	VariantFull   = "full"
	VariantFreeze = "freeze"
	VariantLora   = "lora"
)

// Feature names an image may list once the feature has passed acceptance on that image.
const (
	FeatureFull                 = "full"
	FeatureValLoss              = "valLoss"
	FeaturePPO                  = "ppo"
	FeatureLora                 = "lora"
	FeatureLoraRL               = "loraRl"
	FeatureLoraExperts          = "loraExperts"
	FeatureLoraExpertsPerExpert = "loraExpertsPerExpert"
	FeatureLoraAdapterCkpt      = "loraAdapterCkpt"
	FeaturePeftExport           = "peftExport"
	FeatureFreeze               = "freeze"
	FeatureFP8Rollout           = "fp8Rollout"
	FeatureSeparate             = "separate"
	FeatureAsync                = "async"
	FeaturePD                   = "pd"
)

var knownFeatures = map[string]bool{
	FeatureFull: true, FeatureValLoss: true, FeaturePPO: true, FeatureLora: true, FeatureLoraRL: true,
	FeatureLoraExperts: true, FeatureLoraExpertsPerExpert: true, FeatureLoraAdapterCkpt: true,
	FeaturePeftExport: true, FeatureFreeze: true, FeatureFP8Rollout: true, FeatureSeparate: true,
	FeatureAsync: true, FeaturePD: true,
}

// Registry is the parsed post-training-registry ConfigMap.
type Registry struct {
	Frameworks []Framework       `json:"frameworks"`
	Specs      []Spec            `json:"specs"`
	Images     []Image           `json:"images"`
	GPUArch    map[string]string `json:"gpuArch"`
	Network    Network           `json:"network,omitempty"`
	Evaluation Evaluation        `json:"evaluation,omitempty"`
	Paused     Paused            `json:"paused,omitempty"`
	Imports    Imports           `json:"imports,omitempty"`
}

// Imports configures the mounted-path import of Spur run directories. Without a mount for a cluster only the command
// upload works there; a mount is a storage and network change approved on its own.
type Imports struct {
	Mounts []ImportMount `json:"mounts,omitempty"`
}

// ImportMount is the workspace whose pods see Spur's shared storage read-only at Root, through a ReadOnlyMany volume
// mounted there; the scans of a cluster's import dialog run in it.
type ImportMount struct {
	Cluster   string `json:"cluster"`
	Workspace string `json:"workspace"`
	Root      string `json:"root"`
}

// MountFor returns the import mount of a cluster, or nil when it has none.
func (i Imports) MountFor(cluster string) *ImportMount {
	for k := range i.Mounts {
		if i.Mounts[k].Cluster == cluster {
			return &i.Mounts[k]
		}
	}
	return nil
}

// Paused stops new submissions, A/B experiments, re-exports and re-evaluations; running runs carry on.
type Paused struct {
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
}

// Framework is one training framework: the compiler plugin, the launcher adapter and its metric panels.
type Framework struct {
	Name          string   `json:"name"`
	Default       bool     `json:"default,omitempty"`
	Compiler      string   `json:"compiler"`
	Adapter       string   `json:"adapter"`
	ImportFormats []string `json:"importFormats,omitempty"`
	Metrics       Metrics  `json:"metrics"`
}

// Metrics configures which metric names the launcher forwards and how the pages draw them.
type Metrics struct {
	// Include lists the name patterns the launcher forwards; "*" does not cross "/".
	Include []string `json:"include"`
	// Summary names the main metric of each train type, shown in the list.
	Summary map[string]string `json:"summary,omitempty"`
	// Better tells, per name pattern, whether lower or higher values are better.
	Better  map[string]string  `json:"better,omitempty"`
	Samples *Samples           `json:"samples,omitempty"`
	Panels  map[string][]Panel `json:"panels,omitempty"`
	// MaxPointsPerRun caps the stored metric points of one run.
	MaxPointsPerRun int `json:"maxPointsPerRun"`
}

// Samples configures the RL sample viewer.
type Samples struct {
	RolloutGroups     int `json:"rolloutGroups"`
	EvalPerSet        int `json:"evalPerSet"`
	PromptChars       int `json:"promptChars,omitempty"`
	ResponseHeadChars int `json:"responseHeadChars,omitempty"`
	ResponseTailChars int `json:"responseTailChars,omitempty"`
	MaxRows           int `json:"maxRows,omitempty"`
}

// Panel is one chart of the metrics tab.
type Panel struct {
	ID     string   `json:"id"`
	Series []string `json:"series"`
	Band   []string `json:"band,omitempty"`
	Smooth bool     `json:"smooth,omitempty"`
	LogY   bool     `json:"logY,omitempty"`
	Stack  bool     `json:"stack,omitempty"`
	Per    string   `json:"per,omitempty"`
	When   string   `json:"when,omitempty"`
	Rest   string   `json:"rest,omitempty"`
	Axis   string   `json:"axis,omitempty"`
}

// Spec is one supported model structure. Matching, evaluation, retention and the watchdog are framework
// independent; everything else sits in Frameworks under the framework name.
type Spec struct {
	Name          string                    `json:"name"`
	Match         Match                     `json:"match"`
	EvalTp        int                       `json:"evalTp"`
	EvalInference *Inference                `json:"evalInference,omitempty"`
	Checkpoint    Checkpoint                `json:"checkpoint,omitempty"`
	Watchdog      Watchdog                  `json:"watchdog,omitempty"`
	Frameworks    map[string]*FrameworkSpec `json:"frameworks"`
	Ext           map[string]interface{}    `json:"ext,omitempty"`
}

// Match lists config.json structure fields; a model hits the spec when its architectures contain every listed
// architecture and every listed field is equal.
type Match struct {
	Architectures         []string `json:"architectures"`
	ModelType             string   `json:"model_type,omitempty"`
	NumHiddenLayers       *int     `json:"num_hidden_layers,omitempty"`
	HiddenSize            *int     `json:"hidden_size,omitempty"`
	NumAttentionHeads     *int     `json:"num_attention_heads,omitempty"`
	NumKeyValueHeads      *int     `json:"num_key_value_heads,omitempty"`
	NumExperts            *int     `json:"num_experts,omitempty"`
	RopeTheta             *float64 `json:"rope_theta,omitempty"`
	MaxPositionEmbeddings *int     `json:"max_position_embeddings,omitempty"`
}

// Checkpoint is the retention policy; KeepLast 0 keeps every checkpoint.
type Checkpoint struct {
	KeepLast int  `json:"keepLast"`
	KeepBest bool `json:"keepBest,omitempty"`
}

// Watchdog holds the progress watchdog thresholds in minutes.
type Watchdog struct {
	StartupMinutes  int `json:"startupMinutes,omitempty"`
	ConvertMinutes  int `json:"convertMinutes,omitempty"`
	MinStallMinutes int `json:"minStallMinutes,omitempty"`
	StallFactor     int `json:"stallFactor,omitempty"`
	SaveMinutes     int `json:"saveMinutes,omitempty"`
}

// FrameworkSpec is the part of a spec that belongs to one framework.
type FrameworkSpec struct {
	ModelScript string              `json:"modelScript"`
	Methods     map[string][]string `json:"methods"`
	Algorithms  []string            `json:"algorithms,omitempty"`
	Lora        *Lora               `json:"lora,omitempty"`
	Rollout     *Rollout            `json:"rollout,omitempty"`
	Freeze      *Freeze             `json:"freeze,omitempty"`
	// Defaults holds the framework's parallel defaults, interpreted by its compiler plugin; a nested map under an
	// architecture name (e.g. gfx942) overrides them on that architecture.
	Defaults map[string]interface{} `json:"defaults,omitempty"`
	// MinNodes is keyed by <trainType>-<variant>[-separate]; see MinNodesFor.
	MinNodes        map[string]int `json:"minNodes"`
	Unverified      []string       `json:"unverified,omitempty"`
	MinHostMemoryTB float64        `json:"minHostMemoryTB,omitempty"`
	// ConvertNodes is how many nodes the torch_dist conversion needs. Unused when HFDirect is set.
	ConvertNodes int `json:"convertNodes,omitempty"`
	// HFDirect means training loads the base HF weights as --ref-load; the convert stage only builds FP8
	// weights when the recipe asks for an FP8 rollout.
	HFDirect bool `json:"hfDirect,omitempty"`
}

// Lora lists the modules a spec allows LoRA on.
type Lora struct {
	Targets               []string `json:"targets"`
	ExpertTargets         []string `json:"expertTargets,omitempty"`
	AdapterOnlyCheckpoint bool     `json:"adapterOnlyCheckpoint,omitempty"`
}

// Rollout lists the RL rollout options a spec allows.
type Rollout struct {
	FP8             bool `json:"fp8,omitempty"`
	PD              bool `json:"pd,omitempty"`
	RequireSeparate bool `json:"requireSeparate,omitempty"`
}

// Freeze tells whether full training may freeze parameters, and the default patterns to train.
type Freeze struct {
	Allowed bool     `json:"allowed"`
	Default []string `json:"default,omitempty"`
}

// Image is one training image of a framework.
type Image struct {
	Image           string            `json:"image"`
	Framework       string            `json:"framework"`
	Arch            []string          `json:"arch"`
	FrameworkCommit string            `json:"frameworkCommit"`
	MegatronBridge  string            `json:"megatronBridge,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	Validated       bool              `json:"validated"`
	// Default marks the image new runs use for its framework and architecture; once any image is validated for
	// them, exactly one validated image is the default.
	Default bool `json:"default,omitempty"`
	// Deprecated images are no longer offered for new runs but still serve the runs that use them.
	Deprecated bool                   `json:"deprecated,omitempty"`
	Features   []string               `json:"features,omitempty"`
	Ext        map[string]interface{} `json:"ext,omitempty"`
}

// Network holds multi-node network settings.
type Network struct {
	AINIC *AINIC `json:"ainic,omitempty"`
	// RdmaDevices lists, per cluster, the RDMA NICs Mooncake PD may use.
	RdmaDevices map[string][]string `json:"rdmaDevices,omitempty"`
}

// AINIC holds the environment for clusters with AINIC networking.
type AINIC struct {
	Env map[string]string `json:"env,omitempty"`
}

// Evaluation configures the temporary inference service and the output probes of automatic evaluation.
type Evaluation struct {
	Inference Inference `json:"inference,omitempty"`
	Probes    []Probe   `json:"probes,omitempty"`
}

// Inference describes how to start an evaluation inference service.
type Inference struct {
	// Image is a concrete image, or "fromImages" for the default image of the run's framework and architecture.
	Image          string            `json:"image,omitempty"`
	Command        string            `json:"command,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	ExtraArgs      string            `json:"extraArgs,omitempty"`
	Port           int               `json:"port,omitempty"`
	ReadinessPath  string            `json:"readinessPath,omitempty"`
	MaxWaitSeconds int               `json:"maxWaitSeconds,omitempty"`
}

// Probe is one greedy request the evaluation pre-check sends to the inference service.
type Probe struct {
	Prompt string `json:"prompt"`
	Expect string `json:"expect"`
	Answer string `json:"answer,omitempty"`
}

// Parse decodes registry YAML strictly and validates it.
func Parse(data []byte) (*Registry, error) {
	r := &Registry{}
	if err := yaml.UnmarshalStrict(data, r); err != nil {
		return nil, fmt.Errorf("invalid registry: %v", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return r, nil
}

// Validate checks the rules a registry version must satisfy before it replaces the previous one.
func (r *Registry) Validate() error {
	var errs []string
	add := func(format string, args ...interface{}) { errs = append(errs, fmt.Sprintf(format, args...)) }

	for _, m := range r.Imports.Mounts {
		if m.Cluster == "" || m.Workspace == "" || !path.IsAbs(m.Root) || path.Clean(m.Root) != m.Root {
			add("import mount %q needs a cluster, a workspace and a clean absolute root", m.Cluster)
		}
	}

	frameworks := map[string]bool{}
	defaults := 0
	for _, f := range r.Frameworks {
		switch {
		case f.Name == "":
			add("a framework has no name")
		case frameworks[f.Name]:
			add("framework %s is listed twice", f.Name)
		case f.Compiler == "":
			add("framework %s has no compiler", f.Name)
		case len(f.Metrics.Include) == 0:
			add("framework %s has no metrics.include", f.Name)
		case f.Metrics.MaxPointsPerRun <= 0:
			add("framework %s needs a positive metrics.maxPointsPerRun", f.Name)
		}
		frameworks[f.Name] = true
		if f.Default {
			defaults++
		}
	}
	if len(r.Frameworks) == 0 {
		add("no framework is listed")
	} else if defaults != 1 {
		add("exactly one framework must be the default, found %d", defaults)
	}

	archs := map[string]bool{}
	for model, arch := range r.GPUArch {
		if arch == "" {
			add("gpuArch %s maps to an empty architecture", model)
		}
		archs[arch] = true
	}

	names := map[string]bool{}
	for i := range r.Specs {
		s := &r.Specs[i]
		if s.Name == "" {
			add("a spec has no name")
			continue
		}
		if names[s.Name] {
			add("spec %s is listed twice", s.Name)
		}
		names[s.Name] = true
		if len(s.Match.Architectures) == 0 {
			add("spec %s has no match.architectures", s.Name)
		}
		if len(s.Frameworks) == 0 {
			add("spec %s supports no framework", s.Name)
		}
		for fw, fs := range s.Frameworks {
			if !frameworks[fw] {
				add("spec %s refers to unknown framework %s", s.Name, fw)
			}
			if fs == nil {
				add("spec %s has an empty %s section", s.Name, fw)
				continue
			}
			for _, msg := range fs.validate() {
				add("spec %s, framework %s: %s", s.Name, fw, msg)
			}
		}
		for j := 0; j < i; j++ {
			if r.Specs[j].Name != "" && s.Match.overlaps(&r.Specs[j].Match) {
				add("specs %s and %s can match the same model", r.Specs[j].Name, s.Name)
			}
		}
	}

	validated, defaultImages := map[string]int{}, map[string][]string{}
	for _, img := range r.Images {
		if img.Image == "" {
			add("an image has no name")
			continue
		}
		if !frameworks[img.Framework] {
			add("image %s refers to unknown framework %q", img.Image, img.Framework)
		}
		if len(img.Arch) == 0 {
			add("image %s lists no architecture", img.Image)
		}
		for _, arch := range img.Arch {
			if !archs[arch] {
				add("image %s lists architecture %s that gpuArch does not map to", img.Image, arch)
			}
			key := img.Framework + "/" + arch
			if img.Validated && !img.Deprecated {
				validated[key]++
			}
			if img.Default {
				defaultImages[key] = append(defaultImages[key], img.Image)
			}
		}
		for _, f := range img.Features {
			if !knownFeatures[f] {
				add("image %s lists unknown feature %s", img.Image, f)
			}
		}
		if img.Default && (!img.Validated || img.Deprecated) {
			add("default image %s must be validated and not deprecated", img.Image)
		}
	}
	for key, n := range validated {
		if d := defaultImages[key]; n > 0 && len(d) != 1 {
			add("%s needs exactly one default image, found %d", key, len(d))
		}
	}
	for key, d := range defaultImages {
		if len(d) > 1 && validated[key] == 0 {
			add("%s has %d default images", key, len(d))
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("invalid registry: %s", strings.Join(errs, "; "))
	}
	return nil
}

// validate checks one framework section of a spec, including that minNodes covers every allowed combination.
func (fs *FrameworkSpec) validate() []string {
	var errs []string
	if len(fs.Methods) == 0 {
		errs = append(errs, "no methods")
	}
	for trainType, methods := range fs.Methods {
		if trainType != TrainTypeSFT && trainType != TrainTypeRL {
			errs = append(errs, fmt.Sprintf("unknown train type %s", trainType))
			continue
		}
		for _, m := range methods {
			if m != MethodFull && m != MethodLora {
				errs = append(errs, fmt.Sprintf("unknown method %s", m))
			}
		}
		for _, c := range fs.Combinations(trainType) {
			if _, ok := fs.MinNodesFor(trainType, c.Variant, c.Separate); !ok {
				errs = append(errs, fmt.Sprintf("minNodes has no entry for %s", MinNodesKey(trainType, c.Variant, c.Separate)))
			}
		}
	}
	if fs.Lora != nil && len(fs.Lora.Targets) == 0 {
		errs = append(errs, "lora.targets is empty")
	}
	return errs
}

// Combination is one allowed variant of a train type, with or without a separate rollout.
type Combination struct {
	Variant  string
	Separate bool
}

// Combinations lists the variant and layout combinations a framework section allows for a train type. RL with LoRA
// only runs with a separate rollout, and so does every RL combination of a spec that requires it.
func (fs *FrameworkSpec) Combinations(trainType string) []Combination {
	var variants []string
	for _, m := range fs.Methods[trainType] {
		switch m {
		case MethodFull:
			variants = append(variants, VariantFull)
			if fs.Freeze != nil && fs.Freeze.Allowed {
				variants = append(variants, VariantFreeze)
			}
		case MethodLora:
			variants = append(variants, VariantLora)
		}
	}
	var out []Combination
	for _, v := range variants {
		if trainType == TrainTypeSFT {
			out = append(out, Combination{Variant: v})
			continue
		}
		requireSeparate := v == VariantLora || (fs.Rollout != nil && fs.Rollout.RequireSeparate)
		if !requireSeparate {
			out = append(out, Combination{Variant: v})
		}
		out = append(out, Combination{Variant: v, Separate: true})
	}
	return out
}

// MinNodesKey builds the minNodes key <trainType>-<variant>[-separate].
func MinNodesKey(trainType, variant string, separate bool) string {
	key := trainType
	if variant != "" {
		key += "-" + variant
	}
	if separate {
		key += "-separate"
	}
	return key
}

// MinNodesFor looks up the full key first and then the key without the variant, so rl-separate covers every RL
// variant that has no entry of its own.
func (fs *FrameworkSpec) MinNodesFor(trainType, variant string, separate bool) (int, bool) {
	if n, ok := fs.MinNodes[MinNodesKey(trainType, variant, separate)]; ok {
		return n, true
	}
	n, ok := fs.MinNodes[MinNodesKey(trainType, "", separate)]
	return n, ok
}

// IsUnverified tells whether a combination is selectable but has not been verified, using the minNodes key rules.
func (fs *FrameworkSpec) IsUnverified(trainType, variant string, separate bool) bool {
	full, short := MinNodesKey(trainType, variant, separate), MinNodesKey(trainType, "", separate)
	for _, k := range fs.Unverified {
		if k == full || k == short {
			return true
		}
	}
	return false
}

// overlaps tells whether some model could hit both matches: their architectures intersect and no field is set to
// different values in both.
func (m *Match) overlaps(o *Match) bool {
	shared := false
	for _, a := range m.Architectures {
		for _, b := range o.Architectures {
			if a == b {
				shared = true
			}
		}
	}
	if !shared {
		return false
	}
	if m.ModelType != "" && o.ModelType != "" && m.ModelType != o.ModelType {
		return false
	}
	return !differs(m.NumHiddenLayers, o.NumHiddenLayers) && !differs(m.HiddenSize, o.HiddenSize) &&
		!differs(m.NumAttentionHeads, o.NumAttentionHeads) && !differs(m.NumKeyValueHeads, o.NumKeyValueHeads) &&
		!differs(m.NumExperts, o.NumExperts) && !differs(m.MaxPositionEmbeddings, o.MaxPositionEmbeddings) &&
		!(m.RopeTheta != nil && o.RopeTheta != nil && *m.RopeTheta != *o.RopeTheta)
}

// differs tells whether both values are set and unequal.
func differs(a, b *int) bool {
	return a != nil && b != nil && *a != *b
}

// Hits tells whether a model structure summary satisfies the match.
func (m *Match) Hits(cfg *v1.ModelConfig) bool {
	if cfg == nil {
		return false
	}
	for _, want := range m.Architectures {
		found := false
		for _, have := range cfg.Architectures {
			if have == want {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if m.ModelType != "" && m.ModelType != cfg.ModelType {
		return false
	}
	eq := func(want *int, have int) bool { return want == nil || *want == have }
	if !eq(m.NumHiddenLayers, cfg.NumHiddenLayers) || !eq(m.HiddenSize, cfg.HiddenSize) ||
		!eq(m.NumAttentionHeads, cfg.NumAttentionHeads) || !eq(m.NumKeyValueHeads, cfg.NumKeyValueHeads) ||
		!eq(m.NumExperts, cfg.NumExperts) || !eq(m.MaxPositionEmbeddings, cfg.MaxPositionEmbeddings) {
		return false
	}
	if m.RopeTheta != nil {
		theta, err := strconv.ParseFloat(cfg.RopeTheta, 64)
		if err != nil || theta != *m.RopeTheta {
			return false
		}
	}
	return true
}

// MatchSpec returns the only spec a model structure hits, or nil when none does.
func (r *Registry) MatchSpec(cfg *v1.ModelConfig) *Spec {
	for i := range r.Specs {
		if r.Specs[i].Match.Hits(cfg) {
			return &r.Specs[i]
		}
	}
	return nil
}

// FrameworkByName returns a framework, or the default framework when name is empty.
func (r *Registry) FrameworkByName(name string) *Framework {
	for i := range r.Frameworks {
		f := &r.Frameworks[i]
		if (name == "" && f.Default) || (name != "" && f.Name == name) {
			return f
		}
	}
	return nil
}

// Arch maps a GPU model, such as MI355X, to its architecture; it returns "" for an unknown model.
func (r *Registry) Arch(gpuModel string) string {
	return r.GPUArch[gpuModel]
}

// DefaultImage returns the default image of a framework on an architecture, or nil before any image is validated
// there.
func (r *Registry) DefaultImage(framework, arch string) *Image {
	for i := range r.Images {
		img := &r.Images[i]
		if img.Framework == framework && img.Default && img.HasArch(arch) {
			return img
		}
	}
	return nil
}

// ImageByName returns a listed image by its name.
func (r *Registry) ImageByName(name string) *Image {
	for i := range r.Images {
		if r.Images[i].Image == name {
			return &r.Images[i]
		}
	}
	return nil
}

// HasArch tells whether the image is built for an architecture.
func (img *Image) HasArch(arch string) bool {
	for _, a := range img.Arch {
		if a == arch {
			return true
		}
	}
	return false
}

// HasFeature tells whether a feature has passed acceptance on the image.
func (img *Image) HasFeature(feature string) bool {
	for _, f := range img.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// CheckRemovedImages rejects a new version that drops an image of the previous version which unfinished runs
// still use; inUse lists those runs' images.
func CheckRemovedImages(prev, next *Registry, inUse []string) error {
	if prev == nil {
		return nil
	}
	used := map[string]bool{}
	for _, img := range inUse {
		used[img] = true
	}
	var removed []string
	for _, img := range prev.Images {
		if used[img.Image] && next.ImageByName(img.Image) == nil {
			removed = append(removed, img.Image)
		}
	}
	if len(removed) > 0 {
		sort.Strings(removed)
		return fmt.Errorf("invalid registry: unfinished runs still use the removed images %s", strings.Join(removed, ", "))
	}
	return nil
}

// Store keeps the last valid registry; a version that fails to parse or validate is rejected and the previous one
// stays in use.
type Store struct {
	mu      sync.RWMutex
	current *Registry
	lastErr error
}

// Load parses data and, when it is valid and passes check, makes it the current registry. check may be nil.
func (s *Store) Load(data []byte, check func(*Registry) error) error {
	r, err := Parse(data)
	if err == nil && check != nil {
		err = check(r)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err
	if err == nil {
		s.current = r
	}
	return err
}

// Get returns the current registry, or the latest load error when no version has been valid yet.
func (s *Store) Get() (*Registry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.current != nil {
		return s.current, nil
	}
	if s.lastErr != nil {
		return nil, s.lastErr
	}
	return nil, fmt.Errorf("the Post Training registry has not been loaded")
}

// LastError returns the error of the latest load, nil when it succeeded.
func (s *Store) LastError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastErr
}
