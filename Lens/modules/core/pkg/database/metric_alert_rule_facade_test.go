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

// mockMetricAlertRuleFacade creates a MetricAlertRuleFacade with the test database
type mockMetricAlertRuleFacade struct {
	MetricAlertRuleFacade
	db *gorm.DB
}

func (f *mockMetricAlertRuleFacade) getDB() *gorm.DB {
	return f.db
}

// newTestMetricAlertRuleFacade creates a test MetricAlertRuleFacade
func newTestMetricAlertRuleFacade(db *gorm.DB) MetricAlertRuleFacadeInterface {
	return &mockMetricAlertRuleFacade{
		db: db,
	}
}

// TestMetricAlertRuleFacade_CreateMetricAlertRule tests creating a metric alert rule
func TestMetricAlertRuleFacade_CreateMetricAlertRule(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.MetricAlertRules{
		Name:        "high-cpu-usage",
		ClusterName: "test-cluster",
		Enabled:     true,
		Groups: model.ExtType{
			"name": "alerts",
			"rules": []interface{}{
				map[string]interface{}{
					"alert": "HighCPU",
					"expr":  "cpu_usage > 90",
				},
			},
		},
		Description: "Alert for high CPU usage",
		SyncStatus:  "pending",
	}
	
	err := facade.CreateMetricAlertRule(ctx, rule)
	require.NoError(t, err)
	assert.NotZero(t, rule.ID)
}

// TestMetricAlertRuleFacade_GetMetricAlertRuleByID tests getting rule by ID
func TestMetricAlertRuleFacade_GetMetricAlertRuleByID(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.MetricAlertRules{
		Name:        "test-rule",
		ClusterName: "test-cluster",
		Groups:      model.ExtType{},
	}
	err := facade.CreateMetricAlertRule(ctx, rule)
	require.NoError(t, err)
	
	result, err := facade.GetMetricAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, rule.Name, result.Name)
	assert.Equal(t, rule.ClusterName, result.ClusterName)
}

// TestMetricAlertRuleFacade_GetMetricAlertRuleByID_NotFound tests getting non-existent rule
func TestMetricAlertRuleFacade_GetMetricAlertRuleByID_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	result, err := facade.GetMetricAlertRuleByID(ctx, 99999)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestMetricAlertRuleFacade_GetMetricAlertRuleByNameAndCluster tests getting rule by name and cluster
func TestMetricAlertRuleFacade_GetMetricAlertRuleByNameAndCluster(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.MetricAlertRules{
		Name:        "unique-rule",
		ClusterName: "prod-cluster",
		Groups:      model.ExtType{},
	}
	err := facade.CreateMetricAlertRule(ctx, rule)
	require.NoError(t, err)
	
	result, err := facade.GetMetricAlertRuleByNameAndCluster(ctx, "unique-rule", "prod-cluster")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, rule.Name, result.Name)
	assert.Equal(t, rule.ClusterName, result.ClusterName)
}

// TestMetricAlertRuleFacade_ListMetricAlertRules tests listing with filters
func TestMetricAlertRuleFacade_ListMetricAlertRules(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create multiple rules
	rules := []*model.MetricAlertRules{
		{Name: "rule-1", ClusterName: "cluster-1", Enabled: true, SyncStatus: "synced", Groups: model.ExtType{}},
		{Name: "rule-2", ClusterName: "cluster-1", Enabled: false, SyncStatus: "pending", Groups: model.ExtType{}},
		{Name: "rule-3", ClusterName: "cluster-2", Enabled: true, SyncStatus: "synced", Groups: model.ExtType{}},
	}
	for _, r := range rules {
		err := facade.CreateMetricAlertRule(ctx, r)
		require.NoError(t, err)
	}
	
	tests := []struct {
		name          string
		filter        *MetricAlertRuleFilter
		expectedCount int
	}{
		{
			name:          "No filter",
			filter:        &MetricAlertRuleFilter{},
			expectedCount: 3,
		},
		{
			name: "Filter by name",
			filter: &MetricAlertRuleFilter{
				Name: stringPtr("rule-1"),
			},
			expectedCount: 1,
		},
		{
			name: "Filter by cluster",
			filter: &MetricAlertRuleFilter{
				ClusterName: stringPtr("cluster-1"),
			},
			expectedCount: 2,
		},
		{
			name: "Filter by enabled",
			filter: &MetricAlertRuleFilter{
				Enabled: boolPtr(true),
			},
			expectedCount: 2,
		},
		{
			name: "Filter by sync status",
			filter: &MetricAlertRuleFilter{
				SyncStatus: stringPtr("synced"),
			},
			expectedCount: 2,
		},
		{
			name: "With pagination",
			filter: &MetricAlertRuleFilter{
				Limit:  2,
				Offset: 0,
			},
			expectedCount: 2,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := facade.ListMetricAlertRules(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.expectedCount)
			assert.Equal(t, int64(3), total)
		})
	}
}

// TestMetricAlertRuleFacade_UpdateMetricAlertRule tests updating a rule
func TestMetricAlertRuleFacade_UpdateMetricAlertRule(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.MetricAlertRules{
		Name:        "update-test",
		ClusterName: "test-cluster",
		Enabled:     true,
		SyncStatus:  "pending",
		Groups:      model.ExtType{},
	}
	err := facade.CreateMetricAlertRule(ctx, rule)
	require.NoError(t, err)
	
	// Update the rule
	rule.Enabled = false
	rule.SyncStatus = "synced"
	err = facade.UpdateMetricAlertRule(ctx, rule)
	require.NoError(t, err)
	
	// Verify update
	result, err := facade.GetMetricAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Enabled)
	assert.Equal(t, "synced", result.SyncStatus)
}

// TestMetricAlertRuleFacade_DeleteMetricAlertRule tests deleting a rule
func TestMetricAlertRuleFacade_DeleteMetricAlertRule(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.MetricAlertRules{
		Name:        "delete-test",
		ClusterName: "test-cluster",
		Groups:      model.ExtType{},
	}
	err := facade.CreateMetricAlertRule(ctx, rule)
	require.NoError(t, err)
	
	// Delete the rule
	err = facade.DeleteMetricAlertRule(ctx, rule.ID)
	require.NoError(t, err)
	
	// Verify deletion
	result, err := facade.GetMetricAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestMetricAlertRuleFacade_UpdateSyncStatus tests updating sync status
func TestMetricAlertRuleFacade_UpdateSyncStatus(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.MetricAlertRules{
		Name:        "sync-test",
		ClusterName: "test-cluster",
		SyncStatus:  "pending",
		Groups:      model.ExtType{},
	}
	err := facade.CreateMetricAlertRule(ctx, rule)
	require.NoError(t, err)
	
	// Update sync status
	err = facade.UpdateSyncStatus(ctx, rule.ID, "synced", "Successfully synced")
	require.NoError(t, err)
	
	// Verify update
	result, err := facade.GetMetricAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "synced", result.SyncStatus)
	assert.Equal(t, "Successfully synced", result.SyncMessage)
	assert.False(t, result.LastSyncAt.IsZero())
}

// TestMetricAlertRuleFacade_UpdateVMRuleStatus tests updating VMRule status
func TestMetricAlertRuleFacade_UpdateVMRuleStatus(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.MetricAlertRules{
		Name:        "vmrule-test",
		ClusterName: "test-cluster",
		Groups:      model.ExtType{},
	}
	err := facade.CreateMetricAlertRule(ctx, rule)
	require.NoError(t, err)
	
	// Update VMRule status
	vmruleStatus := model.ExtType{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Ready",
				"status": "True",
			},
		},
	}
	err = facade.UpdateVMRuleStatus(ctx, rule.ID, vmruleStatus)
	require.NoError(t, err)
	
	// Verify update
	result, err := facade.GetMetricAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.VmruleStatus)
}

// TestMetricAlertRuleFacade_ListRulesByCluster tests listing rules by cluster
func TestMetricAlertRuleFacade_ListRulesByCluster(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create rules for different clusters
	rules := []*model.MetricAlertRules{
		{Name: "rule-1", ClusterName: "target-cluster", Enabled: true, Groups: model.ExtType{}},
		{Name: "rule-2", ClusterName: "target-cluster", Enabled: false, Groups: model.ExtType{}},
		{Name: "rule-3", ClusterName: "other-cluster", Enabled: true, Groups: model.ExtType{}},
	}
	for _, r := range rules {
		err := facade.CreateMetricAlertRule(ctx, r)
		require.NoError(t, err)
	}
	
	// List all rules for target cluster
	results, err := facade.ListRulesByCluster(ctx, "target-cluster", nil)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	// List only enabled rules for target cluster
	enabled := true
	results, err = facade.ListRulesByCluster(ctx, "target-cluster", &enabled)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Enabled)
}

// TestMetricAlertRuleFacade_ListPendingSyncRules tests listing pending sync rules
func TestMetricAlertRuleFacade_ListPendingSyncRules(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create rules with different sync statuses
	rules := []*model.MetricAlertRules{
		{Name: "pending-1", ClusterName: "cluster-1", Enabled: true, SyncStatus: "pending", Groups: model.ExtType{}},
		{Name: "synced-1", ClusterName: "cluster-1", Enabled: true, SyncStatus: "synced", Groups: model.ExtType{}},
		{Name: "pending-2", ClusterName: "cluster-2", Enabled: true, SyncStatus: "pending", Groups: model.ExtType{}},
		{Name: "pending-disabled", ClusterName: "cluster-2", Enabled: false, SyncStatus: "pending", Groups: model.ExtType{}},
	}
	for _, r := range rules {
		err := facade.CreateMetricAlertRule(ctx, r)
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond) // Ensure different creation times
	}
	
	// List pending sync rules (limit 1)
	results, err := facade.ListPendingSyncRules(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "pending", results[0].SyncStatus)
	assert.True(t, results[0].Enabled)
	
	// List all pending sync rules
	results, err = facade.ListPendingSyncRules(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, results, 2) // Only enabled and pending
}

// TestMetricAlertRuleFacade_WithCluster tests the WithCluster method
func TestMetricAlertRuleFacade_WithCluster(t *testing.T) {
	facade := NewMetricAlertRuleFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*MetricAlertRuleFacadeInterface)(nil), clusterFacade)
}

// ==================== Benchmark Tests ====================

func BenchmarkMetricAlertRuleFacade_CreateMetricAlertRule(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		rule := &model.MetricAlertRules{
			Name:        "bench-rule",
			ClusterName: "test-cluster",
			Groups:      model.ExtType{},
		}
		_ = facade.CreateMetricAlertRule(ctx, rule)
	}
}

func BenchmarkMetricAlertRuleFacade_ListMetricAlertRules(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	for i := 0; i < 50; i++ {
		rule := &model.MetricAlertRules{
			Name:        "bench-rule-" + string(rune('0'+i%10)),
			ClusterName: "test-cluster",
			Groups:      model.ExtType{},
		}
		_ = facade.CreateMetricAlertRule(ctx, rule)
	}
	
	filter := &MetricAlertRuleFilter{Limit: 10}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _, _ = facade.ListMetricAlertRules(ctx, filter)
	}
}

func BenchmarkMetricAlertRuleFacade_UpdateSyncStatus(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestMetricAlertRuleFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	rule := &model.MetricAlertRules{
		Name:        "bench-sync",
		ClusterName: "test-cluster",
		SyncStatus:  "pending",
		Groups:      model.ExtType{},
	}
	_ = facade.CreateMetricAlertRule(ctx, rule)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_ = facade.UpdateSyncStatus(ctx, rule.ID, "synced", "test")
	}
}

// Helper function to create bool pointer
func boolPtr(b bool) *bool {
	return &b
}

