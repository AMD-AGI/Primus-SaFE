// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ===== Request / Response types =====

// IntentRuleCreateRequest - create a new intent rule
type IntentRuleCreateRequest struct {
	DetectsField string  `json:"detects_field" mcp:"detects_field,description=Detection target dimension: category or model_family or training_method,required"`
	DetectsValue string  `json:"detects_value" mcp:"detects_value,description=Detection target value (e.g. inference or pre_training or fine_tuning or benchmark),required"`
	Dimension    string  `json:"dimension" mcp:"dimension,description=Matching dimension: image or cmdline or env_key or env_value or pip or code,required"`
	Pattern      string  `json:"pattern" mcp:"pattern,description=Regular expression pattern for matching,required"`
	Confidence   float64 `json:"confidence" mcp:"confidence,description=Confidence score when this rule matches [0.0-1.0],required"`
	Reasoning    string  `json:"reasoning" mcp:"reasoning,description=Explanation of why this rule works"`
	Status       string  `json:"status" mcp:"status,description=Initial status (default: proposed)"`
}

// IntentRuleCreateResponse - created rule
type IntentRuleCreateResponse struct {
	ID      int64  `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// IntentRuleGetRequest - get a single rule by ID
type IntentRuleGetRequest struct {
	RuleID string `json:"rule_id" param:"rule_id" mcp:"rule_id,description=Intent rule ID,required"`
}

// IntentRuleGetResponse - single rule detail
type IntentRuleGetResponse struct {
	ID                 int64       `json:"id"`
	DetectsField       string      `json:"detects_field"`
	DetectsValue       string      `json:"detects_value"`
	Dimension          string      `json:"dimension"`
	Pattern            string      `json:"pattern"`
	Confidence         float64     `json:"confidence"`
	Reasoning          string      `json:"reasoning,omitempty"`
	DerivedFrom        interface{} `json:"derived_from,omitempty"`
	Status             string      `json:"status"`
	BacktestResult     interface{} `json:"backtest_result,omitempty"`
	LastBacktestedAt   string      `json:"last_backtested_at,omitempty"`
	MatchCount         int32       `json:"match_count"`
	CorrectCount       int32       `json:"correct_count"`
	FalsePositiveCount int32       `json:"false_positive_count"`
	CreatedAt          string      `json:"created_at"`
	UpdatedAt          string      `json:"updated_at"`
}

// IntentRuleUpdateRequest - full update of a rule
type IntentRuleUpdateRequest struct {
	RuleID       string  `json:"rule_id" param:"rule_id" mcp:"rule_id,description=Intent rule ID to update,required"`
	DetectsField string  `json:"detects_field" mcp:"detects_field,description=Detection target dimension"`
	DetectsValue string  `json:"detects_value" mcp:"detects_value,description=Detection target value"`
	Dimension    string  `json:"dimension" mcp:"dimension,description=Matching dimension"`
	Pattern      string  `json:"pattern" mcp:"pattern,description=Regular expression pattern"`
	Confidence   float64 `json:"confidence" mcp:"confidence,description=Confidence score [0.0-1.0]"`
	Reasoning    string  `json:"reasoning" mcp:"reasoning,description=Explanation of why this rule works"`
	Status       string  `json:"status" mcp:"status,description=Lifecycle status"`
}

// IntentRuleUpdateResponse - update result
type IntentRuleUpdateResponse struct {
	ID      int64  `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// IntentRuleDeleteRequest - delete a rule by ID
type IntentRuleDeleteRequest struct {
	RuleID string `json:"rule_id" param:"rule_id" mcp:"rule_id,description=Intent rule ID to delete,required"`
}

// IntentRuleDeleteResponse - delete result
type IntentRuleDeleteResponse struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

// IntentRuleListRequest - list rules with filtering
type IntentRuleListRequest struct {
	Status       string `json:"status" query:"status" mcp:"status,description=Filter by status (proposed/testing/validated/promoted/retired/rejected). Use 'all' to list all rules"`
	DetectsField string `json:"detects_field" query:"detects_field" mcp:"detects_field,description=Filter by detects_field (category/model_family/training_method)"`
	Dimension    string `json:"dimension" query:"dimension" mcp:"dimension,description=Filter by dimension (image/cmdline/env_key/env_value/pip/code)"`
	Limit        int    `json:"limit" query:"limit" mcp:"limit,description=Max results (default 50)"`
	Offset       int    `json:"offset" query:"offset" mcp:"offset,description=Offset for pagination"`
}

// IntentRuleListResponse - list of rules
type IntentRuleListResponse struct {
	Rules []*IntentRuleGetResponse `json:"rules"`
	Total int                      `json:"total"`
}

// IntentRulePromoteRequest - force promote a rule
type IntentRulePromoteRequest struct {
	RuleID string `json:"rule_id" param:"rule_id" mcp:"rule_id,description=Intent rule ID to promote,required"`
}

// IntentRulePromoteResponse - promotion result
type IntentRulePromoteResponse struct {
	RuleID  int64  `json:"rule_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// IntentRuleRetireRequest - force retire a rule
type IntentRuleRetireRequest struct {
	RuleID string `json:"rule_id" param:"rule_id" mcp:"rule_id,description=Intent rule ID to retire,required"`
}

// IntentRuleRetireResponse - retirement result
type IntentRuleRetireResponse struct {
	RuleID  int64  `json:"rule_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// IntentRuleLabelRequest - manual label/update a rule
type IntentRuleLabelRequest struct {
	RuleID     string  `json:"rule_id" param:"rule_id" mcp:"rule_id,description=Intent rule ID to label,required"`
	Status     string  `json:"status" mcp:"status,description=New status (optional)"`
	Confidence float64 `json:"confidence" mcp:"confidence,description=Updated confidence (optional)"`
	Reasoning  string  `json:"reasoning" mcp:"reasoning,description=Manual reasoning note (optional)"`
}

// IntentRuleLabelResponse - label result
type IntentRuleLabelResponse struct {
	RuleID  int64  `json:"rule_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// IntentRuleBacktestRequest - trigger backtest for a rule
type IntentRuleBacktestRequest struct {
	RuleID string `json:"rule_id" param:"rule_id" mcp:"rule_id,description=Intent rule ID to backtest,required"`
}

// IntentRuleBacktestResponse - backtest result
type IntentRuleBacktestResponse struct {
	RuleID         int64       `json:"rule_id"`
	BacktestResult interface{} `json:"backtest_result"`
	Status         string      `json:"status"`
	Message        string      `json:"message"`
}

// ===== Registration =====

func init() {
	// CRUD endpoints
	unified.Register(&unified.EndpointDef[IntentRuleCreateRequest, IntentRuleCreateResponse]{
		Name:        "intent_rule_create",
		Description: "Create a new intent detection rule",
		HTTPMethod:  "POST",
		HTTPPath:    "/intent-rule",
		MCPToolName: "lens_intent_rule_create",
		Handler:     handleIntentRuleCreate,
	})

	unified.Register(&unified.EndpointDef[IntentRuleGetRequest, IntentRuleGetResponse]{
		Name:        "intent_rule_get",
		Description: "Get a single intent rule by ID with full details including backtest results",
		HTTPMethod:  "GET",
		HTTPPath:    "/intent-rule/:rule_id",
		MCPToolName: "lens_intent_rule_get",
		Handler:     handleIntentRuleGet,
	})

	unified.Register(&unified.EndpointDef[IntentRuleUpdateRequest, IntentRuleUpdateResponse]{
		Name:        "intent_rule_update",
		Description: "Update an existing intent rule (pattern, confidence, dimension, etc.)",
		HTTPMethod:  "PUT",
		HTTPPath:    "/intent-rule/:rule_id",
		MCPToolName: "lens_intent_rule_update",
		Handler:     handleIntentRuleUpdate,
	})

	unified.Register(&unified.EndpointDef[IntentRuleDeleteRequest, IntentRuleDeleteResponse]{
		Name:        "intent_rule_delete",
		Description: "Delete an intent rule by ID",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/intent-rule/:rule_id",
		MCPToolName: "lens_intent_rule_delete",
		Handler:     handleIntentRuleDelete,
	})

	unified.Register(&unified.EndpointDef[IntentRuleListRequest, IntentRuleListResponse]{
		Name:        "intent_rule_list",
		Description: "List intent rules with optional filtering by status, dimension, and detects_field. Defaults to all rules.",
		HTTPMethod:  "GET",
		HTTPPath:    "/intent-rule",
		MCPToolName: "lens_intent_rule_list",
		Handler:     handleIntentRuleList,
	})

	// Action endpoints
	unified.Register(&unified.EndpointDef[IntentRulePromoteRequest, IntentRulePromoteResponse]{
		Name:        "intent_rule_promote",
		Description: "Force promote a validated rule to production (admin action)",
		HTTPMethod:  "POST",
		HTTPPath:    "/intent-rule/:rule_id/promote",
		MCPToolName: "lens_intent_rule_promote",
		Handler:     handleIntentRulePromote,
	})

	unified.Register(&unified.EndpointDef[IntentRuleRetireRequest, IntentRuleRetireResponse]{
		Name:        "intent_rule_retire",
		Description: "Force retire a promoted rule (admin action)",
		HTTPMethod:  "POST",
		HTTPPath:    "/intent-rule/:rule_id/retire",
		MCPToolName: "lens_intent_rule_retire",
		Handler:     handleIntentRuleRetire,
	})

	unified.Register(&unified.EndpointDef[IntentRuleLabelRequest, IntentRuleLabelResponse]{
		Name:        "intent_rule_label",
		Description: "Manually label or update a rule's metadata (status, confidence, reasoning)",
		HTTPMethod:  "PUT",
		HTTPPath:    "/intent-rule/:rule_id/label",
		MCPToolName: "lens_intent_rule_label",
		Handler:     handleIntentRuleLabel,
	})

	unified.Register(&unified.EndpointDef[IntentRuleBacktestRequest, IntentRuleBacktestResponse]{
		Name:        "intent_rule_backtest",
		Description: "Trigger backtesting for a specific rule against historical data",
		HTTPMethod:  "POST",
		HTTPPath:    "/intent-rule/:rule_id/backtest",
		MCPToolName: "lens_intent_rule_backtest",
		Handler:     handleIntentRuleBacktest,
	})
}

// ===== Handlers =====

// valid dimension values for input validation
var validDimensions = map[string]bool{
	"image": true, "cmdline": true, "env_key": true,
	"env_value": true, "pip": true, "code": true, "process": true, "config": true,
}

func handleIntentRuleCreate(ctx context.Context, req *IntentRuleCreateRequest) (*IntentRuleCreateResponse, error) {
	if req.DetectsField == "" || req.DetectsValue == "" || req.Dimension == "" || req.Pattern == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).
			WithMessage("detects_field, detects_value, dimension and pattern are required")
	}
	if !validDimensions[req.Dimension] {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).
			WithMessage("invalid dimension, must be one of: image, cmdline, env_key, env_value, pip, code, process, config")
	}
	if req.Confidence < 0 || req.Confidence > 1.0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).
			WithMessage("confidence must be between 0.0 and 1.0")
	}

	// Validate regex
	if _, err := regexp.Compile(req.Pattern); err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).
			WithMessage("invalid regex pattern: " + err.Error())
	}

	status := req.Status
	if status == "" {
		status = "proposed"
	}

	rule := &dbModel.IntentRule{
		DetectsField: req.DetectsField,
		DetectsValue: req.DetectsValue,
		Dimension:    req.Dimension,
		Pattern:      req.Pattern,
		Confidence:   req.Confidence,
		Reasoning:    req.Reasoning,
		Status:       status,
	}

	facade := database.NewIntentRuleFacade()
	if err := facade.CreateRule(ctx, rule); err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to create rule: " + err.Error())
	}

	return &IntentRuleCreateResponse{
		ID:      rule.ID,
		Status:  rule.Status,
		Message: "Rule created successfully",
	}, nil
}

func handleIntentRuleGet(ctx context.Context, req *IntentRuleGetRequest) (*IntentRuleGetResponse, error) {
	ruleID, err := strconv.ParseInt(req.RuleID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid rule_id")
	}

	facade := database.NewIntentRuleFacade()
	rule, err := facade.GetRule(ctx, ruleID)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to get rule: " + err.Error())
	}
	if rule == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("rule not found")
	}

	return convertRuleToResponse(rule), nil
}

func handleIntentRuleList(ctx context.Context, req *IntentRuleListRequest) (*IntentRuleListResponse, error) {
	facade := database.NewIntentRuleFacade()

	var rules []*dbModel.IntentRule
	var err error

	switch {
	case req.Status != "" && req.Status != "all":
		rules, err = facade.ListByStatus(ctx, req.Status)
	case req.Dimension != "":
		rules, err = facade.ListByDimension(ctx, req.Dimension)
	case req.DetectsField != "":
		rules, err = facade.GetByDetectsField(ctx, req.DetectsField)
	default:
		rules, err = facade.ListAll(ctx)
	}

	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to list rules: " + err.Error())
	}

	// Apply pagination
	total := len(rules)
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := req.Offset
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	paginated := rules[offset:end]

	responses := make([]*IntentRuleGetResponse, len(paginated))
	for i, r := range paginated {
		responses[i] = convertRuleToResponse(r)
	}

	return &IntentRuleListResponse{
		Rules: responses,
		Total: total,
	}, nil
}

func handleIntentRuleUpdate(ctx context.Context, req *IntentRuleUpdateRequest) (*IntentRuleUpdateResponse, error) {
	ruleID, err := strconv.ParseInt(req.RuleID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid rule_id")
	}

	facade := database.NewIntentRuleFacade()
	rule, err := facade.GetRule(ctx, ruleID)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to get rule: " + err.Error())
	}
	if rule == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("rule not found")
	}

	// Apply partial updates
	if req.DetectsField != "" {
		rule.DetectsField = req.DetectsField
	}
	if req.DetectsValue != "" {
		rule.DetectsValue = req.DetectsValue
	}
	if req.Dimension != "" {
		if !validDimensions[req.Dimension] {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).
				WithMessage("invalid dimension")
		}
		rule.Dimension = req.Dimension
	}
	if req.Pattern != "" {
		if _, err := regexp.Compile(req.Pattern); err != nil {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).
				WithMessage("invalid regex pattern: " + err.Error())
		}
		rule.Pattern = req.Pattern
	}
	if req.Confidence > 0 {
		if req.Confidence > 1.0 {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).
				WithMessage("confidence must be between 0.0 and 1.0")
		}
		rule.Confidence = req.Confidence
	}
	if req.Reasoning != "" {
		rule.Reasoning = req.Reasoning
	}
	if req.Status != "" {
		rule.Status = req.Status
	}

	if err := facade.UpdateRule(ctx, rule); err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to update rule: " + err.Error())
	}

	return &IntentRuleUpdateResponse{
		ID:      ruleID,
		Status:  rule.Status,
		Message: "Rule updated successfully",
	}, nil
}

func handleIntentRuleDelete(ctx context.Context, req *IntentRuleDeleteRequest) (*IntentRuleDeleteResponse, error) {
	ruleID, err := strconv.ParseInt(req.RuleID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid rule_id")
	}

	facade := database.NewIntentRuleFacade()

	// Check existence first
	rule, err := facade.GetRule(ctx, ruleID)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to get rule: " + err.Error())
	}
	if rule == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("rule not found")
	}

	if err := facade.DeleteRule(ctx, ruleID); err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to delete rule: " + err.Error())
	}

	return &IntentRuleDeleteResponse{
		ID:      ruleID,
		Message: "Rule deleted successfully",
	}, nil
}

func handleIntentRulePromote(ctx context.Context, req *IntentRulePromoteRequest) (*IntentRulePromoteResponse, error) {
	ruleID, err := strconv.ParseInt(req.RuleID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid rule_id")
	}

	facade := database.NewIntentRuleFacade()
	if err := facade.UpdateStatus(ctx, ruleID, "promoted"); err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to promote rule: " + err.Error())
	}

	return &IntentRulePromoteResponse{
		RuleID:  ruleID,
		Status:  "promoted",
		Message: "Rule promoted successfully",
	}, nil
}

func handleIntentRuleRetire(ctx context.Context, req *IntentRuleRetireRequest) (*IntentRuleRetireResponse, error) {
	ruleID, err := strconv.ParseInt(req.RuleID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid rule_id")
	}

	facade := database.NewIntentRuleFacade()
	if err := facade.UpdateStatus(ctx, ruleID, "retired"); err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to retire rule: " + err.Error())
	}

	return &IntentRuleRetireResponse{
		RuleID:  ruleID,
		Status:  "retired",
		Message: "Rule retired successfully",
	}, nil
}

func handleIntentRuleLabel(ctx context.Context, req *IntentRuleLabelRequest) (*IntentRuleLabelResponse, error) {
	ruleID, err := strconv.ParseInt(req.RuleID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid rule_id")
	}

	facade := database.NewIntentRuleFacade()

	// Get current rule
	rule, err := facade.GetRule(ctx, ruleID)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to get rule: " + err.Error())
	}
	if rule == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("rule not found")
	}

	// Apply updates
	if req.Status != "" {
		if err := facade.UpdateStatus(ctx, ruleID, req.Status); err != nil {
			return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to update status: " + err.Error())
		}
	}

	if req.Confidence > 0 {
		rule.Confidence = req.Confidence
		if err := facade.UpdateRule(ctx, rule); err != nil {
			return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to update confidence: " + err.Error())
		}
	}

	if req.Reasoning != "" {
		rule.Reasoning = req.Reasoning
		if err := facade.UpdateRule(ctx, rule); err != nil {
			return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to update reasoning: " + err.Error())
		}
	}

	status := rule.Status
	if req.Status != "" {
		status = req.Status
	}

	return &IntentRuleLabelResponse{
		RuleID:  ruleID,
		Status:  status,
		Message: "Rule updated successfully",
	}, nil
}

func handleIntentRuleBacktest(ctx context.Context, req *IntentRuleBacktestRequest) (*IntentRuleBacktestResponse, error) {
	ruleID, err := strconv.ParseInt(req.RuleID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid rule_id")
	}

	facade := database.NewIntentRuleFacade()
	rule, err := facade.GetRule(ctx, ruleID)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to get rule: " + err.Error())
	}
	if rule == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("rule not found")
	}

	// Set status to "testing" so the daily backtester picks it up for evaluation.
	// The actual backtest computation runs in ai-advisor's scheduled flywheel job.
	if err := facade.UpdateStatus(ctx, ruleID, "testing"); err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to queue backtest: " + err.Error())
	}

	return &IntentRuleBacktestResponse{
		RuleID:         ruleID,
		BacktestResult: rule.BacktestResult,
		Status:         "testing",
		Message:        "Rule queued for backtesting (will be processed by the daily flywheel job)",
	}, nil
}

// ===== Helpers =====

func convertRuleToResponse(r *dbModel.IntentRule) *IntentRuleGetResponse {
	resp := &IntentRuleGetResponse{
		ID:                 r.ID,
		DetectsField:       r.DetectsField,
		DetectsValue:       r.DetectsValue,
		Dimension:          r.Dimension,
		Pattern:            r.Pattern,
		Confidence:         r.Confidence,
		Reasoning:          r.Reasoning,
		Status:             r.Status,
		MatchCount:         r.MatchCount,
		CorrectCount:       r.CorrectCount,
		FalsePositiveCount: r.FalsePositiveCount,
		CreatedAt:          r.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:          r.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	// DerivedFrom
	if r.DerivedFrom != nil {
		var derived []string
		if json.Unmarshal(r.DerivedFrom, &derived) == nil {
			resp.DerivedFrom = derived
		}
	}

	// BacktestResult
	if r.BacktestResult != nil {
		resp.BacktestResult = r.BacktestResult
	}

	if r.LastBacktestedAt != nil {
		resp.LastBacktestedAt = r.LastBacktestedAt.Format("2006-01-02T15:04:05Z")
	}

	return resp
}
