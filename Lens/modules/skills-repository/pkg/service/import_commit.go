// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
)

// importOneWithoutEmbedding imports a single skill without generating embedding
// Returns the result item and tool info for batch embedding generation
func (s *ImportService) importOneWithoutEmbedding(
	ctx context.Context,
	zipReader *zip.Reader,
	commonRoot string,
	candidateByPath map[string]DiscoverCandidate,
	sel Selection,
	userID string,
	author string,
) (CommitResultItem, importToolInfo) {
	candidate, ok := candidateByPath[sel.RelativePath]
	if !ok {
		return CommitResultItem{
			RelativePath: sel.RelativePath,
			Status:       "failed",
			Error:        "skill not found in archive",
		}, importToolInfo{}
	}

	// Determine skill name
	skillName := sel.NameOverride
	if skillName == "" {
		skillName = candidate.SkillName
	}
	if skillName == "" {
		return CommitResultItem{
			RelativePath: sel.RelativePath,
			Status:       "failed",
			Error:        "skill name is required",
		}, importToolInfo{}
	}

	// Use parsed description from candidate
	skillDescription := candidate.SkillDescription
	if skillDescription == "" {
		skillDescription = fmt.Sprintf("Imported skill: %s", skillName)
	}

	// Find SKILL.md file and related files
	skillDir := sel.RelativePath
	if skillDir == "." {
		skillDir = ""
	}

	// Read all files in the skill directory
	var skillContent []byte
	var additionalFiles []struct {
		name string
		data []byte
	}

	for _, f := range zipReader.File {
		if f.FileInfo().IsDir() {
			continue
		}

		// Strip common root
		relPath := stripCommonRoot(f.Name, commonRoot)
		if relPath == "" {
			continue
		}

		// Check if file belongs to this skill
		var inSkillDir bool
		if skillDir == "" {
			// Root level skill
			inSkillDir = !strings.Contains(relPath, "/")
		} else {
			inSkillDir = strings.HasPrefix(relPath, skillDir+"/") || relPath == skillDir
		}

		if !inSkillDir {
			continue
		}

		// Read file content
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		// Get file name relative to skill directory
		var fileName string
		if skillDir == "" {
			fileName = relPath
		} else {
			fileName = strings.TrimPrefix(relPath, skillDir+"/")
		}

		if strings.ToLower(path.Base(fileName)) == "skill.md" {
			skillContent = data
		} else if fileName != "" {
			additionalFiles = append(additionalFiles, struct {
				name string
				data []byte
			}{name: fileName, data: data})
		}
	}

	if skillContent == nil {
		return CommitResultItem{
			RelativePath: sel.RelativePath,
			SkillName:    skillName,
			Status:       "failed",
			Error:        "SKILL.md not found",
		}, importToolInfo{}
	}

	// Upload to permanent storage
	timestamp := time.Now().UnixNano()
	s3KeyBase := fmt.Sprintf("skills/%s/%d", skillName, timestamp)
	s3KeyFile := s3KeyBase + "/SKILL.md"

	if err := s.storage.UploadBytes(ctx, s3KeyFile, skillContent); err != nil {
		return CommitResultItem{
			RelativePath: sel.RelativePath,
			SkillName:    skillName,
			Status:       "failed",
			Error:        fmt.Sprintf("failed to upload: %v", err),
		}, importToolInfo{}
	}

	// Upload additional files
	isPrefix := len(additionalFiles) > 0
	for _, af := range additionalFiles {
		afKey := s3KeyBase + "/" + af.name
		_ = s.storage.UploadBytes(ctx, afKey, af.data)
	}

	// Create or update tool record
	// When is_prefix=true, s3_key is the directory prefix (for ListObjects);
	// when is_prefix=false, s3_key is the full path to SKILL.md.
	s3Key := s3KeyFile
	if isPrefix {
		s3Key = s3KeyBase + "/"
	}
	config := model.AppConfig{
		"s3_key":    s3Key,
		"is_prefix": isPrefix,
	}

	// Check if skill already exists
	existing, err := s.facade.GetByTypeAndName(model.AppTypeSkill, skillName)
	if err == nil && existing != nil {
		// Update existing
		existing.Config = config
		existing.SkillSource = model.SkillSourceZIP
		existing.Description = skillDescription
		if err := s.facade.Update(existing); err != nil {
			return CommitResultItem{
				RelativePath: sel.RelativePath,
				SkillName:    skillName,
				Status:       "failed",
				Error:        fmt.Sprintf("failed to update: %v", err),
			}, importToolInfo{}
		}
		return CommitResultItem{
			RelativePath: sel.RelativePath,
			SkillName:    skillName,
			Status:       "success",
			ToolID:       existing.ID,
		}, importToolInfo{ID: existing.ID, Name: existing.Name, Description: existing.Description}
	}

	// Create new
	tool := &model.Tool{
		Type:        model.AppTypeSkill,
		Name:        skillName,
		DisplayName: skillName,
		Description: skillDescription,
		Config:      config,
		SkillSource: model.SkillSourceZIP,
		OwnerUserID: userID,
		Author:      author,
		IsPublic:    true,
		Status:      model.AppStatusActive,
	}

	if err := s.facade.Create(tool); err != nil {
		// Handle race condition: another goroutine may have created the same skill
		existing2, err2 := s.facade.GetByTypeAndName(model.AppTypeSkill, skillName)
		if err2 == nil && existing2 != nil {
			existing2.Config = config
			existing2.SkillSource = model.SkillSourceZIP
			existing2.Description = skillDescription
			if err3 := s.facade.Update(existing2); err3 != nil {
				return CommitResultItem{
					RelativePath: sel.RelativePath,
					SkillName:    skillName,
					Status:       "failed",
					Error:        fmt.Sprintf("failed to update after race: %v", err3),
				}, importToolInfo{}
			}
			return CommitResultItem{
				RelativePath: sel.RelativePath,
				SkillName:    skillName,
				Status:       "success",
				ToolID:       existing2.ID,
			}, importToolInfo{ID: existing2.ID, Name: existing2.Name, Description: existing2.Description}
		}
		return CommitResultItem{
			RelativePath: sel.RelativePath,
			SkillName:    skillName,
			Status:       "failed",
			Error:        fmt.Sprintf("failed to create: %v", err),
		}, importToolInfo{}
	}

	return CommitResultItem{
		RelativePath: sel.RelativePath,
		SkillName:    skillName,
		Status:       "success",
		ToolID:       tool.ID,
	}, importToolInfo{ID: tool.ID, Name: tool.Name, Description: tool.Description}
}

// batchGenerateEmbeddings generates embeddings for multiple tools in batch
func (s *ImportService) batchGenerateEmbeddings(ctx context.Context, toolInfos []importToolInfo) {
	if s.embedding == nil {
		return
	}

	// Collect valid tools
	var validTools []importToolInfo
	for _, info := range toolInfos {
		if info.ID > 0 && info.Name != "" {
			validTools = append(validTools, info)
		}
	}

	if len(validTools) == 0 {
		return
	}

	// Prepare texts for batch embedding
	texts := make([]string, len(validTools))
	for i, t := range validTools {
		texts[i] = fmt.Sprintf("%s: %s", t.Name, t.Description)
	}

	// Batch generate embeddings
	embeddings, err := s.embedding.GenerateBatch(ctx, texts)
	if err != nil {
		fmt.Printf("Failed to batch generate embeddings: %v\n", err)
		// Fallback to individual generation
		for _, t := range validTools {
			s.generateEmbeddingFallback(ctx, t.ID, t.Name, t.Description)
		}
		return
	}

	// Update embeddings in database
	for idx, emb := range embeddings {
		if emb != nil {
			if err := s.facade.UpdateEmbedding(validTools[idx].ID, emb); err != nil {
				fmt.Printf("Failed to update embedding for tool %d: %v\n", validTools[idx].ID, err)
			}
		}
	}
}

// generateEmbeddingFallback generates embedding for a single tool
// This is used as a fallback when batch embedding generation fails
func (s *ImportService) generateEmbeddingFallback(ctx context.Context, toolID int64, name, description string) {
	if s.embedding == nil {
		return
	}

	emb, err := s.embedding.GenerateForTool(ctx, name, description)
	if err != nil {
		fmt.Printf("Failed to generate embedding for tool %d: %v\n", toolID, err)
		return
	}

	if err := s.facade.UpdateEmbedding(toolID, emb); err != nil {
		fmt.Printf("Failed to update embedding for tool %d: %v\n", toolID, err)
	}
}
