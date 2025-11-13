package database

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// mockAlertFacade creates an AlertFacade with the test database
type mockAlertFacade struct {
	AlertFacade
	db *gorm.DB
}

func (f *mockAlertFacade) getDB() *gorm.DB {
	return f.db
}

// newTestAlertFacade creates a test AlertFacade
func newTestAlertFacade(db *gorm.DB) AlertFacadeInterface {
	return &mockAlertFacade{
		db: db,
	}
}

// ==================== AlertEvents Tests ====================

func TestAlertFacade_CreateAlertEvents(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	alert := &model.AlertEvents{
		ID:          "alert-001",
		Source:      "metric",
		AlertName:   "HighCPUUsage",
		Severity:    "critical",
		Status:      "firing",
		StartsAt:    time.Now(),
		Labels:      model.ExtType{"host": "node-1"},
		Annotations: model.ExtType{"description": "CPU usage exceeds 90%"},
		ClusterName: "test-cluster",
	}
	
	err := facade.CreateAlertEvents(ctx, alert)
	require.NoError(t, err)
	assert.NotZero(t, alert.CreatedAt)
}

func TestAlertFacade_GetAlertEventsByID(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create an alert
	alert := &model.AlertEvents{
		ID:        "alert-002",
		Source:    "log",
		AlertName: "ErrorRate",
		Severity:  "high",
		Status:    "firing",
		StartsAt:  time.Now(),
		Labels:    model.ExtType{},
	}
	err := facade.CreateAlertEvents(ctx, alert)
	require.NoError(t, err)
	
	// Get the alert
	result, err := facade.GetAlertEventsByID(ctx, "alert-002")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, alert.ID, result.ID)
	assert.Equal(t, alert.AlertName, result.AlertName)
	assert.Equal(t, alert.Severity, result.Severity)
}

func TestAlertFacade_GetAlertEventsByID_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	result, err := facade.GetAlertEventsByID(ctx, "non-existent")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestAlertFacade_ListAlertEventss(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create multiple alerts
	alerts := []*model.AlertEvents{
		{ID: "alert-1", Source: "metric", AlertName: "CPU", Severity: "critical", Status: "firing", StartsAt: time.Now(), Labels: model.ExtType{}},
		{ID: "alert-2", Source: "metric", AlertName: "Memory", Severity: "high", Status: "firing", StartsAt: time.Now(), Labels: model.ExtType{}},
		{ID: "alert-3", Source: "log", AlertName: "Error", Severity: "warning", Status: "resolved", StartsAt: time.Now(), Labels: model.ExtType{}},
	}
	for _, a := range alerts {
		err := facade.CreateAlertEvents(ctx, a)
		require.NoError(t, err)
	}
	
	// Test filtering
	tests := []struct {
		name          string
		filter        *AlertEventsFilter
		expectedCount int
	}{
		{
			name:          "No filter",
			filter:        &AlertEventsFilter{},
			expectedCount: 3,
		},
		{
			name: "Filter by source",
			filter: &AlertEventsFilter{
				Source: stringPtr("metric"),
			},
			expectedCount: 2,
		},
		{
			name: "Filter by severity",
			filter: &AlertEventsFilter{
				Severity: stringPtr("critical"),
			},
			expectedCount: 1,
		},
		{
			name: "Filter by status",
			filter: &AlertEventsFilter{
				Status: stringPtr("firing"),
			},
			expectedCount: 2,
		},
		{
			name: "With pagination",
			filter: &AlertEventsFilter{
				Limit:  2,
				Offset: 0,
			},
			expectedCount: 2,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := facade.ListAlertEventss(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.expectedCount)
			assert.Equal(t, int64(3), total)
		})
	}
}

func TestAlertFacade_UpdateAlertStatus(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create an alert
	alert := &model.AlertEvents{
		ID:        "alert-status",
		Source:    "metric",
		AlertName: "Test",
		Severity:  "high",
		Status:    "firing",
		StartsAt:  time.Now(),
		Labels:    model.ExtType{},
	}
	err := facade.CreateAlertEvents(ctx, alert)
	require.NoError(t, err)
	
	// Update status
	endsAt := time.Now()
	err = facade.UpdateAlertStatus(ctx, "alert-status", "resolved", &endsAt)
	require.NoError(t, err)
	
	// Verify update
	result, err := facade.GetAlertEventsByID(ctx, "alert-status")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "resolved", result.Status)
	assert.False(t, result.EndsAt.IsZero())
}

// ==================== AlertRules Tests ====================

func TestAlertFacade_CreateAlertRules(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.AlertRules{
		Name:       "CPUHighUsage",
		Source:     "prometheus",
		Enabled:    true,
		RuleType:   "threshold",
		RuleConfig: model.ExtType{"threshold": float64(90)},
		Severity:   "critical",
		Labels:     model.ExtType{"team": "platform"},
	}
	
	err := facade.CreateAlertRules(ctx, rule)
	require.NoError(t, err)
	assert.NotZero(t, rule.ID)
}

func TestAlertFacade_GetAlertRulesByName(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.AlertRules{
		Name:       "DiskFull",
		Source:     "prometheus",
		RuleType:   "threshold",
		RuleConfig: model.ExtType{},
	}
	err := facade.CreateAlertRules(ctx, rule)
	require.NoError(t, err)
	
	result, err := facade.GetAlertRulesByName(ctx, "DiskFull")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "DiskFull", result.Name)
}

func TestAlertFacade_ListAlertRuless(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create rules
	rules := []*model.AlertRules{
		{Name: "Rule1", Source: "prometheus", RuleType: "threshold", RuleConfig: model.ExtType{}, Enabled: true},
		{Name: "Rule2", Source: "prometheus", RuleType: "threshold", RuleConfig: model.ExtType{}, Enabled: false},
		{Name: "Rule3", Source: "loki", RuleType: "pattern", RuleConfig: model.ExtType{}, Enabled: true},
	}
	for _, r := range rules {
		err := facade.CreateAlertRules(ctx, r)
		require.NoError(t, err)
	}
	
	// List all enabled rules
	enabled := true
	results, err := facade.ListAlertRuless(ctx, "", &enabled)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	// List prometheus rules
	results, err = facade.ListAlertRuless(ctx, "prometheus", nil)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestAlertFacade_DeleteAlertRules(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.AlertRules{
		Name:       "DeleteMe",
		Source:     "prometheus",
		RuleType:   "threshold",
		RuleConfig: model.ExtType{},
	}
	err := facade.CreateAlertRules(ctx, rule)
	require.NoError(t, err)
	
	// Delete the rule
	err = facade.DeleteAlertRules(ctx, rule.ID)
	require.NoError(t, err)
	
	// Verify deletion
	result, err := facade.GetAlertRulesByID(ctx, rule.ID)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// ==================== AlertSilences Tests ====================

func TestAlertFacade_CreateAlertSilences(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	silence := &model.AlertSilences{
		ID:          "silence-001",
		ClusterName: "test-cluster",
		SilenceType: "alert_name",
		Enabled:     true,
		StartsAt:    time.Now(),
	}
	
	err := facade.CreateAlertSilences(ctx, silence)
	require.NoError(t, err)
	assert.NotZero(t, silence.CreatedAt)
}

func TestAlertFacade_ListActiveSilences(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	now := time.Now()
	
	// Create active silence
	activeSilence := &model.AlertSilences{
		ID:          "active",
		ClusterName: "cluster1",
		SilenceType: "alert_name",
		Enabled:     true,
		StartsAt:    now.Add(-1 * time.Hour),
	}
	err := facade.CreateAlertSilences(ctx, activeSilence)
	require.NoError(t, err)
	
	// Create inactive silence (disabled)
	inactiveSilence := &model.AlertSilences{
		ID:          "inactive",
		ClusterName: "cluster1",
		SilenceType: "alert_name",
		Enabled:     false,
		StartsAt:    now.Add(-1 * time.Hour),
	}
	err = facade.CreateAlertSilences(ctx, inactiveSilence)
	require.NoError(t, err)
	
	// List active silences
	results, err := facade.ListActiveSilences(ctx, now, "cluster1")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "active", results[0].ID)
}

func TestAlertFacade_DisableAlertSilences(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	silence := &model.AlertSilences{
		ID:          "disable-me",
		ClusterName: "test",
		SilenceType: "alert_name",
		Enabled:     true,
		StartsAt:    time.Now(),
	}
	err := facade.CreateAlertSilences(ctx, silence)
	require.NoError(t, err)
	
	// Disable the silence
	err = facade.DisableAlertSilences(ctx, "disable-me")
	require.NoError(t, err)
	
	// Verify disabled
	result, err := facade.GetAlertSilencesByID(ctx, "disable-me")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Enabled)
}

// ==================== Helper Methods ====================

func TestAlertFacade_WithCluster(t *testing.T) {
	facade := NewAlertFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*AlertFacadeInterface)(nil), clusterFacade)
}

// ==================== Benchmark Tests ====================

func BenchmarkAlertFacade_CreateAlertEvents(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		alert := &model.AlertEvents{
			ID:        "bench-alert",
			Source:    "metric",
			AlertName: "Test",
			Severity:  "high",
			Status:    "firing",
			StartsAt:  time.Now(),
			Labels:    model.ExtType{},
		}
		_ = facade.CreateAlertEvents(ctx, alert)
	}
}

func BenchmarkAlertFacade_ListAlertEventss(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	for i := 0; i < 50; i++ {
		alert := &model.AlertEvents{
			ID:        "alert-" + string(rune('0'+i%10)),
			Source:    "metric",
			AlertName: "Test",
			Severity:  "high",
			Status:    "firing",
			StartsAt:  time.Now(),
			Labels:    model.ExtType{},
		}
		_ = facade.CreateAlertEvents(ctx, alert)
	}
	
	filter := &AlertEventsFilter{Limit: 10}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _, _ = facade.ListAlertEventss(ctx, filter)
	}
}

func BenchmarkAlertFacade_CreateAlertRules(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestAlertFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		rule := &model.AlertRules{
			Name:       "BenchRule",
			Source:     "prometheus",
			RuleType:   "threshold",
			RuleConfig: model.ExtType{},
		}
		_ = facade.CreateAlertRules(ctx, rule)
	}
}

