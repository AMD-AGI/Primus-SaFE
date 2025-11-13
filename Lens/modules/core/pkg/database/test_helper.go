package database

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestHelper provides common test utilities for database tests
type TestHelper struct {
	DB *gorm.DB
	T  *testing.T
}

// NewTestHelper creates a new TestHelper with an in-memory SQLite database
func NewTestHelper(t *testing.T) *TestHelper {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Silent mode to reduce noise
	})
	require.NoError(t, err, "Failed to open SQLite database")

	// Auto-migrate all models
	err = db.AutoMigrate(
		&model.Node{},
		&model.GpuDevice{},
		&model.RdmaDevice{},
		&model.NodeDeviceChangelog{},
		&model.Storage{},
		&model.GenericCache{},
		&model.SystemConfig{},
		&model.SystemConfigHistory{},
		&model.AlertEvents{},
		&model.AlertCorrelations{},
		&model.AlertStatistics{},
		&model.AlertRules{},
		&model.AlertSilences{},
		&model.SilencedAlerts{},
		&model.AlertNotifications{},
		&model.AlertRuleAdvices{},
		&model.ClusterOverviewCache{},
		&model.NodeContainer{},
		&model.NodeContainerDevices{},
		&model.NodeContainerEvent{},
		&model.ClusterGpuHourlyStats{},
		&model.NamespaceGpuHourlyStats{},
		&model.LabelGpuHourlyStats{},
		&model.WorkloadGpuHourlyStats{},
		&model.GpuAllocationSnapshots{},
		&model.JobExecutionHistory{},
		&model.LogAlertRules{},
		&model.LogAlertRuleVersions{},
		&model.LogAlertRuleStatistics{},
		&model.LogAlertRuleTemplates{},
		&model.MetricAlertRules{},
		// Add more models as needed
	)
	require.NoError(t, err, "Failed to auto-migrate models")

	return &TestHelper{
		DB: db,
		T:  t,
	}
}

// Cleanup closes the database connection
func (h *TestHelper) Cleanup() {
	sqlDB, err := h.DB.DB()
	if err == nil {
		sqlDB.Close()
	}
}

// CreateTestContext creates a test context
func (h *TestHelper) CreateTestContext() context.Context {
	return context.Background()
}

// TruncateTable truncates a table for clean test state
func (h *TestHelper) TruncateTable(tableName string) {
	h.DB.Exec("DELETE FROM " + tableName)
}

// Count returns the number of records in a table
func (h *TestHelper) Count(tableName string) int64 {
	var count int64
	h.DB.Table(tableName).Count(&count)
	return count
}

