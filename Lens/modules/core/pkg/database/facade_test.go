package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewFacade tests the creation of a new Facade instance
func TestNewFacade(t *testing.T) {
	facade := NewFacade()
	
	require.NotNil(t, facade)
	assert.NotNil(t, facade.Node)
	assert.NotNil(t, facade.Pod)
	assert.NotNil(t, facade.Workload)
	assert.NotNil(t, facade.Container)
	assert.NotNil(t, facade.Training)
	assert.NotNil(t, facade.Storage)
	assert.NotNil(t, facade.Alert)
	assert.NotNil(t, facade.MetricAlertRule)
	assert.NotNil(t, facade.LogAlertRule)
	assert.NotNil(t, facade.AlertRuleAdvice)
	assert.NotNil(t, facade.ClusterOverviewCache)
	assert.NotNil(t, facade.GenericCache)
	assert.NotNil(t, facade.GpuAggregation)
	assert.NotNil(t, facade.SystemConfig)
	assert.NotNil(t, facade.JobExecutionHistory)
}

// TestFacade_GetNode tests the GetNode method
func TestFacade_GetNode(t *testing.T) {
	facade := NewFacade()
	
	node := facade.GetNode()
	require.NotNil(t, node)
	assert.Implements(t, (*NodeFacadeInterface)(nil), node)
}

// TestFacade_GetPod tests the GetPod method
func TestFacade_GetPod(t *testing.T) {
	facade := NewFacade()
	
	pod := facade.GetPod()
	require.NotNil(t, pod)
	assert.Implements(t, (*PodFacadeInterface)(nil), pod)
}

// TestFacade_GetWorkload tests the GetWorkload method
func TestFacade_GetWorkload(t *testing.T) {
	facade := NewFacade()
	
	workload := facade.GetWorkload()
	require.NotNil(t, workload)
	assert.Implements(t, (*WorkloadFacadeInterface)(nil), workload)
}

// TestFacade_GetContainer tests the GetContainer method
func TestFacade_GetContainer(t *testing.T) {
	facade := NewFacade()
	
	container := facade.GetContainer()
	require.NotNil(t, container)
	assert.Implements(t, (*ContainerFacadeInterface)(nil), container)
}

// TestFacade_GetTraining tests the GetTraining method
func TestFacade_GetTraining(t *testing.T) {
	facade := NewFacade()
	
	training := facade.GetTraining()
	require.NotNil(t, training)
	assert.Implements(t, (*TrainingFacadeInterface)(nil), training)
}

// TestFacade_GetStorage tests the GetStorage method
func TestFacade_GetStorage(t *testing.T) {
	facade := NewFacade()
	
	storage := facade.GetStorage()
	require.NotNil(t, storage)
	assert.Implements(t, (*StorageFacadeInterface)(nil), storage)
}

// TestFacade_GetAlert tests the GetAlert method
func TestFacade_GetAlert(t *testing.T) {
	facade := NewFacade()
	
	alert := facade.GetAlert()
	require.NotNil(t, alert)
	assert.Implements(t, (*AlertFacadeInterface)(nil), alert)
}

// TestFacade_GetMetricAlertRule tests the GetMetricAlertRule method
func TestFacade_GetMetricAlertRule(t *testing.T) {
	facade := NewFacade()
	
	rule := facade.GetMetricAlertRule()
	require.NotNil(t, rule)
	assert.Implements(t, (*MetricAlertRuleFacadeInterface)(nil), rule)
}

// TestFacade_GetLogAlertRule tests the GetLogAlertRule method
func TestFacade_GetLogAlertRule(t *testing.T) {
	facade := NewFacade()
	
	rule := facade.GetLogAlertRule()
	require.NotNil(t, rule)
	assert.Implements(t, (*LogAlertRuleFacadeInterface)(nil), rule)
}

// TestFacade_GetAlertRuleAdvice tests the GetAlertRuleAdvice method
func TestFacade_GetAlertRuleAdvice(t *testing.T) {
	facade := NewFacade()
	
	advice := facade.GetAlertRuleAdvice()
	require.NotNil(t, advice)
	assert.Implements(t, (*AlertRuleAdviceFacadeInterface)(nil), advice)
}

// TestFacade_GetClusterOverviewCache tests the GetClusterOverviewCache method
func TestFacade_GetClusterOverviewCache(t *testing.T) {
	facade := NewFacade()
	
	cache := facade.GetClusterOverviewCache()
	require.NotNil(t, cache)
	assert.Implements(t, (*ClusterOverviewCacheFacadeInterface)(nil), cache)
}

// TestFacade_GetGenericCache tests the GetGenericCache method
func TestFacade_GetGenericCache(t *testing.T) {
	facade := NewFacade()
	
	cache := facade.GetGenericCache()
	require.NotNil(t, cache)
	assert.Implements(t, (*GenericCacheFacadeInterface)(nil), cache)
}

// TestFacade_GetGpuAggregation tests the GetGpuAggregation method
func TestFacade_GetGpuAggregation(t *testing.T) {
	facade := NewFacade()
	
	agg := facade.GetGpuAggregation()
	require.NotNil(t, agg)
	assert.Implements(t, (*GpuAggregationFacadeInterface)(nil), agg)
}

// TestFacade_GetSystemConfig tests the GetSystemConfig method
func TestFacade_GetSystemConfig(t *testing.T) {
	facade := NewFacade()
	
	config := facade.GetSystemConfig()
	require.NotNil(t, config)
	assert.Implements(t, (*SystemConfigFacadeInterface)(nil), config)
}

// TestFacade_GetJobExecutionHistory tests the GetJobExecutionHistory method
func TestFacade_GetJobExecutionHistory(t *testing.T) {
	facade := NewFacade()
	
	history := facade.GetJobExecutionHistory()
	require.NotNil(t, history)
	assert.Implements(t, (*JobExecutionHistoryFacadeInterface)(nil), history)
}

// TestFacade_WithCluster tests the WithCluster method
func TestFacade_WithCluster(t *testing.T) {
	facade := NewFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*FacadeInterface)(nil), clusterFacade)
	
	// Verify all sub-facades are not nil
	assert.NotNil(t, clusterFacade.GetNode())
	assert.NotNil(t, clusterFacade.GetPod())
	assert.NotNil(t, clusterFacade.GetStorage())
	assert.NotNil(t, clusterFacade.GetGenericCache())
	assert.NotNil(t, clusterFacade.GetSystemConfig())
}

// TestGetFacade tests the global GetFacade function
func TestGetFacade(t *testing.T) {
	facade := GetFacade()
	
	require.NotNil(t, facade)
	assert.Implements(t, (*FacadeInterface)(nil), facade)
}

// TestGetFacadeForCluster tests the GetFacadeForCluster function
func TestGetFacadeForCluster(t *testing.T) {
	facade := GetFacadeForCluster("test-cluster")
	
	require.NotNil(t, facade)
	assert.Implements(t, (*FacadeInterface)(nil), facade)
}

// TestFacade_WithCluster_Independence tests that WithCluster creates independent instances
func TestFacade_WithCluster_Independence(t *testing.T) {
	facade1 := NewFacade()
	facade2 := facade1.WithCluster("cluster-1")
	facade3 := facade1.WithCluster("cluster-2")
	
	// All should be different instances
	assert.NotEqual(t, facade1, facade2)
	assert.NotEqual(t, facade1, facade3)
	assert.NotEqual(t, facade2, facade3)
}

// Benchmark tests
func BenchmarkNewFacade(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewFacade()
	}
}

func BenchmarkFacade_GetNode(b *testing.B) {
	facade := NewFacade()
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_ = facade.GetNode()
	}
}

func BenchmarkFacade_WithCluster(b *testing.B) {
	facade := NewFacade()
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_ = facade.WithCluster("test-cluster")
	}
}

