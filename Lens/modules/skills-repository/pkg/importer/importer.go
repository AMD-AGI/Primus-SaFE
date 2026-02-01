// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/registry"
)

// SkillImporter handles importing skills from various sources
type SkillImporter struct {
	registry   *registry.SkillsRegistry
	httpClient *http.Client
	githubToken string
}

// NewSkillImporter creates a new SkillImporter
func NewSkillImporter(reg *registry.SkillsRegistry, githubToken string) *SkillImporter {
	return &SkillImporter{
		registry:    reg,
		httpClient:  &http.Client{},
		githubToken: githubToken,
	}
}

// ImportResult represents the result of an import operation
type ImportResult struct {
	Imported []string `json:"imported"`
	Skipped  []string `json:"skipped"`
	Errors   []string `json:"errors"`
}

// ImportFromGitHub imports skills from a GitHub repository
// Supports URLs like:
// - https://github.com/owner/repo (imports all SKILL.md files)
// - https://github.com/owner/repo/tree/branch/path (imports from specific path)
// - https://github.com/owner/repo/blob/branch/path/SKILL.md (imports single file)
func (i *SkillImporter) ImportFromGitHub(ctx context.Context, url string) (*ImportResult, error) {
	result := &ImportResult{
		Imported: make([]string, 0),
		Skipped:  make([]string, 0),
		Errors:   make([]string, 0),
	}

	// Parse GitHub URL
	owner, repo, branch, path, err := parseGitHubURL(url)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub URL: %w", err)
	}

	log.Infof("Importing skills from GitHub: %s/%s (branch: %s, path: %s)", owner, repo, branch, path)

	// If path points to a specific file
	if strings.HasSuffix(strings.ToLower(path), ".md") {
		skill, err := i.importSingleFile(ctx, owner, repo, branch, path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, err))
		} else {
			result.Imported = append(result.Imported, skill.Name)
		}
		return result, nil
	}

	// List files in the path and find SKILL.md files
	files, err := i.listGitHubFiles(ctx, owner, repo, branch, path)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	for _, file := range files {
		if strings.ToUpper(filepath.Base(file)) == "SKILL.MD" {
			skill, err := i.importSingleFile(ctx, owner, repo, branch, file)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file, err))
				continue
			}
			result.Imported = append(result.Imported, skill.Name)
		}
	}

	return result, nil
}

// ImportFromFile imports skills from uploaded file content
// Supports: single SKILL.md file, or ZIP archive containing multiple skills
func (i *SkillImporter) ImportFromFile(ctx context.Context, filename string, content []byte) (*ImportResult, error) {
	result := &ImportResult{
		Imported: make([]string, 0),
		Skipped:  make([]string, 0),
		Errors:   make([]string, 0),
	}

	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".zip":
		return i.importFromZip(ctx, content)
	case ".md":
		skill, err := i.parseAndRegisterSkill(ctx, filename, string(content), "upload")
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", filename, err))
		} else {
			result.Imported = append(result.Imported, skill.Name)
		}
	default:
		return nil, fmt.Errorf("unsupported file type: %s", ext)
	}

	return result, nil
}

// importFromZip imports skills from a ZIP archive
func (i *SkillImporter) importFromZip(ctx context.Context, content []byte) (*ImportResult, error) {
	result := &ImportResult{
		Imported: make([]string, 0),
		Skipped:  make([]string, 0),
		Errors:   make([]string, 0),
	}

	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("failed to read ZIP file: %w", err)
	}

	for _, file := range reader.File {
		// Skip directories
		if file.FileInfo().IsDir() {
			continue
		}

		// Only process SKILL.md files
		if strings.ToUpper(filepath.Base(file.Name)) != "SKILL.MD" {
			continue
		}

		// Read file content
		rc, err := file.Open()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file.Name, err))
			continue
		}

		fileContent, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file.Name, err))
			continue
		}

		// Parse and register skill
		skill, err := i.parseAndRegisterSkill(ctx, file.Name, string(fileContent), "upload")
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file.Name, err))
			continue
		}

		result.Imported = append(result.Imported, skill.Name)
	}

	return result, nil
}

// importSingleFile imports a single file from GitHub
func (i *SkillImporter) importSingleFile(ctx context.Context, owner, repo, branch, path string) (*model.Skill, error) {
	content, err := i.fetchGitHubFile(ctx, owner, repo, branch, path)
	if err != nil {
		return nil, err
	}

	return i.parseAndRegisterSkill(ctx, path, content, fmt.Sprintf("github:%s/%s", owner, repo))
}

// parseAndRegisterSkill parses SKILL.md content and registers the skill
func (i *SkillImporter) parseAndRegisterSkill(ctx context.Context, filepath, content, source string) (*model.Skill, error) {
	skill := parseSkillMD(content)
	
	// Use directory name as skill name if not found in content
	if skill.Name == "" {
		dir := strings.TrimSuffix(filepath, "/SKILL.md")
		dir = strings.TrimSuffix(dir, "/SKILL.MD")
		skill.Name = sanitizeSkillName(strings.TrimPrefix(dir, "/"))
	}

	if skill.Name == "" {
		return nil, fmt.Errorf("could not determine skill name from file: %s", filepath)
	}

	skill.Source = source
	skill.FilePath = filepath

	// Register the skill
	if err := i.registry.Register(ctx, skill); err != nil {
		return nil, err
	}

	return skill, nil
}

// parseSkillMD parses SKILL.md content and extracts metadata
func parseSkillMD(content string) *model.Skill {
	skill := &model.Skill{
		Content: content,
	}

	lines := strings.Split(content, "\n")

	// Extract title from first H1
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			skill.Name = sanitizeSkillName(strings.TrimPrefix(line, "# "))
			break
		}
	}

	// Extract description from content after title
	var descLines []string
	foundTitle := false
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			foundTitle = true
			continue
		}
		if foundTitle {
			if strings.HasPrefix(line, "## ") {
				break
			}
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				descLines = append(descLines, trimmed)
			}
		}
	}
	if len(descLines) > 0 {
		skill.Description = strings.Join(descLines, " ")
		if len(skill.Description) > 500 {
			skill.Description = skill.Description[:497] + "..."
		}
	}

	// Extract metadata from YAML front matter if present
	if strings.HasPrefix(content, "---") {
		endIdx := strings.Index(content[3:], "---")
		if endIdx > 0 {
			frontMatter := content[3 : 3+endIdx]
			metadata := parseYAMLFrontMatter(frontMatter)
			
			if name, ok := metadata["name"].(string); ok && skill.Name == "" {
				skill.Name = name
			}
			if desc, ok := metadata["description"].(string); ok && skill.Description == "" {
				skill.Description = desc
			}
			if cat, ok := metadata["category"].(string); ok {
				skill.Category = cat
			}
			if ver, ok := metadata["version"].(string); ok {
				skill.Version = ver
			}
			if lic, ok := metadata["license"].(string); ok {
				skill.License = lic
			}
		}
	}

	return skill
}

// parseYAMLFrontMatter parses simple YAML front matter
func parseYAMLFrontMatter(content string) map[string]interface{} {
	result := make(map[string]interface{})
	lines := strings.Split(content, "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Remove quotes
			value = strings.Trim(value, "\"'")
			result[key] = value
		}
	}
	
	return result
}

// sanitizeSkillName converts a path or title to a valid skill name
func sanitizeSkillName(name string) string {
	// Get the last path component
	name = filepath.Base(name)
	
	// Convert to lowercase
	name = strings.ToLower(name)
	
	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	
	// Remove special characters except hyphens and underscores
	reg := regexp.MustCompile(`[^a-z0-9\-_]`)
	name = reg.ReplaceAllString(name, "")
	
	// Remove multiple consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	name = reg.ReplaceAllString(name, "-")
	
	// Trim hyphens from start and end
	name = strings.Trim(name, "-")
	
	return name
}

// parseGitHubURL parses a GitHub URL and returns owner, repo, branch, and path
func parseGitHubURL(url string) (owner, repo, branch, path string, err error) {
	// Remove trailing slash
	url = strings.TrimSuffix(url, "/")
	
	// Pattern: https://github.com/owner/repo[/tree|blob/branch/path]
	patterns := []string{
		`^https?://github\.com/([^/]+)/([^/]+)/(?:tree|blob)/([^/]+)/(.+)$`,
		`^https?://github\.com/([^/]+)/([^/]+)/(?:tree|blob)/([^/]+)$`,
		`^https?://github\.com/([^/]+)/([^/]+)$`,
	}

	for i, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(url)
		if matches != nil {
			owner = matches[1]
			repo = matches[2]
			if i < 2 && len(matches) > 3 {
				branch = matches[3]
			}
			if i == 0 && len(matches) > 4 {
				path = matches[4]
			}
			break
		}
	}

	if owner == "" || repo == "" {
		return "", "", "", "", fmt.Errorf("could not parse GitHub URL: %s", url)
	}

	// Default branch
	if branch == "" {
		branch = "main"
	}

	return owner, repo, branch, path, nil
}

// fetchGitHubFile fetches a file from GitHub
func (i *SkillImporter) fetchGitHubFile(ctx context.Context, owner, repo, branch, path string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, branch)
	
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if i.githubToken != "" {
		req.Header.Set("Authorization", "token "+i.githubToken)
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var result struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(result.Content, "\n", ""))
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}

	return result.Content, nil
}

// listGitHubFiles lists files in a GitHub repository path recursively
func (i *SkillImporter) listGitHubFiles(ctx context.Context, owner, repo, branch, path string) ([]string, error) {
	var files []string

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, branch)
	
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if i.githubToken != "" {
		req.Header.Set("Authorization", "token "+i.githubToken)
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var items []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	for _, item := range items {
		if item.Type == "dir" {
			// Recursively list directory
			subFiles, err := i.listGitHubFiles(ctx, owner, repo, branch, item.Path)
			if err != nil {
				log.Warnf("Failed to list directory %s: %v", item.Path, err)
				continue
			}
			files = append(files, subFiles...)
		} else if item.Type == "file" {
			files = append(files, item.Path)
		}
	}

	return files, nil
}
