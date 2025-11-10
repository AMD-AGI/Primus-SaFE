package clientsets

import (
	"context"
	"os"
	"testing"
)

func TestGetCurrentClusterName(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{
			name:     "environment variable set",
			envValue: "test-cluster",
			want:     "test-cluster",
		},
		{
			name:     "environment variable not set",
			envValue: "",
			want:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envValue != "" {
				os.Setenv("CLUSTER_NAME", tt.envValue)
				defer os.Unsetenv("CLUSTER_NAME")
			} else {
				os.Unsetenv("CLUSTER_NAME")
			}

			got := getCurrentClusterName()
			if got != tt.want {
				t.Errorf("getCurrentClusterName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClusterManager_GetCurrentClusterName(t *testing.T) {
	// Set test environment variable
	os.Setenv("CLUSTER_NAME", "test-cluster")
	defer os.Unsetenv("CLUSTER_NAME")

	// Create a mock ClusterManager
	cm := &ClusterManager{
		currentCluster: &ClusterClientSet{
			ClusterName: "test-cluster",
		},
		clusters:     make(map[string]*ClusterClientSet),
		multiCluster: false,
	}

	got := cm.GetCurrentClusterName()
	want := "test-cluster"

	if got != want {
		t.Errorf("GetCurrentClusterName() = %v, want %v", got, want)
	}
}

func TestClusterManager_IsMultiCluster(t *testing.T) {
	tests := []struct {
		name         string
		multiCluster bool
	}{
		{
			name:         "single cluster mode",
			multiCluster: false,
		},
		{
			name:         "multi cluster mode",
			multiCluster: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &ClusterManager{
				multiCluster: tt.multiCluster,
				clusters:     make(map[string]*ClusterClientSet),
			}

			if got := cm.IsMultiCluster(); got != tt.multiCluster {
				t.Errorf("IsMultiCluster() = %v, want %v", got, tt.multiCluster)
			}
		})
	}
}

func TestClusterManager_GetClusterCount(t *testing.T) {
	cm := &ClusterManager{
		clusters: map[string]*ClusterClientSet{
			"cluster1": {ClusterName: "cluster1"},
			"cluster2": {ClusterName: "cluster2"},
			"cluster3": {ClusterName: "cluster3"},
		},
	}

	got := cm.GetClusterCount()
	want := 3

	if got != want {
		t.Errorf("GetClusterCount() = %v, want %v", got, want)
	}
}

func TestClusterManager_GetClusterNames(t *testing.T) {
	cm := &ClusterManager{
		clusters: map[string]*ClusterClientSet{
			"cluster1": {ClusterName: "cluster1"},
			"cluster2": {ClusterName: "cluster2"},
		},
	}

	names := cm.GetClusterNames()

	if len(names) != 2 {
		t.Errorf("GetClusterNames() returned %d names, want 2", len(names))
	}

	// Verify expected cluster names are included
	nameMap := make(map[string]bool)
	for _, name := range names {
		nameMap[name] = true
	}

	if !nameMap["cluster1"] || !nameMap["cluster2"] {
		t.Errorf("GetClusterNames() = %v, want [cluster1, cluster2]", names)
	}
}

func TestClusterManager_HasCluster(t *testing.T) {
	cm := &ClusterManager{
		clusters: map[string]*ClusterClientSet{
			"cluster1": {ClusterName: "cluster1"},
			"cluster2": {ClusterName: "cluster2"},
		},
	}

	tests := []struct {
		name        string
		clusterName string
		want        bool
	}{
		{
			name:        "existing cluster",
			clusterName: "cluster1",
			want:        true,
		},
		{
			name:        "non-existing cluster",
			clusterName: "cluster3",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cm.HasCluster(tt.clusterName); got != tt.want {
				t.Errorf("HasCluster(%s) = %v, want %v", tt.clusterName, got, tt.want)
			}
		})
	}
}

func TestClusterManager_GetClientSetByClusterName(t *testing.T) {
	cluster1 := &ClusterClientSet{ClusterName: "cluster1"}
	cluster2 := &ClusterClientSet{ClusterName: "cluster2"}

	cm := &ClusterManager{
		clusters: map[string]*ClusterClientSet{
			"cluster1": cluster1,
			"cluster2": cluster2,
		},
	}

	tests := []struct {
		name        string
		clusterName string
		wantErr     bool
	}{
		{
			name:        "get existing cluster",
			clusterName: "cluster1",
			wantErr:     false,
		},
		{
			name:        "get non-existing cluster",
			clusterName: "cluster3",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cm.GetClientSetByClusterName(tt.clusterName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetClientSetByClusterName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.ClusterName != tt.clusterName {
				t.Errorf("GetClientSetByClusterName() = %v, want cluster name %v", got.ClusterName, tt.clusterName)
			}
		})
	}
}

func TestClusterManager_ListAllClientSets(t *testing.T) {
	cluster1 := &ClusterClientSet{ClusterName: "cluster1"}
	cluster2 := &ClusterClientSet{ClusterName: "cluster2"}

	cm := &ClusterManager{
		clusters: map[string]*ClusterClientSet{
			"cluster1": cluster1,
			"cluster2": cluster2,
		},
	}

	got := cm.ListAllClientSets()

	if len(got) != 2 {
		t.Errorf("ListAllClientSets() returned %d items, want 2", len(got))
	}

	if got["cluster1"].ClusterName != "cluster1" {
		t.Errorf("ListAllClientSets()[cluster1] = %v, want cluster1", got["cluster1"].ClusterName)
	}

	if got["cluster2"].ClusterName != "cluster2" {
		t.Errorf("ListAllClientSets()[cluster2] = %v, want cluster2", got["cluster2"].ClusterName)
	}
}

func TestClusterManager_GetCurrentClusterClients(t *testing.T) {
	currentCluster := &ClusterClientSet{ClusterName: "current"}

	cm := &ClusterManager{
		currentCluster: currentCluster,
		clusters:       make(map[string]*ClusterClientSet),
	}

	got := cm.GetCurrentClusterClients()

	if got.ClusterName != "current" {
		t.Errorf("GetCurrentClusterClients() = %v, want current", got.ClusterName)
	}
}

func TestClusterManager_GetCurrentClusterClients_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("GetCurrentClusterClients() should panic when currentCluster is nil")
		}
	}()

	cm := &ClusterManager{
		currentCluster: nil,
		clusters:       make(map[string]*ClusterClientSet),
	}

	cm.GetCurrentClusterClients()
}

// Test concurrent access
func TestClusterManager_ConcurrentAccess(t *testing.T) {
	cm := &ClusterManager{
		currentCluster: &ClusterClientSet{ClusterName: "current"},
		clusters: map[string]*ClusterClientSet{
			"cluster1": {ClusterName: "cluster1"},
			"cluster2": {ClusterName: "cluster2"},
		},
		multiCluster: true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start multiple goroutines for concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				select {
				case <-ctx.Done():
					done <- true
					return
				default:
					// Test various read operations
					_ = cm.GetClusterCount()
					_ = cm.GetClusterNames()
					_ = cm.HasCluster("cluster1")
					_ = cm.ListAllClientSets()
					_, _ = cm.GetClientSetByClusterName("cluster1")
				}
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Benchmark tests
func BenchmarkClusterManager_GetCurrentClusterClients(b *testing.B) {
	cm := &ClusterManager{
		currentCluster: &ClusterClientSet{ClusterName: "current"},
		clusters:       make(map[string]*ClusterClientSet),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cm.GetCurrentClusterClients()
	}
}

func BenchmarkClusterManager_GetClientSetByClusterName(b *testing.B) {
	cm := &ClusterManager{
		clusters: map[string]*ClusterClientSet{
			"cluster1": {ClusterName: "cluster1"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cm.GetClientSetByClusterName("cluster1")
	}
}

func BenchmarkClusterManager_ListAllClientSets(b *testing.B) {
	cm := &ClusterManager{
		clusters: map[string]*ClusterClientSet{
			"cluster1": {ClusterName: "cluster1"},
			"cluster2": {ClusterName: "cluster2"},
			"cluster3": {ClusterName: "cluster3"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cm.ListAllClientSets()
	}
}
