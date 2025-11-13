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

// mockAlertRuleAdviceFacade creates an AlertRuleAdviceFacade with the test database
type mockAlertRuleAdviceFacade struct {
	AlertRuleAdviceFacade
	db *gorm.DB
}

func (f *mockAlertRuleAdviceFacade) getDB() *gorm.DB {
	return f.db
}

// newTestAlertRuleAdviceFacade creates a test AlertRuleAdviceFacade
func newTestAlertRuleAdviceFacade(db *gorm.DB) AlertRuleAdviceFacadeInterface {
	return &mockAlertRuleAdviceFacade{
		db: db,
	}
}

// TestAlertRuleAdviceFacade_CreateAlertRuleAdvices tests creating advice
func TestAlertRuleAdviceFacade_CreateAlertRuleAdvices(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	advice := &model.AlertRuleAdvices{
		RuleType:        "log",
		Name:            "high-error-rate-detection",
		Title:           "High Error Rate Detected",
		Description:     "Error rate exceeds threshold",
		Category:        "performance",
		ClusterName:     "test-cluster",
		TargetResource:  "deployment",
		TargetName:      "api-server",
		RuleConfig:      model.ExtType{"query": "rate(errors[5m]) > 0.05"},
		Severity:        "critical",
		Priority:        8,
		Reason:          "Error rate increased significantly",
		Evidence:        model.ExtType{"error_count": float64(500)},
		Status:          "pending",
		InspectionID:    "inspection-001",
		InspectionTime:  time.Now(),
		ConfidenceScore: 0.85,
	}
	
	err := facade.CreateAlertRuleAdvices(ctx, advice)
	require.NoError(t, err)
	assert.NotZero(t, advice.ID)
	assert.NotZero(t, advice.CreatedAt)
}

// TestAlertRuleAdviceFacade_GetAlertRuleAdvicesByID tests getting advice by ID
func TestAlertRuleAdviceFacade_GetAlertRuleAdvicesByID(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create an advice
	advice := &model.AlertRuleAdvices{
		RuleType:       "metric",
		Name:           "cpu-threshold",
		Title:          "CPU Threshold",
		Category:       "resource",
		ClusterName:    "test-cluster",
		RuleConfig:     model.ExtType{},
		InspectionTime: time.Now(),
	}
	err := facade.CreateAlertRuleAdvices(ctx, advice)
	require.NoError(t, err)
	
	// Get the advice
	result, err := facade.GetAlertRuleAdvicesByID(ctx, advice.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, advice.ID, result.ID)
	assert.Equal(t, advice.Name, result.Name)
	assert.Equal(t, advice.Title, result.Title)
}

// TestAlertRuleAdviceFacade_GetAlertRuleAdvicesByID_NotFound tests getting non-existent advice
func TestAlertRuleAdviceFacade_GetAlertRuleAdvicesByID_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	result, err := facade.GetAlertRuleAdvicesByID(ctx, 99999)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestAlertRuleAdviceFacade_ListAlertRuleAdvicess tests listing with filters
func TestAlertRuleAdviceFacade_ListAlertRuleAdvicess(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create multiple advices
	advices := []*model.AlertRuleAdvices{
		{RuleType: "log", Name: "advice1", Title: "A1", Category: "performance", ClusterName: "cluster1", Status: "pending", RuleConfig: model.ExtType{}, InspectionTime: time.Now(), Priority: 8, ConfidenceScore: 0.9},
		{RuleType: "metric", Name: "advice2", Title: "A2", Category: "resource", ClusterName: "cluster1", Status: "reviewed", RuleConfig: model.ExtType{}, InspectionTime: time.Now(), Priority: 5, ConfidenceScore: 0.7},
		{RuleType: "log", Name: "advice3", Title: "A3", Category: "error", ClusterName: "cluster2", Status: "pending", RuleConfig: model.ExtType{}, InspectionTime: time.Now(), Priority: 9, ConfidenceScore: 0.95},
	}
	for _, a := range advices {
		err := facade.CreateAlertRuleAdvices(ctx, a)
		require.NoError(t, err)
	}
	
	tests := []struct {
		name          string
		filter        *AlertRuleAdvicesFilter
		expectedCount int
	}{
		{
			name:          "No filter",
			filter:        &AlertRuleAdvicesFilter{},
			expectedCount: 3,
		},
		{
			name: "Filter by cluster",
			filter: &AlertRuleAdvicesFilter{
				ClusterName: "cluster1",
			},
			expectedCount: 2,
		},
		{
			name: "Filter by rule type",
			filter: &AlertRuleAdvicesFilter{
				RuleType: "log",
			},
			expectedCount: 2,
		},
		{
			name: "Filter by status",
			filter: &AlertRuleAdvicesFilter{
				Status: "pending",
			},
			expectedCount: 2,
		},
		{
			name: "Filter by category",
			filter: &AlertRuleAdvicesFilter{
				Category: "performance",
			},
			expectedCount: 1,
		},
		{
			name: "Filter by min priority",
			filter: &AlertRuleAdvicesFilter{
				MinPriority: intPtr(8),
			},
			expectedCount: 2,
		},
		{
			name: "Filter by confidence range",
			filter: &AlertRuleAdvicesFilter{
				MinConfidence: float64Ptr(0.8),
			},
			expectedCount: 2,
		},
		{
			name: "With pagination",
			filter: &AlertRuleAdvicesFilter{
				Limit:  2,
				Offset: 0,
			},
			expectedCount: 2,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := facade.ListAlertRuleAdvicess(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.expectedCount)
			assert.Equal(t, int64(3), total)
		})
	}
}

// TestAlertRuleAdviceFacade_UpdateAlertRuleAdvices tests updating advice
func TestAlertRuleAdviceFacade_UpdateAlertRuleAdvices(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create an advice
	advice := &model.AlertRuleAdvices{
		RuleType:       "log",
		Name:           "update-test",
		Title:          "Original Title",
		Category:       "performance",
		ClusterName:    "test",
		RuleConfig:     model.ExtType{},
		InspectionTime: time.Now(),
		Priority:       5,
	}
	err := facade.CreateAlertRuleAdvices(ctx, advice)
	require.NoError(t, err)
	
	// Update the advice
	advice.Title = "Updated Title"
	advice.Priority = 8
	err = facade.UpdateAlertRuleAdvices(ctx, advice)
	require.NoError(t, err)
	
	// Verify update
	result, err := facade.GetAlertRuleAdvicesByID(ctx, advice.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Updated Title", result.Title)
	assert.Equal(t, int32(8), result.Priority)
}

// TestAlertRuleAdviceFacade_DeleteAlertRuleAdvices tests deleting advice
func TestAlertRuleAdviceFacade_DeleteAlertRuleAdvices(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	advice := &model.AlertRuleAdvices{
		RuleType:       "log",
		Name:           "delete-test",
		Title:          "Delete Me",
		Category:       "error",
		ClusterName:    "test",
		RuleConfig:     model.ExtType{},
		InspectionTime: time.Now(),
	}
	err := facade.CreateAlertRuleAdvices(ctx, advice)
	require.NoError(t, err)
	
	// Delete the advice
	err = facade.DeleteAlertRuleAdvices(ctx, advice.ID)
	require.NoError(t, err)
	
	// Verify deletion
	result, err := facade.GetAlertRuleAdvicesByID(ctx, advice.ID)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestAlertRuleAdviceFacade_BatchCreateAlertRuleAdvicess tests batch creation
func TestAlertRuleAdviceFacade_BatchCreateAlertRuleAdvicess(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	advices := []*model.AlertRuleAdvices{
		{RuleType: "log", Name: "batch1", Title: "B1", Category: "performance", ClusterName: "test", RuleConfig: model.ExtType{}, InspectionTime: time.Now()},
		{RuleType: "metric", Name: "batch2", Title: "B2", Category: "resource", ClusterName: "test", RuleConfig: model.ExtType{}, InspectionTime: time.Now()},
		{RuleType: "log", Name: "batch3", Title: "B3", Category: "error", ClusterName: "test", RuleConfig: model.ExtType{}, InspectionTime: time.Now()},
	}
	
	err := facade.BatchCreateAlertRuleAdvicess(ctx, advices)
	require.NoError(t, err)
	
	// Verify all were created
	for _, a := range advices {
		assert.NotZero(t, a.ID)
	}
	
	// Count total
	filter := &AlertRuleAdvicesFilter{ClusterName: "test"}
	results, total, err := facade.ListAlertRuleAdvicess(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, int64(3), total)
}

// TestAlertRuleAdviceFacade_UpdateAdviceStatus tests updating status
func TestAlertRuleAdviceFacade_UpdateAdviceStatus(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	advice := &model.AlertRuleAdvices{
		RuleType:       "log",
		Name:           "status-test",
		Title:          "Status Test",
		Category:       "performance",
		ClusterName:    "test",
		Status:         "pending",
		RuleConfig:     model.ExtType{},
		InspectionTime: time.Now(),
	}
	err := facade.CreateAlertRuleAdvices(ctx, advice)
	require.NoError(t, err)
	
	// Update status
	err = facade.UpdateAdviceStatus(ctx, advice.ID, "reviewed", "admin", "Looks good")
	require.NoError(t, err)
	
	// Verify status update
	result, err := facade.GetAlertRuleAdvicesByID(ctx, advice.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "reviewed", result.Status)
	assert.Equal(t, "admin", result.ReviewedBy)
	assert.Equal(t, "Looks good", result.ReviewNotes)
	assert.False(t, result.ReviewedAt.IsZero())
}

// TestAlertRuleAdviceFacade_MarkAsApplied tests marking advice as applied
func TestAlertRuleAdviceFacade_MarkAsApplied(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	advice := &model.AlertRuleAdvices{
		RuleType:       "log",
		Name:           "apply-test",
		Title:          "Apply Test",
		Category:       "performance",
		ClusterName:    "test",
		Status:         "reviewed",
		RuleConfig:     model.ExtType{},
		InspectionTime: time.Now(),
	}
	err := facade.CreateAlertRuleAdvices(ctx, advice)
	require.NoError(t, err)
	
	// Mark as applied
	err = facade.MarkAsApplied(ctx, advice.ID, 123)
	require.NoError(t, err)
	
	// Verify
	result, err := facade.GetAlertRuleAdvicesByID(ctx, advice.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "applied", result.Status)
	assert.Equal(t, int64(123), result.AppliedRuleID)
	assert.False(t, result.AppliedAt.IsZero())
}

// TestAlertRuleAdviceFacade_GetAdvicesByInspectionID tests getting by inspection ID
func TestAlertRuleAdviceFacade_GetAdvicesByInspectionID(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	inspectionID := "inspection-123"
	
	// Create advices with same inspection ID
	advices := []*model.AlertRuleAdvices{
		{RuleType: "log", Name: "a1", Title: "A1", Category: "performance", ClusterName: "test", RuleConfig: model.ExtType{}, InspectionID: inspectionID, InspectionTime: time.Now(), Priority: 8, ConfidenceScore: 0.9},
		{RuleType: "metric", Name: "a2", Title: "A2", Category: "resource", ClusterName: "test", RuleConfig: model.ExtType{}, InspectionID: inspectionID, InspectionTime: time.Now(), Priority: 5, ConfidenceScore: 0.7},
	}
	for _, a := range advices {
		err := facade.CreateAlertRuleAdvices(ctx, a)
		require.NoError(t, err)
	}
	
	// Get by inspection ID
	results, err := facade.GetAdvicesByInspectionID(ctx, inspectionID)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	// Verify ordering (by priority DESC, confidence DESC)
	assert.GreaterOrEqual(t, results[0].Priority, results[1].Priority)
}

// TestAlertRuleAdviceFacade_GetExpiredAdvices tests getting expired advices
func TestAlertRuleAdviceFacade_GetExpiredAdvices(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	now := time.Now()
	
	// Create expired advice
	expiredAdvice := &model.AlertRuleAdvices{
		RuleType:       "log",
		Name:           "expired",
		Title:          "Expired",
		Category:       "performance",
		ClusterName:    "test",
		RuleConfig:     model.ExtType{},
		InspectionTime: time.Now(),
		ExpiresAt:      now.Add(-1 * time.Hour),
	}
	err := facade.CreateAlertRuleAdvices(ctx, expiredAdvice)
	require.NoError(t, err)
	
	// Create non-expired advice
	validAdvice := &model.AlertRuleAdvices{
		RuleType:       "metric",
		Name:           "valid",
		Title:          "Valid",
		Category:       "resource",
		ClusterName:    "test",
		RuleConfig:     model.ExtType{},
		InspectionTime: time.Now(),
		ExpiresAt:      now.Add(1 * time.Hour),
	}
	err = facade.CreateAlertRuleAdvices(ctx, validAdvice)
	require.NoError(t, err)
	
	// Get expired advices
	results, err := facade.GetExpiredAdvices(ctx)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "expired", results[0].Name)
}

// TestAlertRuleAdviceFacade_DeleteExpiredAdvices tests deleting expired advices
func TestAlertRuleAdviceFacade_DeleteExpiredAdvices(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	now := time.Now()
	
	// Create expired advices
	for i := 0; i < 3; i++ {
		advice := &model.AlertRuleAdvices{
			RuleType:       "log",
			Name:           "expired-" + string(rune('a'+i)),
			Title:          "Expired",
			Category:       "performance",
			ClusterName:    "test",
			RuleConfig:     model.ExtType{},
			InspectionTime: time.Now(),
			ExpiresAt:      now.Add(-1 * time.Hour),
		}
		err := facade.CreateAlertRuleAdvices(ctx, advice)
		require.NoError(t, err)
	}
	
	// Delete expired advices
	count, err := facade.DeleteExpiredAdvices(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
	
	// Verify deletion
	results, err := facade.GetExpiredAdvices(ctx)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestAlertRuleAdviceFacade_WithCluster tests the WithCluster method
func TestAlertRuleAdviceFacade_WithCluster(t *testing.T) {
	facade := NewAlertRuleAdviceFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*AlertRuleAdviceFacadeInterface)(nil), clusterFacade)
}

// ==================== Benchmark Tests ====================

func BenchmarkAlertRuleAdviceFacade_CreateAlertRuleAdvices(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		advice := &model.AlertRuleAdvices{
			RuleType:       "log",
			Name:           "bench-advice",
			Title:          "Benchmark",
			Category:       "performance",
			ClusterName:    "test",
			RuleConfig:     model.ExtType{},
			InspectionTime: time.Now(),
		}
		_ = facade.CreateAlertRuleAdvices(ctx, advice)
	}
}

func BenchmarkAlertRuleAdviceFacade_ListAlertRuleAdvicess(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	for i := 0; i < 50; i++ {
		advice := &model.AlertRuleAdvices{
			RuleType:       "log",
			Name:           "advice-" + string(rune('0'+i%10)),
			Title:          "Test",
			Category:       "performance",
			ClusterName:    "test",
			RuleConfig:     model.ExtType{},
			InspectionTime: time.Now(),
		}
		_ = facade.CreateAlertRuleAdvices(ctx, advice)
	}
	
	filter := &AlertRuleAdvicesFilter{Limit: 10}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _, _ = facade.ListAlertRuleAdvicess(ctx, filter)
	}
}

func BenchmarkAlertRuleAdviceFacade_UpdateAdviceStatus(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestAlertRuleAdviceFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	advice := &model.AlertRuleAdvices{
		RuleType:       "log",
		Name:           "bench-status",
		Title:          "Benchmark",
		Category:       "performance",
		ClusterName:    "test",
		Status:         "pending",
		RuleConfig:     model.ExtType{},
		InspectionTime: time.Now(),
	}
	_ = facade.CreateAlertRuleAdvices(ctx, advice)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_ = facade.UpdateAdviceStatus(ctx, advice.ID, "reviewed", "admin", "test")
	}
}

