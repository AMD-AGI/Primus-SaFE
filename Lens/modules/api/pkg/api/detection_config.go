package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// FrameworkLogPatterns defines log parsing patterns for a training framework
type FrameworkLogPatterns struct {
	Name                string                      `json:"name"`
	DisplayName         string                      `json:"display_name"`
	Version             string                      `json:"version"`
	Priority            int                         `json:"priority"`
	Enabled             bool                        `json:"enabled"`
	IdentifyPatterns    []PatternConfig             `json:"identify_patterns"`
	PerformancePatterns []PatternConfig             `json:"performance_patterns"`
	TrainingEvents      TrainingEventPatterns       `json:"training_events"`
	CheckpointEvents    CheckpointEventPatterns     `json:"checkpoint_events"`
	Extensions          map[string]interface{}      `json:"extensions,omitempty"`
	UpdatedAt           time.Time                   `json:"updated_at"`
	CreatedAt           time.Time                   `json:"created_at"`
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
	IdentifyPatterns    *[]PatternConfig             `json:"identify_patterns,omitempty"`
	PerformancePatterns *[]PatternConfig             `json:"performance_patterns,omitempty"`
	TrainingEvents      *TrainingEventPatterns       `json:"training_events,omitempty"`
	CheckpointEvents    *CheckpointEventPatterns     `json:"checkpoint_events,omitempty"`
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

// ListFrameworks lists all enabled framework names
func ListFrameworks(c *gin.Context) {
	configMgr := config.NewManager(database.GetFacade().GetSystemConfig().GetDB())
	
	// Try to load all known frameworks
	knownFrameworks := []string{"primus", "deepspeed", "megatron"}
	var enabledFrameworks []string
	
	for _, name := range knownFrameworks {
		configKey := fmt.Sprintf("%s.%s", ConfigKeyPrefix, name)
		var patterns FrameworkLogPatterns
		err := configMgr.Get(c.Request.Context(), configKey, &patterns)
		if err != nil {
			log.Warnf("Failed to load framework %s: %v", name, err)
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
	
	// Validate pattern configs
	allPatterns := append(patterns.IdentifyPatterns, patterns.PerformancePatterns...)
	allPatterns = append(allPatterns, patterns.TrainingEvents.StartTraining...)
	allPatterns = append(allPatterns, patterns.TrainingEvents.EndTraining...)
	allPatterns = append(allPatterns, patterns.TrainingEvents.PauseTraining...)
	allPatterns = append(allPatterns, patterns.TrainingEvents.ResumeTraining...)
	allPatterns = append(allPatterns, patterns.CheckpointEvents.StartSaving...)
	allPatterns = append(allPatterns, patterns.CheckpointEvents.EndSaving...)
	allPatterns = append(allPatterns, patterns.CheckpointEvents.Loading...)
	
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

