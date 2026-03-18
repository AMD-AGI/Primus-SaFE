// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for alert silence operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/google/uuid"
)

// ======================== Request / Response Types ========================

// --- Create Alert Silence ---

type CreateAlertSilenceRequest struct {
	Name            string                 `json:"name" mcp:"name,description=Silence rule name,required"`
	Description     string                 `json:"description" mcp:"description,description=Optional description"`
	ClusterName     string                 `json:"cluster_name" mcp:"cluster_name,description=Target cluster name"`
	Enabled         bool                   `json:"enabled" mcp:"enabled,description=Whether the silence is enabled"`
	SilenceType     string                 `json:"silence_type" mcp:"silence_type,description=Type of silence: resource/label/alert_name/expression,required"`
	ResourceFilters []model.ResourceFilter `json:"resource_filters,omitempty" mcp:"resource_filters,description=Resource filters array"`
	LabelMatchers   []model.LabelMatcher   `json:"label_matchers,omitempty" mcp:"label_matchers,description=Label matchers array"`
	AlertNames      []string               `json:"alert_names,omitempty" mcp:"alert_names,description=Alert names to silence"`
	MatchExpression string                 `json:"match_expression,omitempty" mcp:"match_expression,description=Match expression"`
	StartsAt        time.Time              `json:"starts_at" mcp:"starts_at,description=Start time of silence"`
	EndsAt          *time.Time             `json:"ends_at,omitempty" mcp:"ends_at,description=End time (null = permanent)"`
	TimeWindows     []model.TimeWindow     `json:"time_windows,omitempty" mcp:"time_windows,description=Recurring time windows"`
	Reason          string                 `json:"reason" mcp:"reason,description=Reason for silencing"`
	TicketURL       string                 `json:"ticket_url,omitempty" mcp:"ticket_url,description=Related ticket URL"`
}

type CreateAlertSilenceResponse struct {
	SilenceID string `json:"silence_id"`
	Message   string `json:"message"`
}

// --- List Alert Silences ---

type ListAlertSilencesRequest struct {
	ClusterName string `json:"cluster_name" query:"cluster_name" mcp:"cluster_name,description=Cluster name filter"`
	SilenceType string `json:"silence_type" query:"silence_type" mcp:"silence_type,description=Silence type filter"`
	Enabled     string `json:"enabled" query:"enabled" mcp:"enabled,description=Enabled filter (true/false)"`
	ActiveOnly  string `json:"active_only" query:"active_only" mcp:"active_only,description=Only active silences (default false)"`
	PageNum     int    `json:"pageNum" query:"pageNum" mcp:"pageNum,description=Page number (default 1)"`
	PageSize    int    `json:"pageSize" query:"pageSize" mcp:"pageSize,description=Items per page (default 20)"`
}

type ListAlertSilencesResponse struct {
	Data     []*dbModel.AlertSilences `json:"data"`
	Total    int64                    `json:"total"`
	PageNum  int                      `json:"pageNum"`
	PageSize int                      `json:"pageSize"`
}

// --- Get Alert Silence ---

type GetAlertSilenceRequest struct {
	ID string `json:"id" param:"id" mcp:"id,description=Alert silence ID,required"`
}

type GetAlertSilenceResponse = dbModel.AlertSilences

// --- Update Alert Silence ---

type UpdateAlertSilenceRequest struct {
	ID              string                 `json:"id" param:"id" mcp:"id,description=Alert silence ID,required"`
	Name            string                 `json:"name" mcp:"name,description=Silence rule name,required"`
	Description     string                 `json:"description" mcp:"description,description=Optional description"`
	ClusterName     string                 `json:"cluster_name" mcp:"cluster_name,description=Target cluster name"`
	Enabled         bool                   `json:"enabled" mcp:"enabled,description=Whether the silence is enabled"`
	SilenceType     string                 `json:"silence_type" mcp:"silence_type,description=Type of silence,required"`
	ResourceFilters []model.ResourceFilter `json:"resource_filters,omitempty" mcp:"resource_filters,description=Resource filters array"`
	LabelMatchers   []model.LabelMatcher   `json:"label_matchers,omitempty" mcp:"label_matchers,description=Label matchers array"`
	AlertNames      []string               `json:"alert_names,omitempty" mcp:"alert_names,description=Alert names to silence"`
	MatchExpression string                 `json:"match_expression,omitempty" mcp:"match_expression,description=Match expression"`
	StartsAt        time.Time              `json:"starts_at" mcp:"starts_at,description=Start time of silence"`
	EndsAt          *time.Time             `json:"ends_at,omitempty" mcp:"ends_at,description=End time (null = permanent)"`
	TimeWindows     []model.TimeWindow     `json:"time_windows,omitempty" mcp:"time_windows,description=Recurring time windows"`
	Reason          string                 `json:"reason" mcp:"reason,description=Reason for silencing"`
	TicketURL       string                 `json:"ticket_url,omitempty" mcp:"ticket_url,description=Related ticket URL"`
}

type UpdateAlertSilenceResponse struct {
	SilenceID string `json:"silence_id"`
	Message   string `json:"message"`
}

// --- Delete Alert Silence ---

type DeleteAlertSilenceRequest struct {
	ID string `json:"id" param:"id" mcp:"id,description=Alert silence ID,required"`
}

type DeleteAlertSilenceResponse struct {
	Message string `json:"message"`
}

// --- Disable Alert Silence ---

type DisableAlertSilenceRequest struct {
	ID string `json:"id" param:"id" mcp:"id,description=Alert silence ID,required"`
}

type DisableAlertSilenceResponse struct {
	SilenceID string `json:"silence_id"`
	Message   string `json:"message"`
}

// --- List Silenced Alerts ---

type ListSilencedAlertsRequest struct {
	SilenceID   string `json:"silence_id" query:"silence_id" mcp:"silence_id,description=Silence ID filter"`
	AlertName   string `json:"alert_name" query:"alert_name" mcp:"alert_name,description=Alert name filter"`
	ClusterName string `json:"cluster_name" query:"cluster_name" mcp:"cluster_name,description=Cluster name filter"`
	PageNum     int    `json:"pageNum" query:"pageNum" mcp:"pageNum,description=Page number (default 1)"`
	PageSize    int    `json:"pageSize" query:"pageSize" mcp:"pageSize,description=Items per page (default 20)"`
}

type ListSilencedAlertsResponse struct {
	Data     []*dbModel.SilencedAlerts `json:"data"`
	Total    int64                     `json:"total"`
	PageNum  int                       `json:"pageNum"`
	PageSize int                       `json:"pageSize"`
}

// ======================== Registration ========================

func init() {
	// Create Alert Silence
	unified.Register(&unified.EndpointDef[CreateAlertSilenceRequest, CreateAlertSilenceResponse]{
		Name:        "alert_silence_create",
		Description: "Create a new alert silence rule",
		HTTPMethod:  "POST",
		HTTPPath:    "/alert-silences",
		MCPToolName: "lens_alert_silence_create",
		Handler:     handleCreateAlertSilence,
	})

	// List Alert Silences
	unified.Register(&unified.EndpointDef[ListAlertSilencesRequest, ListAlertSilencesResponse]{
		Name:        "alert_silences_list",
		Description: "List alert silences with filters and pagination",
		HTTPMethod:  "GET",
		HTTPPath:    "/alert-silences",
		MCPToolName: "lens_alert_silences_list",
		Handler:     handleListAlertSilences,
	})

	// List Silenced Alerts
	unified.Register(&unified.EndpointDef[ListSilencedAlertsRequest, ListSilencedAlertsResponse]{
		Name:        "silenced_alerts_list",
		Description: "List alerts that have been silenced",
		HTTPMethod:  "GET",
		HTTPPath:    "/alert-silences/silenced-alerts",
		MCPToolName: "lens_silenced_alerts_list",
		Handler:     handleListSilencedAlerts,
	})

	// Get Alert Silence
	unified.Register(&unified.EndpointDef[GetAlertSilenceRequest, GetAlertSilenceResponse]{
		Name:        "alert_silence_get",
		Description: "Get alert silence details by ID",
		HTTPMethod:  "GET",
		HTTPPath:    "/alert-silences/:id",
		MCPToolName: "lens_alert_silence_get",
		Handler:     handleGetAlertSilence,
	})

	// Update Alert Silence
	unified.Register(&unified.EndpointDef[UpdateAlertSilenceRequest, UpdateAlertSilenceResponse]{
		Name:        "alert_silence_update",
		Description: "Update an existing alert silence rule",
		HTTPMethod:  "PUT",
		HTTPPath:    "/alert-silences/:id",
		MCPToolName: "lens_alert_silence_update",
		Handler:     handleUpdateAlertSilence,
	})

	// Delete Alert Silence
	unified.Register(&unified.EndpointDef[DeleteAlertSilenceRequest, DeleteAlertSilenceResponse]{
		Name:        "alert_silence_delete",
		Description: "Delete an alert silence rule",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/alert-silences/:id",
		MCPToolName: "lens_alert_silence_delete",
		Handler:     handleDeleteAlertSilence,
	})

	// Disable Alert Silence
	unified.Register(&unified.EndpointDef[DisableAlertSilenceRequest, DisableAlertSilenceResponse]{
		Name:        "alert_silence_disable",
		Description: "Disable an alert silence rule without deleting it",
		HTTPMethod:  "PATCH",
		HTTPPath:    "/alert-silences/:id/disable",
		MCPToolName: "lens_alert_silence_disable",
		Handler:     handleDisableAlertSilence,
	})
}

// ======================== Handler Implementations ========================

func handleCreateAlertSilence(ctx context.Context, req *CreateAlertSilenceRequest) (*CreateAlertSilenceResponse, error) {
	// Validate silence type
	validTypes := map[string]bool{
		"resource":   true,
		"label":      true,
		"alert_name": true,
		"expression": true,
	}
	if !validTypes[req.SilenceType] {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid silence_type")
	}

	// Set default start time if not provided
	startsAt := req.StartsAt
	if startsAt.IsZero() {
		startsAt = time.Now()
	}

	silenceID := uuid.New().String()

	// Convert filters to ExtType
	resourceFiltersExt, err := toExtType(req.ResourceFilters)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid resource_filters format: " + err.Error())
	}

	labelMatchersExt, err := toExtType(req.LabelMatchers)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid label_matchers format: " + err.Error())
	}

	alertNamesExt, err := toExtType(req.AlertNames)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid alert_names format: " + err.Error())
	}

	timeWindowsExt, err := toExtType(req.TimeWindows)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid time_windows format: " + err.Error())
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
		StartsAt:        startsAt,
		TimeWindows:     timeWindowsExt,
		Reason:          req.Reason,
		TicketURL:       req.TicketURL,
	}

	if req.EndsAt != nil {
		silence.EndsAt = *req.EndsAt
	}

	facade := database.GetFacade().GetAlert()
	if err := facade.CreateAlertSilences(ctx, silence); err != nil {
		return nil, errors.WrapError(err, "failed to create alert silence", errors.CodeDatabaseError)
	}

	return &CreateAlertSilenceResponse{
		SilenceID: silenceID,
		Message:   "alert silence created successfully",
	}, nil
}

func handleListAlertSilences(ctx context.Context, req *ListAlertSilencesRequest) (*ListAlertSilencesResponse, error) {
	pageNum := req.PageNum
	pageSize := req.PageSize
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

	if req.ClusterName != "" {
		filter.ClusterName = &req.ClusterName
	}
	if req.SilenceType != "" {
		filter.SilenceType = &req.SilenceType
	}
	if req.Enabled != "" {
		enabled, err := parseBool(req.Enabled)
		if err == nil {
			filter.Enabled = &enabled
		}
	}
	if req.ActiveOnly != "" {
		activeOnly, _ := parseBool(req.ActiveOnly)
		filter.ActiveOnly = activeOnly
	}

	facade := database.GetFacade().GetAlert()
	silences, total, err := facade.ListAlertSilencess(ctx, filter)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list alert silences", errors.CodeDatabaseError)
	}

	return &ListAlertSilencesResponse{
		Data:     silences,
		Total:    total,
		PageNum:  pageNum,
		PageSize: pageSize,
	}, nil
}

func handleGetAlertSilence(ctx context.Context, req *GetAlertSilenceRequest) (*GetAlertSilenceResponse, error) {
	facade := database.GetFacade().GetAlert()
	silence, err := facade.GetAlertSilencesByID(ctx, req.ID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get alert silence", errors.CodeDatabaseError)
	}
	if silence == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("silence not found")
	}
	return silence, nil
}

func handleUpdateAlertSilence(ctx context.Context, req *UpdateAlertSilenceRequest) (*UpdateAlertSilenceResponse, error) {
	facade := database.GetFacade().GetAlert()
	silence, err := facade.GetAlertSilencesByID(ctx, req.ID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get alert silence", errors.CodeDatabaseError)
	}
	if silence == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("silence not found")
	}

	// Update fields
	silence.Name = req.Name
	silence.Description = req.Description
	silence.Enabled = req.Enabled
	silence.Reason = req.Reason
	silence.TicketURL = req.TicketURL

	if req.EndsAt != nil {
		silence.EndsAt = *req.EndsAt
	}

	// Update ExtType fields
	if req.ResourceFilters != nil {
		ext, _ := toExtType(req.ResourceFilters)
		silence.ResourceFilters = ext
	}
	if req.LabelMatchers != nil {
		ext, _ := toExtType(req.LabelMatchers)
		silence.LabelMatchers = ext
	}
	if req.AlertNames != nil {
		ext, _ := toExtType(req.AlertNames)
		silence.AlertNames = ext
	}
	if req.TimeWindows != nil {
		ext, _ := toExtType(req.TimeWindows)
		silence.TimeWindows = ext
	}

	if err := facade.UpdateAlertSilences(ctx, silence); err != nil {
		return nil, errors.WrapError(err, "failed to update alert silence", errors.CodeDatabaseError)
	}

	return &UpdateAlertSilenceResponse{
		SilenceID: req.ID,
		Message:   "alert silence updated successfully",
	}, nil
}

func handleDeleteAlertSilence(ctx context.Context, req *DeleteAlertSilenceRequest) (*DeleteAlertSilenceResponse, error) {
	facade := database.GetFacade().GetAlert()
	if err := facade.DeleteAlertSilences(ctx, req.ID); err != nil {
		return nil, errors.WrapError(err, "failed to delete alert silence", errors.CodeDatabaseError)
	}
	return &DeleteAlertSilenceResponse{
		Message: "alert silence deleted successfully",
	}, nil
}

func handleDisableAlertSilence(ctx context.Context, req *DisableAlertSilenceRequest) (*DisableAlertSilenceResponse, error) {
	facade := database.GetFacade().GetAlert()
	if err := facade.DisableAlertSilences(ctx, req.ID); err != nil {
		return nil, errors.WrapError(err, "failed to disable alert silence", errors.CodeDatabaseError)
	}
	return &DisableAlertSilenceResponse{
		SilenceID: req.ID,
		Message:   "alert silence disabled successfully",
	}, nil
}

func handleListSilencedAlerts(ctx context.Context, req *ListSilencedAlertsRequest) (*ListSilencedAlertsResponse, error) {
	pageNum := req.PageNum
	pageSize := req.PageSize
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

	if req.SilenceID != "" {
		filter.SilenceID = &req.SilenceID
	}
	if req.AlertName != "" {
		filter.AlertName = &req.AlertName
	}
	if req.ClusterName != "" {
		filter.ClusterName = &req.ClusterName
	}

	facade := database.GetFacade().GetAlert()
	silencedAlerts, total, err := facade.ListSilencedAlertss(ctx, filter)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list silenced alerts", errors.CodeDatabaseError)
	}

	return &ListSilencedAlertsResponse{
		Data:     silencedAlerts,
		Total:    total,
		PageNum:  pageNum,
		PageSize: pageSize,
	}, nil
}

// ======================== Helpers ========================

// toExtType converts any JSON-serializable value to dbModel.ExtType.
func toExtType(v any) (dbModel.ExtType, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var ext dbModel.ExtType
	if err := json.Unmarshal(b, &ext); err != nil {
		return nil, err
	}
	return ext, nil
}

// parseBool parses a boolean string value.
func parseBool(s string) (bool, error) {
	switch s {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value: %s", s)
	}
}
