/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

type CollectionConfig struct {
	ID               int64    `db:"id" json:"id"`
	Name             string   `db:"name" json:"name"`
	GithubOwner      string   `db:"github_owner" json:"github_owner"`
	GithubRepo       string   `db:"github_repo" json:"github_repo"`
	WorkflowPatterns []string `json:"workflow_patterns"`
	BranchPatterns   []string `json:"branch_patterns"`
	FilePatterns     []string `json:"file_patterns"`
	Enabled          bool     `db:"enabled" json:"enabled"`
	CreatedBy        string   `db:"created_by" json:"created_by"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at"`
}

// MatchesRun checks if a workflow run matches this collection config.
func (c *CollectionConfig) MatchesRun(owner, repo, workflow, branch string) bool {
	if !c.Enabled {
		return false
	}
	if !strings.EqualFold(c.GithubOwner, owner) || !strings.EqualFold(c.GithubRepo, repo) {
		return false
	}
	if len(c.WorkflowPatterns) > 0 {
		matched := false
		for _, p := range c.WorkflowPatterns {
			if m, _ := filepath.Match(p, workflow); m {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(c.BranchPatterns) > 0 {
		matched := false
		for _, p := range c.BranchPatterns {
			if m, _ := filepath.Match(p, branch); m {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// CollectionConfigStore provides CRUD for github_collection_configs.
type CollectionConfigStore struct {
	db *sql.DB
}

func NewCollectionConfigStore(db *sql.DB) *CollectionConfigStore {
	return &CollectionConfigStore{db: db}
}

func (s *CollectionConfigStore) List(ctx context.Context) ([]CollectionConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, github_owner, github_repo, workflow_patterns, branch_patterns,
		       file_patterns, enabled, COALESCE(created_by, ''), created_at, updated_at
		FROM github_collection_configs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []CollectionConfig
	for rows.Next() {
		var c CollectionConfig
		var wp, bp, fp string
		rows.Scan(&c.ID, &c.Name, &c.GithubOwner, &c.GithubRepo,
			&wp, &bp, &fp, &c.Enabled, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
		c.WorkflowPatterns = pgArrayToSlice(wp)
		c.BranchPatterns = pgArrayToSlice(bp)
		c.FilePatterns = pgArrayToSlice(fp)
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

func (s *CollectionConfigStore) FindMatchingConfig(ctx context.Context, owner, repo, workflow, branch string) *CollectionConfig {
	configs, err := s.List(ctx)
	if err != nil {
		klog.V(1).Infof("[collection-config] list: %v", err)
		return nil
	}
	for _, c := range configs {
		if c.MatchesRun(owner, repo, workflow, branch) {
			return &c
		}
	}
	return nil
}

func (s *CollectionConfigStore) Create(ctx context.Context, c *CollectionConfig) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_collection_configs
			(name, github_owner, github_repo, workflow_patterns, branch_patterns, file_patterns, enabled, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		c.Name, c.GithubOwner, c.GithubRepo,
		sliceToPgArray(c.WorkflowPatterns),
		sliceToPgArray(c.BranchPatterns),
		sliceToPgArray(c.FilePatterns),
		c.Enabled, c.CreatedBy)
	return err
}

func (s *CollectionConfigStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_collection_configs WHERE id = $1`, id)
	return err
}

func pgArrayToSlice(s string) []string {
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "\"")
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func sliceToPgArray(s []string) string {
	if len(s) == 0 {
		return "{}"
	}
	return "{" + strings.Join(s, ",") + "}"
}
