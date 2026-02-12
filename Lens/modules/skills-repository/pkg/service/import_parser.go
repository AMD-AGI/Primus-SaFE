// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// skillFrontmatter represents the YAML frontmatter structure in SKILL.md
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// scanCandidates scans a ZIP for SKILL.md files and returns discovered candidates
func scanCandidates(zipData []byte) ([]DiscoverCandidate, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip archive: %w", err)
	}

	commonRoot := findCommonRoot(zipReader)

	foundDirs := make(map[string]bool)
	var candidates []DiscoverCandidate

	for _, f := range zipReader.File {
		if f.FileInfo().IsDir() {
			continue
		}

		// Check for SKILL.md
		if strings.ToLower(path.Base(f.Name)) != "skill.md" {
			continue
		}

		// Strip common root
		relPath := stripCommonRoot(f.Name, commonRoot)
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
		skillName, skillDescription := parseSkillMD(f)

		// Fallback to directory name if parsing failed
		var requiresName bool
		if skillName == "" {
			if dir != "." {
				skillName = path.Base(dir)
			} else {
				requiresName = true
			}
		}

		candidates = append(candidates, DiscoverCandidate{
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
func parseSkillMD(f *zip.File) (name, description string) {
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

	// Strip UTF-8 BOM if present
	text = strings.TrimPrefix(text, "\xEF\xBB\xBF")
	// Strip leading whitespace/newlines before frontmatter
	text = strings.TrimLeft(text, " \t\r\n")

	// Try to parse YAML frontmatter: ---\nname: xxx\ndescription: xxx\n---
	if strings.HasPrefix(text, "---") {
		parts := strings.SplitN(text, "---", 3)
		if len(parts) >= 2 {
			frontmatter := parts[1]
			var fm skillFrontmatter
			if err := yaml.Unmarshal([]byte(frontmatter), &fm); err == nil {
				name = strings.TrimSpace(fm.Name)
				description = strings.TrimSpace(fm.Description)
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

// isMDFile checks if the filename is a markdown file
func isMDFile(filename string) bool {
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

// isValidSkillMD performs basic validation on skill MD content
func isValidSkillMD(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	return true
}

// wrapMDInZip wraps a single MD file into a ZIP archive
func wrapMDInZip(mdContent []byte, filename string) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

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

// findCommonRoot finds the common root directory in a ZIP
func findCommonRoot(zipReader *zip.Reader) string {
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
func stripCommonRoot(filePath, commonRoot string) string {
	if commonRoot == "" {
		return filePath
	}
	return strings.TrimPrefix(filePath, commonRoot+"/")
}
