package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// AlertFacadeInterface defines the database operation interface for Alert system
type AlertFacadeInterface interface {
	// AlertEvents operations
	CreateAlertEvents(ctx context.Context, alert *model.AlertEvents) error
	UpdateAlertEvents(ctx context.Context, alert *model.AlertEvents) error
	GetAlertEventsByID(ctx context.Context, id string) (*model.AlertEvents, error)
	ListAlertEventss(ctx context.Context, filter *AlertEventsFilter) ([]*model.AlertEvents, int64, error)
	UpdateAlertStatus(ctx context.Context, id string, status string, endsAt *time.Time) error
	DeleteOldAlertEventss(ctx context.Context, before time.Time) error

	// AlertCorrelations operations
	CreateAlertCorrelations(ctx context.Context, correlation *model.AlertCorrelations) error
	ListAlertCorrelationssByCorrelationID(ctx context.Context, correlationID string) ([]*model.AlertCorrelations, error)
	ListAlertCorrelationssByAlertID(ctx context.Context, alertID string) ([]*model.AlertCorrelations, error)

	// AlertStatistics operations
	CreateOrUpdateAlertStatistics(ctx context.Context, stat *model.AlertStatistics) error
	GetAlertStatistics(ctx context.Context, date time.Time, hour *int, alertName, source, workloadID, clusterName string) (*model.AlertStatistics, error)
	ListAlertStatisticss(ctx context.Context, filter *AlertStatisticsFilter) ([]*model.AlertStatistics, error)

	// AlertRules operations
	CreateAlertRules(ctx context.Context, rule *model.AlertRules) error
	UpdateAlertRules(ctx context.Context, rule *model.AlertRules) error
	GetAlertRulesByID(ctx context.Context, id int64) (*model.AlertRules, error)
	GetAlertRulesByName(ctx context.Context, name string) (*model.AlertRules, error)
	ListAlertRuless(ctx context.Context, source string, enabled *bool) ([]*model.AlertRules, error)
	DeleteAlertRules(ctx context.Context, id int64) error

	// AlertSilences operations
	CreateAlertSilences(ctx context.Context, silence *model.AlertSilences) error
	UpdateAlertSilences(ctx context.Context, silence *model.AlertSilences) error
	GetAlertSilencesByID(ctx context.Context, id string) (*model.AlertSilences, error)
	ListAlertSilencess(ctx context.Context, filter *AlertSilencesFilter) ([]*model.AlertSilences, int64, error)
	ListActiveSilences(ctx context.Context, now time.Time, clusterName string) ([]*model.AlertSilences, error)
	DeleteAlertSilences(ctx context.Context, id string) error
	DisableAlertSilences(ctx context.Context, id string) error

	// SilencedAlerts operations (audit trail)
	CreateSilencedAlerts(ctx context.Context, SilencedAlerts *model.SilencedAlerts) error
	ListSilencedAlertss(ctx context.Context, filter *SilencedAlertsFilter) ([]*model.SilencedAlerts, int64, error)

	// AlertNotifications operations
	CreateAlertNotifications(ctx context.Context, notification *model.AlertNotifications) error
	UpdateAlertNotifications(ctx context.Context, notification *model.AlertNotifications) error
	ListAlertNotificationssByAlertID(ctx context.Context, alertID string) ([]*model.AlertNotifications, error)
	ListPendingNotifications(ctx context.Context, limit int) ([]*model.AlertNotifications, error)

	// WithCluster method
	WithCluster(clusterName string) AlertFacadeInterface
}

// AlertEventsFilter defines filter conditions for querying alert events
type AlertEventsFilter struct {
	Source       *string
	AlertName    *string
	Severity     *string
	Status       *string
	WorkloadID   *string
	PodName      *string
	NodeName     *string
	ClusterName  *string
	StartsAfter  *time.Time
	StartsBefore *time.Time
	Offset       int
	Limit        int
}

// AlertStatisticsFilter defines filter conditions for querying alert statistics
type AlertStatisticsFilter struct {
	DateFrom    *time.Time
	DateTo      *time.Time
	AlertName   *string
	Source      *string
	WorkloadID  *string
	ClusterName *string
	Offset      int
	Limit       int
}

// AlertSilencesFilter defines filter conditions for querying alert silences
type AlertSilencesFilter struct {
	ClusterName *string
	SilenceType *string
	Enabled     *bool
	ActiveOnly  bool // Only return silences active at current time
	Offset      int
	Limit       int
}

// SilencedAlertsFilter defines filter conditions for querying silenced alerts
type SilencedAlertsFilter struct {
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

// AlertEvents operation implementations

func (f *AlertFacade) CreateAlertEvents(ctx context.Context, alert *model.AlertEvents) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(alert).Error
}

func (f *AlertFacade) UpdateAlertEvents(ctx context.Context, alert *model.AlertEvents) error {
	db := f.getDB().WithContext(ctx)
	return db.Save(alert).Error
}

func (f *AlertFacade) GetAlertEventsByID(ctx context.Context, id string) (*model.AlertEvents, error) {
	db := f.getDB().WithContext(ctx)
	var alert model.AlertEvents
	err := db.Where("id = ?", id).First(&alert).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if alert.ID == "" {
		return nil, nil
	}
	return &alert, nil
}

func (f *AlertFacade) ListAlertEventss(ctx context.Context, filter *AlertEventsFilter) ([]*model.AlertEvents, int64, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.AlertEvents{})

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

	var alerts []*model.AlertEvents
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
	return db.Model(&model.AlertEvents{}).Where("id = ?", id).Updates(updates).Error
}

func (f *AlertFacade) DeleteOldAlertEventss(ctx context.Context, before time.Time) error {
	db := f.getDB().WithContext(ctx)
	return db.Where("starts_at < ?", before).Delete(&model.AlertEvents{}).Error
}

// AlertCorrelations operation implementations

func (f *AlertFacade) CreateAlertCorrelations(ctx context.Context, correlation *model.AlertCorrelations) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(correlation).Error
}

func (f *AlertFacade) ListAlertCorrelationssByCorrelationID(ctx context.Context, correlationID string) ([]*model.AlertCorrelations, error) {
	db := f.getDB().WithContext(ctx)
	var correlations []*model.AlertCorrelations
	err := db.Where("correlation_id = ?", correlationID).Find(&correlations).Error
	return correlations, err
}

func (f *AlertFacade) ListAlertCorrelationssByAlertID(ctx context.Context, alertID string) ([]*model.AlertCorrelations, error) {
	db := f.getDB().WithContext(ctx)
	var correlations []*model.AlertCorrelations
	err := db.Where("alert_id = ?", alertID).Find(&correlations).Error
	return correlations, err
}

// AlertStatistics operation implementations

func (f *AlertFacade) CreateOrUpdateAlertStatistics(ctx context.Context, stat *model.AlertStatistics) error {
	db := f.getDB().WithContext(ctx)

	// Try to find existing record
	var existing model.AlertStatistics
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

func (f *AlertFacade) GetAlertStatistics(ctx context.Context, date time.Time, hour *int, alertName, source, workloadID, clusterName string) (*model.AlertStatistics, error) {
	db := f.getDB().WithContext(ctx)
	var stat model.AlertStatistics

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
	if stat.ID == 0 {
		return nil, nil
	}
	return &stat, nil
}

func (f *AlertFacade) ListAlertStatisticss(ctx context.Context, filter *AlertStatisticsFilter) ([]*model.AlertStatistics, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.AlertStatistics{})

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

	var stats []*model.AlertStatistics
	query = query.Order("date DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit).Offset(filter.Offset)
	}

	err := query.Find(&stats).Error
	return stats, err
}

// AlertRules operation implementations

func (f *AlertFacade) CreateAlertRules(ctx context.Context, rule *model.AlertRules) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(rule).Error
}

func (f *AlertFacade) UpdateAlertRules(ctx context.Context, rule *model.AlertRules) error {
	db := f.getDB().WithContext(ctx)
	return db.Save(rule).Error
}

func (f *AlertFacade) GetAlertRulesByID(ctx context.Context, id int64) (*model.AlertRules, error) {
	db := f.getDB().WithContext(ctx)
	var rule model.AlertRules
	err := db.Where("id = ?", id).First(&rule).Error
	if err != nil {
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

func (f *AlertFacade) GetAlertRulesByName(ctx context.Context, name string) (*model.AlertRules, error) {
	db := f.getDB().WithContext(ctx)
	var rule model.AlertRules
	err := db.Where("name = ?", name).First(&rule).Error
	if err != nil {
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

func (f *AlertFacade) ListAlertRuless(ctx context.Context, source string, enabled *bool) ([]*model.AlertRules, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.AlertRules{})

	if source != "" {
		query = query.Where("source = ?", source)
	}
	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}

	var rules []*model.AlertRules
	err := query.Order("created_at DESC").Find(&rules).Error
	return rules, err
}

func (f *AlertFacade) DeleteAlertRules(ctx context.Context, id int64) error {
	db := f.getDB().WithContext(ctx)
	return db.Delete(&model.AlertRules{}, id).Error
}

// AlertSilences operation implementations

func (f *AlertFacade) CreateAlertSilences(ctx context.Context, silence *model.AlertSilences) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(silence).Error
}

func (f *AlertFacade) UpdateAlertSilences(ctx context.Context, silence *model.AlertSilences) error {
	db := f.getDB().WithContext(ctx)
	return db.Save(silence).Error
}

func (f *AlertFacade) GetAlertSilencesByID(ctx context.Context, id string) (*model.AlertSilences, error) {
	db := f.getDB().WithContext(ctx)
	var silence model.AlertSilences
	err := db.Where("id = ?", id).First(&silence).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if silence.ID == "" {
		return nil, nil
	}
	return &silence, nil
}

func (f *AlertFacade) ListAlertSilencess(ctx context.Context, filter *AlertSilencesFilter) ([]*model.AlertSilences, int64, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.AlertSilences{})

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

	var silences []*model.AlertSilences
	query = query.Order("created_at DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit).Offset(filter.Offset)
	}

	err := query.Find(&silences).Error
	return silences, total, err
}

func (f *AlertFacade) ListActiveSilences(ctx context.Context, now time.Time, clusterName string) ([]*model.AlertSilences, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Where("enabled = ? AND starts_at <= ?", true, now)
	query = query.Where("ends_at IS NULL OR ends_at > ?", now)

	if clusterName != "" {
		query = query.Where("cluster_name = ? OR cluster_name = ''", clusterName)
	}

	var silences []*model.AlertSilences
	err := query.Find(&silences).Error
	return silences, err
}

func (f *AlertFacade) DeleteAlertSilences(ctx context.Context, id string) error {
	db := f.getDB().WithContext(ctx)
	return db.Delete(&model.AlertSilences{}, "id = ?", id).Error
}

func (f *AlertFacade) DisableAlertSilences(ctx context.Context, id string) error {
	db := f.getDB().WithContext(ctx)
	return db.Model(&model.AlertSilences{}).Where("id = ?", id).Update("enabled", false).Error
}

// SilencedAlerts operation implementations

func (f *AlertFacade) CreateSilencedAlerts(ctx context.Context, SilencedAlerts *model.SilencedAlerts) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(SilencedAlerts).Error
}

func (f *AlertFacade) ListSilencedAlertss(ctx context.Context, filter *SilencedAlertsFilter) ([]*model.SilencedAlerts, int64, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.SilencedAlerts{})

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

	var SilencedAlertss []*model.SilencedAlerts
	query = query.Order("silenced_at DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit).Offset(filter.Offset)
	}

	err := query.Find(&SilencedAlertss).Error
	return SilencedAlertss, total, err
}

// AlertNotifications operation implementations

func (f *AlertFacade) CreateAlertNotifications(ctx context.Context, notification *model.AlertNotifications) error {
	db := f.getDB().WithContext(ctx)
	return db.Create(notification).Error
}

func (f *AlertFacade) UpdateAlertNotifications(ctx context.Context, notification *model.AlertNotifications) error {
	db := f.getDB().WithContext(ctx)
	return db.Save(notification).Error
}

func (f *AlertFacade) ListAlertNotificationssByAlertID(ctx context.Context, alertID string) ([]*model.AlertNotifications, error) {
	db := f.getDB().WithContext(ctx)
	var notifications []*model.AlertNotifications
	err := db.Where("alert_id = ?", alertID).Order("created_at DESC").Find(&notifications).Error
	return notifications, err
}

func (f *AlertFacade) ListPendingNotifications(ctx context.Context, limit int) ([]*model.AlertNotifications, error) {
	db := f.getDB().WithContext(ctx)
	var notifications []*model.AlertNotifications
	query := db.Where("status = ?", "pending").Order("created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&notifications).Error
	return notifications, err
}
