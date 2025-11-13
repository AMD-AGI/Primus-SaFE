package database

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// mockLogAlertRuleFacade creates a LogAlertRuleFacade with the test database
type mockLogAlertRuleFacade struct {
	LogAlertRuleFacade
	db *gorm.DB
}

func (f *mockLogAlertRuleFacade) getDB() *gorm.DB {
	return f.db
}

// newTestLogAlertRuleFacade creates a test LogAlertRuleFacade
func newTestLogAlertRuleFacade(db *gorm.DB) LogAlertRuleFacadeInterface {
	return &mockLogAlertRuleFacade{
		db: db,
	}
}

// ==================== LogAlertRules Tests ====================

func TestLogAlertRuleFacade_CreateLogAlertRule(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.LogAlertRules{
		Name:           "error-detection",
		Description:    "Detect error logs",
		ClusterName:    "test-cluster",
		Enabled:        true,
		Priority:       8,
		LabelSelectors: model.ExtType{},
		MatchType:      "pattern",
		MatchConfig:    model.ExtType{"pattern": "ERROR|FATAL"},
		Severity:       "critical",
	}
	
	err := facade.CreateLogAlertRule(ctx, rule)
	require.NoError(t, err)
	assert.NotZero(t, rule.ID)
}

func TestLogAlertRuleFacade_GetLogAlertRuleByID(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.LogAlertRules{
		Name:           "test-rule",
		ClusterName:    "test-cluster",
		LabelSelectors: model.ExtType{},
		MatchType:      "pattern",
		MatchConfig:    model.ExtType{},
	}
	err := facade.CreateLogAlertRule(ctx, rule)
	require.NoError(t, err)
	
	result, err := facade.GetLogAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, rule.Name, result.Name)
	assert.Equal(t, rule.ClusterName, result.ClusterName)
}

func TestLogAlertRuleFacade_GetLogAlertRuleByName(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.LogAlertRules{
		Name:           "unique-rule",
		ClusterName:    "prod-cluster",
		LabelSelectors: model.ExtType{},
		MatchType:      "pattern",
		MatchConfig:    model.ExtType{},
	}
	err := facade.CreateLogAlertRule(ctx, rule)
	require.NoError(t, err)
	
	result, err := facade.GetLogAlertRuleByName(ctx, "prod-cluster", "unique-rule")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, rule.Name, result.Name)
}

func TestLogAlertRuleFacade_ListLogAlertRules(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create multiple rules
	rules := []*model.LogAlertRules{
		{Name: "rule-1", ClusterName: "cluster-1", Enabled: true, MatchType: "pattern", Severity: "critical", Priority: 8, LabelSelectors: model.ExtType{}, MatchConfig: model.ExtType{}, CreatedBy: "user1"},
		{Name: "rule-2", ClusterName: "cluster-1", Enabled: false, MatchType: "threshold", Severity: "warning", Priority: 5, LabelSelectors: model.ExtType{}, MatchConfig: model.ExtType{}, CreatedBy: "user1"},
		{Name: "rule-3", ClusterName: "cluster-2", Enabled: true, MatchType: "pattern", Severity: "critical", Priority: 8, LabelSelectors: model.ExtType{}, MatchConfig: model.ExtType{}, CreatedBy: "user2"},
	}
	for _, r := range rules {
		err := facade.CreateLogAlertRule(ctx, r)
		require.NoError(t, err)
	}
	
	tests := []struct {
		name          string
		filter        *LogAlertRuleFilter
		expectedCount int
	}{
		{
			name:          "No filter",
			filter:        &LogAlertRuleFilter{},
			expectedCount: 3,
		},
		{
			name: "Filter by cluster",
			filter: &LogAlertRuleFilter{
				ClusterName: "cluster-1",
			},
			expectedCount: 2,
		},
		{
			name: "Filter by enabled",
			filter: &LogAlertRuleFilter{
				Enabled: boolPtr(true),
			},
			expectedCount: 2,
		},
		{
			name: "Filter by match type",
			filter: &LogAlertRuleFilter{
				MatchType: "pattern",
			},
			expectedCount: 2,
		},
		{
			name: "Filter by severity",
			filter: &LogAlertRuleFilter{
				Severity: "critical",
			},
			expectedCount: 2,
		},
		{
			name: "Filter by priority",
			filter: &LogAlertRuleFilter{
				Priority: intPtr(8),
			},
			expectedCount: 2,
		},
		{
			name: "Filter by keyword",
			filter: &LogAlertRuleFilter{
				Keyword: "rule-1",
			},
			expectedCount: 1,
		},
		{
			name: "With pagination",
			filter: &LogAlertRuleFilter{
				Limit:  2,
				Offset: 0,
			},
			expectedCount: 2,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := facade.ListLogAlertRules(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.expectedCount)
			assert.Equal(t, int64(3), total)
		})
	}
}

func TestLogAlertRuleFacade_UpdateLogAlertRule(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.LogAlertRules{
		Name:           "update-test",
		ClusterName:    "test-cluster",
		Enabled:        true,
		Priority:       5,
		LabelSelectors: model.ExtType{},
		MatchType:      "pattern",
		MatchConfig:    model.ExtType{},
	}
	err := facade.CreateLogAlertRule(ctx, rule)
	require.NoError(t, err)
	
	// Update the rule
	rule.Priority = 8
	rule.Enabled = false
	err = facade.UpdateLogAlertRule(ctx, rule)
	require.NoError(t, err)
	
	// Verify update
	result, err := facade.GetLogAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(8), result.Priority)
	assert.False(t, result.Enabled)
}

func TestLogAlertRuleFacade_DeleteLogAlertRule(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.LogAlertRules{
		Name:           "delete-test",
		ClusterName:    "test-cluster",
		LabelSelectors: model.ExtType{},
		MatchType:      "pattern",
		MatchConfig:    model.ExtType{},
	}
	err := facade.CreateLogAlertRule(ctx, rule)
	require.NoError(t, err)
	
	// Delete the rule
	err = facade.DeleteLogAlertRule(ctx, rule.ID)
	require.NoError(t, err)
	
	// Verify deletion
	result, err := facade.GetLogAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestLogAlertRuleFacade_BatchUpdateEnabledStatus(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create multiple rules
	var ids []int64
	for i := 0; i < 3; i++ {
		rule := &model.LogAlertRules{
			Name:           "batch-rule-" + string(rune('a'+i)),
			ClusterName:    "test-cluster",
			Enabled:        true,
			LabelSelectors: model.ExtType{},
			MatchType:      "pattern",
			MatchConfig:    model.ExtType{},
		}
		err := facade.CreateLogAlertRule(ctx, rule)
		require.NoError(t, err)
		ids = append(ids, rule.ID)
	}
	
	// Batch update
	err := facade.BatchUpdateEnabledStatus(ctx, ids, false)
	require.NoError(t, err)
	
	// Verify all are disabled
	for _, id := range ids {
		result, err := facade.GetLogAlertRuleByID(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Enabled)
	}
}

func TestLogAlertRuleFacade_BatchDeleteLogAlertRules(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create multiple rules
	var ids []int64
	for i := 0; i < 3; i++ {
		rule := &model.LogAlertRules{
			Name:           "batch-delete-" + string(rune('a'+i)),
			ClusterName:    "test-cluster",
			LabelSelectors: model.ExtType{},
			MatchType:      "pattern",
			MatchConfig:    model.ExtType{},
		}
		err := facade.CreateLogAlertRule(ctx, rule)
		require.NoError(t, err)
		ids = append(ids, rule.ID)
	}
	
	// Batch delete
	err := facade.BatchDeleteLogAlertRules(ctx, ids)
	require.NoError(t, err)
	
	// Verify all are deleted
	for _, id := range ids {
		result, err := facade.GetLogAlertRuleByID(ctx, id)
		require.NoError(t, err)
		assert.Nil(t, result)
	}
}

func TestLogAlertRuleFacade_UpdateRuleTriggerInfo(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	rule := &model.LogAlertRules{
		Name:           "trigger-test",
		ClusterName:    "test-cluster",
		LabelSelectors: model.ExtType{},
		MatchType:      "pattern",
		MatchConfig:    model.ExtType{},
		TriggerCount:   0,
	}
	err := facade.CreateLogAlertRule(ctx, rule)
	require.NoError(t, err)
	
	// Update trigger info
	err = facade.UpdateRuleTriggerInfo(ctx, rule.ID)
	require.NoError(t, err)
	
	// Verify update
	result, err := facade.GetLogAlertRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.LastTriggeredAt.IsZero())
	assert.Equal(t, int64(1), result.TriggerCount)
}

// ==================== Template Tests ====================

func TestLogAlertRuleFacade_CreateLogAlertRuleTemplate(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	template := &model.LogAlertRuleTemplates{
		Name:           "error-template",
		Category:       "basic",
		Description:    "Generic error detection",
		TemplateConfig: model.ExtType{"pattern": "ERROR"},
		IsBuiltin:      false,
		CreatedBy:      "user1",
	}
	
	err := facade.CreateLogAlertRuleTemplate(ctx, template)
	require.NoError(t, err)
	assert.NotZero(t, template.ID)
}

func TestLogAlertRuleFacade_GetLogAlertRuleTemplateByID(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	template := &model.LogAlertRuleTemplates{
		Name:           "test-template",
		Category:       "test",
		TemplateConfig: model.ExtType{},
	}
	err := facade.CreateLogAlertRuleTemplate(ctx, template)
	require.NoError(t, err)
	
	result, err := facade.GetLogAlertRuleTemplateByID(ctx, template.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, template.Name, result.Name)
}

func TestLogAlertRuleFacade_ListLogAlertRuleTemplates(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create templates with different categories
	templates := []*model.LogAlertRuleTemplates{
		{Name: "tpl-1", Category: "basic", TemplateConfig: model.ExtType{}, UsageCount: 10},
		{Name: "tpl-2", Category: "gpu", TemplateConfig: model.ExtType{}, UsageCount: 5},
		{Name: "tpl-3", Category: "basic", TemplateConfig: model.ExtType{}, UsageCount: 20},
	}
	for _, tpl := range templates {
		err := facade.CreateLogAlertRuleTemplate(ctx, tpl)
		require.NoError(t, err)
	}
	
	// List all templates
	results, err := facade.ListLogAlertRuleTemplates(ctx, "")
	require.NoError(t, err)
	assert.Len(t, results, 3)
	
	// List basic templates
	results, err = facade.ListLogAlertRuleTemplates(ctx, "basic")
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	// Verify ordering (by usage count DESC)
	assert.GreaterOrEqual(t, results[0].UsageCount, results[1].UsageCount)
}

func TestLogAlertRuleFacade_IncrementTemplateUsage(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	template := &model.LogAlertRuleTemplates{
		Name:           "usage-test",
		Category:       "test",
		TemplateConfig: model.ExtType{},
		UsageCount:     5,
	}
	err := facade.CreateLogAlertRuleTemplate(ctx, template)
	require.NoError(t, err)
	
	// Increment usage
	err = facade.IncrementTemplateUsage(ctx, template.ID)
	require.NoError(t, err)
	
	// Verify increment
	result, err := facade.GetLogAlertRuleTemplateByID(ctx, template.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(6), result.UsageCount)
}

// ==================== Helper Methods ====================

func TestLogAlertRuleFacade_WithCluster(t *testing.T) {
	facade := NewLogAlertRuleFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*LogAlertRuleFacadeInterface)(nil), clusterFacade)
}

// ==================== Benchmark Tests ====================

func BenchmarkLogAlertRuleFacade_CreateLogAlertRule(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		rule := &model.LogAlertRules{
			Name:           "bench-rule",
			ClusterName:    "test-cluster",
			LabelSelectors: model.ExtType{},
			MatchType:      "pattern",
			MatchConfig:    model.ExtType{},
		}
		_ = facade.CreateLogAlertRule(ctx, rule)
	}
}

func BenchmarkLogAlertRuleFacade_ListLogAlertRules(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	for i := 0; i < 50; i++ {
		rule := &model.LogAlertRules{
			Name:           "bench-" + string(rune('0'+i%10)),
			ClusterName:    "test-cluster",
			LabelSelectors: model.ExtType{},
			MatchType:      "pattern",
			MatchConfig:    model.ExtType{},
		}
		_ = facade.CreateLogAlertRule(ctx, rule)
	}
	
	filter := &LogAlertRuleFilter{Limit: 10}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _, _ = facade.ListLogAlertRules(ctx, filter)
	}
}

func BenchmarkLogAlertRuleFacade_UpdateRuleTriggerInfo(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestLogAlertRuleFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	rule := &model.LogAlertRules{
		Name:           "bench-trigger",
		ClusterName:    "test-cluster",
		LabelSelectors: model.ExtType{},
		MatchType:      "pattern",
		MatchConfig:    model.ExtType{},
	}
	_ = facade.CreateLogAlertRule(ctx, rule)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_ = facade.UpdateRuleTriggerInfo(ctx, rule.ID)
	}
}

