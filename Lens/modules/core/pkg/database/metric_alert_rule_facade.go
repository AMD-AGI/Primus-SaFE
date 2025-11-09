package database

import (
	"context"
	"errors"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// MetricAlertRuleFacadeInterface defines the database operation interface for MetricAlertRule
type MetricAlertRuleFacadeInterface interface {
	// CRUD operations
	CreateMetricAlertRule(ctx context.Context, rule *model.MetricAlertRules) error
	UpdateMetricAlertRule(ctx context.Context, rule *model.MetricAlertRules) error
	GetMetricAlertRuleByID(ctx context.Context, id int64) (*model.MetricAlertRules, error)
	GetMetricAlertRuleByNameAndCluster(ctx context.Context, name, clusterName string) (*model.MetricAlertRules, error)
	ListMetricAlertRules(ctx context.Context, filter *MetricAlertRuleFilter) ([]*model.MetricAlertRules, int64, error)
	DeleteMetricAlertRule(ctx context.Context, id int64) error

	// Sync status operations
	UpdateSyncStatus(ctx context.Context, id int64, status, message string) error
	UpdateVMRuleStatus(ctx context.Context, id int64, vmruleStatus model.ExtType) error

	// Batch operations
	ListRulesByCluster(ctx context.Context, clusterName string, enabled *bool) ([]*model.MetricAlertRules, error)
	ListPendingSyncRules(ctx context.Context, limit int) ([]*model.MetricAlertRules, error)

	// WithCluster method
	WithCluster(clusterName string) MetricAlertRuleFacadeInterface
}

// MetricAlertRuleFilter defines filter conditions for querying metric alert rules
type MetricAlertRuleFilter struct {
	Name        *string
	ClusterName *string
	Enabled     *bool
	SyncStatus  *string
	Offset      int
	Limit       int
}

// MetricAlertRuleFacade implements MetricAlertRuleFacadeInterface
type MetricAlertRuleFacade struct {
	BaseFacade
}

// NewMetricAlertRuleFacade creates a new MetricAlertRuleFacade instance
func NewMetricAlertRuleFacade() MetricAlertRuleFacadeInterface {
	return &MetricAlertRuleFacade{}
}

func (f *MetricAlertRuleFacade) WithCluster(clusterName string) MetricAlertRuleFacadeInterface {
	return &MetricAlertRuleFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// CreateMetricAlertRule creates a new metric alert rule
func (f *MetricAlertRuleFacade) CreateMetricAlertRule(ctx context.Context, rule *model.MetricAlertRules) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(rule).Error
}

// UpdateMetricAlertRule updates an existing metric alert rule
func (f *MetricAlertRuleFacade) UpdateMetricAlertRule(ctx context.Context, rule *model.MetricAlertRules) error {
	db := f.getDB().WithContext(ctx)
	return db.Save(rule).Error
}

// GetMetricAlertRuleByID retrieves a metric alert rule by ID
func (f *MetricAlertRuleFacade) GetMetricAlertRuleByID(ctx context.Context, id int64) (*model.MetricAlertRules, error) {
	db := f.getDB().WithContext(ctx)
	var rule model.MetricAlertRules
	err := db.Where("id = ?", id).First(&rule).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

// GetMetricAlertRuleByNameAndCluster retrieves a metric alert rule by name and cluster
func (f *MetricAlertRuleFacade) GetMetricAlertRuleByNameAndCluster(ctx context.Context, name, clusterName string) (*model.MetricAlertRules, error) {
	db := f.getDB().WithContext(ctx)
	var rule model.MetricAlertRules
	err := db.Where("name = ? AND cluster_name = ?", name, clusterName).First(&rule).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

// ListMetricAlertRules lists metric alert rules with filtering
func (f *MetricAlertRuleFacade) ListMetricAlertRules(ctx context.Context, filter *MetricAlertRuleFilter) ([]*model.MetricAlertRules, int64, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.MetricAlertRules{})

	if filter.Name != nil {
		query = query.Where("name LIKE ?", "%"+*filter.Name+"%")
	}
	if filter.ClusterName != nil {
		query = query.Where("cluster_name = ?", *filter.ClusterName)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	if filter.SyncStatus != nil {
		query = query.Where("sync_status = ?", *filter.SyncStatus)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rules []*model.MetricAlertRules
	query = query.Order("created_at DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit).Offset(filter.Offset)
	}

	err := query.Find(&rules).Error
	return rules, total, err
}

// DeleteMetricAlertRule deletes a metric alert rule
func (f *MetricAlertRuleFacade) DeleteMetricAlertRule(ctx context.Context, id int64) error {
	db := f.getDB().WithContext(ctx)
	return db.Delete(&model.MetricAlertRules{}, id).Error
}

// UpdateSyncStatus updates the sync status of a metric alert rule
func (f *MetricAlertRuleFacade) UpdateSyncStatus(ctx context.Context, id int64, status, message string) error {
	db := f.getDB().WithContext(ctx)
	updates := map[string]interface{}{
		"sync_status":  status,
		"sync_message": message,
		"last_sync_at": gorm.Expr("NOW()"),
	}
	return db.Model(&model.MetricAlertRules{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateVMRuleStatus updates the VMRule status from Kubernetes
func (f *MetricAlertRuleFacade) UpdateVMRuleStatus(ctx context.Context, id int64, vmruleStatus model.ExtType) error {
	db := f.getDB().WithContext(ctx)
	return db.Model(&model.MetricAlertRules{}).Where("id = ?", id).Update("vmrule_status", vmruleStatus).Error
}

// ListRulesByCluster lists all rules for a specific cluster
func (f *MetricAlertRuleFacade) ListRulesByCluster(ctx context.Context, clusterName string, enabled *bool) ([]*model.MetricAlertRules, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Where("cluster_name = ?", clusterName)

	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}

	var rules []*model.MetricAlertRules
	err := query.Order("created_at DESC").Find(&rules).Error
	return rules, err
}

// ListPendingSyncRules lists rules that need to be synced
func (f *MetricAlertRuleFacade) ListPendingSyncRules(ctx context.Context, limit int) ([]*model.MetricAlertRules, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Where("sync_status = ? AND enabled = ?", "pending", true)

	if limit > 0 {
		query = query.Limit(limit)
	}

	var rules []*model.MetricAlertRules
	err := query.Order("created_at ASC").Find(&rules).Error
	return rules, err
}
