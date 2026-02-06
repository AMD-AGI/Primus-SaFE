// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/storage"
	"github.com/google/uuid"
)

// Importer handles skill import from ZIP files or GitHub URLs
type Importer struct {
	facade  *database.ToolFacade
	storage storage.Storage
}

// NewImporter creates a new Importer
func NewImporter(facade *database.ToolFacade, storage storage.Storage) *Importer {
	return &Importer{
		facade:  facade,
		storage: storage,
	}
}

// DiscoverRequest represents a discover request
type DiscoverRequest struct {
	UserID    string
	File      io.Reader
	FileName  string
	GitHubURL string
}

// DiscoverResponse represents a discover response
type DiscoverResponse struct {
	ArchiveKey string      `json:"archive_key"`
	Candidates []Candidate `json:"candidates"`
}

// Candidate represents a discovered skill candidate
type Candidate struct {
	RelativePath     string `json:"relative_path"`
	SkillName        string `json:"skill_name"`
	SkillDescription string `json:"skill_description"`
	RequiresName     bool   `json:"requires_name"`
	WillOverwrite    bool   `json:"will_overwrite"`
}

// CommitRequest represents a commit request
type CommitRequest struct {
	UserID     string
	ArchiveKey string
	Selections []Selection
}

// Selection represents a skill selection for import
type Selection struct {
	RelativePath string `json:"relative_path"`
	NameOverride string `json:"name_override"`
}

// CommitResponse represents a commit response
type CommitResponse struct {
	Items []CommitResultItem `json:"items"`
}

// CommitResultItem represents the result of importing a single skill
type CommitResultItem struct {
	RelativePath string `json:"relative_path"`
	SkillName    string `json:"skill_name"`
	Status       string `json:"status"` // success, failed, skipped
	ToolID       int64  `json:"tool_id,omitempty"`
	Error        string `json:"error,omitempty"`
}

// Discover scans a ZIP/MD file or GitHub repo for skills
func (i *Importer) Discover(ctx context.Context, req *DiscoverRequest) (*DiscoverResponse, error) {
	if req.File == nil && req.GitHubURL == "" {
		return nil, fmt.Errorf("either file or github_url must be provided")
	}
	if req.File != nil && req.GitHubURL != "" {
		return nil, fmt.Errorf("only one of file or github_url can be provided")
	}

	var zipData []byte
	var filename string
	var err error

	if req.GitHubURL != "" {
		// Download from GitHub
		zipData, err = i.downloadGitHubZip(ctx, req.GitHubURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download from GitHub: %w", err)
		}
		filename = "github.zip"
	} else {
		// Read uploaded file
		fileData, err := io.ReadAll(req.File)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		filename = req.FileName
		if filename == "" {
			filename = "upload.zip"
		}

		// Check if it's a single MD file (not a ZIP)
		if i.isMDFile(filename) {
			// Validate it's a valid SKILL.md content
			if !i.isValidSkillMD(fileData) {
				return nil, fmt.Errorf("invalid SKILL.md file: must contain valid skill definition")
			}
			// Wrap single MD file into a ZIP
			zipData, err = i.wrapMDInZip(fileData, "SKILL.md")
			if err != nil {
				return nil, fmt.Errorf("failed to process MD file: %w", err)
			}
			filename = "upload.zip"
		} else {
			zipData = fileData
		}
	}

	// Scan for SKILL.md files
	candidates, err := i.scanCandidates(zipData)
	if err != nil {
		return nil, fmt.Errorf("failed to scan archive: %w", err)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no SKILL.md found in the archive")
	}

	// Check for existing skills
	for idx := range candidates {
		if candidates[idx].SkillName != "" {
			_, err := i.facade.GetByTypeAndName(model.AppTypeSkill, candidates[idx].SkillName)
			if err == nil {
				candidates[idx].WillOverwrite = true
			}
		}
	}

	// Upload to temporary storage
	userID := req.UserID
	if userID == "" {
		userID = "anonymous"
	}
	archiveKey := fmt.Sprintf("skill-imports/%s/%s/%s", userID, uuid.New().String(), filename)
	if err := i.storage.UploadBytes(ctx, archiveKey, zipData); err != nil {
		return nil, fmt.Errorf("failed to upload archive: %w", err)
	}

	return &DiscoverResponse{
		ArchiveKey: archiveKey,
		Candidates: candidates,
	}, nil
}

// isMDFile checks if the filename is a markdown file
func (i *Importer) isMDFile(filename string) bool {
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

// isValidSkillMD performs basic validation on skill MD content
func (i *Importer) isValidSkillMD(content []byte) bool {
	// Basic validation: must not be empty and should look like markdown
	if len(content) == 0 {
		return false
	}
	// Could add more sophisticated validation here (e.g., check for required sections)
	return true
}

// wrapMDInZip wraps a single MD file into a ZIP archive
func (i *Importer) wrapMDInZip(mdContent []byte, filename string) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Create the file in the ZIP
	w, err := zipWriter.Create(filename)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(mdContent); err != nil {
		return nil, err
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Commit imports selected skills from a previously uploaded archive
func (i *Importer) Commit(ctx context.Context, req *CommitRequest) (*CommitResponse, error) {
	// Handle empty UserID
	userID := req.UserID
	if userID == "" {
		userID = "anonymous"
	}

	// Verify archive belongs to user
	expectedPrefix := fmt.Sprintf("skill-imports/%s/", userID)
	if !strings.HasPrefix(req.ArchiveKey, expectedPrefix) {
		return nil, fmt.Errorf("archive does not belong to the user")
	}

	// Download archive
	zipData, err := i.storage.DownloadBytes(ctx, req.ArchiveKey)
	if err != nil {
		return nil, fmt.Errorf("failed to download archive: %w", err)
	}

	// Open zip
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip archive: %w", err)
	}

	// Scan candidates to get mapping
	candidates, err := i.scanCandidates(zipData)
	if err != nil {
		return nil, fmt.Errorf("failed to scan archive: %w", err)
	}
	candidateByPath := make(map[string]Candidate)
	for _, c := range candidates {
		candidateByPath[c.RelativePath] = c
	}

	// Find common root
	commonRoot := i.findCommonRoot(zipReader)

	// Process selections
	var items []CommitResultItem
	for _, sel := range req.Selections {
		item := i.importOne(ctx, zipReader, commonRoot, candidateByPath, sel, userID)
		items = append(items, item)
	}

	return &CommitResponse{Items: items}, nil
}

// importOne imports a single skill
func (i *Importer) importOne(
	ctx context.Context,
	zipReader *zip.Reader,
	commonRoot string,
	candidateByPath map[string]Candidate,
	sel Selection,
	userID string,
) CommitResultItem {
	candidate, ok := candidateByPath[sel.RelativePath]
	if !ok {
		return CommitResultItem{
			RelativePath: sel.RelativePath,
			Status:       "failed",
			Error:        "skill not found in archive",
		}
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
		}
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
		relPath := i.stripCommonRoot(f.Name, commonRoot)
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
		}
	}

	// Upload to permanent storage
	timestamp := time.Now().UnixNano()
	s3KeyBase := fmt.Sprintf("skills/%d", timestamp)
	s3Key := s3KeyBase + "/SKILL.md"

	if err := i.storage.UploadBytes(ctx, s3Key, skillContent); err != nil {
		return CommitResultItem{
			RelativePath: sel.RelativePath,
			SkillName:    skillName,
			Status:       "failed",
			Error:        fmt.Sprintf("failed to upload: %v", err),
		}
	}

	// Upload additional files
	isPrefix := len(additionalFiles) > 0
	for _, af := range additionalFiles {
		afKey := s3KeyBase + "/" + af.name
		_ = i.storage.UploadBytes(ctx, afKey, af.data)
	}

	// Create or update tool record
	config := model.AppConfig{
		"s3_key":    s3Key,
		"is_prefix": isPrefix,
	}

	// Check if skill already exists
	existing, err := i.facade.GetByTypeAndName(model.AppTypeSkill, skillName)
	if err == nil && existing != nil {
		// Update existing
		existing.Config = config
		existing.SkillSource = model.SkillSourceZIP
		existing.Description = skillDescription // Update description from SKILL.md
		if err := i.facade.Update(existing); err != nil {
			return CommitResultItem{
				RelativePath: sel.RelativePath,
				SkillName:    skillName,
				Status:       "failed",
				Error:        fmt.Sprintf("failed to update: %v", err),
			}
		}
		return CommitResultItem{
			RelativePath: sel.RelativePath,
			SkillName:    skillName,
			Status:       "success",
			ToolID:       existing.ID,
		}
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
		IsPublic:    true,
		Status:      model.AppStatusActive,
	}

	if err := i.facade.Create(tool); err != nil {
		return CommitResultItem{
			RelativePath: sel.RelativePath,
			SkillName:    skillName,
			Status:       "failed",
			Error:        fmt.Sprintf("failed to create: %v", err),
		}
	}

	return CommitResultItem{
		RelativePath: sel.RelativePath,
		SkillName:    skillName,
		Status:       "success",
		ToolID:       tool.ID,
	}
}

// scanCandidates scans a ZIP for SKILL.md files
func (i *Importer) scanCandidates(zipData []byte) ([]Candidate, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip archive: %w", err)
	}

	commonRoot := i.findCommonRoot(zipReader)

	foundDirs := make(map[string]bool)
	var candidates []Candidate

	for _, f := range zipReader.File {
		if f.FileInfo().IsDir() {
			continue
		}

		// Check for SKILL.md
		if strings.ToLower(path.Base(f.Name)) != "skill.md" {
			continue
		}

		// Strip common root
		relPath := i.stripCommonRoot(f.Name, commonRoot)
		if relPath == "" {
			continue
		}

		// Get directory
		dir := path.Dir(relPath)
		if dir == "." {
			dir = "."
		}

		// Skip duplicates
		if foundDirs[dir] {
			continue
		}
		foundDirs[dir] = true

		// Read and parse SKILL.md content
		skillName, skillDescription := i.parseSkillMD(f)

		// Fallback to directory name if parsing failed
		var requiresName bool
		if skillName == "" {
			if dir != "." {
				skillName = path.Base(dir)
			} else {
				requiresName = true
			}
		}

		candidates = append(candidates, Candidate{
			RelativePath:     dir,
			SkillName:        skillName,
			SkillDescription: skillDescription,
			RequiresName:     requiresName,
			WillOverwrite:    false,
		})
	}

	return candidates, nil
}

// parseSkillMD parses name and description from a SKILL.md file
func (i *Importer) parseSkillMD(f *zip.File) (name, description string) {
	rc, err := f.Open()
	if err != nil {
		return "", ""
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return "", ""
	}

	text := string(content)

	// Try to parse YAML frontmatter: ---\nname: xxx\ndescription: xxx\n---
	if strings.HasPrefix(text, "---") {
		parts := strings.SplitN(text, "---", 3)
		if len(parts) >= 2 {
			frontmatter := parts[1]
			for _, line := range strings.Split(frontmatter, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				} else if strings.HasPrefix(line, "description:") {
					description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
				}
			}
		}
	}

	// Fallback: try to parse first heading as name
	if name == "" {
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				name = strings.TrimPrefix(line, "# ")
				break
			}
		}
	}

	// Fallback: use first non-empty, non-heading line as description
	if description == "" {
		lines := strings.Split(text, "\n")
		inFrontmatter := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "---" {
				inFrontmatter = !inFrontmatter
				continue
			}
			if inFrontmatter || line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			description = line
			if len(description) > 200 {
				description = description[:200] + "..."
			}
			break
		}
	}

	return name, description
}

// findCommonRoot finds the common root directory in a ZIP
func (i *Importer) findCommonRoot(zipReader *zip.Reader) string {
	if len(zipReader.File) == 0 {
		return ""
	}

	// Get first non-empty path
	var firstPath string
	for _, f := range zipReader.File {
		if f.Name != "" && !f.FileInfo().IsDir() {
			firstPath = f.Name
			break
		}
	}
	if firstPath == "" {
		return ""
	}

	// Split first path
	parts := strings.Split(firstPath, "/")
	if len(parts) <= 1 {
		return ""
	}

	// Check if all files share the same root
	commonRoot := parts[0]
	for _, f := range zipReader.File {
		if f.Name == "" || f.FileInfo().IsDir() {
			continue
		}
		if !strings.HasPrefix(f.Name, commonRoot+"/") {
			return ""
		}
	}

	return commonRoot
}

// stripCommonRoot removes the common root from a path
func (i *Importer) stripCommonRoot(filePath, commonRoot string) string {
	if commonRoot == "" {
		return filePath
	}
	return strings.TrimPrefix(filePath, commonRoot+"/")
}

// downloadGitHubZip downloads a repository as ZIP from GitHub
func (i *Importer) downloadGitHubZip(ctx context.Context, githubURL string) ([]byte, error) {
	parsed, err := url.Parse(githubURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Host != "github.com" && parsed.Host != "www.github.com" {
		return nil, fmt.Errorf("only github.com URLs are supported")
	}

	// Parse path: /owner/repo or /owner/repo/tree/branch
	pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(pathParts) < 2 {
		return nil, fmt.Errorf("invalid GitHub repository URL")
	}

	owner := pathParts[0]
	repo := strings.TrimSuffix(pathParts[1], ".git")

	// Determine branch
	branch := ""
	if len(pathParts) >= 4 && pathParts[2] == "tree" {
		branch = pathParts[3]
	}

	// Try branches
	branches := []string{branch, "main", "master"}
	var lastErr error

	for _, b := range branches {
		if b == "" {
			continue
		}
		downloadURL := fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", owner, repo, b)
		data, err := i.downloadWithLimit(ctx, downloadURL)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("failed to download from GitHub: %w", lastErr)
}

// downloadWithLimit downloads a URL with size limit
func (i *Importer) downloadWithLimit(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "tools-importer/1.0")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Limit to 100MB
	maxSize := int64(100 * 1024 * 1024)
	limitedReader := io.LimitReader(resp.Body, maxSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("file too large (max 100MB)")
	}

	return data, nil
}
