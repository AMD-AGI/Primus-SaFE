// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package profiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// FrameworkConfig represents the collected framework configuration
type FrameworkConfig struct {
	Framework      string                 `json:"framework"`       // Framework type: primus, megatron, deepspeed, etc.
	Version        string                 `json:"version"`         // Config schema version
	Source         *ConfigSource          `json:"source"`          // Config source information
	RawConfig      map[string]interface{} `json:"raw_config"`      // Original config (optional)
	ExtractedPaths *ExtractedPaths        `json:"extracted_paths"` // Pre-extracted key paths
	CollectedAt    time.Time              `json:"collected_at"`    // Collection timestamp
}

// ConfigSource represents the source of the configuration
type ConfigSource struct {
	Type string `json:"type"` // config_file, env, cmdline, etc.
	Path string `json:"path"` // File path if type is config_file
}

// ExtractedPaths contains pre-extracted key paths from framework config
type ExtractedPaths struct {
	ProfilerDir    string            `json:"profiler_dir,omitempty"`
	TensorBoardDir string            `json:"tensorboard_dir,omitempty"`
	CheckpointDir  string            `json:"checkpoint_dir,omitempty"`
	LogDir         string            `json:"log_dir,omitempty"`
	WorkspaceDir   string            `json:"workspace_dir,omitempty"`
	CustomPaths    map[string]string `json:"custom_paths,omitempty"` // Framework-specific paths
}

// ProfilerLocation represents a profiler file location to scan
type ProfilerLocation struct {
	Directory    string   `json:"directory"`               // Directory to scan
	Patterns     []string `json:"patterns"`                // File patterns to match (e.g., "*.pt.trace.json")
	Recursive    bool     `json:"recursive"`               // Whether to scan recursively
	MaxDepth     int      `json:"max_depth,omitempty"`     // Max depth for recursive scan
	Priority     int      `json:"priority"`                // Priority order (lower = higher priority)
	Source       string   `json:"source"`                  // Source of this location (config, default, fallback)
	ProfileRanks []int    `json:"profile_ranks,omitempty"` // Specific ranks to look for
}

// FrameworkConfigExtractor interface for extracting config from different frameworks
type FrameworkConfigExtractor interface {
	// FrameworkType returns the framework type this extractor handles
	FrameworkType() string

	// Priority returns the priority of this extractor (lower = higher priority)
	Priority() int

	// CanHandle checks if this extractor can handle the given raw config
	CanHandle(rawConfig map[string]interface{}) bool

	// ExtractPaths extracts key paths from raw config
	ExtractPaths(rawConfig map[string]interface{}, env map[string]string) *ExtractedPaths

	// GetProfilerLocations returns profiler file locations to scan
	GetProfilerLocations(config *FrameworkConfig) []ProfilerLocation

	// GetTensorBoardDir returns the TensorBoard log directory
	GetTensorBoardDir(config *FrameworkConfig) string

	// GetCheckpointDir returns the checkpoint directory
	GetCheckpointDir(config *FrameworkConfig) string
}

// ExtractorRegistry manages framework config extractors
type ExtractorRegistry struct {
	mu         sync.RWMutex
	extractors map[string]FrameworkConfigExtractor
	ordered    []FrameworkConfigExtractor // Ordered by priority
}

var (
	globalRegistry     *ExtractorRegistry
	globalRegistryOnce sync.Once
)

// GetExtractorRegistry returns the global extractor registry
func GetExtractorRegistry() *ExtractorRegistry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewExtractorRegistry()
		// Register default extractors - these are registered via RegisterDefaultExtractors()
	})
	return globalRegistry
}

// RegisterDefaultExtractors registers all default framework config extractors
// This should be called during application initialization
func RegisterDefaultExtractors() {
	registry := GetExtractorRegistry()
	registry.Register(&PrimusExtractor{})
	registry.Register(&MegatronExtractor{})
	registry.Register(&DeepSpeedExtractor{})
	registry.Register(&GenericPyTorchExtractor{})
}

// init registers default extractors
func init() {
	RegisterDefaultExtractors()
}

// NewExtractorRegistry creates a new extractor registry
func NewExtractorRegistry() *ExtractorRegistry {
	return &ExtractorRegistry{
		extractors: make(map[string]FrameworkConfigExtractor),
		ordered:    make([]FrameworkConfigExtractor, 0),
	}
}

// Register registers an extractor
func (r *ExtractorRegistry) Register(extractor FrameworkConfigExtractor) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.extractors[extractor.FrameworkType()] = extractor

	// Insert in priority order
	inserted := false
	for i, e := range r.ordered {
		if extractor.Priority() < e.Priority() {
			r.ordered = append(r.ordered[:i], append([]FrameworkConfigExtractor{extractor}, r.ordered[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		r.ordered = append(r.ordered, extractor)
	}

	log.Infof("Registered framework config extractor: %s (priority=%d)", extractor.FrameworkType(), extractor.Priority())
}

// GetExtractor returns an extractor by framework type
func (r *ExtractorRegistry) GetExtractor(frameworkType string) FrameworkConfigExtractor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.extractors[strings.ToLower(frameworkType)]
}

// GetAllExtractors returns all registered extractors ordered by priority
func (r *ExtractorRegistry) GetAllExtractors() []FrameworkConfigExtractor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]FrameworkConfigExtractor, len(r.ordered))
	copy(result, r.ordered)
	return result
}

// FindExtractor finds the best extractor for the given config
func (r *ExtractorRegistry) FindExtractor(rawConfig map[string]interface{}) FrameworkConfigExtractor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, extractor := range r.ordered {
		if extractor.CanHandle(rawConfig) {
			return extractor
		}
	}
	return nil
}

// FrameworkConfigService provides framework config operations
type FrameworkConfigService struct {
	registry *ExtractorRegistry
}

// NewFrameworkConfigService creates a new framework config service
func NewFrameworkConfigService() *FrameworkConfigService {
	return &FrameworkConfigService{
		registry: GetExtractorRegistry(),
	}
}

// BuildFrameworkConfig builds a FrameworkConfig from raw config and metadata
func (s *FrameworkConfigService) BuildFrameworkConfig(
	ctx context.Context,
	framework string,
	rawConfig map[string]interface{},
	configPath string,
	env map[string]string,
) *FrameworkConfig {
	extractor := s.registry.GetExtractor(framework)
	if extractor == nil {
		// Try to find by auto-detection
		extractor = s.registry.FindExtractor(rawConfig)
	}

	config := &FrameworkConfig{
		Framework:   framework,
		Version:     "1.0",
		RawConfig:   rawConfig,
		CollectedAt: time.Now(),
		Source: &ConfigSource{
			Type: "config_file",
			Path: configPath,
		},
	}

	if extractor != nil {
		config.ExtractedPaths = extractor.ExtractPaths(rawConfig, env)
		log.Infof("Extracted paths for framework %s: profiler_dir=%s, tensorboard_dir=%s",
			framework,
			config.ExtractedPaths.ProfilerDir,
			config.ExtractedPaths.TensorBoardDir)
	} else {
		log.Warnf("No extractor found for framework %s, using empty paths", framework)
		config.ExtractedPaths = &ExtractedPaths{}
	}

	return config
}

// GetProfilerLocations returns profiler locations for a framework config
func (s *FrameworkConfigService) GetProfilerLocations(config *FrameworkConfig) []ProfilerLocation {
	if config == nil {
		return s.getDefaultLocations()
	}

	extractor := s.registry.GetExtractor(config.Framework)
	if extractor != nil {
		locations := extractor.GetProfilerLocations(config)
		if len(locations) > 0 {
			return locations
		}
	}

	// Fallback: use extracted paths directly
	if config.ExtractedPaths != nil && config.ExtractedPaths.ProfilerDir != "" {
		return []ProfilerLocation{
			{
				Directory: config.ExtractedPaths.ProfilerDir,
				Patterns:  []string{"*.pt.trace.json", "*.pt.trace.json.gz", "kineto*.json"},
				Recursive: true,
				MaxDepth:  3,
				Priority:  1,
				Source:    "extracted_paths",
			},
		}
	}

	return s.getDefaultLocations()
}

// getDefaultLocations returns default profiler locations as fallback
func (s *FrameworkConfigService) getDefaultLocations() []ProfilerLocation {
	return []ProfilerLocation{
		{
			Directory: "/tmp/profiler",
			Patterns:  []string{"*.pt.trace.json", "*.pt.trace.json.gz"},
			Recursive: false,
			Priority:  99,
			Source:    "fallback",
		},
		{
			Directory: "/workspace/profiler",
			Patterns:  []string{"*.pt.trace.json", "*.pt.trace.json.gz"},
			Recursive: true,
			MaxDepth:  2,
			Priority:  99,
			Source:    "fallback",
		},
	}
}

// ResolveEnvVar resolves environment variable in string
// Supports format: ${VAR_NAME:default_value} or ${VAR_NAME}
func ResolveEnvVar(s string, env map[string]string) string {
	if !strings.Contains(s, "${") {
		return s
	}

	result := s
	for {
		start := strings.Index(result, "${")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		varExpr := result[start+2 : end]
		var varName, defaultVal string

		if colonIdx := strings.Index(varExpr, ":"); colonIdx != -1 {
			varName = varExpr[:colonIdx]
			defaultVal = varExpr[colonIdx+1:]
		} else {
			varName = varExpr
		}

		// Try env map first, then os.Getenv
		var value string
		if env != nil {
			value = env[varName]
		}
		if value == "" {
			value = os.Getenv(varName)
		}
		if value == "" {
			value = defaultVal
		}

		result = result[:start] + value + result[end+1:]
	}

	return result
}

// JoinPaths joins path components, handling relative paths
func JoinPaths(base string, paths ...string) string {
	if len(paths) == 0 {
		return base
	}

	result := base
	for _, p := range paths {
		if p == "" {
			continue
		}
		if filepath.IsAbs(p) {
			result = p
		} else {
			result = filepath.Join(result, p)
		}
	}
	return filepath.Clean(result)
}

// BuildProfilerFilenamePattern builds the filename pattern for profiler files
// Based on Primus format: primus-megatron-exp[{exp_name}]-rank[{rank}].{timestamp}.pt.trace.json{.gz}
func BuildProfilerFilenamePattern(expName string, ranks []int, useGzip bool) []string {
	patterns := make([]string, 0)

	// Base pattern
	basePattern := fmt.Sprintf("primus-megatron-exp[%s]", expName)

	if len(ranks) == 0 {
		// Match all ranks
		if useGzip {
			patterns = append(patterns, basePattern+"-rank[*].*.pt.trace.json.gz")
		}
		patterns = append(patterns, basePattern+"-rank[*].*.pt.trace.json")
	} else {
		for _, rank := range ranks {
			rankPattern := fmt.Sprintf("%s-rank[%d]", basePattern, rank)
			if useGzip {
				patterns = append(patterns, rankPattern+".*.pt.trace.json.gz")
			}
			patterns = append(patterns, rankPattern+".*.pt.trace.json")
		}
	}

	return patterns
}
