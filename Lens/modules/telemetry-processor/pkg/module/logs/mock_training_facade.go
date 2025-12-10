package logs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// MockTrainingFacade implements TrainingFacadeInterface for testing
type MockTrainingFacade struct {
	mu                      sync.RWMutex
	trainingPerformanceData map[string]*model.TrainingPerformance // key: workloadUID_serial_iteration
}

// NewMockTrainingFacade creates a new mock training facade
func NewMockTrainingFacade() *MockTrainingFacade {
	return &MockTrainingFacade{
		trainingPerformanceData: make(map[string]*model.TrainingPerformance),
	}
}

func (m *MockTrainingFacade) makeKey(workloadUID string, serial, iteration int) string {
	return fmt.Sprintf("%s_%d_%d", workloadUID, serial, iteration)
}

// GetTrainingPerformanceByWorkloadIdSerialAndIteration implements TrainingFacadeInterface
func (m *MockTrainingFacade) GetTrainingPerformanceByWorkloadIdSerialAndIteration(
	ctx context.Context,
	workloadUID string,
	serial int,
	iteration int,
) (*model.TrainingPerformance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.makeKey(workloadUID, serial, iteration)
	if perf, exists := m.trainingPerformanceData[key]; exists {
		// Return a copy to simulate database behavior
		perfCopy := *perf
		return &perfCopy, nil
	}
	return nil, nil
}

// CreateTrainingPerformance implements TrainingFacadeInterface
func (m *MockTrainingFacade) CreateTrainingPerformance(
	ctx context.Context,
	trainingPerformance *model.TrainingPerformance,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.makeKey(
		trainingPerformance.WorkloadUID,
		int(trainingPerformance.Serial),
		int(trainingPerformance.Iteration),
	)

	// Simulate auto-increment ID
	if trainingPerformance.ID == 0 {
		trainingPerformance.ID = int32(len(m.trainingPerformanceData) + 1)
	}

	// Store a copy
	perfCopy := *trainingPerformance
	m.trainingPerformanceData[key] = &perfCopy

	return nil
}

// UpdateTrainingPerformance implements TrainingFacadeInterface
func (m *MockTrainingFacade) UpdateTrainingPerformance(
	ctx context.Context,
	trainingPerformance *model.TrainingPerformance,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.makeKey(
		trainingPerformance.WorkloadUID,
		int(trainingPerformance.Serial),
		int(trainingPerformance.Iteration),
	)

	// Simulate delete and recreate (as in real implementation)
	// Keep the same ID but update data
	perfCopy := *trainingPerformance
	m.trainingPerformanceData[key] = &perfCopy

	return nil
}

// ListWorkloadPerformanceByWorkloadIdAndTimeRange implements TrainingFacadeInterface
func (m *MockTrainingFacade) ListWorkloadPerformanceByWorkloadIdAndTimeRange(
	ctx context.Context,
	workloadUID string,
	start, end time.Time,
) ([]*model.TrainingPerformance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*model.TrainingPerformance
	for _, perf := range m.trainingPerformanceData {
		if perf.WorkloadUID == workloadUID &&
			!perf.CreatedAt.Before(start) &&
			!perf.CreatedAt.After(end) {
			perfCopy := *perf
			result = append(result, &perfCopy)
		}
	}
	return result, nil
}

// ListTrainingPerformanceByWorkloadIdsAndTimeRange implements TrainingFacadeInterface
func (m *MockTrainingFacade) ListTrainingPerformanceByWorkloadIdsAndTimeRange(
	ctx context.Context,
	workloadUIDs []string,
	start, end time.Time,
) ([]*model.TrainingPerformance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	workloadMap := make(map[string]bool)
	for _, uid := range workloadUIDs {
		workloadMap[uid] = true
	}

	var result []*model.TrainingPerformance
	for _, perf := range m.trainingPerformanceData {
		if workloadMap[perf.WorkloadUID] &&
			!perf.CreatedAt.Before(start) &&
			!perf.CreatedAt.After(end) {
			perfCopy := *perf
			result = append(result, &perfCopy)
		}
	}
	return result, nil
}

// ListTrainingPerformanceByWorkloadUID implements TrainingFacadeInterface
func (m *MockTrainingFacade) ListTrainingPerformanceByWorkloadUID(
	ctx context.Context,
	workloadUID string,
) ([]*model.TrainingPerformance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*model.TrainingPerformance
	for _, perf := range m.trainingPerformanceData {
		if perf.WorkloadUID == workloadUID {
			perfCopy := *perf
			result = append(result, &perfCopy)
		}
	}
	return result, nil
}

// ListTrainingPerformanceByWorkloadUIDAndDataSource implements TrainingFacadeInterface
func (m *MockTrainingFacade) ListTrainingPerformanceByWorkloadUIDAndDataSource(
	ctx context.Context,
	workloadUID string,
	dataSource string,
) ([]*model.TrainingPerformance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*model.TrainingPerformance
	for _, perf := range m.trainingPerformanceData {
		if perf.WorkloadUID == workloadUID &&
			(dataSource == "" || perf.DataSource == dataSource) {
			perfCopy := *perf
			result = append(result, &perfCopy)
		}
	}
	return result, nil
}

// ListTrainingPerformanceByWorkloadUIDDataSourceAndTimeRange implements TrainingFacadeInterface
func (m *MockTrainingFacade) ListTrainingPerformanceByWorkloadUIDDataSourceAndTimeRange(
	ctx context.Context,
	workloadUID string,
	dataSource string,
	start, end time.Time,
) ([]*model.TrainingPerformance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*model.TrainingPerformance
	for _, perf := range m.trainingPerformanceData {
		if perf.WorkloadUID == workloadUID &&
			(dataSource == "" || perf.DataSource == dataSource) &&
			!perf.CreatedAt.Before(start) &&
			!perf.CreatedAt.After(end) {
			perfCopy := *perf
			result = append(result, &perfCopy)
		}
	}
	return result, nil
}

// WithCluster implements TrainingFacadeInterface
func (m *MockTrainingFacade) WithCluster(clusterName string) database.TrainingFacadeInterface {
	// For testing, just return the same instance
	return m
}

// GetStoredData returns all stored training performance data (for testing verification)
func (m *MockTrainingFacade) GetStoredData() map[string]*model.TrainingPerformance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*model.TrainingPerformance)
	for k, v := range m.trainingPerformanceData {
		perfCopy := *v
		result[k] = &perfCopy
	}
	return result
}
