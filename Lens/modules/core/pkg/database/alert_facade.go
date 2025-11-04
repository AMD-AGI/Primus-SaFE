package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// AlertFacadeInterface defines the database operation interface for Alert system
type AlertFacadeInterface interface {
	// AlertEvent operations
	CreateAlertEvent(ctx context.Context, alert *model.AlertEvent) error
	UpdateAlertEvent(ctx context.Context, alert *model.AlertEvent) error
	GetAlertEventByID(ctx context.Context, id string) (*model.AlertEvent, error)
	ListAlertEvents(ctx context.Context, filter *AlertEventFilter) ([]*model.AlertEvent, int64, error)
	UpdateAlertStatus(ctx context.Context, id string, status string, endsAt *time.Time) error
	DeleteOldAlertEvents(ctx context.Context, before time.Time) error
	
	// AlertCorrelation operations
	CreateAlertCorrelation(ctx context.Context, correlation *model.AlertCorrelation) error
	ListAlertCorrelationsByCorrelationID(ctx context.Context, correlationID string) ([]*model.AlertCorrelation, error)
	ListAlertCorrelationsByAlertID(ctx context.Context, alertID string) ([]*model.AlertCorrelation, error)
	
	// AlertStatistic operations
	CreateOrUpdateAlertStatistic(ctx context.Context, stat *model.AlertStatistic) error
	GetAlertStatistic(ctx context.Context, date time.Time, hour *int, alertName, source, workloadID, clusterName string) (*model.AlertStatistic, error)
	ListAlertStatistics(ctx context.Context, filter *AlertStatisticFilter) ([]*model.AlertStatistic, error)
	
	// AlertRule operations
	CreateAlertRule(ctx context.Context, rule *model.AlertRule) error
	UpdateAlertRule(ctx context.Context, rule *model.AlertRule) error
	GetAlertRuleByID(ctx context.Context, id int64) (*model.AlertRule, error)
	GetAlertRuleByName(ctx context.Context, name string) (*model.AlertRule, error)
	ListAlertRules(ctx context.Context, source string, enabled *bool) ([]*model.AlertRule, error)
	DeleteAlertRule(ctx context.Context, id int64) error
	
	// AlertSilence operations
	CreateAlertSilence(ctx context.Context, silence *model.AlertSilence) error
	UpdateAlertSilence(ctx context.Context, silence *model.AlertSilence) error
	GetAlertSilenceByID(ctx context.Context, id string) (*model.AlertSilence, error)
	ListAlertSilences(ctx context.Context, filter *AlertSilenceFilter) ([]*model.AlertSilence, int64, error)
	ListActiveSilences(ctx context.Context, now time.Time, clusterName string) ([]*model.AlertSilence, error)
	DeleteAlertSilence(ctx context.Context, id string) error
	DisableAlertSilence(ctx context.Context, id string) error
	
	// SilencedAlert operations (audit trail)
	CreateSilencedAlert(ctx context.Context, silencedAlert *model.SilencedAlert) error
	ListSilencedAlerts(ctx context.Context, filter *SilencedAlertFilter) ([]*model.SilencedAlert, int64, error)
	
	// AlertNotification operations
	CreateAlertNotification(ctx context.Context, notification *model.AlertNotification) error
	UpdateAlertNotification(ctx context.Context, notification *model.AlertNotification) error
	ListAlertNotificationsByAlertID(ctx context.Context, alertID string) ([]*model.AlertNotification, error)
	ListPendingNotifications(ctx context.Context, limit int) ([]*model.AlertNotification, error)
	
	// WithCluster method
	WithCluster(clusterName string) AlertFacadeInterface
}

// AlertEventFilter defines filter conditions for querying alert events
type AlertEventFilter struct {
	Source      *string
	AlertName   *string
	Severity    *string
	Status      *string
	WorkloadID  *string
	PodName     *string
	NodeName    *string
	ClusterName *string
	StartsAfter *time.Time
	StartsBefore *time.Time
	Offset      int
	Limit       int
}

// AlertStatisticFilter defines filter conditions for querying alert statistics
type AlertStatisticFilter struct {
	DateFrom    *time.Time
	DateTo      *time.Time
	AlertName   *string
	Source      *string
	WorkloadID  *string
	ClusterName *string
	Offset      int
	Limit       int
}

// AlertSilenceFilter defines filter conditions for querying alert silences
type AlertSilenceFilter struct {
	ClusterName *string
	SilenceType *string
	Enabled     *bool
	ActiveOnly  bool // Only return silences active at current time
	Offset      int
	Limit       int
}

// SilencedAlertFilter defines filter conditions for querying silenced alerts
type SilencedAlertFilter struct {
	SilenceID   *string
	AlertName   *string
	ClusterName *string
	StartTime   *time.Time
	EndTime     *time.Time
	Offset      int
	Limit       int
}

// AlertFacade implements AlertFacadeInterface
type AlertFacade struct {
	BaseFacade
}

// NewAlertFacade creates a new AlertFacade instance
func NewAlertFacade() AlertFacadeInterface {
	return &AlertFacade{}
}

func (f *AlertFacade) WithCluster(clusterName string) AlertFacadeInterface {
	return &AlertFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// AlertEvent operation implementations

func (f *AlertFacade) CreateAlertEvent(ctx context.Context, alert *model.AlertEvent) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(alert).Error
}

func (f *AlertFacade) UpdateAlertEvent(ctx context.Context, alert *model.AlertEvent) error {
	db := f.getDB().WithContext(ctx)
	return db.Save(alert).Error
}

func (f *AlertFacade) GetAlertEventByID(ctx context.Context, id string) (*model.AlertEvent, error) {
	db := f.getDB().WithContext(ctx)
	var alert model.AlertEvent
	err := db.Where("id = ?", id).First(&alert).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &alert, nil
}

func (f *AlertFacade) ListAlertEvents(ctx context.Context, filter *AlertEventFilter) ([]*model.AlertEvent, int64, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.AlertEvent{})
	
	if filter.Source != nil {
		query = query.Where("source = ?", *filter.Source)
	}
	if filter.AlertName != nil {
		query = query.Where("alert_name = ?", *filter.AlertName)
	}
	if filter.Severity != nil {
		query = query.Where("severity = ?", *filter.Severity)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.WorkloadID != nil {
		query = query.Where("workload_id = ?", *filter.WorkloadID)
	}
	if filter.PodName != nil {
		query = query.Where("pod_name = ?", *filter.PodName)
	}
	if filter.NodeName != nil {
		query = query.Where("node_name = ?", *filter.NodeName)
	}
	if filter.ClusterName != nil {
		query = query.Where("cluster_name = ?", *filter.ClusterName)
	}
	if filter.StartsAfter != nil {
		query = query.Where("starts_at >= ?", *filter.StartsAfter)
	}
	if filter.StartsBefore != nil {
		query = query.Where("starts_at < ?", *filter.StartsBefore)
	}
	
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	
	var alerts []*model.AlertEvent
	query = query.Order("starts_at DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit).Offset(filter.Offset)
	}
	
	err := query.Find(&alerts).Error
	return alerts, total, err
}

func (f *AlertFacade) UpdateAlertStatus(ctx context.Context, id string, status string, endsAt *time.Time) error {
	db := f.getDB().WithContext(ctx)
	updates := map[string]interface{}{
		"status": status,
	}
	if endsAt != nil {
		updates["ends_at"] = endsAt
	}
	return db.Model(&model.AlertEvent{}).Where("id = ?", id).Updates(updates).Error
}

func (f *AlertFacade) DeleteOldAlertEvents(ctx context.Context, before time.Time) error {
	db := f.getDB().WithContext(ctx)
	return db.Where("starts_at < ?", before).Delete(&model.AlertEvent{}).Error
}

// AlertCorrelation operation implementations

func (f *AlertFacade) CreateAlertCorrelation(ctx context.Context, correlation *model.AlertCorrelation) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(correlation).Error
}

func (f *AlertFacade) ListAlertCorrelationsByCorrelationID(ctx context.Context, correlationID string) ([]*model.AlertCorrelation, error) {
	db := f.getDB().WithContext(ctx)
	var correlations []*model.AlertCorrelation
	err := db.Where("correlation_id = ?", correlationID).Find(&correlations).Error
	return correlations, err
}

func (f *AlertFacade) ListAlertCorrelationsByAlertID(ctx context.Context, alertID string) ([]*model.AlertCorrelation, error) {
	db := f.getDB().WithContext(ctx)
	var correlations []*model.AlertCorrelation
	err := db.Where("alert_id = ?", alertID).Find(&correlations).Error
	return correlations, err
}

// AlertStatistic operation implementations

func (f *AlertFacade) CreateOrUpdateAlertStatistic(ctx context.Context, stat *model.AlertStatistic) error {
	db := f.getDB().WithContext(ctx)
	
	// Try to find existing record
	var existing model.AlertStatistic
	query := db.Where("date = ? AND alert_name = ? AND source = ?", stat.Date, stat.AlertName, stat.Source)
	if stat.Hour > 0 {
		query = query.Where("hour = ?", stat.Hour)
	} else {
		query = query.Where("hour IS NULL")
	}
	if stat.WorkloadID != "" {
		query = query.Where("workload_id = ?", stat.WorkloadID)
	} else {
		query = query.Where("workload_id IS NULL OR workload_id = ''")
	}
	if stat.ClusterName != "" {
		query = query.Where("cluster_name = ?", stat.ClusterName)
	} else {
		query = query.Where("cluster_name IS NULL OR cluster_name = ''")
	}
	
	err := query.First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create new record
			return db.Create(stat).Error
		}
		return err
	}
	
	// Update existing record
	existing.FiringCount += stat.FiringCount
	existing.ResolvedCount += stat.ResolvedCount
	existing.TotalDurationSeconds += stat.TotalDurationSeconds
	if existing.FiringCount+existing.ResolvedCount > 0 {
		existing.AvgDurationSeconds = float64(existing.TotalDurationSeconds) / float64(existing.FiringCount+existing.ResolvedCount)
	}
	
	return db.Save(&existing).Error
}

func (f *AlertFacade) GetAlertStatistic(ctx context.Context, date time.Time, hour *int, alertName, source, workloadID, clusterName string) (*model.AlertStatistic, error) {
	db := f.getDB().WithContext(ctx)
	var stat model.AlertStatistic
	
	query := db.Where("date = ? AND alert_name = ? AND source = ?", date, alertName, source)
	if hour != nil {
		query = query.Where("hour = ?", *hour)
	} else {
		query = query.Where("hour IS NULL")
	}
	if workloadID != "" {
		query = query.Where("workload_id = ?", workloadID)
	}
	if clusterName != "" {
		query = query.Where("cluster_name = ?", clusterName)
	}
	
	err := query.First(&stat).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &stat, nil
}

func (f *AlertFacade) ListAlertStatistics(ctx context.Context, filter *AlertStatisticFilter) ([]*model.AlertStatistic, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.AlertStatistic{})
	
	if filter.DateFrom != nil {
		query = query.Where("date >= ?", *filter.DateFrom)
	}
	if filter.DateTo != nil {
		query = query.Where("date <= ?", *filter.DateTo)
	}
	if filter.AlertName != nil {
		query = query.Where("alert_name = ?", *filter.AlertName)
	}
	if filter.Source != nil {
		query = query.Where("source = ?", *filter.Source)
	}
	if filter.WorkloadID != nil {
		query = query.Where("workload_id = ?", *filter.WorkloadID)
	}
	if filter.ClusterName != nil {
		query = query.Where("cluster_name = ?", *filter.ClusterName)
	}
	
	var stats []*model.AlertStatistic
	query = query.Order("date DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit).Offset(filter.Offset)
	}
	
	err := query.Find(&stats).Error
	return stats, err
}

// AlertRule operation implementations

func (f *AlertFacade) CreateAlertRule(ctx context.Context, rule *model.AlertRule) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(rule).Error
}

func (f *AlertFacade) UpdateAlertRule(ctx context.Context, rule *model.AlertRule) error {
	db := f.getDB().WithContext(ctx)
	return db.Save(rule).Error
}

func (f *AlertFacade) GetAlertRuleByID(ctx context.Context, id int64) (*model.AlertRule, error) {
	db := f.getDB().WithContext(ctx)
	var rule model.AlertRule
	err := db.Where("id = ?", id).First(&rule).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

func (f *AlertFacade) GetAlertRuleByName(ctx context.Context, name string) (*model.AlertRule, error) {
	db := f.getDB().WithContext(ctx)
	var rule model.AlertRule
	err := db.Where("name = ?", name).First(&rule).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

func (f *AlertFacade) ListAlertRules(ctx context.Context, source string, enabled *bool) ([]*model.AlertRule, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.AlertRule{})
	
	if source != "" {
		query = query.Where("source = ?", source)
	}
	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}
	
	var rules []*model.AlertRule
	err := query.Order("created_at DESC").Find(&rules).Error
	return rules, err
}

func (f *AlertFacade) DeleteAlertRule(ctx context.Context, id int64) error {
	db := f.getDB().WithContext(ctx)
	return db.Delete(&model.AlertRule{}, id).Error
}

// AlertSilence operation implementations

func (f *AlertFacade) CreateAlertSilence(ctx context.Context, silence *model.AlertSilence) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(silence).Error
}

func (f *AlertFacade) UpdateAlertSilence(ctx context.Context, silence *model.AlertSilence) error {
	db := f.getDB().WithContext(ctx)
	return db.Save(silence).Error
}

func (f *AlertFacade) GetAlertSilenceByID(ctx context.Context, id string) (*model.AlertSilence, error) {
	db := f.getDB().WithContext(ctx)
	var silence model.AlertSilence
	err := db.Where("id = ?", id).First(&silence).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &silence, nil
}

func (f *AlertFacade) ListAlertSilences(ctx context.Context, filter *AlertSilenceFilter) ([]*model.AlertSilence, int64, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.AlertSilence{})
	
	if filter.ClusterName != nil {
		query = query.Where("cluster_name = ?", *filter.ClusterName)
	}
	if filter.SilenceType != nil {
		query = query.Where("silence_type = ?", *filter.SilenceType)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	if filter.ActiveOnly {
		now := time.Now()
		query = query.Where("enabled = ? AND starts_at <= ?", true, now)
		query = query.Where("ends_at IS NULL OR ends_at > ?", now)
	}
	
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	
	var silences []*model.AlertSilence
	query = query.Order("created_at DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit).Offset(filter.Offset)
	}
	
	err := query.Find(&silences).Error
	return silences, total, err
}

func (f *AlertFacade) ListActiveSilences(ctx context.Context, now time.Time, clusterName string) ([]*model.AlertSilence, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Where("enabled = ? AND starts_at <= ?", true, now)
	query = query.Where("ends_at IS NULL OR ends_at > ?", now)
	
	if clusterName != "" {
		query = query.Where("cluster_name = ? OR cluster_name = ''", clusterName)
	}
	
	var silences []*model.AlertSilence
	err := query.Find(&silences).Error
	return silences, err
}

func (f *AlertFacade) DeleteAlertSilence(ctx context.Context, id string) error {
	db := f.getDB().WithContext(ctx)
	return db.Delete(&model.AlertSilence{}, "id = ?", id).Error
}

func (f *AlertFacade) DisableAlertSilence(ctx context.Context, id string) error {
	db := f.getDB().WithContext(ctx)
	return db.Model(&model.AlertSilence{}).Where("id = ?", id).Update("enabled", false).Error
}

// SilencedAlert operation implementations

func (f *AlertFacade) CreateSilencedAlert(ctx context.Context, silencedAlert *model.SilencedAlert) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(silencedAlert).Error
}

func (f *AlertFacade) ListSilencedAlerts(ctx context.Context, filter *SilencedAlertFilter) ([]*model.SilencedAlert, int64, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.SilencedAlert{})
	
	if filter.SilenceID != nil {
		query = query.Where("silence_id = ?", *filter.SilenceID)
	}
	if filter.AlertName != nil {
		query = query.Where("alert_name = ?", *filter.AlertName)
	}
	if filter.ClusterName != nil {
		query = query.Where("cluster_name = ?", *filter.ClusterName)
	}
	if filter.StartTime != nil {
		query = query.Where("silenced_at >= ?", *filter.StartTime)
	}
	if filter.EndTime != nil {
		query = query.Where("silenced_at < ?", *filter.EndTime)
	}
	
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	
	var silencedAlerts []*model.SilencedAlert
	query = query.Order("silenced_at DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit).Offset(filter.Offset)
	}
	
	err := query.Find(&silencedAlerts).Error
	return silencedAlerts, total, err
}

// AlertNotification operation implementations

func (f *AlertFacade) CreateAlertNotification(ctx context.Context, notification *model.AlertNotification) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(notification).Error
}

func (f *AlertFacade) UpdateAlertNotification(ctx context.Context, notification *model.AlertNotification) error {
	db := f.getDB().WithContext(ctx)
	return db.Save(notification).Error
}

func (f *AlertFacade) ListAlertNotificationsByAlertID(ctx context.Context, alertID string) ([]*model.AlertNotification, error) {
	db := f.getDB().WithContext(ctx)
	var notifications []*model.AlertNotification
	err := db.Where("alert_id = ?", alertID).Order("created_at DESC").Find(&notifications).Error
	return notifications, err
}

func (f *AlertFacade) ListPendingNotifications(ctx context.Context, limit int) ([]*model.AlertNotification, error) {
	db := f.getDB().WithContext(ctx)
	var notifications []*model.AlertNotification
	query := db.Where("status = ?", "pending").Order("created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&notifications).Error
	return notifications, err
}

