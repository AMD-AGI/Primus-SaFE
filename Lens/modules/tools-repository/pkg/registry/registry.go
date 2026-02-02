// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/lib/pq"
)

// ToolsRegistry manages tool registration and discovery
type ToolsRegistry struct {
	db       *sql.DB
	mu       sync.RWMutex
	cache    map[string]*Tool
	cacheExp time.Time
	cacheTTL time.Duration
}

// NewToolsRegistry creates a new tools registry
func NewToolsRegistry(db *sql.DB) *ToolsRegistry {
	return &ToolsRegistry{
		db:       db,
		cache:    make(map[string]*Tool),
		cacheTTL: 5 * time.Minute,
	}
}

// Register registers a new tool
func (r *ToolsRegistry) Register(ctx context.Context, req *RegisterToolRequest, createdBy string) (*Tool, error) {
	tool := &Tool{
		Name:           req.Name,
		Version:        req.Version,
		Description:    req.Description,
		ProviderType:   req.ProviderType,
		ProviderConfig: req.ProviderConfig,
		InputSchema:    req.InputSchema,
		OutputSchema:   req.OutputSchema,
		Category:       req.Category,
		Tags:           req.Tags,
		Scope:          req.Scope,
		ScopeID:        req.ScopeID,
		Enabled:        true,
		CreatedBy:      createdBy,
	}

	if tool.Version == "" {
		tool.Version = "1.0.0"
	}
	if tool.Scope == "" {
		tool.Scope = ScopePlatform
	}

	query := `
		INSERT INTO tools (name, version, description, provider_type, provider_config, 
			input_schema, output_schema, category, tags, scope, scope_id, enabled, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (name, version, scope, scope_id) 
		DO UPDATE SET 
			description = EXCLUDED.description,
			provider_type = EXCLUDED.provider_type,
			provider_config = EXCLUDED.provider_config,
			input_schema = EXCLUDED.input_schema,
			output_schema = EXCLUDED.output_schema,
			category = EXCLUDED.category,
			tags = EXCLUDED.tags,
			enabled = EXCLUDED.enabled,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		tool.Name, tool.Version, tool.Description, tool.ProviderType, tool.ProviderConfig,
		tool.InputSchema, tool.OutputSchema, tool.Category, pq.Array(tool.Tags),
		tool.Scope, tool.ScopeID, tool.Enabled, tool.CreatedBy,
	).Scan(&tool.ID, &tool.CreatedAt, &tool.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to register tool: %w", err)
	}

	// Invalidate cache
	r.invalidateCache()

	return tool, nil
}

// Get retrieves a tool by name
func (r *ToolsRegistry) Get(ctx context.Context, name string) (*Tool, error) {
	// Check cache first
	r.mu.RLock()
	if tool, ok := r.cache[name]; ok && time.Now().Before(r.cacheExp) {
		r.mu.RUnlock()
		return tool, nil
	}
	r.mu.RUnlock()

	query := `
		SELECT id, name, version, description, provider_type, provider_config,
			input_schema, output_schema, category, tags, scope, scope_id, 
			enabled, created_at, updated_at, created_by
		FROM tools
		WHERE name = $1 AND enabled = true
		ORDER BY version DESC
		LIMIT 1
	`

	tool := &Tool{}
	var tags pq.StringArray

	err := r.db.QueryRowContext(ctx, query, name).Scan(
		&tool.ID, &tool.Name, &tool.Version, &tool.Description,
		&tool.ProviderType, &tool.ProviderConfig, &tool.InputSchema,
		&tool.OutputSchema, &tool.Category, &tags, &tool.Scope,
		&tool.ScopeID, &tool.Enabled, &tool.CreatedAt, &tool.UpdatedAt,
		&tool.CreatedBy,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get tool: %w", err)
	}

	tool.Tags = []string(tags)

	// Update cache
	r.mu.Lock()
	r.cache[name] = tool
	r.cacheExp = time.Now().Add(r.cacheTTL)
	r.mu.Unlock()

	return tool, nil
}

// List lists all tools with optional filters
func (r *ToolsRegistry) List(ctx context.Context, category, providerType string, scope Scope, offset, limit int) ([]*Tool, int64, error) {
	if limit == 0 {
		limit = 50
	}

	countQuery := `SELECT COUNT(*) FROM tools WHERE enabled = true`
	selectQuery := `
		SELECT id, name, version, description, provider_type, provider_config,
			input_schema, output_schema, category, tags, scope, scope_id,
			enabled, created_at, updated_at, created_by
		FROM tools
		WHERE enabled = true
	`

	var args []interface{}
	argIdx := 1

	if category != "" {
		countQuery += fmt.Sprintf(" AND category = $%d", argIdx)
		selectQuery += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, category)
		argIdx++
	}

	if providerType != "" {
		countQuery += fmt.Sprintf(" AND provider_type = $%d", argIdx)
		selectQuery += fmt.Sprintf(" AND provider_type = $%d", argIdx)
		args = append(args, providerType)
		argIdx++
	}

	if scope != "" {
		countQuery += fmt.Sprintf(" AND scope = $%d", argIdx)
		selectQuery += fmt.Sprintf(" AND scope = $%d", argIdx)
		args = append(args, scope)
		argIdx++
	}

	selectQuery += fmt.Sprintf(" ORDER BY name ASC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	// Get count
	var total int64
	countArgs := args[:len(args)-2] // Exclude limit and offset
	if err := r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count tools: %w", err)
	}

	// Get tools
	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list tools: %w", err)
	}
	defer rows.Close()

	var tools []*Tool
	for rows.Next() {
		tool := &Tool{}
		var tags pq.StringArray

		err := rows.Scan(
			&tool.ID, &tool.Name, &tool.Version, &tool.Description,
			&tool.ProviderType, &tool.ProviderConfig, &tool.InputSchema,
			&tool.OutputSchema, &tool.Category, &tags, &tool.Scope,
			&tool.ScopeID, &tool.Enabled, &tool.CreatedAt, &tool.UpdatedAt,
			&tool.CreatedBy,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan tool: %w", err)
		}

		tool.Tags = []string(tags)
		tools = append(tools, tool)
	}

	return tools, total, nil
}

// Search searches tools by query (keyword-based)
func (r *ToolsRegistry) Search(ctx context.Context, query string, limit int) ([]*SearchResult, error) {
	if limit == 0 {
		limit = 10
	}

	sqlQuery := `
		SELECT id, name, version, description, provider_type, provider_config,
			input_schema, output_schema, category, tags, scope, scope_id,
			enabled, created_at, updated_at, created_by,
			ts_rank(to_tsvector('english', name || ' ' || description), plainto_tsquery('english', $1)) as rank
		FROM tools
		WHERE enabled = true
			AND (
				name ILIKE '%' || $1 || '%'
				OR description ILIKE '%' || $1 || '%'
				OR $1 = ANY(tags)
			)
		ORDER BY rank DESC, name ASC
		LIMIT $2
	`

	rows, err := r.db.QueryContext(ctx, sqlQuery, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search tools: %w", err)
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		tool := &Tool{}
		var tags pq.StringArray
		var rank float64

		err := rows.Scan(
			&tool.ID, &tool.Name, &tool.Version, &tool.Description,
			&tool.ProviderType, &tool.ProviderConfig, &tool.InputSchema,
			&tool.OutputSchema, &tool.Category, &tags, &tool.Scope,
			&tool.ScopeID, &tool.Enabled, &tool.CreatedAt, &tool.UpdatedAt,
			&tool.CreatedBy, &rank,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}

		tool.Tags = []string(tags)
		results = append(results, &SearchResult{
			Tool:      tool,
			Score:     rank,
			MatchType: "keyword",
		})
	}

	return results, nil
}

// Delete deletes a tool by name
func (r *ToolsRegistry) Delete(ctx context.Context, name string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM tools WHERE name = $1", name)
	if err != nil {
		return fmt.Errorf("failed to delete tool: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("tool not found: %s", name)
	}

	r.invalidateCache()
	return nil
}

// RecordExecution records a tool execution
func (r *ToolsRegistry) RecordExecution(ctx context.Context, exec *ToolExecution) error {
	query := `
		INSERT INTO tool_executions (tool_id, tool_name, user_id, session_id, 
			input, output, status, error_message, duration_ms, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`

	var toolID interface{}
	if exec.ToolID != "" {
		toolID = exec.ToolID
	}

	err := r.db.QueryRowContext(ctx, query,
		toolID, exec.ToolName, exec.UserID, exec.SessionID,
		exec.Input, exec.Output, exec.Status, exec.ErrorMessage,
		exec.DurationMs, exec.StartedAt, exec.CompletedAt,
	).Scan(&exec.ID, &exec.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to record execution: %w", err)
	}

	// Update stats asynchronously
	go r.updateStats(context.Background(), exec)

	return nil
}

// GetStats gets statistics for a tool
func (r *ToolsRegistry) GetStats(ctx context.Context, toolName string) (*ToolStats, error) {
	query := `
		SELECT tool_name, total_executions, success_count, error_count,
			avg_duration_ms, p95_duration_ms, last_used_at, updated_at
		FROM tool_stats
		WHERE tool_name = $1
	`

	stats := &ToolStats{}
	err := r.db.QueryRowContext(ctx, query, toolName).Scan(
		&stats.ToolName, &stats.TotalExecutions, &stats.SuccessCount,
		&stats.ErrorCount, &stats.AvgDurationMs, &stats.P95DurationMs,
		&stats.LastUsedAt, &stats.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return &ToolStats{ToolName: toolName}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return stats, nil
}

// GetToolDefinition returns tool definition in MCP format
func (r *ToolsRegistry) GetToolDefinition(ctx context.Context, name string) (map[string]interface{}, error) {
	tool, err := r.Get(ctx, name)
	if err != nil {
		return nil, err
	}

	var inputSchema map[string]interface{}
	if len(tool.InputSchema) > 0 {
		json.Unmarshal(tool.InputSchema, &inputSchema)
	}

	return map[string]interface{}{
		"name":         tool.Name,
		"description":  tool.Description,
		"input_schema": inputSchema,
	}, nil
}

func (r *ToolsRegistry) invalidateCache() {
	r.mu.Lock()
	r.cache = make(map[string]*Tool)
	r.cacheExp = time.Time{}
	r.mu.Unlock()
}

func (r *ToolsRegistry) updateStats(ctx context.Context, exec *ToolExecution) {
	query := `
		INSERT INTO tool_stats (tool_name, total_executions, success_count, error_count, avg_duration_ms, last_used_at)
		VALUES ($1, 1, $2, $3, $4, $5)
		ON CONFLICT (tool_name) DO UPDATE SET
			total_executions = tool_stats.total_executions + 1,
			success_count = tool_stats.success_count + $2,
			error_count = tool_stats.error_count + $3,
			avg_duration_ms = (tool_stats.avg_duration_ms * tool_stats.total_executions + $4) / (tool_stats.total_executions + 1),
			last_used_at = $5,
			updated_at = NOW()
	`

	successCount := 0
	errorCount := 0
	if exec.Status == "success" {
		successCount = 1
	} else {
		errorCount = 1
	}

	r.db.ExecContext(ctx, query, exec.ToolName, successCount, errorCount, exec.DurationMs, exec.StartedAt)
}
