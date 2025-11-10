package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
)

// AlertRuleAdviceFacadeInterface defines the interface for alert rule advice operations
type AlertRuleAdviceFacadeInterface interface {
	// CRUD Operations
	CreateAlertRuleAdvices(ctx context.Context, advice *model.AlertRuleAdvices) error
	GetAlertRuleAdvicesByID(ctx context.Context, id int64) (*model.AlertRuleAdvices, error)
	ListAlertRuleAdvicess(ctx context.Context, filter *AlertRuleAdvicesFilter) ([]*model.AlertRuleAdvices, int64, error)
	UpdateAlertRuleAdvices(ctx context.Context, advice *model.AlertRuleAdvices) error
	DeleteAlertRuleAdvices(ctx context.Context, id int64) error

	// Batch Operations
	BatchCreateAlertRuleAdvicess(ctx context.Context, advices []*model.AlertRuleAdvices) error
	BatchUpdateStatus(ctx context.Context, ids []int64, status, reviewedBy, reviewNotes string) error
	BatchDeleteAlertRuleAdvicess(ctx context.Context, ids []int64) error

	// Status Operations
	UpdateAdviceStatus(ctx context.Context, id int64, status, reviewedBy, reviewNotes string) error
	MarkAsApplied(ctx context.Context, id int64, appliedRuleID int64) error

	// Query Operations
	GetAdvicesByInspectionID(ctx context.Context, inspectionID string) ([]*model.AlertRuleAdvices, error)
	GetAdvicesByCluster(ctx context.Context, clusterName string, status string) ([]*model.AlertRuleAdvices, error)
	GetExpiredAdvices(ctx context.Context) ([]*model.AlertRuleAdvices, error)

	// Statistics Operations
	CreateOrUpdateAdviceStatistics(ctx context.Context, stats *model.LogAlertRuleStatistics) error
	GetAdviceStatistics(ctx context.Context, clusterName string, dateFrom, dateTo time.Time) ([]*model.LogAlertRuleStatistics, error)
	GetAdviceSummary(ctx context.Context, filter *AlertRuleAdvicesFilter) (*AlertRuleAdvicesSummary, error)

	// Cleanup Operations
	DeleteExpiredAdvices(ctx context.Context) (int64, error)

	// Multi-cluster support
	WithCluster(clusterName string) AlertRuleAdviceFacadeInterface
}

// AlertRuleAdviceFacade implements AlertRuleAdviceFacadeInterface
type AlertRuleAdviceFacade struct {
	BaseFacade
}

// NewAlertRuleAdviceFacade creates a new AlertRuleAdviceFacade instance
func NewAlertRuleAdviceFacade() AlertRuleAdviceFacadeInterface {
	return &AlertRuleAdviceFacade{}
}

// WithCluster returns a new facade instance using the specified cluster
func (f *AlertRuleAdviceFacade) WithCluster(clusterName string) AlertRuleAdviceFacadeInterface {
	return &AlertRuleAdviceFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// AlertRuleAdvicesFilter defines filter criteria for listing advices
type AlertRuleAdvicesFilter struct {
	ClusterName    string
	RuleType       string // log/metric
	Category       string // performance/error/resource/security/availability
	Status         string // pending/reviewed/accepted/rejected/applied
	InspectionID   string
	TargetResource string
	TargetName     string
	SeverityList   []string // Filter by multiple severities
	MinPriority    *int
	MaxPriority    *int
	MinConfidence  *float64
	MaxConfidence  *float64
	Tags           []string // Filter by tags
	ExcludeExpired bool     // Exclude expired advices
	DateFrom       *time.Time
	DateTo         *time.Time
	Keyword        string // Search in name, title, description, reason
	Offset         int
	Limit          int
	OrderBy        string // Field to order by (default: created_at)
	OrderDesc      bool   // Order direction (default: true)
}

// AlertRuleAdvicesSummary provides summary statistics for advices
type AlertRuleAdvicesSummary struct {
	Total              int64            `json:"total"`
	ByRuleType         map[string]int64 `json:"by_rule_type"`
	ByCategory         map[string]int64 `json:"by_category"`
	ByStatus           map[string]int64 `json:"by_status"`
	BySeverity         map[string]int64 `json:"by_severity"`
	AvgConfidenceScore float64          `json:"avg_confidence_score"`
	AvgPriority        float64          `json:"avg_priority"`
	ExpiredCount       int64            `json:"expired_count"`
}

// CreateAlertRuleAdvices creates a new alert rule advice
func (f *AlertRuleAdviceFacade) CreateAlertRuleAdvices(ctx context.Context, advice *model.AlertRuleAdvices) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Create(advice).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create alert rule advice: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Created alert rule advice: %s (ID: %d)", advice.Name, advice.ID)
	return nil
}

// GetAlertRuleAdvicesByID retrieves an alert rule advice by ID
func (f *AlertRuleAdviceFacade) GetAlertRuleAdvicesByID(ctx context.Context, id int64) (*model.AlertRuleAdvices, error) {
	db := f.getDB().WithContext(ctx)

	var advice model.AlertRuleAdvices
	if err := db.Where("id = ?", id).First(&advice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alert rule advice: %v", err)
		return nil, err
	}

	if advice.ID == 0 {
		return nil, nil
	}

	return &advice, nil
}

// ListAlertRuleAdvicess lists alert rule advices with filters
func (f *AlertRuleAdviceFacade) ListAlertRuleAdvicess(ctx context.Context, filter *AlertRuleAdvicesFilter) ([]*model.AlertRuleAdvices, int64, error) {
	db := f.getDB().WithContext(ctx)

	query := db.Model(&model.AlertRuleAdvices{})

	// Apply filters
	if filter != nil {
		if filter.ClusterName != "" {
			query = query.Where("cluster_name = ?", filter.ClusterName)
		}
		if filter.RuleType != "" {
			query = query.Where("rule_type = ?", filter.RuleType)
		}
		if filter.Category != "" {
			query = query.Where("category = ?", filter.Category)
		}
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.InspectionID != "" {
			query = query.Where("inspection_id = ?", filter.InspectionID)
		}
		if filter.TargetResource != "" {
			query = query.Where("target_resource = ?", filter.TargetResource)
		}
		if filter.TargetName != "" {
			query = query.Where("target_name = ?", filter.TargetName)
		}
		if len(filter.SeverityList) > 0 {
			query = query.Where("severity IN ?", filter.SeverityList)
		}
		if filter.MinPriority != nil {
			query = query.Where("priority >= ?", *filter.MinPriority)
		}
		if filter.MaxPriority != nil {
			query = query.Where("priority <= ?", *filter.MaxPriority)
		}
		if filter.MinConfidence != nil {
			query = query.Where("confidence_score >= ?", *filter.MinConfidence)
		}
		if filter.MaxConfidence != nil {
			query = query.Where("confidence_score <= ?", *filter.MaxConfidence)
		}
		if len(filter.Tags) > 0 {
			query = query.Where("tags && ?", filter.Tags)
		}
		if filter.ExcludeExpired {
			query = query.Where("(expires_at IS NULL OR expires_at > ?)", time.Now())
		}
		if filter.DateFrom != nil {
			query = query.Where("inspection_time >= ?", *filter.DateFrom)
		}
		if filter.DateTo != nil {
			query = query.Where("inspection_time <= ?", *filter.DateTo)
		}
		if filter.Keyword != "" {
			keyword := "%" + filter.Keyword + "%"
			query = query.Where("name ILIKE ? OR title ILIKE ? OR description ILIKE ? OR reason ILIKE ?",
				keyword, keyword, keyword, keyword)
		}
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to count alert rule advices: %v", err)
		return nil, 0, err
	}

	// Apply ordering
	orderBy := "created_at"
	if filter != nil && filter.OrderBy != "" {
		orderBy = filter.OrderBy
	}
	orderDir := "DESC"
	if filter != nil && !filter.OrderDesc {
		orderDir = "ASC"
	}
	query = query.Order(orderBy + " " + orderDir)

	// Apply pagination
	if filter != nil {
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	// Execute query
	var advices []*model.AlertRuleAdvices
	if err := query.Find(&advices).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list alert rule advices: %v", err)
		return nil, 0, err
	}

	return advices, total, nil
}

// UpdateAlertRuleAdvices updates an alert rule advice
func (f *AlertRuleAdviceFacade) UpdateAlertRuleAdvices(ctx context.Context, advice *model.AlertRuleAdvices) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Save(advice).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update alert rule advice: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Updated alert rule advice: ID %d", advice.ID)
	return nil
}

// DeleteAlertRuleAdvices deletes an alert rule advice
func (f *AlertRuleAdviceFacade) DeleteAlertRuleAdvices(ctx context.Context, id int64) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Delete(&model.AlertRuleAdvices{}, id).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to delete alert rule advice: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Deleted alert rule advice: ID %d", id)
	return nil
}

// BatchCreateAlertRuleAdvicess creates multiple alert rule advices in a transaction
func (f *AlertRuleAdviceFacade) BatchCreateAlertRuleAdvicess(ctx context.Context, advices []*model.AlertRuleAdvices) error {
	db := f.getDB().WithContext(ctx)

	return db.Transaction(func(tx *gorm.DB) error {
		for _, advice := range advices {
			if err := tx.Create(advice).Error; err != nil {
				log.GlobalLogger().WithContext(ctx).Errorf("Failed to create alert rule advice in batch: %v", err)
				return err
			}
		}
		log.GlobalLogger().WithContext(ctx).Infof("Batch created %d alert rule advices", len(advices))
		return nil
	})
}

// BatchUpdateStatus updates the status of multiple advices
func (f *AlertRuleAdviceFacade) BatchUpdateStatus(ctx context.Context, ids []int64, status, reviewedBy, reviewNotes string) error {
	db := f.getDB().WithContext(ctx)

	updates := map[string]interface{}{
		"status":       status,
		"reviewed_by":  reviewedBy,
		"reviewed_at":  time.Now(),
		"review_notes": reviewNotes,
	}

	if err := db.Model(&model.AlertRuleAdvices{}).Where("id IN ?", ids).Updates(updates).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to batch update advice status: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Batch updated status for %d advices to %s", len(ids), status)
	return nil
}

// BatchDeleteAlertRuleAdvicess deletes multiple alert rule advices
func (f *AlertRuleAdviceFacade) BatchDeleteAlertRuleAdvicess(ctx context.Context, ids []int64) error {
	db := f.getDB().WithContext(ctx)

	if err := db.Delete(&model.AlertRuleAdvices{}, ids).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to batch delete alert rule advices: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Batch deleted %d alert rule advices", len(ids))
	return nil
}

// UpdateAdviceStatus updates the status of an advice
func (f *AlertRuleAdviceFacade) UpdateAdviceStatus(ctx context.Context, id int64, status, reviewedBy, reviewNotes string) error {
	db := f.getDB().WithContext(ctx)

	now := time.Now()
	updates := map[string]interface{}{
		"status":       status,
		"reviewed_by":  reviewedBy,
		"reviewed_at":  now,
		"review_notes": reviewNotes,
	}

	if err := db.Model(&model.AlertRuleAdvices{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update advice status: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Updated advice %d status to %s", id, status)
	return nil
}

// MarkAsApplied marks an advice as applied with the created rule ID
func (f *AlertRuleAdviceFacade) MarkAsApplied(ctx context.Context, id int64, appliedRuleID int64) error {
	db := f.getDB().WithContext(ctx)

	now := time.Now()
	updates := map[string]interface{}{
		"status":          "applied",
		"applied_rule_id": appliedRuleID,
		"applied_at":      now,
	}

	if err := db.Model(&model.AlertRuleAdvices{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to mark advice as applied: %v", err)
		return err
	}

	log.GlobalLogger().WithContext(ctx).Infof("Marked advice %d as applied with rule ID %d", id, appliedRuleID)
	return nil
}

// GetAdvicesByInspectionID retrieves all advices for a specific inspection
func (f *AlertRuleAdviceFacade) GetAdvicesByInspectionID(ctx context.Context, inspectionID string) ([]*model.AlertRuleAdvices, error) {
	db := f.getDB().WithContext(ctx)

	var advices []*model.AlertRuleAdvices
	if err := db.Where("inspection_id = ?", inspectionID).Order("priority DESC, confidence_score DESC").Find(&advices).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get advices by inspection ID: %v", err)
		return nil, err
	}

	return advices, nil
}

// GetAdvicesByCluster retrieves advices for a specific cluster
func (f *AlertRuleAdviceFacade) GetAdvicesByCluster(ctx context.Context, clusterName string, status string) ([]*model.AlertRuleAdvices, error) {
	db := f.getDB().WithContext(ctx)

	query := db.Where("cluster_name = ?", clusterName)
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var advices []*model.AlertRuleAdvices
	if err := query.Order("priority DESC, confidence_score DESC, created_at DESC").Find(&advices).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get advices by cluster: %v", err)
		return nil, err
	}

	return advices, nil
}

// GetExpiredAdvices retrieves all expired advices
func (f *AlertRuleAdviceFacade) GetExpiredAdvices(ctx context.Context) ([]*model.AlertRuleAdvices, error) {
	db := f.getDB().WithContext(ctx)

	var advices []*model.AlertRuleAdvices
	if err := db.Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).Find(&advices).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get expired advices: %v", err)
		return nil, err
	}

	return advices, nil
}

// CreateOrUpdateAdviceStatistics creates or updates advice statistics
func (f *AlertRuleAdviceFacade) CreateOrUpdateAdviceStatistics(ctx context.Context, stats *model.LogAlertRuleStatistics) error {
	db := f.getDB().WithContext(ctx)

	// Try to find existing record
	var existing model.LogAlertRuleStatistics
	err := db.Where("cluster_name = ? AND date = ?", stats.ClusterName, stats.Date).First(&existing).Error

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to check existing statistics: %v", err)
		return err
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new record
		if err := db.Create(stats).Error; err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to create advice statistics: %v", err)
			return err
		}
		log.GlobalLogger().WithContext(ctx).Infof("Created advice statistics for cluster %s, date %s", stats.ClusterName, stats.Date)
	} else {
		// Update existing record
		stats.ID = existing.ID
		if err := db.Save(stats).Error; err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to update advice statistics: %v", err)
			return err
		}
		log.GlobalLogger().WithContext(ctx).Infof("Updated advice statistics for cluster %s, date %s", stats.ClusterName, stats.Date)
	}

	return nil
}

// GetAdviceStatistics retrieves advice statistics for a cluster and date range
func (f *AlertRuleAdviceFacade) GetAdviceStatistics(ctx context.Context, clusterName string, dateFrom, dateTo time.Time) ([]*model.LogAlertRuleStatistics, error) {
	db := f.getDB().WithContext(ctx)

	query := db.Model(&model.LogAlertRuleStatistics{})

	if clusterName != "" {
		query = query.Where("cluster_name = ?", clusterName)
	}
	query = query.Where("date >= ? AND date <= ?", dateFrom, dateTo)

	var stats []*model.LogAlertRuleStatistics
	if err := query.Order("date DESC").Find(&stats).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get advice statistics: %v", err)
		return nil, err
	}

	return stats, nil
}

// GetAdviceSummary retrieves summary statistics for advices
func (f *AlertRuleAdviceFacade) GetAdviceSummary(ctx context.Context, filter *AlertRuleAdvicesFilter) (*AlertRuleAdvicesSummary, error) {
	db := f.getDB().WithContext(ctx)

	query := db.Model(&model.AlertRuleAdvices{})

	// Apply filters (similar to ListAlertRuleAdvicess)
	if filter != nil {
		if filter.ClusterName != "" {
			query = query.Where("cluster_name = ?", filter.ClusterName)
		}
		if filter.InspectionID != "" {
			query = query.Where("inspection_id = ?", filter.InspectionID)
		}
		if filter.ExcludeExpired {
			query = query.Where("(expires_at IS NULL OR expires_at > ?)", time.Now())
		}
		if filter.DateFrom != nil {
			query = query.Where("inspection_time >= ?", *filter.DateFrom)
		}
		if filter.DateTo != nil {
			query = query.Where("inspection_time <= ?", *filter.DateTo)
		}
	}

	summary := &AlertRuleAdvicesSummary{
		ByRuleType: make(map[string]int64),
		ByCategory: make(map[string]int64),
		ByStatus:   make(map[string]int64),
		BySeverity: make(map[string]int64),
	}

	// Total count
	if err := query.Count(&summary.Total).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to count total advices: %v", err)
		return nil, err
	}

	// Group by rule type
	var ruleTypeCounts []struct {
		RuleType string
		Count    int64
	}
	if err := query.Select("rule_type, COUNT(*) as count").Group("rule_type").Scan(&ruleTypeCounts).Error; err == nil {
		for _, rt := range ruleTypeCounts {
			summary.ByRuleType[rt.RuleType] = rt.Count
		}
	}

	// Group by category
	var categoryCounts []struct {
		Category string
		Count    int64
	}
	if err := query.Select("category, COUNT(*) as count").Group("category").Scan(&categoryCounts).Error; err == nil {
		for _, c := range categoryCounts {
			summary.ByCategory[c.Category] = c.Count
		}
	}

	// Group by status
	var statusCounts []struct {
		Status string
		Count  int64
	}
	if err := query.Select("status, COUNT(*) as count").Group("status").Scan(&statusCounts).Error; err == nil {
		for _, s := range statusCounts {
			summary.ByStatus[s.Status] = s.Count
		}
	}

	// Group by severity
	var severityCounts []struct {
		Severity string
		Count    int64
	}
	if err := query.Select("severity, COUNT(*) as count").Group("severity").Scan(&severityCounts).Error; err == nil {
		for _, sv := range severityCounts {
			summary.BySeverity[sv.Severity] = sv.Count
		}
	}

	// Average scores
	var avgScores struct {
		AvgConfidence float64
		AvgPriority   float64
	}
	if err := query.Select("AVG(confidence_score) as avg_confidence, AVG(priority) as avg_priority").Scan(&avgScores).Error; err == nil {
		summary.AvgConfidenceScore = avgScores.AvgConfidence
		summary.AvgPriority = avgScores.AvgPriority
	}

	// Expired count
	expiredQuery := query
	if err := expiredQuery.Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).Count(&summary.ExpiredCount).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to count expired advices: %v", err)
	}

	return summary, nil
}

// DeleteExpiredAdvices deletes all expired advices
func (f *AlertRuleAdviceFacade) DeleteExpiredAdvices(ctx context.Context) (int64, error) {
	db := f.getDB().WithContext(ctx)

	result := db.Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).Delete(&model.AlertRuleAdvices{})
	if result.Error != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to delete expired advices: %v", result.Error)
		return 0, result.Error
	}

	log.GlobalLogger().WithContext(ctx).Infof("Deleted %d expired advices", result.RowsAffected)
	return result.RowsAffected, nil
}
