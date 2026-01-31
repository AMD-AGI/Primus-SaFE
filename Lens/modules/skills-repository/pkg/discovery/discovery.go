// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package discovery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/registry"
	"gopkg.in/yaml.v3"
)

// SkillsDiscovery handles skill discovery from multiple sources
type SkillsDiscovery struct {
	config   config.DiscoveryConfig
	registry *registry.SkillsRegistry
	stopCh   chan struct{}
}

// NewSkillsDiscovery creates a new SkillsDiscovery
func NewSkillsDiscovery(cfg config.DiscoveryConfig, reg *registry.SkillsRegistry) (*SkillsDiscovery, error) {
	return &SkillsDiscovery{
		config:   cfg,
		registry: reg,
		stopCh:   make(chan struct{}),
	}, nil
}

// Start begins skill discovery
func (d *SkillsDiscovery) Start(ctx context.Context) error {
	// Initial load from all sources
	for _, source := range d.config.Sources {
		if err := d.loadFromSource(ctx, source); err != nil {
			log.Warnf("Failed to load skills from %s: %v", source.Name, err)
		}
	}

	// Start periodic sync
	if d.config.SyncInterval != "" {
		interval, err := time.ParseDuration(d.config.SyncInterval)
		if err != nil {
			interval = 5 * time.Minute
		}
		go d.periodicSync(ctx, interval)
	}

	return nil
}

// Stop stops skill discovery
func (d *SkillsDiscovery) Stop() {
	close(d.stopCh)
}

func (d *SkillsDiscovery) loadFromSource(ctx context.Context, source config.SourceConfig) error {
	switch source.Type {
	case "local":
		return d.loadFromLocal(ctx, source)
	case "git":
		return d.loadFromGit(ctx, source)
	default:
		log.Warnf("Unknown source type: %s", source.Type)
		return nil
	}
}

func (d *SkillsDiscovery) loadFromLocal(ctx context.Context, source config.SourceConfig) error {
	return filepath.Walk(source.URL, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Name() == "SKILL.md" {
			skill, err := d.loadSkillFromFile(path, source.Name)
			if err != nil {
				log.Warnf("Failed to load skill from %s: %v", path, err)
				return nil
			}
			if err := d.registry.Register(ctx, skill); err != nil {
				log.Warnf("Failed to register skill %s: %v", skill.Name, err)
			}
		}

		return nil
	})
}

func (d *SkillsDiscovery) loadFromGit(ctx context.Context, source config.SourceConfig) error {
	// TODO: Implement git sync
	// For now, assume git repo is already cloned to a local path
	log.Infof("Git source %s: %s (not implemented, use local path)", source.Name, source.URL)
	return nil
}

func (d *SkillsDiscovery) loadSkillFromFile(path string, sourceName string) (*model.Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	skill, err := parseSkillMd(string(content))
	if err != nil {
		return nil, err
	}

	// Set runtime fields
	skill.FilePath = path
	skill.Source = sourceName
	skill.Category = extractCategory(path)
	skill.Content = string(content)

	return skill, nil
}

// parseSkillMd parses SKILL.md content with YAML frontmatter
func parseSkillMd(content string) (*model.Skill, error) {
	// Split frontmatter and body
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		// No frontmatter, create skill with content as description
		lines := strings.SplitN(content, "\n", 2)
		name := strings.TrimPrefix(strings.TrimSpace(lines[0]), "# ")
		return &model.Skill{
			Name:        name,
			Description: name,
		}, nil
	}

	// Parse YAML frontmatter
	var frontmatter struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		License     string `yaml:"license"`
	}

	if err := yaml.Unmarshal([]byte(parts[1]), &frontmatter); err != nil {
		return nil, err
	}

	return &model.Skill{
		Name:        frontmatter.Name,
		Description: frontmatter.Description,
		License:     frontmatter.License,
	}, nil
}

// extractCategory extracts category from file path
func extractCategory(path string) string {
	// Example: /skills/k8s/oom-diagnose/SKILL.md -> k8s
	dir := filepath.Dir(path)
	parts := strings.Split(dir, string(os.PathSeparator))

	// Find "skills" in path and return next segment
	for i, part := range parts {
		if part == "skills" && i+1 < len(parts) {
			return parts[i+1]
		}
	}

	// Return parent directory name
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return ""
}

func (d *SkillsDiscovery) periodicSync(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			for _, source := range d.config.Sources {
				if err := d.loadFromSource(ctx, source); err != nil {
					log.Warnf("Failed to sync skills from %s: %v", source.Name, err)
				}
			}
		}
	}
}
