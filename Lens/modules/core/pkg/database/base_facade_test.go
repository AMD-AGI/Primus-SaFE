package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBaseFacade_WithCluster tests the withCluster method
func TestBaseFacade_WithCluster(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
	}{
		{
			name:        "Empty cluster name",
			clusterName: "",
		},
		{
			name:        "Valid cluster name",
			clusterName: "test-cluster",
		},
		{
			name:        "Cluster with special characters",
			clusterName: "test-cluster-123",
		},
		{
			name:        "Long cluster name",
			clusterName: "very-long-cluster-name-with-many-characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := BaseFacade{}
			
			result := base.withCluster(tt.clusterName)
			
			assert.Equal(t, tt.clusterName, result.clusterName)
		})
	}
}

// TestBaseFacade_WithCluster_Immutability tests that withCluster doesn't modify the original
func TestBaseFacade_WithCluster_Immutability(t *testing.T) {
	original := BaseFacade{clusterName: "original"}
	
	modified := original.withCluster("modified")
	
	assert.Equal(t, "original", original.clusterName, "Original should not be modified")
	assert.Equal(t, "modified", modified.clusterName, "Modified should have new cluster name")
}

// Benchmark tests
func BenchmarkBaseFacade_WithCluster(b *testing.B) {
	base := BaseFacade{}
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_ = base.withCluster("test-cluster")
	}
}

