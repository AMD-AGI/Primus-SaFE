// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/embedding"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/storage"
	"github.com/google/uuid"
)

// ImportService handles skill import workflows (discover + commit)
type ImportService struct {
	facade    *database.ToolFacade
	storage   storage.Storage
	embedding *embedding.Service
}

// NewImportService creates a new ImportService
func NewImportService(
	facade *database.ToolFacade,
	storageSvc storage.Storage,
	embeddingSvc *embedding.Service,
) *ImportService {
	return &ImportService{
		facade:    facade,
		storage:   storageSvc,
		embedding: embeddingSvc,
	}
}

// --- Service-layer types ---

// Selection represents a skill selection for import
type Selection struct {
	RelativePath string `json:"relative_path"`
	NameOverride string `json:"name_override"`
}

// DiscoverInput represents input for discovering skills
type DiscoverInput struct {
	UserID    string
	File      io.Reader
	FileName  string
	GitHubURL string
	Offset    int
	Limit     int
}

// DiscoverResult represents the result of skill discovery
type DiscoverResult struct {
	ArchiveKey string              `json:"archive_key"`
	Candidates []DiscoverCandidate `json:"candidates"`
	Total      int                 `json:"total"`
}

// DiscoverCandidate represents a discovered skill candidate
type DiscoverCandidate struct {
	RelativePath     string `json:"relative_path"`
	SkillName        string `json:"skill_name"`
	SkillDescription string `json:"skill_description"`
	RequiresName     bool   `json:"requires_name"`
	WillOverwrite    bool   `json:"will_overwrite"`
}

// CommitInput represents input for committing selected skills
type CommitInput struct {
	UserID     string
	Username   string
	ArchiveKey string
	Selections []Selection
}

// CommitResult represents the result of committing skills
type CommitResult struct {
	Items []CommitResultItem `json:"items"`
}

// CommitResultItem represents the result of importing a single skill
type CommitResultItem struct {
	RelativePath string `json:"relative_path"`
	SkillName    string `json:"skill_name"`
	Status       string `json:"status"`
	ToolID       int64  `json:"tool_id,omitempty"`
	Error        string `json:"error,omitempty"`
}

// importToolInfo holds tool info for batch embedding generation
type importToolInfo struct {
	ID          int64
	Name        string
	Description string
}

// --- Service Methods ---

// Discover scans a ZIP/MD file or GitHub repo for skills
func (s *ImportService) Discover(ctx context.Context, input *DiscoverInput) (*DiscoverResult, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("%w: storage not configured", ErrNotConfigured)
	}

	if input.File == nil && input.GitHubURL == "" {
		return nil, fmt.Errorf("either file or github_url must be provided")
	}
	if input.File != nil && input.GitHubURL != "" {
		return nil, fmt.Errorf("only one of file or github_url can be provided")
	}

	var zipData []byte
	var filename string
	var err error

	if input.GitHubURL != "" {
		// Download from GitHub
		zipData, err = downloadGitHubZip(ctx, input.GitHubURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download from GitHub: %w", err)
		}
		filename = "github.zip"
	} else {
		// Read uploaded file
		fileData, err := io.ReadAll(input.File)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		filename = input.FileName
		if filename == "" {
			filename = "upload.zip"
		}

		// Check if it's a single MD file (not a ZIP)
		if isMDFile(filename) {
			if !isValidSkillMD(fileData) {
				return nil, fmt.Errorf("invalid SKILL.md file: must contain valid skill definition")
			}
			zipData, err = wrapMDInZip(fileData, "SKILL.md")
			if err != nil {
				return nil, fmt.Errorf("failed to process MD file: %w", err)
			}
			filename = "upload.zip"
		} else {
			zipData = fileData
		}
	}

	// Scan for SKILL.md files
	candidates, err := scanCandidates(zipData)
	if err != nil {
		return nil, fmt.Errorf("failed to scan archive: %w", err)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no SKILL.md found in the archive")
	}

	// Check for existing skills
	for idx := range candidates {
		if candidates[idx].SkillName != "" {
			_, err := s.facade.GetByTypeAndName(model.AppTypeSkill, candidates[idx].SkillName)
			if err == nil {
				candidates[idx].WillOverwrite = true
			}
		}
	}

	// Upload to temporary storage
	userID := input.UserID
	if userID == "" {
		userID = "anonymous"
	}
	archiveKey := fmt.Sprintf("skill-imports/%s/%s/%s", userID, uuid.New().String(), filename)
	if err := s.storage.UploadBytes(ctx, archiveKey, zipData); err != nil {
		return nil, fmt.Errorf("failed to upload archive: %w", err)
	}

	// Apply pagination
	total := len(candidates)
	if input.Offset > 0 || input.Limit > 0 {
		start := input.Offset
		if start > total {
			start = total
		}
		end := total
		if input.Limit > 0 && start+input.Limit < total {
			end = start + input.Limit
		}
		candidates = candidates[start:end]
	}

	return &DiscoverResult{
		ArchiveKey: archiveKey,
		Candidates: candidates,
		Total:      total,
	}, nil
}

// Commit imports selected skills from a previously uploaded archive
func (s *ImportService) Commit(ctx context.Context, input *CommitInput) (*CommitResult, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("%w: storage not configured", ErrNotConfigured)
	}

	// Handle empty UserID
	userID := input.UserID
	if userID == "" {
		userID = "anonymous"
	}

	// Verify archive belongs to user
	expectedPrefix := fmt.Sprintf("skill-imports/%s/", userID)
	if !strings.HasPrefix(input.ArchiveKey, expectedPrefix) {
		return nil, fmt.Errorf("archive does not belong to the user")
	}

	// Download archive
	zipData, err := s.storage.DownloadBytes(ctx, input.ArchiveKey)
	if err != nil {
		return nil, fmt.Errorf("failed to download archive: %w", err)
	}

	// Open zip
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip archive: %w", err)
	}

	// Scan candidates to get mapping
	candidates, err := scanCandidates(zipData)
	if err != nil {
		return nil, fmt.Errorf("failed to scan archive: %w", err)
	}
	candidateByPath := make(map[string]DiscoverCandidate)
	for _, c := range candidates {
		candidateByPath[c.RelativePath] = c
	}

	// Find common root
	commonRoot := findCommonRoot(zipReader)

	// Phase 1: Process selections in parallel (S3 upload + DB insert, no embedding)
	const maxWorkers = 10

	items := make([]CommitResultItem, len(input.Selections))
	toolInfos := make([]importToolInfo, len(input.Selections))
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxWorkers)

	for idx, sel := range input.Selections {
		wg.Add(1)
		go func(idx int, sel Selection) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			item, info := s.importOneWithoutEmbedding(ctx, zipReader, commonRoot, candidateByPath, sel, userID, input.Username)
			mu.Lock()
			items[idx] = item
			if item.Status == "success" && info.ID > 0 {
				toolInfos[idx] = info
			}
			mu.Unlock()
		}(idx, sel)
	}
	wg.Wait()

	// Phase 2: Batch generate embeddings for successful imports
	// Use independent context to avoid cancellation when client disconnects
	embeddingCtx, embeddingCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer embeddingCancel()
	s.batchGenerateEmbeddings(embeddingCtx, toolInfos)

	return &CommitResult{Items: items}, nil
}
