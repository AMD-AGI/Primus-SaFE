package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AlertSilenceRequest represents a request to create or update an alert silence
type AlertSilenceRequest struct {
	Name            string                 `json:"name" binding:"required"`
	Description     string                 `json:"description"`
	ClusterName     string                 `json:"cluster_name"`
	Enabled         bool                   `json:"enabled"`
	SilenceType     string                 `json:"silence_type" binding:"required"` // resource/label/alert_name/expression
	ResourceFilters []model.ResourceFilter `json:"resource_filters,omitempty"`
	LabelMatchers   []model.LabelMatcher   `json:"label_matchers,omitempty"`
	AlertNames      []string               `json:"alert_names,omitempty"`
	MatchExpression string                 `json:"match_expression,omitempty"`
	StartsAt        time.Time              `json:"starts_at"`
	EndsAt          *time.Time             `json:"ends_at,omitempty"` // null = permanent
	TimeWindows     []model.TimeWindow     `json:"time_windows,omitempty"`
	Reason          string                 `json:"reason"`
	TicketURL       string                 `json:"ticket_url,omitempty"`
}

// CreateAlertSilence handles POST /api/alert-silences
func CreateAlertSilence(c *gin.Context) {
	var req AlertSilenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to parse request body: %v", err)
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Validate silence type
	validTypes := map[string]bool{
		"resource":   true,
		"label":      true,
		"alert_name": true,
		"expression": true,
	}
	if !validTypes[req.SilenceType] {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid silence_type", nil))
		return
	}

	// Set default start time if not provided
	if req.StartsAt.IsZero() {
		req.StartsAt = time.Now()
	}

	// Get user from context if available
	createdBy, _ := c.Get("user")
	createdByStr := ""
	if createdBy != nil {
		createdByStr = fmt.Sprintf("%v", createdBy)
	}

	// Generate UUID for silence
	silenceID := uuid.New().String()

	// Convert filters to ExtType
	var resourceFiltersExt dbModel.ExtType
	if req.ResourceFilters != nil {
		resourceFiltersBytes, err := json.Marshal(req.ResourceFilters)
		if err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid resource_filters format: "+err.Error(), nil))
			return
		}
		if err := json.Unmarshal(resourceFiltersBytes, &resourceFiltersExt); err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid resource_filters format: "+err.Error(), nil))
			return
		}
	}

	var labelMatchersExt dbModel.ExtType
	if req.LabelMatchers != nil {
		labelMatchersBytes, err := json.Marshal(req.LabelMatchers)
		if err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid label_matchers format: "+err.Error(), nil))
			return
		}
		if err := json.Unmarshal(labelMatchersBytes, &labelMatchersExt); err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid label_matchers format: "+err.Error(), nil))
			return
		}
	}

	var alertNamesExt dbModel.ExtType
	if req.AlertNames != nil {
		alertNamesBytes, err := json.Marshal(req.AlertNames)
		if err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid alert_names format: "+err.Error(), nil))
			return
		}
		if err := json.Unmarshal(alertNamesBytes, &alertNamesExt); err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid alert_names format: "+err.Error(), nil))
			return
		}
	}

	var timeWindowsExt dbModel.ExtType
	if req.TimeWindows != nil {
		timeWindowsBytes, err := json.Marshal(req.TimeWindows)
		if err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid time_windows format: "+err.Error(), nil))
			return
		}
		if err := json.Unmarshal(timeWindowsBytes, &timeWindowsExt); err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid time_windows format: "+err.Error(), nil))
			return
		}
	}

	silence := &dbModel.AlertSilences{
		ID:              silenceID,
		Name:            req.Name,
		Description:     req.Description,
		ClusterName:     req.ClusterName,
		Enabled:         req.Enabled,
		SilenceType:     req.SilenceType,
		ResourceFilters: resourceFiltersExt,
		LabelMatchers:   labelMatchersExt,
		AlertNames:      alertNamesExt,
		MatchExpression: req.MatchExpression,
		StartsAt:        req.StartsAt,
		TimeWindows:     timeWindowsExt,
		Reason:          req.Reason,
		TicketURL:       req.TicketURL,
		CreatedBy:       createdByStr,
	}

	// Handle EndsAt pointer
	if req.EndsAt != nil {
		silence.EndsAt = *req.EndsAt
	}

	// Save to database
	facade := database.GetFacade().GetAlert()
	if err := facade.CreateAlertSilences(c.Request.Context(), silence); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to create alert silence: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusCreated, rest.SuccessResp(c.Request.Context(), gin.H{
		"silence_id": silenceID,
		"message":    "alert silence created successfully",
	}))
}

// ListAlertSilences handles GET /api/alert-silences
func ListAlertSilences(c *gin.Context) {
	clusterName := c.Query("cluster_name")
	silenceType := c.Query("silence_type")
	enabledStr := c.Query("enabled")
	activeOnlyStr := c.DefaultQuery("active_only", "false")

	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filter := &database.AlertSilencesFilter{
		Offset: (pageNum - 1) * pageSize,
		Limit:  pageSize,
	}

	if clusterName != "" {
		filter.ClusterName = &clusterName
	}
	if silenceType != "" {
		filter.SilenceType = &silenceType
	}
	if enabledStr != "" {
		enabled, err := strconv.ParseBool(enabledStr)
		if err == nil {
			filter.Enabled = &enabled
		}
	}
	activeOnly, _ := strconv.ParseBool(activeOnlyStr)
	filter.ActiveOnly = activeOnly

	facade := database.GetFacade().GetAlert()
	silences, total, err := facade.ListAlertSilencess(c.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to list alert silences: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"data":     silences,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}))
}

// GetAlertSilence handles GET /api/alert-silences/:id
func GetAlertSilence(c *gin.Context) {
	id := c.Param("id")

	facade := database.GetFacade().GetAlert()
	silence, err := facade.GetAlertSilencesByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get alert silence: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if silence == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "silence not found", nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), silence))
}

// UpdateAlertSilence handles PUT /api/alert-silences/:id
func UpdateAlertSilence(c *gin.Context) {
	id := c.Param("id")

	var req AlertSilenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to parse request body: %v", err)
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Get existing silence
	facade := database.GetFacade().GetAlert()
	silence, err := facade.GetAlertSilencesByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get alert silence: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if silence == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "silence not found", nil))
		return
	}

	// Get user from context if available
	updatedBy, _ := c.Get("user")
	updatedByStr := ""
	if updatedBy != nil {
		updatedByStr = fmt.Sprintf("%v", updatedBy)
	}

	// Update fields
	silence.Name = req.Name
	silence.Description = req.Description
	silence.Enabled = req.Enabled
	silence.Reason = req.Reason
	silence.TicketURL = req.TicketURL
	silence.UpdatedBy = updatedByStr

	// Handle EndsAt pointer
	if req.EndsAt != nil {
		silence.EndsAt = *req.EndsAt
	}

	// Update ExtType fields
	if req.ResourceFilters != nil {
		var resourceFiltersExt dbModel.ExtType
		resourceFiltersBytes, _ := json.Marshal(req.ResourceFilters)
		json.Unmarshal(resourceFiltersBytes, &resourceFiltersExt)
		silence.ResourceFilters = resourceFiltersExt
	}

	if req.LabelMatchers != nil {
		var labelMatchersExt dbModel.ExtType
		labelMatchersBytes, _ := json.Marshal(req.LabelMatchers)
		json.Unmarshal(labelMatchersBytes, &labelMatchersExt)
		silence.LabelMatchers = labelMatchersExt
	}

	if req.AlertNames != nil {
		var alertNamesExt dbModel.ExtType
		alertNamesBytes, _ := json.Marshal(req.AlertNames)
		json.Unmarshal(alertNamesBytes, &alertNamesExt)
		silence.AlertNames = alertNamesExt
	}

	if req.TimeWindows != nil {
		var timeWindowsExt dbModel.ExtType
		timeWindowsBytes, _ := json.Marshal(req.TimeWindows)
		json.Unmarshal(timeWindowsBytes, &timeWindowsExt)
		silence.TimeWindows = timeWindowsExt
	}

	// Save to database
	if err := facade.UpdateAlertSilences(c.Request.Context(), silence); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to update alert silence: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"silence_id": id,
		"message":    "alert silence updated successfully",
	}))
}

// DeleteAlertSilence handles DELETE /api/alert-silences/:id
func DeleteAlertSilence(c *gin.Context) {
	id := c.Param("id")

	facade := database.GetFacade().GetAlert()
	if err := facade.DeleteAlertSilences(c.Request.Context(), id); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to delete alert silence: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"message": "alert silence deleted successfully",
	}))
}

// DisableAlertSilence handles PATCH /api/alert-silences/:id/disable
func DisableAlertSilence(c *gin.Context) {
	id := c.Param("id")

	facade := database.GetFacade().GetAlert()
	if err := facade.DisableAlertSilences(c.Request.Context(), id); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to disable alert silence: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"silence_id": id,
		"message":    "alert silence disabled successfully",
	}))
}

// ListSilencedAlerts handles GET /api/alert-silences/silenced-alerts
func ListSilencedAlerts(c *gin.Context) {
	silenceID := c.Query("silence_id")
	alertName := c.Query("alert_name")
	clusterName := c.Query("cluster_name")

	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filter := &database.SilencedAlertsFilter{
		Offset: (pageNum - 1) * pageSize,
		Limit:  pageSize,
	}

	if silenceID != "" {
		filter.SilenceID = &silenceID
	}
	if alertName != "" {
		filter.AlertName = &alertName
	}
	if clusterName != "" {
		filter.ClusterName = &clusterName
	}

	facade := database.GetFacade().GetAlert()
	silencedAlerts, total, err := facade.ListSilencedAlertss(c.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to list silenced alerts: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"data":     silencedAlerts,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}))
}
