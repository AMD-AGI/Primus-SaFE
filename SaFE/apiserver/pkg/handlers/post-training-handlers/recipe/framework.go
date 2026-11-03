/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package recipe compiles Post Training requests into recipes. It is the only place the rules live: the models,
// config, preview and submit APIs all call Compile or Support, and the training framework parts are plugins that
// register themselves by name.
package recipe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

const (
	// CompilerVersion is recorded in every recipe and is part of the recipe hash.
	CompilerVersion = "1.0.0"
	// RecipeSchema identifies the recipe layout the launcher reads.
	RecipeSchema = "safe.post-training.recipe/v1"

	LevelError   = "error"
	LevelWarning = "warning"

	LayoutColocate = "colocate"
	LayoutSeparate = "separate"

	AlgorithmGRPO = "grpo"
	AlgorithmPPO  = "ppo"

	RolloutPrecisionFP8 = "fp8"
	ScheduleAsync       = "async"
	ExpertModePerExpert = "perExpert"

	// DatasetTypeSFT and DatasetTypeRL are the dataset types each train type accepts.
	DatasetTypeSFT = "sft"
	DatasetTypeRL  = "rlhf"
)

// SubmitRequest is the body of the preview and submit APIs.
type SubmitRequest struct {
	Workspace        string                 `json:"workspace"`
	DisplayName      string                 `json:"displayName"`
	TrainType        string                 `json:"trainType"`
	Method           string                 `json:"method"`
	Framework        string                 `json:"framework,omitempty"`
	FrameworkOptions map[string]interface{} `json:"frameworkOptions,omitempty"`
	Ext              map[string]interface{} `json:"ext,omitempty"`
	TemplateID       string                 `json:"templateId,omitempty"`
	ModelID          string                 `json:"modelId"`
	DatasetID        string                 `json:"datasetId"`
	EvalDatasetID    string                 `json:"evalDatasetId,omitempty"`
	Preset           string                 `json:"preset,omitempty"`
	Choices          Choices                `json:"choices"`
	Resources        Resources              `json:"resources"`
	Export           Export                 `json:"export"`
	Tracking         Tracking               `json:"tracking"`
	Evaluation       EvaluationRequest      `json:"evaluation"`
}

// Choices are the training options. Zero values mean "use the default"; the framework plugin fills the defaults
// in, and the filled-in choices are what the recipe stores and the hash covers.
type Choices struct {
	FieldMap         map[string]string `json:"fieldMap,omitempty"`
	NumEpoch         int               `json:"numEpoch,omitempty"`
	NumRollout       int               `json:"numRollout,omitempty"`
	BatchSize        int               `json:"batchSize,omitempty"`
	MaxTokensPerGpu  int               `json:"maxTokensPerGpu,omitempty"`
	Lr               float64           `json:"lr,omitempty"`
	MinLr            float64           `json:"minLr,omitempty"`
	WarmupIters      int               `json:"warmupIters,omitempty"`
	SaveInterval     int               `json:"saveInterval,omitempty"`
	ValInterval      int               `json:"valInterval,omitempty"`
	ValMaxSamples    int               `json:"valMaxSamples,omitempty"`
	ValSplit         float64           `json:"valSplit,omitempty"`
	TP               int               `json:"tp,omitempty"`
	PP               int               `json:"pp,omitempty"`
	CP               int               `json:"cp,omitempty"`
	EP               int               `json:"ep,omitempty"`
	SequenceParallel bool              `json:"sequenceParallel,omitempty"`
	Lora             *LoraChoice       `json:"lora,omitempty"`
	Freeze           *FreezeChoice     `json:"freeze,omitempty"`

	EvalInterval         int           `json:"evalInterval,omitempty"`
	AdvantageEstimator   string        `json:"advantageEstimator,omitempty"`
	RewardType           string        `json:"rewardType,omitempty"`
	RewardHook           string        `json:"rewardHook,omitempty"`
	RewardRemote         *RewardRemote `json:"rewardRemote,omitempty"`
	GenerateHook         string        `json:"generateHook,omitempty"`
	NSamplesPerPrompt    int           `json:"nSamplesPerPrompt,omitempty"`
	MaxPromptLen         int           `json:"maxPromptLen,omitempty"`
	MaxResponseLen       int           `json:"maxResponseLen,omitempty"`
	Layout               string        `json:"layout,omitempty"`
	RolloutGpus          int           `json:"rolloutGpus,omitempty"`
	GpusPerEngine        int           `json:"gpusPerEngine,omitempty"`
	UseKlLoss            *bool         `json:"useKlLoss,omitempty"`
	KlLossCoef           float64       `json:"klLossCoef,omitempty"`
	Correction           string        `json:"correction,omitempty"`
	RolloutPrecision     string        `json:"rolloutPrecision,omitempty"`
	KvCacheDtype         string        `json:"kvCacheDtype,omitempty"`
	Schedule             string        `json:"schedule,omitempty"`
	Concurrency          int           `json:"concurrency,omitempty"`
	PD                   *PDChoice     `json:"pd,omitempty"`
	CriticLr             float64       `json:"criticLr,omitempty"`
	CriticWarmupRollouts int           `json:"criticWarmupRollouts,omitempty"`
}

// LoraChoice holds the LoRA options.
type LoraChoice struct {
	Rank                  int      `json:"rank,omitempty"`
	Alpha                 float64  `json:"alpha,omitempty"`
	Dropout               float64  `json:"dropout,omitempty"`
	Targets               []string `json:"targets,omitempty"`
	Experts               bool     `json:"experts,omitempty"`
	ExpertMode            string   `json:"expertMode,omitempty"`
	AdapterOnlyCheckpoint bool     `json:"adapterOnlyCheckpoint,omitempty"`
}

// FreezeChoice trains only the parameters matching TrainOnly during full training.
type FreezeChoice struct {
	TrainOnly []string `json:"trainOnly,omitempty"`
}

// RewardRemote points RL at a remote reward service.
type RewardRemote struct {
	URL       string `json:"url,omitempty"`
	SecretID  string `json:"secretId,omitempty"`
	RewardKey string `json:"rewardKey,omitempty"`
}

// PDChoice enables Mooncake prefill-decode disaggregation for the rollout.
type PDChoice struct {
	Enabled     bool `json:"enabled,omitempty"`
	PrefillGpus int  `json:"prefillGpus,omitempty"`
}

// Resources is the requested layout and scheduling of the run.
type Resources struct {
	Nodes          int    `json:"nodes"`
	GpusPerNode    int    `json:"gpusPerNode"`
	Priority       int    `json:"priority,omitempty"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
	MaxRetry       *int   `json:"maxRetry,omitempty"`
	Image          string `json:"image,omitempty"`
}

// Export selects what the run exports when training ends.
type Export struct {
	HF          *bool `json:"hf,omitempty"`
	PeftAdapter bool  `json:"peftAdapter,omitempty"`
}

// Tracking holds optional experiment tracking.
type Tracking struct {
	Wandb Wandb `json:"wandb"`
}

// Wandb reports metrics to a W&B project with a key from a general Secret.
type Wandb struct {
	Enabled  bool   `json:"enabled,omitempty"`
	SecretID string `json:"secretId,omitempty"`
	Project  string `json:"project,omitempty"`
	Team     string `json:"team,omitempty"`
}

// EvaluationRequest controls the automatic evaluation after training.
type EvaluationRequest struct {
	Enabled     bool     `json:"enabled,omitempty"`
	Benchmarks  []string `json:"benchmarks,omitempty"`
	KeepService bool     `json:"keepService,omitempty"`
}

// ModelInfo is what the compiler knows about the base model in the target workspace.
type ModelInfo struct {
	ID    string
	Path  string
	Ready bool
	// Config is the structure summary; nil while it is being read or when reading failed (ConfigError is set).
	Config      *v1.ModelConfig
	ConfigError string
	MaxTokens   int
}

// DatasetInfo is what the compiler knows about a dataset in the target workspace.
type DatasetInfo struct {
	ID     string
	Type   string
	Path   string
	Format string
	Ready  bool
	// Fields lists the field names found in the first rows; nil when no sample was read.
	Fields []string
}

// WorkspaceInfo describes the target workspace.
type WorkspaceInfo struct {
	ID       string
	Cluster  string
	GPUModel string
	Nodes    int
}

// Input is everything one compilation needs; the API handlers gather it.
type Input struct {
	Registry    *registry.Registry
	Request     *SubmitRequest
	RunID       string
	RunDir      string
	Model       ModelInfo
	Dataset     *DatasetInfo
	EvalDataset *DatasetInfo
	Workspace   WorkspaceInfo
}

// Layout is the GPU split of a run.
type Layout struct {
	Nodes       int  `json:"nodes"`
	GpusPerNode int  `json:"gpusPerNode"`
	Colocate    bool `json:"colocate"`
	ActorGpus   int  `json:"actorGpus"`
	RolloutGpus int  `json:"rolloutGpus"`
}

// Diagnostic is one error or warning shown next to a field.
type Diagnostic struct {
	Level   string `json:"level"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// Context is what a framework plugin sees of one compilation.
type Context struct {
	Input         *Input
	Request       *SubmitRequest
	Spec          *registry.Spec
	FrameworkSpec *registry.FrameworkSpec
	Image         *registry.Image
	Arch          string
	Layout        Layout
	// CustomImage is set when the request names an image outside the image table; such runs use no conversion
	// cache.
	CustomImage bool
}

// Options are the wizard's choices for one framework: defaults, presets and per-field values with reasons for the
// disabled ones.
type Options struct {
	Defaults Choices                 `json:"defaults"`
	Presets  []Preset                `json:"presets,omitempty"`
	Fields   map[string]FieldOptions `json:"fields,omitempty"`
}

// Preset is a named starting point in the wizard.
type Preset struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	TrainType string  `json:"trainType"`
	Method    string  `json:"method"`
	Choices   Choices `json:"choices"`
}

// FieldOptions lists the allowed values of one field.
type FieldOptions struct {
	Values   []OptionValue `json:"values,omitempty"`
	Disabled bool          `json:"disabled,omitempty"`
	Reason   string        `json:"reason,omitempty"`
}

// OptionValue is one value of a field.
type OptionValue struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Disabled bool   `json:"disabled,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// FrameworkPlan is a plugin's compilation result.
type FrameworkPlan struct {
	// Choices are the request choices with the framework defaults filled in.
	Choices Choices
	// Args are the training arguments passed to the framework.
	Args []string
	// Files are written into the run directory before training, keyed by relative path.
	Files map[string]string
	// Env is added to the training workload.
	Env     map[string]string
	Lora    *LoraChoice
	Val     *RecipeVal
	Rollout map[string]interface{}
	Critic  map[string]interface{}
	Convert *RecipeConvert
}

// Framework is a training framework's compiler plugin.
type Framework interface {
	// Name matches frameworks[].compiler in the registry.
	Name() string
	// Options returns the wizard's choices for the request's train type and method.
	Options(ctx *Context) *Options
	// Validate applies the framework's rules on top of the platform rules.
	Validate(ctx *Context) []Diagnostic
	// Compile turns a valid request into training arguments and files.
	Compile(ctx *Context) (*FrameworkPlan, error)
}

var (
	frameworksMu sync.RWMutex
	frameworks   = map[string]Framework{}
)

// Register adds a framework plugin; plugins call it from init(). Registering a name twice panics.
func Register(f Framework) {
	frameworksMu.Lock()
	defer frameworksMu.Unlock()
	if _, ok := frameworks[f.Name()]; ok {
		panic(fmt.Sprintf("post training framework %s registered twice", f.Name()))
	}
	frameworks[f.Name()] = f
}

// Lookup returns the plugin registered under name, or nil.
func Lookup(name string) Framework {
	frameworksMu.RLock()
	defer frameworksMu.RUnlock()
	return frameworks[name]
}

// CheckRegistry rejects a registry that names a compiler no plugin registered; use it as the registry Store check.
func CheckRegistry(r *registry.Registry) error {
	for _, f := range r.Frameworks {
		if Lookup(f.Compiler) == nil {
			return fmt.Errorf("invalid registry: framework %s uses compiler %s, which is not built in", f.Name, f.Compiler)
		}
	}
	return nil
}

// Recipe is the compiled run description: the launcher's input and the snapshot stored with the run.
type Recipe struct {
	Schema           string                 `json:"schema"`
	RunID            string                 `json:"runId"`
	TrainType        string                 `json:"trainType"`
	Method           string                 `json:"method"`
	Algorithm        string                 `json:"algorithm,omitempty"`
	Framework        string                 `json:"framework"`
	FrameworkVersion string                 `json:"frameworkVersion"`
	FrameworkOptions map[string]interface{} `json:"frameworkOptions"`
	Ext              map[string]interface{} `json:"ext"`
	Model            RecipeModel            `json:"model"`
	Data             RecipeData             `json:"data"`
	Layout           Layout                 `json:"layout"`
	Choices          Choices                `json:"choices"`
	FrameworkArgs    []string               `json:"frameworkArgs"`
	Files            map[string]string      `json:"files,omitempty"`
	Lora             *LoraChoice            `json:"lora"`
	Val              *RecipeVal             `json:"val"`
	Rollout          map[string]interface{} `json:"rollout"`
	Critic           map[string]interface{} `json:"critic"`
	Checkpoint       registry.Checkpoint    `json:"checkpoint"`
	Watchdog         registry.Watchdog      `json:"watchdog"`
	Metrics          RecipeMetrics          `json:"metrics"`
	Convert          *RecipeConvert         `json:"convert,omitempty"`
	Output           RecipeOutput           `json:"output"`
	Provenance       Provenance             `json:"provenance"`
}

// RecipeModel identifies the base model. Script is the framework's model script, which the launcher adapter turns
// into the model structure arguments.
type RecipeModel struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Spec   string `json:"spec"`
	Script string `json:"script,omitempty"`
}

// RecipeData points at the training and evaluation data.
type RecipeData struct {
	TrainID  string            `json:"trainId"`
	Train    string            `json:"train"`
	Eval     string            `json:"eval"`
	Format   string            `json:"format,omitempty"`
	FieldMap map[string]string `json:"fieldMap,omitempty"`
}

// RecipeVal configures the SFT validation loss.
type RecipeVal struct {
	Interval   int     `json:"interval"`
	MaxSamples int     `json:"maxSamples"`
	Split      float64 `json:"split"`
}

// RecipeMetrics tells the launcher which metric names to forward.
type RecipeMetrics struct {
	Include     []string          `json:"include"`
	Tensorboard bool              `json:"tensorboard"`
	Samples     *registry.Samples `json:"samples,omitempty"`
}

// RecipeConvert describes the weight conversion before training.
type RecipeConvert struct {
	CacheKey    string `json:"cacheKey"`
	Dir         string `json:"dir"`
	Nodes       int    `json:"nodes"`
	SeparateJob bool   `json:"separateJob"`
	FP8         bool   `json:"fp8"`
}

// RecipeOutput is where the run writes its outputs.
type RecipeOutput struct {
	Base        string `json:"base"`
	ExportHF    bool   `json:"exportHf"`
	PeftAdapter bool   `json:"peftAdapter"`
}

// Provenance records what produced the recipe.
type Provenance struct {
	Compiler        string `json:"compiler"`
	FrameworkCommit string `json:"frameworkCommit"`
	Image           string `json:"image"`
	SpecSha256      string `json:"specSha256"`
	RecipeHash      string `json:"recipeHash"`
}

// Result is the outcome of Compile: diagnostics, and the recipe when there is no error.
type Result struct {
	Diagnostics []Diagnostic
	Recipe      *Recipe
	Spec        *registry.Spec
	Image       *registry.Image
	Plan        *FrameworkPlan
}

// HasErrors tells whether any diagnostic is an error.
func (r *Result) HasErrors() bool {
	for _, d := range r.Diagnostics {
		if d.Level == LevelError {
			return true
		}
	}
	return false
}

// errorf appends an error diagnostic.
func (r *Result) errorf(field, format string, args ...interface{}) {
	r.Diagnostics = append(r.Diagnostics, Diagnostic{Level: LevelError, Field: field, Message: fmt.Sprintf(format, args...)})
}

// warnf appends a warning diagnostic.
func (r *Result) warnf(field, format string, args ...interface{}) {
	r.Diagnostics = append(r.Diagnostics, Diagnostic{Level: LevelWarning, Field: field, Message: fmt.Sprintf(format, args...)})
}

// Compile applies the platform rules and the framework plugin to a request and, when nothing is wrong, assembles
// the recipe and its hash. It only computes in memory.
func Compile(in *Input) *Result {
	res := &Result{}
	ctx, plugin, fw := resolve(in, res)
	if ctx == nil || res.HasErrors() {
		return res
	}
	res.Spec, res.Image = ctx.Spec, ctx.Image
	platformRules(ctx, res)
	res.Diagnostics = append(res.Diagnostics, plugin.Validate(ctx)...)
	if res.HasErrors() {
		return res
	}
	plan, err := plugin.Compile(ctx)
	if err != nil {
		res.errorf("", "%v", err)
		return res
	}
	res.Plan = plan
	res.Recipe = assemble(ctx, fw, plan)
	return res
}

// Support tells whether a model can be trained with a framework, train type and method in a workspace, and why
// not; trainType and method may be empty to ask about the model as a whole.
func Support(in *Input) (bool, string) {
	res := &Result{}
	ctx, _, _ := resolve(in, res)
	if ctx != nil && !res.HasErrors() && in.Request.TrainType != "" {
		checkMethod(ctx, res)
	}
	for _, d := range res.Diagnostics {
		if d.Level == LevelError {
			return false, d.Message
		}
	}
	return true, ""
}

// FrameworkOptions returns the wizard's options of the request's framework, or the reason they are unavailable.
func FrameworkOptions(in *Input) (*Options, string) {
	res := &Result{}
	ctx, plugin, _ := resolve(in, res)
	for _, d := range res.Diagnostics {
		if d.Level == LevelError {
			return nil, d.Message
		}
	}
	return plugin.Options(ctx), ""
}

// resolve finds the framework, plugin, spec, architecture and image of a request, recording why any is missing.
func resolve(in *Input, res *Result) (*Context, Framework, *registry.Framework) {
	reg, req := in.Registry, in.Request
	if reg == nil {
		res.errorf("", "The Post Training registry is not loaded")
		return nil, nil, nil
	}
	fw := reg.FrameworkByName(req.Framework)
	if fw == nil {
		res.errorf("framework", "Framework %s is not available", req.Framework)
		return nil, nil, nil
	}
	plugin := Lookup(fw.Compiler)
	if plugin == nil {
		res.errorf("framework", "Framework %s has no compiler", fw.Name)
		return nil, nil, nil
	}
	if !in.Model.Ready {
		res.errorf("modelId", "The model is not ready in this workspace")
		return nil, nil, nil
	}
	if in.Model.Config == nil {
		if in.Model.ConfigError != "" {
			res.errorf("modelId", "Cannot read config.json: %s", in.Model.ConfigError)
		} else {
			res.errorf("modelId", "Reading the model structure")
		}
		return nil, nil, nil
	}
	spec := reg.MatchSpec(in.Model.Config)
	if spec == nil {
		res.errorf("modelId", "The model structure is not supported")
		return nil, nil, nil
	}
	fs := spec.Frameworks[fw.Name]
	if fs == nil {
		res.errorf("framework", "%s does not support %s", fw.Name, spec.Name)
		return nil, nil, nil
	}
	arch := reg.Arch(in.Workspace.GPUModel)
	if arch == "" {
		res.errorf("workspace", "The workspace GPU %q has no known architecture", in.Workspace.GPUModel)
		return nil, nil, nil
	}
	img, custom := selectImage(reg, req, fw.Name, arch, res)
	if res.HasErrors() {
		return nil, nil, nil
	}
	return &Context{Input: in, Request: req, Spec: spec, FrameworkSpec: fs, Image: img, Arch: arch,
		Layout: layoutOf(req), CustomImage: custom}, plugin, fw
}

// selectImage picks the image of a request: the default image of the framework and architecture, or the named one.
// A named image outside the table is compiled with the default image's framework commit.
func selectImage(reg *registry.Registry, req *SubmitRequest, framework, arch string,
	res *Result) (*registry.Image, bool) {
	def := reg.DefaultImage(framework, arch)
	name := req.Resources.Image
	if name == "" {
		if def == nil {
			res.errorf("workspace", "No validated %s image for workspace GPU architecture %s", framework, arch)
		}
		return def, false
	}
	img := reg.ImageByName(name)
	switch {
	case img == nil && def == nil:
		res.errorf("resources.image", "Image %s is not in the image table, and no validated %s image for %s "+
			"tells how to compile for it", name, framework, arch)
		return nil, false
	case img == nil:
		res.warnf("resources.image", "Image %s is not in the image table: only full SFT and GRPO without other "+
			"features, and no conversion cache", name)
		return &registry.Image{Image: name, Framework: framework, Arch: []string{arch},
			FrameworkCommit: def.FrameworkCommit}, true
	case img.Framework != framework:
		res.errorf("resources.image", "Image %s is not a %s image", name, framework)
	case !img.HasArch(arch):
		res.errorf("resources.image", "Image %s is not built for %s", name, arch)
	case img.Deprecated:
		res.warnf("resources.image", "Image %s is deprecated; new runs use the default image", name)
	case !img.Validated:
		res.warnf("resources.image", "Image %s has not been validated: only full SFT and GRPO without other "+
			"features", name)
	}
	return img, false
}

// layoutOf derives the GPU split from the request.
func layoutOf(req *SubmitRequest) Layout {
	l := Layout{Nodes: req.Resources.Nodes, GpusPerNode: req.Resources.GpusPerNode}
	total := l.Nodes * l.GpusPerNode
	switch {
	case req.TrainType != registry.TrainTypeRL:
		l.Colocate, l.ActorGpus = true, total
	case req.Choices.Layout == LayoutSeparate:
		l.RolloutGpus = req.Choices.RolloutGpus
		l.ActorGpus = total - l.RolloutGpus
	default:
		l.Colocate, l.ActorGpus, l.RolloutGpus = true, total, total
	}
	return l
}

// Variant returns the minNodes variant of a request: lora, freeze (full training with frozen parameters) or full.
func Variant(req *SubmitRequest) string {
	switch {
	case req.Method == registry.MethodLora:
		return registry.VariantLora
	case req.Choices.Freeze != nil && len(req.Choices.Freeze.TrainOnly) > 0:
		return registry.VariantFreeze
	default:
		return registry.VariantFull
	}
}

// separate tells whether an RL request uses a separate rollout.
func separate(req *SubmitRequest) bool {
	return req.TrainType == registry.TrainTypeRL && req.Choices.Layout == LayoutSeparate
}

// checkMethod checks that the spec and the image both allow the train type, method and requested features.
func checkMethod(ctx *Context, res *Result) {
	req, fs, img := ctx.Request, ctx.FrameworkSpec, ctx.Image
	if !contains(fs.Methods[req.TrainType], req.Method) {
		res.errorf("method", "%s does not support %s with %s", ctx.Spec.Name, req.TrainType, req.Method)
		return
	}
	for _, f := range requiredFeatures(req) {
		switch {
		case !img.Validated || ctx.CustomImage:
			if f != registry.FeatureFull {
				res.errorf("", "Image %s allows only full SFT and GRPO without other features, not %s", img.Image, f)
			}
		case !img.HasFeature(f):
			res.errorf("", "The image has not been validated for %s", f)
		}
	}
}

// requiredFeatures lists the image features a request uses.
func requiredFeatures(req *SubmitRequest) []string {
	c, rl := req.Choices, req.TrainType == registry.TrainTypeRL
	var f []string
	if req.Method == registry.MethodLora {
		f = append(f, registry.FeatureLora)
		if rl {
			f = append(f, registry.FeatureLoraRL)
		}
		if c.Lora != nil && c.Lora.Experts {
			f = append(f, registry.FeatureLoraExperts)
			if c.Lora.ExpertMode == ExpertModePerExpert {
				f = append(f, registry.FeatureLoraExpertsPerExpert)
			}
		}
		if c.Lora != nil && c.Lora.AdapterOnlyCheckpoint {
			f = append(f, registry.FeatureLoraAdapterCkpt)
		}
	} else {
		f = append(f, registry.FeatureFull)
	}
	if Variant(req) == registry.VariantFreeze {
		f = append(f, registry.FeatureFreeze)
	}
	if req.Export.PeftAdapter {
		f = append(f, registry.FeaturePeftExport)
	}
	if !rl {
		if req.EvalDatasetID != "" || c.ValSplit > 0 {
			f = append(f, registry.FeatureValLoss)
		}
		return f
	}
	if c.AdvantageEstimator == AlgorithmPPO {
		f = append(f, registry.FeaturePPO)
	}
	if c.Layout == LayoutSeparate {
		f = append(f, registry.FeatureSeparate)
	}
	if c.RolloutPrecision == RolloutPrecisionFP8 {
		f = append(f, registry.FeatureFP8Rollout)
	}
	if c.Schedule == ScheduleAsync {
		f = append(f, registry.FeatureAsync)
	}
	if c.PD != nil && c.PD.Enabled {
		f = append(f, registry.FeaturePD)
	}
	return f
}

// platformRules applies the framework-independent rules of a full request.
func platformRules(ctx *Context, res *Result) {
	in, req, fs := ctx.Input, ctx.Request, ctx.FrameworkSpec
	c, rl := req.Choices, req.TrainType == registry.TrainTypeRL
	if req.TrainType != registry.TrainTypeSFT && !rl {
		res.errorf("trainType", "Unknown train type %q", req.TrainType)
		return
	}
	if req.Method != registry.MethodFull && req.Method != registry.MethodLora {
		res.errorf("method", "Unknown method %q", req.Method)
		return
	}
	checkMethod(ctx, res)

	variant, sep := Variant(req), separate(req)
	if variant == registry.VariantFreeze {
		if fs.Freeze == nil || !fs.Freeze.Allowed {
			res.errorf("choices.freeze", "%s does not allow freezing parameters", ctx.Spec.Name)
		}
		if req.Method == registry.MethodLora {
			res.errorf("choices.freeze", "Freezing parameters cannot be combined with LoRA")
		}
	}
	if rl {
		if len(fs.Algorithms) > 0 && !contains(fs.Algorithms, c.AdvantageEstimator) {
			res.errorf("choices.advantageEstimator", "%s supports only %v", ctx.Spec.Name, fs.Algorithms)
		}
		if !sep && req.Method == registry.MethodLora {
			res.errorf("choices.layout", "RL with LoRA needs a separate rollout")
		}
		if !sep && fs.Rollout != nil && fs.Rollout.RequireSeparate {
			res.errorf("choices.layout", "%s needs a separate rollout", ctx.Spec.Name)
		}
		if c.RolloutPrecision == RolloutPrecisionFP8 && (fs.Rollout == nil || !fs.Rollout.FP8) {
			res.errorf("choices.rolloutPrecision", "FP8 rollout is not enabled for %s", ctx.Spec.Name)
		}
		if c.PD != nil && c.PD.Enabled && (fs.Rollout == nil || !fs.Rollout.PD) {
			res.errorf("choices.pd", "Mooncake PD is not enabled for %s", ctx.Spec.Name)
		}
		if c.MaxPromptLen > 0 && in.Model.MaxTokens > 0 && c.MaxPromptLen+c.MaxResponseLen > in.Model.MaxTokens {
			res.errorf("choices.maxResponseLen", "Prompt and response lengths exceed the model's %d tokens",
				in.Model.MaxTokens)
		}
	}
	if c.Lora != nil && c.Lora.Experts && (fs.Lora == nil || len(fs.Lora.ExpertTargets) == 0) {
		res.errorf("choices.lora.experts", "%s has no expert layers for LoRA", ctx.Spec.Name)
	}

	nodes := req.Resources.Nodes
	if nodes < 1 || req.Resources.GpusPerNode < 1 {
		res.errorf("resources.nodes", "Nodes and GPUs per node must be positive")
	}
	if min, ok := fs.MinNodesFor(req.TrainType, variant, sep); ok && nodes < min {
		res.errorf("resources.nodes", "This configuration needs at least %d nodes", min)
	}
	if in.Workspace.Nodes > 0 && nodes > in.Workspace.Nodes {
		res.errorf("resources.nodes", "The workspace has only %d nodes", in.Workspace.Nodes)
	}
	if ctx.CustomImage && fs.ConvertNodes > nodes {
		res.errorf("resources.nodes", "A run with an image outside the table converts weights in its own workload "+
			"and needs at least %d nodes", fs.ConvertNodes)
	}
	if fs.IsUnverified(req.TrainType, variant, sep) {
		res.warnf("", "This configuration of %s has not been verified", ctx.Spec.Name)
	}
	checkDataset(in, req, res)
}

// checkDataset checks the training and evaluation datasets.
func checkDataset(in *Input, req *SubmitRequest, res *Result) {
	want := DatasetTypeSFT
	if req.TrainType == registry.TrainTypeRL {
		want = DatasetTypeRL
	}
	ds := in.Dataset
	switch {
	case ds == nil:
		res.errorf("datasetId", "Select a training dataset")
		return
	case ds.Type != want:
		res.errorf("datasetId", "A %s run needs a %s dataset, not %s", req.TrainType, want, ds.Type)
	case !ds.Ready:
		res.errorf("datasetId", "The dataset is not ready in this workspace")
	}
	if ds.Fields != nil {
		keys := make([]string, 0, len(req.Choices.FieldMap))
		for k := range req.Choices.FieldMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if field := req.Choices.FieldMap[k]; field != "" && !contains(ds.Fields, field) {
				res.errorf("choices.fieldMap."+k, "Field %s is not in the first rows of the dataset", field)
			}
		}
	}
	if req.EvalDatasetID != "" && (in.EvalDataset == nil || !in.EvalDataset.Ready) {
		res.errorf("evalDatasetId", "The evaluation dataset is not ready in this workspace")
	}
}

// assemble builds the recipe from the request, the spec and the plugin's plan.
func assemble(ctx *Context, fw *registry.Framework, plan *FrameworkPlan) *Recipe {
	in, req := ctx.Input, ctx.Request
	exportHF := req.Export.HF == nil || *req.Export.HF
	r := &Recipe{
		Schema:           RecipeSchema,
		RunID:            in.RunID,
		TrainType:        req.TrainType,
		Method:           req.Method,
		Framework:        fw.Name,
		FrameworkVersion: ctx.Image.FrameworkCommit,
		FrameworkOptions: orEmpty(req.FrameworkOptions),
		Ext:              orEmpty(req.Ext),
		Model: RecipeModel{ID: in.Model.ID, Path: in.Model.Path, Spec: ctx.Spec.Name,
			Script: ctx.FrameworkSpec.ModelScript},
		Data:          RecipeData{TrainID: in.Dataset.ID, Train: in.Dataset.Path, FieldMap: plan.Choices.FieldMap},
		Layout:        ctx.Layout,
		Choices:       plan.Choices,
		FrameworkArgs: plan.Args,
		Files:         plan.Files,
		Lora:          plan.Lora,
		Val:           plan.Val,
		Rollout:       plan.Rollout,
		Critic:        plan.Critic,
		Checkpoint:    ctx.Spec.Checkpoint,
		Watchdog:      ctx.Spec.Watchdog,
		Metrics:       RecipeMetrics{Include: fw.Metrics.Include},
		Convert:       plan.Convert,
		Output:        RecipeOutput{Base: in.RunDir, ExportHF: exportHF, PeftAdapter: req.Export.PeftAdapter},
		Provenance: Provenance{Compiler: CompilerVersion, FrameworkCommit: ctx.Image.FrameworkCommit,
			Image: ctx.Image.Image, SpecSha256: SpecSha256(ctx.Spec)},
	}
	if req.TrainType == registry.TrainTypeRL {
		r.Algorithm = plan.Choices.AdvantageEstimator
	}
	r.Data.Format = in.Dataset.Format
	if in.EvalDataset != nil {
		r.Data.Eval = in.EvalDataset.Path
	}
	r.Provenance.RecipeHash = Hash(r)
	return r
}

// hashInput is the part of a recipe that decides whether two runs are the same configuration. Run ids, names,
// paths, templates, experiments, scheduling, tracking, evaluation and ext are left out.
type hashInput struct {
	Framework        string                 `json:"framework"`
	TrainType        string                 `json:"trainType"`
	Method           string                 `json:"method"`
	Algorithm        string                 `json:"algorithm,omitempty"`
	Choices          Choices                `json:"choices"`
	FrameworkOptions map[string]interface{} `json:"frameworkOptions,omitempty"`
	BaseModelID      string                 `json:"baseModelId"`
	DatasetID        string                 `json:"datasetId"`
	Layout           Layout                 `json:"layout"`
	Spec             string                 `json:"spec"`
	SpecSha256       string                 `json:"specSha256"`
	Image            string                 `json:"image"`
	Compiler         string                 `json:"compiler"`
}

// Hash returns the recipe hash: the sha256 of the canonical JSON of its hashed part.
func Hash(r *Recipe) string {
	return sha256Hex(hashInput{
		Framework: r.Framework, TrainType: r.TrainType, Method: r.Method, Algorithm: r.Algorithm,
		Choices: r.Choices, FrameworkOptions: r.FrameworkOptions, BaseModelID: r.Model.ID, DatasetID: r.Data.TrainID,
		Layout: r.Layout, Spec: r.Model.Spec, SpecSha256: r.Provenance.SpecSha256, Image: r.Provenance.Image,
		Compiler: r.Provenance.Compiler,
	})
}

// SpecSha256 returns the hash of a spec entry, so editing a spec changes the hash of the recipes that use it.
func SpecSha256(s *registry.Spec) string {
	return sha256Hex(s)
}

// sha256Hex hashes the JSON encoding of v; encoding/json sorts map keys, which keeps it canonical.
func sha256Hex(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// orEmpty returns m, or an empty map when m is nil, so recipes always carry the field.
func orEmpty(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	return m
}

// contains tells whether list holds s.
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
