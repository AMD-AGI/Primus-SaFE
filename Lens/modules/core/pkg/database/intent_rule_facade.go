// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// IntentRuleFacadeInterface defines the database operation interface for intent rules
type IntentRuleFacadeInterface interface {
	// CreateRule creates a new intent rule
	CreateRule(ctx context.Context, rule *model.IntentRule) error

	// GetRule retrieves a rule by ID
	GetRule(ctx context.Context, id int64) (*model.IntentRule, error)

	// UpdateRule updates an existing rule
	UpdateRule(ctx context.Context, rule *model.IntentRule) error

	// DeleteRule deletes a rule by ID
	DeleteRule(ctx context.Context, id int64) error

	// ListByStatus lists rules by lifecycle status
	ListByStatus(ctx context.Context, status string) ([]*model.IntentRule, error)

	// GetPromotedRules returns all promoted rules for the confidence router
	GetPromotedRules(ctx context.Context) ([]*model.IntentRule, error)

	// GetByDetectsField lists rules by detection target field
	GetByDetectsField(ctx context.Context, detectsField string) ([]*model.IntentRule, error)

	// ListByDimension lists rules by matching dimension (image, cmdline, env_key, pip, etc.)
	ListByDimension(ctx context.Context, dimension string) ([]*model.IntentRule, error)

	// ListAll lists all rules ordered by updated_at desc
	ListAll(ctx context.Context) ([]*model.IntentRule, error)

	// UpdateStatus updates the lifecycle status of a rule
	UpdateStatus(ctx context.Context, id int64, status string) error

	// UpdateBacktestResult stores the backtest metrics for a rule
	UpdateBacktestResult(ctx context.Context, id int64, result map[string]interface{}) error

	// IncrementMatchCount increments the production match counter
	IncrementMatchCount(ctx context.Context, id int64) error

	// IncrementCorrectCount increments the audit-confirmed correct counter
	IncrementCorrectCount(ctx context.Context, id int64) error

	// IncrementFalsePositiveCount increments the audit-found false positive counter
	IncrementFalsePositiveCount(ctx context.Context, id int64) error

	// ExistsByPatternAndValue checks if a rule with the same pattern+detects_value already exists
	ExistsByPatternAndValue(ctx context.Context, pattern string, detectsValue string) (bool, error)

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) IntentRuleFacadeInterface
}

// IntentRuleFacade implements IntentRuleFacadeInterface
type IntentRuleFacade struct {
	BaseFacade
}

// NewIntentRuleFacade creates a new IntentRuleFacade instance
func NewIntentRuleFacade() IntentRuleFacadeInterface {
	return &IntentRuleFacade{}
}

func (f *IntentRuleFacade) WithCluster(clusterName string) IntentRuleFacadeInterface {
	return &IntentRuleFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

func (f *IntentRuleFacade) CreateRule(ctx context.Context, rule *model.IntentRule) error {
	now := time.Now()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now
	if rule.Status == "" {
		rule.Status = "proposed"
	}
	return f.getDB().WithContext(ctx).Create(rule).Error
}

func (f *IntentRuleFacade) GetRule(ctx context.Context, id int64) (*model.IntentRule, error) {
	var result model.IntentRule
	err := f.getDB().WithContext(ctx).Where("id = ?", id).First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func (f *IntentRuleFacade) UpdateRule(ctx context.Context, rule *model.IntentRule) error {
	rule.UpdatedAt = time.Now()
	return f.getDB().WithContext(ctx).Save(rule).Error
}

func (f *IntentRuleFacade) DeleteRule(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).
		Where("id = ?", id).
		Delete(&model.IntentRule{}).Error
}

func (f *IntentRuleFacade) ListByStatus(ctx context.Context, status string) ([]*model.IntentRule, error) {
	var results []*model.IntentRule
	err := f.getDB().WithContext(ctx).
		Where("status = ?", status).
		Order("updated_at DESC").
		Find(&results).Error
	return results, err
}

func (f *IntentRuleFacade) GetPromotedRules(ctx context.Context) ([]*model.IntentRule, error) {
	return f.ListByStatus(ctx, "promoted")
}

func (f *IntentRuleFacade) GetByDetectsField(ctx context.Context, detectsField string) ([]*model.IntentRule, error) {
	var results []*model.IntentRule
	err := f.getDB().WithContext(ctx).
		Where("detects_field = ?", detectsField).
		Order("confidence DESC").
		Find(&results).Error
	return results, err
}

func (f *IntentRuleFacade) ListByDimension(ctx context.Context, dimension string) ([]*model.IntentRule, error) {
	var results []*model.IntentRule
	err := f.getDB().WithContext(ctx).
		Where("dimension = ?", dimension).
		Order("updated_at DESC").
		Find(&results).Error
	return results, err
}

func (f *IntentRuleFacade) ListAll(ctx context.Context) ([]*model.IntentRule, error) {
	var results []*model.IntentRule
	err := f.getDB().WithContext(ctx).
		Order("updated_at DESC").
		Find(&results).Error
	return results, err
}

func (f *IntentRuleFacade) UpdateStatus(ctx context.Context, id int64, status string) error {
	return f.getDB().WithContext(ctx).
		Table(model.TableNameIntentRule).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

func (f *IntentRuleFacade) UpdateBacktestResult(ctx context.Context, id int64, result map[string]interface{}) error {
	return f.getDB().WithContext(ctx).
		Table(model.TableNameIntentRule).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"backtest_result":    result,
			"last_backtested_at": time.Now(),
			"updated_at":         time.Now(),
		}).Error
}

func (f *IntentRuleFacade) IncrementMatchCount(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).
		Table(model.TableNameIntentRule).
		Where("id = ?", id).
		UpdateColumn("match_count", gorm.Expr("match_count + 1")).Error
}

func (f *IntentRuleFacade) IncrementCorrectCount(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).
		Table(model.TableNameIntentRule).
		Where("id = ?", id).
		UpdateColumn("correct_count", gorm.Expr("correct_count + 1")).Error
}

func (f *IntentRuleFacade) IncrementFalsePositiveCount(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).
		Table(model.TableNameIntentRule).
		Where("id = ?", id).
		UpdateColumn("false_positive_count", gorm.Expr("false_positive_count + 1")).Error
}

func (f *IntentRuleFacade) ExistsByPatternAndValue(ctx context.Context, pattern string, detectsValue string) (bool, error) {
	var count int64
	err := f.getDB().WithContext(ctx).
		Table(model.TableNameIntentRule).
		Where("pattern = ? AND detects_value = ? AND status != ?", pattern, detectsValue, "rejected").
		Count(&count).Error
	return count > 0, err
}
