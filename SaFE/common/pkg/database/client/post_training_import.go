/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TPostTrainingImport      = "post_training_import"
	TPostTrainingImportToken = "post_training_import_token"

	// PostTrainingSourceSafe and PostTrainingSourceSpur are the sources of a run: trained on the platform, or
	// imported from a Spur run directory.
	PostTrainingSourceSafe = "safe"
	PostTrainingSourceSpur = "spur"

	// PostTrainingImportStaged is a scanned directory waiting to be committed; PostTrainingImportImported is an
	// import that wrote its run; PostTrainingImportCommitted is a staged record replaced by its import.
	PostTrainingImportStaged    = "staged"
	PostTrainingImportImported  = "imported"
	PostTrainingImportCommitted = "committed"
)

var (
	// ErrPostTrainingImportExists means the workspace has already imported the directory.
	ErrPostTrainingImportExists = errors.New("the directory is already imported in this workspace")
	// ErrPostTrainingImportTokenSpent means the import token is unknown, expired or already used.
	ErrPostTrainingImportTokenSpent = errors.New("the import token is invalid, expired or already used")
)

// PostTrainingImport records a Spur run directory parsed into a Post Training run: staged while a scan waits to be
// committed, imported once its run exists. Summary holds the parsed data the run row and metric points do not.
type PostTrainingImport struct {
	ID         int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id" db:"id"`
	ImportID   string `gorm:"column:import_id" json:"importId" db:"import_id"`
	RunID      string `gorm:"column:run_id" json:"runId,omitempty" db:"run_id"`
	Workspace  string `gorm:"column:workspace" json:"workspace" db:"workspace"`
	SourcePath string `gorm:"column:source_path" json:"sourcePath" db:"source_path"`
	Format     string `gorm:"column:format;default:slime-rocm" json:"format" db:"format"`
	// PathKey is the SHA-256 of the workspace and the normalized path, unique among imports; empty for staged and
	// deleted records, which a NULL lets the unique index hold any number of.
	PathKey        sql.NullString `gorm:"column:path_key" json:"-" db:"path_key"`
	SlurmJobID     string         `gorm:"column:slurm_job_id" json:"slurmJobId,omitempty" db:"slurm_job_id"`
	Via            string         `gorm:"column:via" json:"via" db:"via"`
	State          string         `gorm:"column:state" json:"state" db:"state"`
	Verdict        string         `gorm:"column:verdict" json:"verdict,omitempty" db:"verdict"`
	Summary        string         `gorm:"column:summary" json:"summary,omitempty" db:"summary"`
	Resources      string         `gorm:"column:resources" json:"resources,omitempty" db:"resources"`
	Warnings       string         `gorm:"column:warnings" json:"warnings,omitempty" db:"warnings"`
	ParserVersion  string         `gorm:"column:parser_version" json:"parserVersion,omitempty" db:"parser_version"`
	ScanWorkloadID string         `gorm:"column:scan_workload_id" json:"scanWorkloadId,omitempty" db:"scan_workload_id"`
	Message        string         `gorm:"column:message" json:"message,omitempty" db:"message"`
	UserID         string         `gorm:"column:user_id" json:"userId" db:"user_id"`
	UserName       string         `gorm:"column:user_name" json:"userName" db:"user_name"`
	CreatedAt      pq.NullTime    `gorm:"column:created_at;autoCreateTime" json:"createdAt" db:"created_at"`
	UpdatedAt      pq.NullTime    `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt" db:"updated_at"`
	IsDeleted      bool           `gorm:"column:is_deleted" json:"-" db:"is_deleted"`
}

// TableName sets the SQL table name for PostTrainingImport.
func (PostTrainingImport) TableName() string {
	return TPostTrainingImport
}

// PostTrainingImportToken is a one-time credential to upload one import: a user's command upload, or the
// internal token of a scan. Only the SHA-256 of the token is stored.
type PostTrainingImportToken struct {
	ID        int64        `gorm:"column:id;primaryKey;autoIncrement" json:"id" db:"id"`
	TokenID   string       `gorm:"column:token_id" json:"tokenId" db:"token_id"`
	TokenHash string       `gorm:"column:token_hash" json:"-" db:"token_hash"`
	Workspace string       `gorm:"column:workspace" json:"workspace" db:"workspace"`
	Purpose   string       `gorm:"column:purpose" json:"purpose" db:"purpose"`
	UserID    string       `gorm:"column:user_id" json:"userId" db:"user_id"`
	UserName  string       `gorm:"column:user_name" json:"userName" db:"user_name"`
	ExpiresAt time.Time    `gorm:"column:expires_at" json:"expiresAt" db:"expires_at"`
	UsedAt    sql.NullTime `gorm:"column:used_at" json:"usedAt" db:"used_at"`
	ImportID  string       `gorm:"column:import_id" json:"importId,omitempty" db:"import_id"`
	LastError string       `gorm:"column:last_error" json:"lastError,omitempty" db:"last_error"`
	CreatedAt pq.NullTime  `gorm:"column:created_at;autoCreateTime" json:"createdAt" db:"created_at"`
}

// TableName sets the SQL table name for PostTrainingImportToken.
func (PostTrainingImportToken) TableName() string {
	return TPostTrainingImportToken
}

// PostTrainingImportWrite is what SavePostTrainingImport writes in one transaction.
type PostTrainingImportWrite struct {
	Import *PostTrainingImport
	// Run and Points are the imported run and its metric points; without a run the import is only staged.
	Run    *PostTrainingRun
	Points []*PostTrainingRunMetric
	// TokenID is the token the upload spends; StagedID is the staged import a commit replaces.
	TokenID  string
	StagedID string
}

// CreatePostTrainingImportToken inserts a token; token_id and token_hash must be unique.
func (c *Client) CreatePostTrainingImportToken(ctx context.Context, t *PostTrainingImportToken) error {
	if t == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Create(t).Error
}

// GetPostTrainingImportToken fetches a token by id. It returns gorm.ErrRecordNotFound when there is none.
func (c *Client) GetPostTrainingImportToken(ctx context.Context, tokenID string) (*PostTrainingImportToken, error) {
	return c.getPostTrainingImportToken(ctx, "token_id = ?", tokenID)
}

// GetPostTrainingImportTokenByHash fetches the token whose SHA-256 is hash. It returns gorm.ErrRecordNotFound when
// there is none.
func (c *Client) GetPostTrainingImportTokenByHash(ctx context.Context, hash string) (*PostTrainingImportToken, error) {
	return c.getPostTrainingImportToken(ctx, "token_hash = ?", hash)
}

// getPostTrainingImportToken fetches the token matching one condition.
func (c *Client) getPostTrainingImportToken(ctx context.Context, cond string, arg string) (*PostTrainingImportToken, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var t PostTrainingImportToken
	if err = db.WithContext(ctx).Where(cond, arg).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// SetPostTrainingImportTokenError records why an upload with the token was refused; the token stays usable.
func (c *Client) SetPostTrainingImportTokenError(ctx context.Context, tokenID, message string) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Model(&PostTrainingImportToken{}).Where("token_id = ?", tokenID).
		Update("last_error", message).Error
}

// SavePostTrainingImport writes an import in one transaction. With a run it inserts the import record, the run and
// its metric points; the record's path key must be new, or ErrPostTrainingImportExists is returned. Without a run
// the record is only staged. A token is spent for the import, and must be unused and unexpired, or
// ErrPostTrainingImportTokenSpent is returned; a staged record is marked committed. Nothing is written on an error.
func (c *Client) SavePostTrainingImport(ctx context.Context, w PostTrainingImportWrite) error {
	if w.Import == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "path_key"}}, DoNothing: true}).
			Create(w.Import)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrPostTrainingImportExists
		}
		if w.Run != nil {
			if err := tx.Create(w.Run).Error; err != nil {
				return err
			}
			if points := dedupePostTrainingRunMetrics(w.Run.RunID, w.Points); len(points) > 0 {
				if err := tx.CreateInBatches(points, postTrainingMetricBatchSize).Error; err != nil {
					return err
				}
			}
		}
		if w.TokenID != "" {
			res := tx.Model(&PostTrainingImportToken{}).
				Where("token_id = ? AND used_at IS NULL AND expires_at > ?", w.TokenID, time.Now().UTC()).
				Updates(map[string]interface{}{"used_at": time.Now().UTC(), "import_id": w.Import.ImportID,
					"last_error": ""})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrPostTrainingImportTokenSpent
			}
		}
		if w.StagedID != "" {
			return tx.Model(&PostTrainingImport{}).
				Where("import_id = ? AND state = ?", w.StagedID, PostTrainingImportStaged).
				Updates(map[string]interface{}{"state": PostTrainingImportCommitted, "run_id": w.Import.RunID,
					"updated_at": time.Now().UTC()}).Error
		}
		return nil
	})
}

// GetPostTrainingImport fetches a non-deleted import by id. It returns gorm.ErrRecordNotFound when there is none.
func (c *Client) GetPostTrainingImport(ctx context.Context, importID string) (*PostTrainingImport, error) {
	return c.getPostTrainingImport(ctx, "import_id = ?", importID)
}

// GetPostTrainingImportByRun fetches the import that wrote a run. It returns gorm.ErrRecordNotFound when there is
// none.
func (c *Client) GetPostTrainingImportByRun(ctx context.Context, runID string) (*PostTrainingImport, error) {
	return c.getPostTrainingImport(ctx, "run_id = ? AND state = ?", runID, PostTrainingImportImported)
}

// GetPostTrainingImportByPathKey fetches the import of a directory in a workspace. It returns
// gorm.ErrRecordNotFound when there is none.
func (c *Client) GetPostTrainingImportByPathKey(ctx context.Context, pathKey string) (*PostTrainingImport, error) {
	return c.getPostTrainingImport(ctx, "path_key = ?", pathKey)
}

// getPostTrainingImport fetches the non-deleted import matching a condition.
func (c *Client) getPostTrainingImport(ctx context.Context, cond string, args ...interface{}) (*PostTrainingImport, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var imp PostTrainingImport
	if err = db.WithContext(ctx).Where(cond, args...).Where("is_deleted = ?", false).First(&imp).Error; err != nil {
		return nil, err
	}
	return &imp, nil
}

// DeletePostTrainingImportOfRun soft-deletes the import of a run and clears its path key, so the directory can be
// imported again.
func (c *Client) DeletePostTrainingImportOfRun(ctx context.Context, runID string) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Model(&PostTrainingImport{}).Where("run_id = ? AND is_deleted = ?", runID, false).
		Updates(map[string]interface{}{"is_deleted": true, "path_key": nil, "updated_at": time.Now().UTC()}).Error
}
