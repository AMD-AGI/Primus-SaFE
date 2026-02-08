// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/runner"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/storage"
)

// RunService handles tool execution and download
type RunService struct {
	facade  *database.ToolFacade
	runner  *runner.Runner
	storage storage.Storage
}

// NewRunService creates a new RunService
func NewRunService(
	facade *database.ToolFacade,
	runnerSvc *runner.Runner,
	storageSvc storage.Storage,
) *RunService {
	return &RunService{
		facade:  facade,
		runner:  runnerSvc,
		storage: storageSvc,
	}
}

// --- Types ---

// ToolRef represents a reference to a tool by ID or type+name
type ToolRef struct {
	ID   *int64
	Type string
	Name string
}

// RunResult represents the result of running tools
type RunResult struct {
	RedirectURL string
	SessionID   string
}

// DownloadResult represents download data for a tool
type DownloadResult struct {
	Data        []byte
	Filename    string
	ContentType string
}

// --- Service Methods ---

// RunTools runs multiple tools via the execution backend
func (s *RunService) RunTools(ctx context.Context, refs []ToolRef) (*RunResult, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("%w: runner", ErrNotConfigured)
	}

	var tools []*model.Tool
	for _, ref := range refs {
		var tool *model.Tool
		var err error

		if ref.ID != nil {
			tool, err = s.facade.GetByID(*ref.ID)
			if err != nil {
				return nil, fmt.Errorf("tool %w: id=%d", ErrNotFound, *ref.ID)
			}
		} else if ref.Type != "" && ref.Name != "" {
			tool, err = s.facade.GetByTypeAndName(ref.Type, ref.Name)
			if err != nil {
				return nil, fmt.Errorf("tool %w: %s/%s", ErrNotFound, ref.Type, ref.Name)
			}
		} else {
			return nil, fmt.Errorf("each tool must have either 'id' or 'type'+'name'")
		}

		tools = append(tools, tool)
	}

	result, err := s.runner.GetRunURL(ctx, tools)
	if err != nil {
		return nil, err
	}

	// Update run counts
	for _, tool := range tools {
		_ = s.facade.IncrementRunCount(tool.ID)
	}

	return &RunResult{
		RedirectURL: result.RedirectURL,
		SessionID:   result.SessionID,
	}, nil
}

// DownloadTool prepares a downloadable file for a tool
func (s *RunService) DownloadTool(ctx context.Context, id int64, userID string) (*DownloadResult, error) {
	tool, err := s.facade.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("tool %w", ErrNotFound)
	}

	if !tool.IsPublic && tool.OwnerUserID != userID {
		return nil, ErrAccessDenied
	}

	if tool.Type == model.AppTypeSkill {
		return s.downloadSkillAsZip(ctx, tool)
	}

	// MCP: generate setup guide
	content := generateMCPSetupGuide(tool)
	_ = s.facade.IncrementDownloadCount(id)

	return &DownloadResult{
		Data:        []byte(content),
		Filename:    tool.Name + "-setup.md",
		ContentType: "text/markdown",
	}, nil
}

// --- Private Helpers ---

// downloadSkillAsZip downloads skill files as a ZIP archive
func (s *RunService) downloadSkillAsZip(ctx context.Context, tool *model.Tool) (*DownloadResult, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("%w: storage", ErrNotConfigured)
	}

	s3Key := tool.GetSkillS3Key()
	if s3Key == "" {
		return nil, fmt.Errorf("skill content %w", ErrNotFound)
	}

	isPrefix := false
	if v, ok := tool.Config["is_prefix"].(bool); ok {
		isPrefix = v
	}

	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	if isPrefix {
		// List and download all files in the skill directory
		objects, err := s.storage.ListObjects(ctx, s3Key)
		if err != nil {
			return nil, fmt.Errorf("failed to list skill files: %w", err)
		}

		for _, obj := range objects {
			data, err := s.storage.DownloadBytes(ctx, obj.Key)
			if err != nil {
				continue
			}
			relPath := strings.TrimPrefix(obj.Key, s3Key)
			relPath = strings.TrimPrefix(relPath, "/")
			if relPath == "" {
				relPath = filepath.Base(obj.Key)
			}

			w, err := zipWriter.Create(relPath)
			if err != nil {
				continue
			}
			w.Write(data)
		}
	} else {
		// Download single file
		data, err := s.storage.DownloadBytes(ctx, s3Key)
		if err != nil {
			return nil, fmt.Errorf("failed to download skill content: %w", err)
		}

		w, err := zipWriter.Create("SKILL.md")
		if err != nil {
			return nil, fmt.Errorf("failed to create zip entry: %w", err)
		}
		w.Write(data)
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to create zip file: %w", err)
	}

	_ = s.facade.IncrementDownloadCount(tool.ID)

	return &DownloadResult{
		Data:        buf.Bytes(),
		Filename:    tool.Name + ".zip",
		ContentType: "application/zip",
	}, nil
}

// generateMCPSetupGuide generates a setup guide markdown for an MCP server
func generateMCPSetupGuide(tool *model.Tool) string {
	command, args, env := tool.GetMCPServerConfig()

	content := "# " + tool.Name + " - MCP Server Setup Guide\n\n"
	content += "## Description\n\n" + tool.Description + "\n\n"
	content += "## Cursor Configuration\n\n"
	content += "Add the following to your Cursor MCP settings:\n\n"
	content += "```json\n"
	content += "{\n"
	content += "  \"mcpServers\": {\n"
	content += "    \"" + tool.Name + "\": {\n"
	content += "      \"command\": \"" + command + "\",\n"
	content += "      \"args\": ["

	for i, arg := range args {
		if i > 0 {
			content += ", "
		}
		content += "\"" + arg + "\""
	}
	content += "]"

	if len(env) > 0 {
		content += ",\n      \"env\": {\n"
		first := true
		for k, v := range env {
			if !first {
				content += ",\n"
			}
			content += "        \"" + k + "\": \"" + v + "\""
			first = false
		}
		content += "\n      }"
	}

	content += "\n    }\n"
	content += "  }\n"
	content += "}\n"
	content += "```\n"

	return content
}
