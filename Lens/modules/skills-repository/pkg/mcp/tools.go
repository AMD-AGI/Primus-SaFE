// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package mcp

import (
	"context"
	"encoding/json"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/registry"
)

// SkillsMCPTools creates MCP tools for the skills repository
func CreateSkillsTools(reg *registry.SkillsRegistry) []*unified.MCPTool {
	return []*unified.MCPTool{
		createListSkillsTool(reg),
		createGetSkillTool(reg),
		createSearchSkillsTool(reg),
		createGetSkillContentTool(reg),
	}
}

func createListSkillsTool(reg *registry.SkillsRegistry) *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "skills_list",
		Description: "List all available skills in the repository with optional filtering by category or source",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"category": map[string]interface{}{
					"type":        "string",
					"description": "Filter by skill category (e.g., database, devops, k8s)",
				},
				"source": map[string]interface{}{
					"type":        "string",
					"description": "Filter by source (manual, git, local)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of skills to return (default: 50)",
				},
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				Category string `json:"category"`
				Source   string `json:"source"`
				Limit    int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			limit := params.Limit
			if limit == 0 {
				limit = 50
			}

			var skills interface{}
			var total int64
			var err error

			if params.Category != "" {
				skills, total, err = reg.ListByCategory(ctx, params.Category, 0, limit)
			} else if params.Source != "" {
				skills, total, err = reg.ListBySource(ctx, params.Source, 0, limit)
			} else {
				skills, total, err = reg.List(ctx, 0, limit)
			}

			if err != nil {
				return nil, err
			}

			return map[string]interface{}{
				"skills": skills,
				"total":  total,
			}, nil
		},
	}
}

func createGetSkillTool(reg *registry.SkillsRegistry) *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "skills_get",
		Description: "Get detailed information about a specific skill by name",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The name of the skill to retrieve",
				},
			},
			"required": []string{"name"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			skill, err := reg.Get(ctx, params.Name)
			if err != nil {
				return nil, err
			}

			return skill, nil
		},
	}
}

func createSearchSkillsTool(reg *registry.SkillsRegistry) *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "skills_search",
		Description: "Semantic search for skills using natural language. Use this to find relevant skills based on what you need to accomplish.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Natural language description of what you're looking for (e.g., 'how to create database migrations', 'deploy to kubernetes')",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of results (default: 5)",
				},
			},
			"required": []string{"query"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			limit := params.Limit
			if limit == 0 {
				limit = 5
			}

			results, err := reg.Search(ctx, params.Query, limit)
			if err != nil {
				return nil, err
			}

			// Format results
			type SearchResult struct {
				Name           string  `json:"name"`
				Description    string  `json:"description"`
				Category       string  `json:"category"`
				RelevanceScore float64 `json:"relevance_score"`
			}

			searchResults := make([]SearchResult, len(results))
			for i, r := range results {
				searchResults[i] = SearchResult{
					Name:           r.Skill.Name,
					Description:    r.Skill.Description,
					Category:       r.Skill.Category,
					RelevanceScore: r.Score,
				}
			}

			return map[string]interface{}{
				"results": searchResults,
				"total":   len(searchResults),
				"hint":    "Use skills_get_content to retrieve the full skill instructions",
			}, nil
		},
	}
}

func createGetSkillContentTool(reg *registry.SkillsRegistry) *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "skills_get_content",
		Description: "Get the full SKILL.md content for a skill. This contains detailed instructions on how to use the skill.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The name of the skill to get content for",
				},
			},
			"required": []string{"name"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			content, err := reg.GetContent(ctx, params.Name)
			if err != nil {
				return nil, err
			}

			return map[string]interface{}{
				"name":    params.Name,
				"content": content,
			}, nil
		},
	}
}
