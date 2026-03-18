// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package gpu_usage_weekly_report

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"gorm.io/gorm"
)

// ReportGeneratorInterface defines the interface for report generation
type ReportGeneratorInterface interface {
	Generate(ctx context.Context, clusterName string, period ReportPeriod) (*ReportData, error)
}

// ReportRendererInterface defines the interface for report rendering
type ReportRendererInterface interface {
	RenderHTML(ctx context.Context, data *ReportData) ([]byte, error)
	RenderPDF(ctx context.Context, htmlContent []byte) ([]byte, error)
}

// ClusterManagerInterface defines the interface for cluster management
type ClusterManagerInterface interface {
	GetClusterNames() []string
	GetClientSetByClusterName(clusterName string) (*clientsets.ClusterClientSet, error)
}

// DatabaseFacadeInterface defines the interface for database operations
// This extends the core FacadeInterface to allow custom mock implementations
type DatabaseFacadeInterface interface {
	GetGpuUsageWeeklyReport() database.GpuUsageWeeklyReportFacadeInterface
	GetGpuAggregation() database.GpuAggregationFacadeInterface
	WithCluster(clusterName string) database.FacadeInterface
}

// DBConnectionProvider provides database connections for clusters
type DBConnectionProvider interface {
	GetDBForCluster(clusterName string) (*gorm.DB, error)
}

// ReportGeneratorFactory creates ReportGenerator instances
type ReportGeneratorFactory func() ReportGeneratorInterface

// ReportRendererFactory creates ReportRenderer instances
type ReportRendererFactory func() ReportRendererInterface

// Dependencies holds all dependencies for GpuUsageWeeklyReportBackfillJob
type Dependencies struct {
	ClusterManager       ClusterManagerInterface
	DatabaseFacade       database.FacadeInterface
	DBConnectionProvider DBConnectionProvider
	GeneratorFactory     ReportGeneratorFactory
	RendererFactory      ReportRendererFactory
}

// DefaultDBConnectionProvider provides default DB connections using ClusterManager
type DefaultDBConnectionProvider struct {
	clusterManager ClusterManagerInterface
	defaultDB      *gorm.DB
}

// NewDefaultDBConnectionProvider creates a new DefaultDBConnectionProvider
func NewDefaultDBConnectionProvider(cm ClusterManagerInterface, defaultDB *gorm.DB) *DefaultDBConnectionProvider {
	return &DefaultDBConnectionProvider{
		clusterManager: cm,
		defaultDB:      defaultDB,
	}
}

// GetDBForCluster implements DBConnectionProvider
func (p *DefaultDBConnectionProvider) GetDBForCluster(clusterName string) (*gorm.DB, error) {
	clientSet, err := p.clusterManager.GetClientSetByClusterName(clusterName)
	if err != nil {
		return p.defaultDB, nil
	}

	if clientSet == nil || clientSet.StorageClientSet == nil || clientSet.StorageClientSet.DB == nil {
		return p.defaultDB, nil
	}

	return clientSet.StorageClientSet.DB, nil
}

// MockReportGenerator is a mock implementation of ReportGeneratorInterface for testing
type MockReportGenerator struct {
	GenerateFunc func(ctx context.Context, clusterName string, period ReportPeriod) (*ReportData, error)
}

// Generate implements ReportGeneratorInterface
func (m *MockReportGenerator) Generate(ctx context.Context, clusterName string, period ReportPeriod) (*ReportData, error) {
	if m.GenerateFunc != nil {
		return m.GenerateFunc(ctx, clusterName, period)
	}
	return &ReportData{
		ClusterName:    clusterName,
		Period:         period,
		MarkdownReport: "# Mock Report\n\nThis is a mock report.",
		Summary: &ReportSummary{
			TotalGPUs:      100,
			AvgUtilization: 50.0,
			AvgAllocation:  60.0,
		},
	}, nil
}

// MockReportRenderer is a mock implementation of ReportRendererInterface for testing
type MockReportRenderer struct {
	RenderHTMLFunc func(ctx context.Context, data *ReportData) ([]byte, error)
	RenderPDFFunc  func(ctx context.Context, htmlContent []byte) ([]byte, error)
}

// RenderHTML implements ReportRendererInterface
func (m *MockReportRenderer) RenderHTML(ctx context.Context, data *ReportData) ([]byte, error) {
	if m.RenderHTMLFunc != nil {
		return m.RenderHTMLFunc(ctx, data)
	}
	return []byte("<html><body>Mock HTML Report</body></html>"), nil
}

// RenderPDF implements ReportRendererInterface
func (m *MockReportRenderer) RenderPDF(ctx context.Context, htmlContent []byte) ([]byte, error) {
	if m.RenderPDFFunc != nil {
		return m.RenderPDFFunc(ctx, htmlContent)
	}
	return []byte("mock pdf content"), nil
}

// MockClusterManager is a mock implementation of ClusterManagerInterface for testing
type MockClusterManager struct {
	Clusters         map[string]*clientsets.ClusterClientSet
	GetClusterNamesFunc func() []string
}

// NewMockClusterManager creates a new MockClusterManager
func NewMockClusterManager() *MockClusterManager {
	return &MockClusterManager{
		Clusters: make(map[string]*clientsets.ClusterClientSet),
	}
}

// GetClusterNames implements ClusterManagerInterface
func (m *MockClusterManager) GetClusterNames() []string {
	if m.GetClusterNamesFunc != nil {
		return m.GetClusterNamesFunc()
	}
	names := make([]string, 0, len(m.Clusters))
	for name := range m.Clusters {
		names = append(names, name)
	}
	return names
}

// GetClientSetByClusterName implements ClusterManagerInterface
func (m *MockClusterManager) GetClientSetByClusterName(clusterName string) (*clientsets.ClusterClientSet, error) {
	if cs, ok := m.Clusters[clusterName]; ok {
		return cs, nil
	}
	return nil, nil
}

// AddCluster adds a cluster to the mock
func (m *MockClusterManager) AddCluster(name string) {
	m.Clusters[name] = &clientsets.ClusterClientSet{
		ClusterName: name,
	}
}

// MockDBConnectionProvider is a mock implementation of DBConnectionProvider for testing
type MockDBConnectionProvider struct {
	GetDBForClusterFunc func(clusterName string) (*gorm.DB, error)
}

// GetDBForCluster implements DBConnectionProvider
func (m *MockDBConnectionProvider) GetDBForCluster(clusterName string) (*gorm.DB, error) {
	if m.GetDBForClusterFunc != nil {
		return m.GetDBForClusterFunc(clusterName)
	}
	return nil, nil
}

// TimeRangeResult holds the result of a time range query
type TimeRangeResult struct {
	MinTime time.Time
	MaxTime time.Time
}

