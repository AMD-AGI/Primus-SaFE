// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// FrameworkType constants
const (
	FrameworkTypeTraining  = "training"
	FrameworkTypeInference = "inference"
)

// FrameworkLogPatterns defines log parsing patterns for a framework (training or inference)
type FrameworkLogPatterns struct {
	Name                string                      `json:"name"`
	DisplayName         string                      `json:"display_name"`
	Version             string                      `json:"version"`
	Priority            int                         `json:"priority"`
	Enabled             bool                        `json:"enabled"`
	Type                string                      `json:"type,omitempty"` // "training" or "inference", defaults to "training"
	IdentifyPatterns    []PatternConfig             `json:"identify_patterns"`
	PerformancePatterns []PatternConfig             `json:"performance_patterns"`
	TrainingEvents      TrainingEventPatterns       `json:"training_events,omitempty"`
	CheckpointEvents    CheckpointEventPatterns     `json:"checkpoint_events,omitempty"`
	InferencePatterns   *InferencePatternConfig     `json:"inference_patterns,omitempty"`
	Extensions          map[string]interface{}      `json:"extensions,omitempty"`
	UpdatedAt           time.Time                   `json:"updated_at"`
	CreatedAt           time.Time                   `json:"created_at"`
}

// InferencePatternConfig defines patterns for inference framework detection
type InferencePatternConfig struct {
	ProcessPatterns []PatternConfig `json:"process_patterns,omitempty"`
	Ports           []int           `json:"ports,omitempty"`
	EnvPatterns     []PatternConfig `json:"env_patterns,omitempty"`
	ImagePatterns   []PatternConfig `json:"image_patterns,omitempty"`
	CmdlinePatterns []PatternConfig `json:"cmdline_patterns,omitempty"`
	HealthEndpoint  string          `json:"health_endpoint,omitempty"`
}

// PatternConfig defines a regex pattern configuration
type PatternConfig struct {
	Name        string   `json:"name"`
	Pattern     string   `json:"pattern"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
	Tags        []string `json:"tags"`
	Confidence  float64  `json:"confidence"`
}

// TrainingEventPatterns defines patterns for training lifecycle events
type TrainingEventPatterns struct {
	StartTraining  []PatternConfig `json:"start_training"`
	EndTraining    []PatternConfig `json:"end_training,omitempty"`
	PauseTraining  []PatternConfig `json:"pause_training,omitempty"`
	ResumeTraining []PatternConfig `json:"resume_training,omitempty"`
}

// CheckpointEventPatterns defines patterns for checkpoint events
type CheckpointEventPatterns struct {
	StartSaving []PatternConfig `json:"start_saving"`
	EndSaving   []PatternConfig `json:"end_saving"`
	Loading     []PatternConfig `json:"loading,omitempty"`
}

// UpdateFrameworkConfigRequest request for updating framework config
type UpdateFrameworkConfigRequest struct {
	DisplayName         *string                      `json:"display_name,omitempty"`
	Version             *string                      `json:"version,omitempty"`
	Priority            *int                         `json:"priority,omitempty"`
	Enabled             *bool                        `json:"enabled,omitempty"`
	Type                *string                      `json:"type,omitempty"` // "training" or "inference"
	IdentifyPatterns    *[]PatternConfig             `json:"identify_patterns,omitempty"`
	PerformancePatterns *[]PatternConfig             `json:"performance_patterns,omitempty"`
	TrainingEvents      *TrainingEventPatterns       `json:"training_events,omitempty"`
	CheckpointEvents    *CheckpointEventPatterns     `json:"checkpoint_events,omitempty"`
	InferencePatterns   *InferencePatternConfig      `json:"inference_patterns,omitempty"`
	Extensions          *map[string]interface{}      `json:"extensions,omitempty"`
}

// SetCacheTTLRequest request for setting cache TTL
type SetCacheTTLRequest struct {
	TTLSeconds int `json:"ttl_seconds" binding:"required,min=1"`
}

// CacheTTLResponse response for cache TTL
type CacheTTLResponse struct {
	TTLSeconds int       `json:"ttl_seconds"`
	LastRefresh time.Time `json:"last_refresh"`
	IsExpired   bool      `json:"is_expired"`
}

// FrameworkListResponse response for framework list
type FrameworkListResponse struct {
	Frameworks []string `json:"frameworks"`
	Total      int      `json:"total"`
}

const (
	ConfigKeyPrefix = "training.log.parser.framework"
)

// ListFrameworks lists all enabled framework names dynamically from system_config
func ListFrameworks(c *gin.Context) {
	configMgr := config.NewManager(database.GetFacade().GetSystemConfig().GetDB())

	// Dynamically discover all framework configs by prefix
	configs, err := configMgr.List(c.Request.Context(), config.WithKeyPrefixFilter(ConfigKeyPrefix+"."))
	if err != nil {
		log.Errorf("Failed to list framework configs: %v", err)
		_ = c.Error(fmt.Errorf("failed to list framework configs: %w", err))
		return
	}

	var enabledFrameworks []string
	prefix := ConfigKeyPrefix + "."

	for _, cfg := range configs {
		// Extract framework name from key
		if len(cfg.Key) <= len(prefix) {
			continue
		}
		name := cfg.Key[len(prefix):]
		// Skip sub-configs (keys with additional dots)
		if len(name) == 0 || name[0] == '.' {
			continue
		}

		// Parse the config to check if enabled
		var patterns FrameworkLogPatterns
		if err := configMgr.Get(c.Request.Context(), cfg.Key, &patterns); err != nil {
			log.Debugf("Failed to parse framework config %s: %v", cfg.Key, err)
			continue
		}

		if patterns.Enabled {
			enabledFrameworks = append(enabledFrameworks, name)
		}
	}

	response := FrameworkListResponse{
		Frameworks: enabledFrameworks,
		Total:      len(enabledFrameworks),
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// GetFrameworkConfig gets configuration for a specific framework
func GetFrameworkConfig(c *gin.Context) {
	frameworkName := c.Param("name")
	if frameworkName == "" {
		_ = c.Error(fmt.Errorf("framework name is required"))
		return
	}
	
	configMgr := config.NewManager(database.GetFacade().GetSystemConfig().GetDB())
	configKey := fmt.Sprintf("%s.%s", ConfigKeyPrefix, frameworkName)
	
	var patterns FrameworkLogPatterns
	err := configMgr.Get(c.Request.Context(), configKey, &patterns)
	if err != nil {
		_ = c.Error(fmt.Errorf("failed to get framework config: %w", err))
		return
	}
	
	c.JSON(http.StatusOK, rest.SuccessResp(c, patterns))
}

// UpdateFrameworkConfig updates configuration for a specific framework
func UpdateFrameworkConfig(c *gin.Context) {
	frameworkName := c.Param("name")
	if frameworkName == "" {
		_ = c.Error(fmt.Errorf("framework name is required"))
		return
	}
	
	var req UpdateFrameworkConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(fmt.Errorf("invalid request body: %w", err))
		return
	}
	
	configMgr := config.NewManager(database.GetFacade().GetSystemConfig().GetDB())
	configKey := fmt.Sprintf("%s.%s", ConfigKeyPrefix, frameworkName)
	
	// Get existing config
	var existing FrameworkLogPatterns
	err := configMgr.Get(c.Request.Context(), configKey, &existing)
	if err != nil {
		_ = c.Error(fmt.Errorf("failed to get existing config: %w", err))
		return
	}
	
	// Apply updates
	if req.DisplayName != nil {
		existing.DisplayName = *req.DisplayName
	}
	if req.Version != nil {
		existing.Version = *req.Version
	}
	if req.Priority != nil {
		existing.Priority = *req.Priority
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.Type != nil {
		existing.Type = *req.Type
	}
	if req.IdentifyPatterns != nil {
		existing.IdentifyPatterns = *req.IdentifyPatterns
	}
	if req.PerformancePatterns != nil {
		existing.PerformancePatterns = *req.PerformancePatterns
	}
	if req.TrainingEvents != nil {
		existing.TrainingEvents = *req.TrainingEvents
	}
	if req.CheckpointEvents != nil {
		existing.CheckpointEvents = *req.CheckpointEvents
	}
	if req.InferencePatterns != nil {
		existing.InferencePatterns = req.InferencePatterns
	}
	if req.Extensions != nil {
		existing.Extensions = *req.Extensions
	}
	
	// Update timestamp
	existing.UpdatedAt = time.Now()
	
	// Validate
	if err := validateFrameworkLogPatterns(&existing); err != nil {
		_ = c.Error(fmt.Errorf("invalid configuration: %w", err))
		return
	}
	
	// Save to database
	err = configMgr.Set(
		c.Request.Context(),
		configKey,
		existing,
		config.WithUpdatedBy(getUserFromContext(c)),
		config.WithChangeReason("Updated via API"),
		config.WithRecordHistory(true),
	)
	if err != nil {
		_ = c.Error(fmt.Errorf("failed to save config: %w", err))
		return
	}
	
	log.Infof("Framework config updated: %s by %s", frameworkName, getUserFromContext(c))
	c.JSON(http.StatusOK, rest.SuccessResp(c, existing))
}

// RefreshDetectionConfigCache forces a cache refresh
func RefreshDetectionConfigCache(c *gin.Context) {
	// Note: This endpoint is a placeholder for cache refresh functionality
	// The actual cache refresh would need to be implemented in the ai-advisor module
	// For now, we just return success
	
	log.Info("Detection config cache refresh requested")
	
	response := map[string]interface{}{
		"message":     "Cache refresh triggered successfully",
		"refreshed_at": time.Now(),
	}
	
	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// GetCacheTTL gets the cache TTL configuration
func GetCacheTTL(c *gin.Context) {
	// Note: This is a placeholder implementation
	// In a real implementation, you would fetch this from the config manager
	// or from the FrameworkConfigManager instance
	
	defaultTTL := 5 * time.Minute
	
	response := CacheTTLResponse{
		TTLSeconds:  int(defaultTTL.Seconds()),
		LastRefresh: time.Now(),
		IsExpired:   false,
	}
	
	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// SetCacheTTL sets the cache TTL configuration
func SetCacheTTL(c *gin.Context) {
	var req SetCacheTTLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(fmt.Errorf("invalid request body: %w", err))
		return
	}
	
	// Note: This is a placeholder implementation
	// In a real implementation, you would save this to the config manager
	// or apply it to the FrameworkConfigManager instance
	
	log.Infof("Cache TTL updated to %d seconds by %s", req.TTLSeconds, getUserFromContext(c))
	
	response := CacheTTLResponse{
		TTLSeconds:  req.TTLSeconds,
		LastRefresh: time.Now(),
		IsExpired:   false,
	}
	
	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// validateFrameworkLogPatterns validates the framework log patterns
func validateFrameworkLogPatterns(patterns *FrameworkLogPatterns) error {
	if patterns.Name == "" {
		return fmt.Errorf("framework name is required")
	}
	if patterns.Priority < 0 {
		return fmt.Errorf("priority must be non-negative")
	}

	// Validate type if specified
	if patterns.Type != "" && patterns.Type != FrameworkTypeTraining && patterns.Type != FrameworkTypeInference {
		return fmt.Errorf("type must be '%s' or '%s'", FrameworkTypeTraining, FrameworkTypeInference)
	}

	// Validate pattern configs
	allPatterns := append(patterns.IdentifyPatterns, patterns.PerformancePatterns...)
	allPatterns = append(allPatterns, patterns.TrainingEvents.StartTraining...)
	allPatterns = append(allPatterns, patterns.TrainingEvents.EndTraining...)
	allPatterns = append(allPatterns, patterns.TrainingEvents.PauseTraining...)
	allPatterns = append(allPatterns, patterns.TrainingEvents.ResumeTraining...)
	allPatterns = append(allPatterns, patterns.CheckpointEvents.StartSaving...)
	allPatterns = append(allPatterns, patterns.CheckpointEvents.EndSaving...)
	allPatterns = append(allPatterns, patterns.CheckpointEvents.Loading...)

	// Validate inference patterns if present
	if patterns.InferencePatterns != nil {
		allPatterns = append(allPatterns, patterns.InferencePatterns.ProcessPatterns...)
		allPatterns = append(allPatterns, patterns.InferencePatterns.EnvPatterns...)
		allPatterns = append(allPatterns, patterns.InferencePatterns.ImagePatterns...)
		allPatterns = append(allPatterns, patterns.InferencePatterns.CmdlinePatterns...)
	}

	for _, pattern := range allPatterns {
		if err := validatePatternConfig(&pattern); err != nil {
			return fmt.Errorf("invalid pattern '%s': %w", pattern.Name, err)
		}
	}

	return nil
}

// validatePatternConfig validates a pattern configuration
func validatePatternConfig(pattern *PatternConfig) error {
	if pattern.Name == "" {
		return fmt.Errorf("pattern name is required")
	}
	if pattern.Pattern == "" {
		return fmt.Errorf("pattern regex is required")
	}
	if pattern.Confidence < 0.0 || pattern.Confidence > 1.0 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0")
	}
	return nil
}

// getUserFromContext extracts user from gin context
func getUserFromContext(c *gin.Context) string {
	// Try to get user from various possible context keys
	if user, exists := c.Get("user"); exists {
		if userStr, ok := user.(string); ok {
			return userStr
		}
	}

	// Fallback to checking headers
	if user := c.GetHeader("X-User"); user != "" {
		return user
	}

	// Default to "system" if no user found
	return "system"
}

// ============================================================================
// Inference Detection API (Phase 3)
// ============================================================================

// InferenceDetectionRequest request for inference framework detection
type InferenceDetectionRequest struct {
	WorkloadUID     string            `json:"workload_uid"`
	PodName         string            `json:"pod_name"`
	Namespace       string            `json:"namespace"`
	ProcessNames    []string          `json:"process_names"`
	ProcessCmdlines []string          `json:"process_cmdlines"`
	ImageName       string            `json:"image_name"`
	ContainerPorts  []int             `json:"container_ports"`
	EnvVars         map[string]string `json:"env_vars"`
}

// InferenceDetectionResponse response for inference framework detection
type InferenceDetectionResponse struct {
	Detected       bool     `json:"detected"`
	FrameworkName  string   `json:"framework_name,omitempty"`
	FrameworkType  string   `json:"framework_type,omitempty"`
	Confidence     float64  `json:"confidence,omitempty"`
	MatchedSources []string `json:"matched_sources,omitempty"`
	Evidence       []string `json:"evidence,omitempty"`
}

// DetectInferenceFramework detects inference framework from provided context
// POST /api/v1/detection/inference/detect
func DetectInferenceFramework(c *gin.Context) {
	var req InferenceDetectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(fmt.Errorf("invalid request body: %w", err))
		return
	}

	// Build match context and try each inference framework
	configMgr := config.NewManager(database.GetFacade().GetSystemConfig().GetDB())

	// Get all inference framework configs
	configs, err := configMgr.List(c.Request.Context(), config.WithKeyPrefixFilter(ConfigKeyPrefix+"."))
	if err != nil {
		_ = c.Error(fmt.Errorf("failed to list framework configs: %w", err))
		return
	}

	var bestMatch *InferenceDetectionResponse
	var bestConfidence float64

	prefix := ConfigKeyPrefix + "."
	for _, cfg := range configs {
		if len(cfg.Key) <= len(prefix) {
			continue
		}
		name := cfg.Key[len(prefix):]

		var patterns FrameworkLogPatterns
		if err := configMgr.Get(c.Request.Context(), cfg.Key, &patterns); err != nil {
			continue
		}

		// Skip non-inference frameworks
		if !patterns.Enabled || patterns.GetType() != FrameworkTypeInference {
			continue
		}

		// Perform matching
		result := matchInferencePatterns(&patterns, &req)
		if result.Detected && result.Confidence > bestConfidence {
			bestMatch = result
			bestConfidence = result.Confidence
			log.Debugf("Inference framework %s matched with confidence %.2f", name, result.Confidence)
		}
	}

	if bestMatch == nil {
		c.JSON(http.StatusOK, rest.SuccessResp(c, InferenceDetectionResponse{Detected: false}))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, bestMatch))
}

// GetType returns the framework type, defaults to "training" for backward compatibility
func (f *FrameworkLogPatterns) GetType() string {
	if f.Type == "" {
		return FrameworkTypeTraining
	}
	return f.Type
}

// matchInferencePatterns performs inference pattern matching
func matchInferencePatterns(patterns *FrameworkLogPatterns, req *InferenceDetectionRequest) *InferenceDetectionResponse {
	if patterns.InferencePatterns == nil {
		return &InferenceDetectionResponse{Detected: false}
	}

	inf := patterns.InferencePatterns
	var matchedSources []string
	var evidence []string
	var totalConfidence float64
	matchCount := 0

	// 1. Match process patterns (weight: 0.35)
	if len(inf.ProcessPatterns) > 0 {
		for _, procName := range req.ProcessNames {
			for _, pattern := range inf.ProcessPatterns {
				if pattern.Enabled && matchPattern(pattern.Pattern, procName) {
					matchedSources = append(matchedSources, "process")
					evidence = append(evidence, fmt.Sprintf("process:%s matched %s", procName, pattern.Name))
					totalConfidence += pattern.Confidence * 0.35
					matchCount++
					break
				}
			}
		}
	}

	// 2. Match image patterns (weight: 0.25)
	if len(inf.ImagePatterns) > 0 && req.ImageName != "" {
		for _, pattern := range inf.ImagePatterns {
			if pattern.Enabled && matchPattern(pattern.Pattern, req.ImageName) {
				matchedSources = append(matchedSources, "image")
				evidence = append(evidence, fmt.Sprintf("image:%s matched %s", req.ImageName, pattern.Name))
				totalConfidence += pattern.Confidence * 0.25
				matchCount++
				break
			}
		}
	}

	// 3. Match env patterns (weight: 0.20)
	if len(inf.EnvPatterns) > 0 && len(req.EnvVars) > 0 {
		for envKey := range req.EnvVars {
			for _, pattern := range inf.EnvPatterns {
				if pattern.Enabled && matchPattern(pattern.Pattern, envKey) {
					matchedSources = append(matchedSources, "env")
					evidence = append(evidence, fmt.Sprintf("env:%s matched %s", envKey, pattern.Name))
					totalConfidence += pattern.Confidence * 0.20
					matchCount++
					break
				}
			}
		}
	}

	// 4. Match ports (weight: 0.10)
	if len(inf.Ports) > 0 && len(req.ContainerPorts) > 0 {
		for _, containerPort := range req.ContainerPorts {
			for _, expectedPort := range inf.Ports {
				if containerPort == expectedPort {
					matchedSources = append(matchedSources, "port")
					evidence = append(evidence, fmt.Sprintf("port:%d matched", containerPort))
					totalConfidence += 0.10
					matchCount++
					break
				}
			}
		}
	}

	// 5. Match cmdline patterns (weight: 0.10)
	if len(inf.CmdlinePatterns) > 0 && len(req.ProcessCmdlines) > 0 {
		for _, cmdline := range req.ProcessCmdlines {
			for _, pattern := range inf.CmdlinePatterns {
				if pattern.Enabled && matchPattern(pattern.Pattern, cmdline) {
					matchedSources = append(matchedSources, "cmdline")
					evidence = append(evidence, fmt.Sprintf("cmdline matched %s", pattern.Name))
					totalConfidence += pattern.Confidence * 0.10
					matchCount++
					break
				}
			}
		}
	}

	// Require at least 2 matches
	if matchCount < 2 {
		return &InferenceDetectionResponse{Detected: false}
	}

	return &InferenceDetectionResponse{
		Detected:       true,
		FrameworkName:  patterns.Name,
		FrameworkType:  FrameworkTypeInference,
		Confidence:     totalConfidence,
		MatchedSources: matchedSources,
		Evidence:       evidence,
	}
}

// matchPattern performs regex pattern matching
func matchPattern(pattern, text string) bool {
	matched, err := regexp.MatchString(pattern, text)
	if err != nil {
		log.Warnf("Invalid regex pattern %s: %v", pattern, err)
		return false
	}
	return matched
}

// ListInferenceFrameworks lists all enabled inference framework names
// GET /api/v1/detection/inference/frameworks
func ListInferenceFrameworks(c *gin.Context) {
	configMgr := config.NewManager(database.GetFacade().GetSystemConfig().GetDB())

	configs, err := configMgr.List(c.Request.Context(), config.WithKeyPrefixFilter(ConfigKeyPrefix+"."))
	if err != nil {
		_ = c.Error(fmt.Errorf("failed to list framework configs: %w", err))
		return
	}

	var inferenceFrameworks []string
	prefix := ConfigKeyPrefix + "."

	for _, cfg := range configs {
		if len(cfg.Key) <= len(prefix) {
			continue
		}
		name := cfg.Key[len(prefix):]

		var patterns FrameworkLogPatterns
		if err := configMgr.Get(c.Request.Context(), cfg.Key, &patterns); err != nil {
			continue
		}

		if patterns.Enabled && patterns.GetType() == FrameworkTypeInference {
			inferenceFrameworks = append(inferenceFrameworks, name)
		}
	}

	response := FrameworkListResponse{
		Frameworks: inferenceFrameworks,
		Total:      len(inferenceFrameworks),
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// ListTrainingFrameworks lists all enabled training framework names
// GET /api/v1/detection/training/frameworks
func ListTrainingFrameworks(c *gin.Context) {
	configMgr := config.NewManager(database.GetFacade().GetSystemConfig().GetDB())

	configs, err := configMgr.List(c.Request.Context(), config.WithKeyPrefixFilter(ConfigKeyPrefix+"."))
	if err != nil {
		_ = c.Error(fmt.Errorf("failed to list framework configs: %w", err))
		return
	}

	var trainingFrameworks []string
	prefix := ConfigKeyPrefix + "."

	for _, cfg := range configs {
		if len(cfg.Key) <= len(prefix) {
			continue
		}
		name := cfg.Key[len(prefix):]

		var patterns FrameworkLogPatterns
		if err := configMgr.Get(c.Request.Context(), cfg.Key, &patterns); err != nil {
			continue
		}

		if patterns.Enabled && patterns.GetType() == FrameworkTypeTraining {
			trainingFrameworks = append(trainingFrameworks, name)
		}
	}

	response := FrameworkListResponse{
		Frameworks: trainingFrameworks,
		Total:      len(trainingFrameworks),
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

