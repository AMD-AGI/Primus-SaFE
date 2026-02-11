// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

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
			want:     "default", // When CLUSTER_NAME is not set and kubeconfig is not available, defaults to "default"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env vars
			origClusterName := os.Getenv("CLUSTER_NAME")
			origKubeconfig := os.Getenv("KUBECONFIG")

			// Set KUBECONFIG to invalid path to ensure getClusterNameFromKubeconfig returns empty
			os.Setenv("KUBECONFIG", "/nonexistent/path/kubeconfig")
			defer func() {
				if origKubeconfig != "" {
					os.Setenv("KUBECONFIG", origKubeconfig)
				} else {
					os.Unsetenv("KUBECONFIG")
				}
				if origClusterName != "" {
					os.Setenv("CLUSTER_NAME", origClusterName)
				} else {
					os.Unsetenv("CLUSTER_NAME")
				}
			}()

			// Set environment variable
			if tt.envValue != "" {
				os.Setenv("CLUSTER_NAME", tt.envValue)
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
		clusters:      make(map[string]*ClusterClientSet),
		componentType: ComponentTypeDataPlane,
	}

	got := cm.GetCurrentClusterName()
	want := "test-cluster"

	if got != want {
		t.Errorf("GetCurrentClusterName() = %v, want %v", got, want)
	}
}

func TestClusterManager_IsMultiCluster(t *testing.T) {
	tests := []struct {
		name          string
		componentType ComponentType
		wantMulti     bool
	}{
		{
			name:          "data plane mode (single cluster)",
			componentType: ComponentTypeDataPlane,
			wantMulti:     false,
		},
		{
			name:          "control plane mode (multi cluster)",
			componentType: ComponentTypeControlPlane,
			wantMulti:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &ClusterManager{
				componentType: tt.componentType,
				clusters:      make(map[string]*ClusterClientSet),
			}

			if got := cm.IsMultiCluster(); got != tt.wantMulti {
				t.Errorf("IsMultiCluster() = %v, want %v", got, tt.wantMulti)
			}
		})
	}
}

func TestClusterManager_GetClusterCount(t *testing.T) {
	// Test control plane component (multi-cluster)
	cm := &ClusterManager{
		componentType: ComponentTypeControlPlane,
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

	// Test data plane component (single cluster)
	cmDataPlane := &ClusterManager{
		componentType:  ComponentTypeDataPlane,
		currentCluster: &ClusterClientSet{ClusterName: "current"},
		clusters: map[string]*ClusterClientSet{
			"cluster1": {ClusterName: "cluster1"},
		},
	}

	gotDataPlane := cmDataPlane.GetClusterCount()
	wantDataPlane := 1 // Data plane always returns 1

	if gotDataPlane != wantDataPlane {
		t.Errorf("GetClusterCount() for DataPlane = %v, want %v", gotDataPlane, wantDataPlane)
	}
}

func TestClusterManager_GetClusterNames(t *testing.T) {
	// Test control plane component (multi-cluster)
	cm := &ClusterManager{
		componentType: ComponentTypeControlPlane,
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

	// Test data plane component (single cluster)
	cmDataPlane := &ClusterManager{
		componentType:  ComponentTypeDataPlane,
		currentCluster: &ClusterClientSet{ClusterName: "current"},
		clusters: map[string]*ClusterClientSet{
			"cluster1": {ClusterName: "cluster1"},
			"cluster2": {ClusterName: "cluster2"},
		},
	}

	namesDataPlane := cmDataPlane.GetClusterNames()
	if len(namesDataPlane) != 1 || namesDataPlane[0] != "current" {
		t.Errorf("GetClusterNames() for DataPlane = %v, want [current]", namesDataPlane)
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

	// Test control plane component (multi-cluster access)
	cm := &ClusterManager{
		componentType: ComponentTypeControlPlane,
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

	// Test data plane component (only current cluster access)
	currentCluster := &ClusterClientSet{ClusterName: "current"}
	cmDataPlane := &ClusterManager{
		componentType:  ComponentTypeDataPlane,
		currentCluster: currentCluster,
		clusters: map[string]*ClusterClientSet{
			"current":  currentCluster,
			"cluster1": cluster1,
		},
	}

	// Data plane can access current cluster
	got, err := cmDataPlane.GetClientSetByClusterName("current")
	if err != nil {
		t.Errorf("DataPlane GetClientSetByClusterName(current) error = %v, want nil", err)
	}
	if got.ClusterName != "current" {
		t.Errorf("DataPlane GetClientSetByClusterName(current) = %v, want current", got.ClusterName)
	}

	// Data plane cannot access other clusters
	_, err = cmDataPlane.GetClientSetByClusterName("cluster1")
	if err == nil {
		t.Errorf("DataPlane GetClientSetByClusterName(cluster1) should return error")
	}
}

func TestClusterManager_ListAllClientSets(t *testing.T) {
	cluster1 := &ClusterClientSet{ClusterName: "cluster1"}
	cluster2 := &ClusterClientSet{ClusterName: "cluster2"}

	// Test control plane component (multi-cluster access)
	cm := &ClusterManager{
		componentType: ComponentTypeControlPlane,
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

	// Test data plane component (only current cluster)
	currentCluster := &ClusterClientSet{ClusterName: "current"}
	cmDataPlane := &ClusterManager{
		componentType:  ComponentTypeDataPlane,
		currentCluster: currentCluster,
		clusters: map[string]*ClusterClientSet{
			"current":  currentCluster,
			"cluster1": cluster1,
			"cluster2": cluster2,
		},
	}

	gotDataPlane := cmDataPlane.ListAllClientSets()

	if len(gotDataPlane) != 1 {
		t.Errorf("DataPlane ListAllClientSets() returned %d items, want 1", len(gotDataPlane))
	}

	if gotDataPlane["current"] == nil || gotDataPlane["current"].ClusterName != "current" {
		t.Errorf("DataPlane ListAllClientSets()[current] = %v, want current", gotDataPlane["current"])
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

func TestClusterManager_GetCurrentClusterClients_ReturnsNil(t *testing.T) {
	cm := &ClusterManager{
		currentCluster: nil,
		clusters:       make(map[string]*ClusterClientSet),
	}

	got := cm.GetCurrentClusterClients()
	if got != nil {
		t.Errorf("GetCurrentClusterClients() = %v, want nil when currentCluster is nil", got)
	}

	if cm.HasCurrentCluster() {
		t.Error("HasCurrentCluster() = true, want false when currentCluster is nil")
	}
}

// Test concurrent access
func TestClusterManager_ConcurrentAccess(t *testing.T) {
	cm := &ClusterManager{
		componentType:  ComponentTypeControlPlane,
		currentCluster: &ClusterClientSet{ClusterName: "current"},
		clusters: map[string]*ClusterClientSet{
			"cluster1": {ClusterName: "cluster1"},
			"cluster2": {ClusterName: "cluster2"},
		},
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
		componentType: ComponentTypeControlPlane,
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
		componentType: ComponentTypeControlPlane,
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

// Test ComponentType
func TestComponentType(t *testing.T) {
	tests := []struct {
		name           string
		ct             ComponentType
		wantString     string
		wantIsControl  bool
		wantIsDataPane bool
	}{
		{
			name:           "DataPlane",
			ct:             ComponentTypeDataPlane,
			wantString:     "DataPlane",
			wantIsControl:  false,
			wantIsDataPane: true,
		},
		{
			name:           "ControlPlane",
			ct:             ComponentTypeControlPlane,
			wantString:     "ControlPlane",
			wantIsControl:  true,
			wantIsDataPane: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ct.String(); got != tt.wantString {
				t.Errorf("ComponentType.String() = %v, want %v", got, tt.wantString)
			}
			if got := tt.ct.IsControlPlane(); got != tt.wantIsControl {
				t.Errorf("ComponentType.IsControlPlane() = %v, want %v", got, tt.wantIsControl)
			}
			if got := tt.ct.IsDataPlane(); got != tt.wantIsDataPane {
				t.Errorf("ComponentType.IsDataPlane() = %v, want %v", got, tt.wantIsDataPane)
			}
		})
	}
}

// Test ComponentDeclaration defaults
func TestComponentDeclaration_Defaults(t *testing.T) {
	// Test DefaultControlPlaneDeclaration
	cpDecl := DefaultControlPlaneDeclaration()
	if cpDecl.Type != ComponentTypeControlPlane {
		t.Errorf("DefaultControlPlaneDeclaration().Type = %v, want ControlPlane", cpDecl.Type)
	}
	if !cpDecl.RequireK8S || !cpDecl.RequireStorage {
		t.Errorf("DefaultControlPlaneDeclaration() should require both K8S and Storage")
	}

	// Test DefaultDataPlaneDeclaration
	dpDecl := DefaultDataPlaneDeclaration()
	if dpDecl.Type != ComponentTypeDataPlane {
		t.Errorf("DefaultDataPlaneDeclaration().Type = %v, want DataPlane", dpDecl.Type)
	}
	if !dpDecl.RequireK8S || !dpDecl.RequireStorage {
		t.Errorf("DefaultDataPlaneDeclaration() should require both K8S and Storage")
	}
}

// Test GetComponentType
func TestClusterManager_GetComponentType(t *testing.T) {
	tests := []struct {
		name string
		ct   ComponentType
	}{
		{"DataPlane", ComponentTypeDataPlane},
		{"ControlPlane", ComponentTypeControlPlane},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &ClusterManager{
				componentType: tt.ct,
				clusters:      make(map[string]*ClusterClientSet),
			}
			if got := cm.GetComponentType(); got != tt.ct {
				t.Errorf("GetComponentType() = %v, want %v", got, tt.ct)
			}
		})
	}
}

// Test GetClusterClientsOrDefault behavior for different component types
func TestClusterManager_GetClusterClientsOrDefault(t *testing.T) {
	currentCluster := &ClusterClientSet{ClusterName: "current"}
	cluster1 := &ClusterClientSet{ClusterName: "cluster1"}
	cluster2 := &ClusterClientSet{ClusterName: "cluster2"}

	// Test control plane component
	cmControl := &ClusterManager{
		componentType:      ComponentTypeControlPlane,
		currentCluster:     currentCluster,
		defaultClusterName: "cluster1",
		clusters: map[string]*ClusterClientSet{
			"current":  currentCluster,
			"cluster1": cluster1,
			"cluster2": cluster2,
		},
	}

	// Control plane with empty cluster name returns default cluster
	got, err := cmControl.GetClusterClientsOrDefault("")
	if err != nil || got.ClusterName != "cluster1" {
		t.Errorf("ControlPlane GetClusterClientsOrDefault('') = %v, want cluster1", got.ClusterName)
	}

	// Control plane with specific cluster name returns that cluster
	got, err = cmControl.GetClusterClientsOrDefault("cluster2")
	if err != nil || got.ClusterName != "cluster2" {
		t.Errorf("ControlPlane GetClusterClientsOrDefault('cluster2') = %v, want cluster2", got.ClusterName)
	}

	// Test data plane component
	cmData := &ClusterManager{
		componentType:      ComponentTypeDataPlane,
		currentCluster:     currentCluster,
		defaultClusterName: "cluster1",
		clusters: map[string]*ClusterClientSet{
			"current":  currentCluster,
			"cluster1": cluster1,
		},
	}

	// Data plane always returns current cluster regardless of input
	got, err = cmData.GetClusterClientsOrDefault("")
	if err != nil || got.ClusterName != "current" {
		t.Errorf("DataPlane GetClusterClientsOrDefault('') = %v, want current", got.ClusterName)
	}

	got, err = cmData.GetClusterClientsOrDefault("cluster1")
	if err != nil || got.ClusterName != "current" {
		t.Errorf("DataPlane GetClusterClientsOrDefault('cluster1') = %v, want current", got.ClusterName)
	}
}
