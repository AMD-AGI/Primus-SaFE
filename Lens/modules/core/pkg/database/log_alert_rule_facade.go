package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
)

// LogAlertRuleFacadeInterface defines the interface for log alert rule operations
type LogAlertRuleFacadeInterface interface {
	// CRUD Operations
	CreateLogAlertRule(ctx context.Context, rule *model.LogAlertRules) error
	GetLogAlertRuleByID(ctx context.Context, id int64) (*model.LogAlertRules, error)
	GetLogAlertRuleByName(ctx context.Context, clusterName, name string) (*model.LogAlertRules, error)
	ListLogAlertRules(ctx context.Context, filter *LogAlertRuleFilter) ([]*model.LogAlertRules, int64, error)
	UpdateLogAlertRule(ctx context.Context, rule *model.LogAlertRules) error
	DeleteLogAlertRule(ctx context.Context, id int64) error

	// Batch Operations
	BatchUpdateEnabledStatus(ctx context.Context, ids []int64, enabled bool) error
	BatchDeleteLogAlertRules(ctx context.Context, ids []int64) error

	// Rule Version Operations
	CreateRuleVersion(ctx context.Context, version *model.LogAlertRuleVersions) error
	ListRuleVersions(ctx context.Context, ruleID int64) ([]*model.LogAlertRuleVersions, error)
	GetRuleVersion(ctx context.Context, ruleID int64, version int) (*model.LogAlertRuleVersions, error)

	// Statistics Operations
	CreateOrUpdateRuleStatistic(ctx context.Context, stat *model.LogAlertRuleStatistics) error
	ListRuleStatistics(ctx context.Context, filter *LogAlertRuleStatisticFilter) ([]*model.LogAlertRuleStatistics, error)
	GetRuleStatisticsSummary(ctx context.Context, ruleID int64, dateFrom, dateTo time.Time) (*RuleStatisticsSummary, error)

	// Template Operations
	CreateLogAlertRuleTemplate(ctx context.Context, template *model.LogAlertRuleTemplates) error
	GetLogAlertRuleTemplateByID(ctx context.Context, id int64) (*model.LogAlertRuleTemplates, error)
	ListLogAlertRuleTemplates(ctx context.Context, category string) ([]*model.LogAlertRuleTemplates, error)
	DeleteLogAlertRuleTemplate(ctx context.Context, id int64) error
	IncrementTemplateUsage(ctx context.Context, templateID int64) error

	// Trigger tracking
	UpdateRuleTriggerInfo(ctx context.Context, ruleID int64) error

	// Multi-cluster support
	WithCluster(clusterName string) LogAlertRuleFacadeInterface
}

// LogAlertRuleFacade implements LogAlertRuleFacadeInterface
type LogAlertRuleFacade struct {
	BaseFacade
}

// NewLogAlertRuleFacade creates a new LogAlertRuleFacade instance
func NewLogAlertRuleFacade() LogAlertRuleFacadeInterface {
	return &LogAlertRuleFacade{}
}

// WithCluster returns a new facade instance using the specified cluster
func (f *LogAlertRuleFacade) WithCluster(clusterName string) LogAlertRuleFacadeInterface {
	return &LogAlertRuleFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// LogAlertRuleFilter defines filter criteria for listing rules
type LogAlertRuleFilter struct {
	ClusterName string
	Enabled     *bool
	MatchType   string
	Severity    string
	CreatedBy   string
	Priority    *int
	Keyword     string // Search in name and description
	Offset      int
	Limit       int
}

// CreateLogAlertRule creates a new log alert rule
func (f *LogAlertRuleFacade) CreateLogAlertRule(ctx context.Context, rule *model.LogAlertRules) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Create(rule).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create log alert rule: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Created log alert rule: %s (ID: %d)", rule.Name, rule.ID)
	return nil
}

// GetLogAlertRuleByID retrieves a log alert rule by ID
func (f *LogAlertRuleFacade) GetLogAlertRuleByID(ctx context.Context, id int64) (*model.LogAlertRules, error) {
	db := f.getDB().WithContext(ctx)

	var rule model.LogAlertRules
	if err := db.Where("id = ?", id).First(&rule).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if rule.ID == 0 {
		return nil, nil
	}

	return &rule, nil
}

// GetLogAlertRuleByName retrieves a log alert rule by cluster and name
func (f *LogAlertRuleFacade) GetLogAlertRuleByName(ctx context.Context, clusterName, name string) (*model.LogAlertRules, error) {
	db := f.getDB().WithContext(ctx)

	var rule model.LogAlertRules
	if err := db.Where("cluster_name = ? AND name = ?", clusterName, name).
		First(&rule).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if rule.ID == 0 {
		return nil, nil
	}

	return &rule, nil
}

// ListLogAlertRules lists log alert rules with filtering
func (f *LogAlertRuleFacade) ListLogAlertRules(ctx context.Context, filter *LogAlertRuleFilter) ([]*model.LogAlertRules, int64, error) {
	db := f.getDB().WithContext(ctx)

	query := db.Model(&model.LogAlertRules{})

	// Apply filters
	if filter != nil {
		if filter.ClusterName != "" {
			query = query.Where("cluster_name = ?", filter.ClusterName)
		}
		if filter.Enabled != nil {
			query = query.Where("enabled = ?", *filter.Enabled)
		}
		if filter.MatchType != "" {
			query = query.Where("match_type = ?", filter.MatchType)
		}
		if filter.Severity != "" {
			query = query.Where("severity = ?", filter.Severity)
		}
		if filter.CreatedBy != "" {
			query = query.Where("created_by = ?", filter.CreatedBy)
		}
		if filter.Priority != nil {
			query = query.Where("priority = ?", *filter.Priority)
		}
		if filter.Keyword != "" {
			query = query.Where("name LIKE ? OR description LIKE ?",
				"%"+filter.Keyword+"%", "%"+filter.Keyword+"%")
		}
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination and sorting
	if filter != nil {
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}
	query = query.Order("priority DESC, created_at DESC")

	var rules []*model.LogAlertRules
	if err := query.Find(&rules).Error; err != nil {
		return nil, 0, err
	}

	return rules, total, nil
}

// UpdateLogAlertRule updates an existing log alert rule
func (f *LogAlertRuleFacade) UpdateLogAlertRule(ctx context.Context, rule *model.LogAlertRules) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Save(rule).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update log alert rule: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Updated log alert rule: %s (ID: %d)", rule.Name, rule.ID)
	return nil
}

// DeleteLogAlertRule deletes a log alert rule by ID
func (f *LogAlertRuleFacade) DeleteLogAlertRule(ctx context.Context, id int64) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Delete(&model.LogAlertRules{}, id).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to delete log alert rule: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Deleted log alert rule ID: %d", id)
	return nil
}

// BatchUpdateEnabledStatus updates the enabled status for multiple rules
func (f *LogAlertRuleFacade) BatchUpdateEnabledStatus(ctx context.Context, ids []int64, enabled bool) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Model(&model.LogAlertRules{}).
		Where("id IN ?", ids).
		Update("enabled", enabled).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to batch update enabled status: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Batch updated %d rules enabled status to %v", len(ids), enabled)
	return nil
}

// BatchDeleteLogAlertRules deletes multiple rules
func (f *LogAlertRuleFacade) BatchDeleteLogAlertRules(ctx context.Context, ids []int64) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Delete(&model.LogAlertRules{}, ids).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to batch delete log alert rules: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Batch deleted %d rules", len(ids))
	return nil
}

// CreateRuleVersion creates a version snapshot of a rule
func (f *LogAlertRuleFacade) CreateRuleVersion(ctx context.Context, version *model.LogAlertRuleVersions) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Create(version).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create rule version: %v", err)
		return err
	}

	return nil
}

// ListRuleVersions lists all versions for a rule
func (f *LogAlertRuleFacade) ListRuleVersions(ctx context.Context, ruleID int64) ([]*model.LogAlertRuleVersions, error) {
	db := f.getDB().WithContext(ctx)

	var versions []*model.LogAlertRuleVersions
	if err := db.Where("rule_id = ?", ruleID).
		Order("version DESC").
		Find(&versions).Error; err != nil {
		return nil, err
	}

	return versions, nil
}

// GetRuleVersion retrieves a specific version of a rule
func (f *LogAlertRuleFacade) GetRuleVersion(ctx context.Context, ruleID int64, version int) (*model.LogAlertRuleVersions, error) {
	db := f.getDB().WithContext(ctx)

	var v model.LogAlertRuleVersions
	if err := db.Where("rule_id = ? AND version = ?", ruleID, version).
		First(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if v.ID == 0 {
		return nil, nil
	}

	return &v, nil
}

// LogAlertRuleStatisticFilter defines filter criteria for rule statistics
type LogAlertRuleStatisticFilter struct {
	RuleID      int64
	ClusterName string
	DateFrom    time.Time
	DateTo      time.Time
	Offset      int
	Limit       int
}

// CreateOrUpdateRuleStatistic creates or updates rule statistics
func (f *LogAlertRuleFacade) CreateOrUpdateRuleStatistic(ctx context.Context, stat *model.LogAlertRuleStatistics) error {
	db := f.getDB().WithContext(ctx)

	// Try to find existing stat
	var existing model.LogAlertRuleStatistics
	err := db.Where("rule_id = ? AND date = ? AND hour = ? AND cluster_name = ?",
		stat.RuleID, stat.Date, stat.Hour, stat.ClusterName).
		First(&existing).Error

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new
		if err := db.Create(stat).Error; err != nil {
			return err
		}
	} else {
		// Update existing
		existing.EvaluatedCount += stat.EvaluatedCount
		existing.MatchedCount += stat.MatchedCount
		existing.FiredCount += stat.FiredCount
		existing.ErrorCount += stat.ErrorCount

		// Update averages
		if stat.AvgEvalTimeMs > 0 {
			existing.AvgEvalTimeMs = (existing.AvgEvalTimeMs + stat.AvgEvalTimeMs) / 2
		}
		if stat.MaxEvalTimeMs > existing.MaxEvalTimeMs {
			existing.MaxEvalTimeMs = stat.MaxEvalTimeMs
		}

		if err := db.Save(&existing).Error; err != nil {
			return err
		}
	}

	return nil
}

// ListRuleStatistics lists rule statistics with filtering
func (f *LogAlertRuleFacade) ListRuleStatistics(ctx context.Context, filter *LogAlertRuleStatisticFilter) ([]*model.LogAlertRuleStatistics, error) {
	db := f.getDB().WithContext(ctx)

	query := db.Model(&model.LogAlertRuleStatistics{})

	if filter != nil {
		if filter.RuleID > 0 {
			query = query.Where("rule_id = ?", filter.RuleID)
		}
		if filter.ClusterName != "" {
			query = query.Where("cluster_name = ?", filter.ClusterName)
		}
		if !filter.DateFrom.IsZero() {
			query = query.Where("date >= ?", filter.DateFrom)
		}
		if !filter.DateTo.IsZero() {
			query = query.Where("date <= ?", filter.DateTo)
		}
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	query = query.Order("date DESC, hour DESC")

	var stats []*model.LogAlertRuleStatistics
	if err := query.Find(&stats).Error; err != nil {
		return nil, err
	}

	return stats, nil
}

// RuleStatisticsSummary represents aggregated statistics for a rule
type RuleStatisticsSummary struct {
	TotalEvaluated int64   `json:"total_evaluated"`
	TotalMatched   int64   `json:"total_matched"`
	TotalFired     int64   `json:"total_fired"`
	TotalErrors    int64   `json:"total_errors"`
	AvgEvalTimeMs  float64 `json:"avg_eval_time_ms"`
	MatchRate      float64 `json:"match_rate"`
	FireRate       float64 `json:"fire_rate"`
}

// GetRuleStatisticsSummary gets aggregated statistics for a rule
func (f *LogAlertRuleFacade) GetRuleStatisticsSummary(ctx context.Context, ruleID int64, dateFrom, dateTo time.Time) (*RuleStatisticsSummary, error) {
	db := f.getDB().WithContext(ctx)

	var summary RuleStatisticsSummary
	err := db.
		Model(&model.LogAlertRuleStatistics{}).
		Select(`
			COALESCE(SUM(evaluated_count), 0) as total_evaluated,
			COALESCE(SUM(matched_count), 0) as total_matched,
			COALESCE(SUM(fired_count), 0) as total_fired,
			COALESCE(SUM(error_count), 0) as total_errors,
			COALESCE(AVG(avg_eval_time_ms), 0) as avg_eval_time_ms
		`).
		Where("rule_id = ? AND date >= ? AND date <= ?", ruleID, dateFrom, dateTo).
		Scan(&summary).Error

	if err != nil {
		return nil, err
	}

	// Calculate rates
	if summary.TotalEvaluated > 0 {
		summary.MatchRate = float64(summary.TotalMatched) / float64(summary.TotalEvaluated) * 100
		summary.FireRate = float64(summary.TotalFired) / float64(summary.TotalEvaluated) * 100
	}

	return &summary, nil
}

// CreateLogAlertRuleTemplate creates a new rule template
func (f *LogAlertRuleFacade) CreateLogAlertRuleTemplate(ctx context.Context, template *model.LogAlertRuleTemplates) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Create(template).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create log alert rule template: %v", err)
		return err
	}

	return nil
}

// GetLogAlertRuleTemplateByID retrieves a template by ID
func (f *LogAlertRuleFacade) GetLogAlertRuleTemplateByID(ctx context.Context, id int64) (*model.LogAlertRuleTemplates, error) {
	db := f.getDB().WithContext(ctx)

	var template model.LogAlertRuleTemplates
	if err := db.Where("id = ?", id).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if template.ID == 0 {
		return nil, nil
	}

	return &template, nil
}

// ListLogAlertRuleTemplates lists templates by category
func (f *LogAlertRuleFacade) ListLogAlertRuleTemplates(ctx context.Context, category string) ([]*model.LogAlertRuleTemplates, error) {
	db := f.getDB().WithContext(ctx)

	query := db.Model(&model.LogAlertRuleTemplates{})
	if category != "" {
		query = query.Where("category = ?", category)
	}

	var templates []*model.LogAlertRuleTemplates
	if err := query.Order("usage_count DESC, name ASC").Find(&templates).Error; err != nil {
		return nil, err
	}

	return templates, nil
}

// DeleteLogAlertRuleTemplate deletes a template
func (f *LogAlertRuleFacade) DeleteLogAlertRuleTemplate(ctx context.Context, id int64) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Delete(&model.LogAlertRuleTemplates{}, id).Error; err != nil {
		return err
	}

	return nil
}

// IncrementTemplateUsage increments the usage count for a template
func (f *LogAlertRuleFacade) IncrementTemplateUsage(ctx context.Context, templateID int64) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Model(&model.LogAlertRuleTemplates{}).
		Where("id = ?", templateID).
		UpdateColumn("usage_count", gorm.Expr("usage_count + 1")).Error; err != nil {
		return err
	}

	return nil
}

// UpdateRuleTriggerInfo updates the last triggered time and count for a rule
func (f *LogAlertRuleFacade) UpdateRuleTriggerInfo(ctx context.Context, ruleID int64) error {
	db := f.getDB().WithContext(ctx)

	now := time.Now()
	if err := db.Model(&model.LogAlertRules{}).
		Where("id = ?", ruleID).
		Updates(map[string]interface{}{
			"last_triggered_at": now,
			"trigger_count":     gorm.Expr("trigger_count + 1"),
		}).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update rule trigger info for rule %d: %v", ruleID, err)
		return err
	}

	return nil
}
