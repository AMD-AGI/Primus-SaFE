/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TestGenerateName tests the GenerateName function
func TestGenerateName(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		validate func(*testing.T, string, string)
	}{
		{
			name: "normal base name",
			base: "myapp",
			validate: func(t *testing.T, base, result string) {
				assert.Contains(t, result, base)
				assert.Contains(t, result, "-")
				assert.Equal(t, len(base)+1+randomLength, len(result))
			},
		},
		{
			name: "empty base name",
			base: "",
			validate: func(t *testing.T, base, result string) {
				assert.Empty(t, result)
			},
		},
		{
			name: "long base name exceeds max length",
			base: strings.Repeat("a", MaxGeneratedNameLength+10),
			validate: func(t *testing.T, base, result string) {
				// Should be truncated to MaxGeneratedNameLength + 1 (dash) + randomLength
				assert.LessOrEqual(t, len(result), MaxNameLength)
				assert.Contains(t, result, "-")
			},
		},
		{
			name: "base name at max length",
			base: strings.Repeat("b", MaxGeneratedNameLength),
			validate: func(t *testing.T, base, result string) {
				assert.Equal(t, MaxNameLength, len(result))
			},
		},
		{
			name: "names with special characters",
			base: "my-app-123",
			validate: func(t *testing.T, base, result string) {
				assert.Contains(t, result, base)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateName(tt.base)
			tt.validate(t, tt.base, result)
		})
	}
}

// TestGetBaseFromName tests extracting base names from generated names
func TestGetBaseFromName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard generated name",
			input:    "myapp-abc12",
			expected: "myapp",
		},
		{
			name:     "name without suffix",
			input:    "myapp",
			expected: "myapp",
		},
		{
			name:     "very short name",
			input:    "ab",
			expected: "ab",
		},
		{
			name:     "name with multiple dashes",
			input:    "my-app-test-xyz12",
			expected: "my-app-test",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "name with exact random length",
			input:    "test-",
			expected: "test-",
		},
		{
			name:     "name without dash before suffix",
			input:    "testxyz12",
			expected: "testxyz12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBaseFromName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGenerateAndGetBaseName tests round-trip of generate and extract
func TestGenerateAndGetBaseName(t *testing.T) {
	bases := []string{"app", "service", "my-deployment", "test123"}

	for _, base := range bases {
		t.Run(base, func(t *testing.T) {
			generated := GenerateName(base)
			extracted := GetBaseFromName(generated)
			assert.Equal(t, base, extracted)
		})
	}
}

// TestGenObjectReference tests ObjectReference generation
func TestGenObjectReference(t *testing.T) {
	tests := []struct {
		name     string
		typeMeta metav1.TypeMeta
		objMeta  metav1.ObjectMeta
		validate func(*testing.T, *corev1.ObjectReference)
	}{
		{
			name: "complete object reference",
			typeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Pod",
			},
			objMeta: metav1.ObjectMeta{
				Name:            "test-pod",
				Namespace:       "default",
				UID:             types.UID("12345"),
				ResourceVersion: "1000",
			},
			validate: func(t *testing.T, ref *corev1.ObjectReference) {
				assert.Equal(t, "test-pod", ref.Name)
				assert.Equal(t, "default", ref.Namespace)
				assert.Equal(t, types.UID("12345"), ref.UID)
				assert.Equal(t, "v1", ref.APIVersion)
				assert.Equal(t, "Pod", ref.Kind)
				assert.Equal(t, "1000", ref.ResourceVersion)
			},
		},
		{
			name: "minimal object reference",
			typeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			objMeta: metav1.ObjectMeta{
				Name: "app-deployment",
			},
			validate: func(t *testing.T, ref *corev1.ObjectReference) {
				assert.Equal(t, "app-deployment", ref.Name)
				assert.Empty(t, ref.Namespace)
				assert.Equal(t, "apps/v1", ref.APIVersion)
				assert.Equal(t, "Deployment", ref.Kind)
			},
		},
		{
			name: "custom resource reference",
			typeMeta: metav1.TypeMeta{
				APIVersion: "amd.com/v1",
				Kind:       "Cluster",
			},
			objMeta: metav1.ObjectMeta{
				Name:      "my-cluster",
				Namespace: "kube-system",
			},
			validate: func(t *testing.T, ref *corev1.ObjectReference) {
				assert.Equal(t, "my-cluster", ref.Name)
				assert.Equal(t, "kube-system", ref.Namespace)
				assert.Equal(t, "amd.com/v1", ref.APIVersion)
				assert.Equal(t, "Cluster", ref.Kind)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenObjectReference(tt.typeMeta, tt.objMeta)
			assert.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

// TestGenerateClusterPriorityClass tests priority class name generation
func TestGenerateClusterPriorityClass(t *testing.T) {
	tests := []struct {
		name          string
		clusterId     string
		priorityClass string
		expected      string
	}{
		{
			name:          "standard priority class",
			clusterId:     "cluster-1",
			priorityClass: "high",
			expected:      "cluster-1-high",
		},
		{
			name:          "system priority class",
			clusterId:     "prod-cluster",
			priorityClass: "system-node-critical",
			expected:      "prod-cluster-system-node-critical",
		},
		{
			name:          "empty cluster id",
			clusterId:     "",
			priorityClass: "medium",
			expected:      "-medium",
		},
		{
			name:          "empty priority class",
			clusterId:     "cluster-2",
			priorityClass: "",
			expected:      "cluster-2-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateClusterPriorityClass(tt.clusterId, tt.priorityClass)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGenerateClusterSecret tests secret name generation
func TestGenerateClusterSecret(t *testing.T) {
	tests := []struct {
		name       string
		clusterId  string
		secretName string
		expected   string
	}{
		{
			name:       "standard secret",
			clusterId:  "cluster-1",
			secretName: "docker-registry",
			expected:   "cluster-1-docker-registry",
		},
		{
			name:       "ssh secret",
			clusterId:  "dev-cluster",
			secretName: "ssh-key",
			expected:   "dev-cluster-ssh-key",
		},
		{
			name:       "empty cluster id",
			clusterId:  "",
			secretName: "my-secret",
			expected:   "-my-secret",
		},
		{
			name:       "empty secret name",
			clusterId:  "cluster-3",
			secretName: "",
			expected:   "cluster-3-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateClusterSecret(tt.clusterId, tt.secretName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTransMapToStruct tests map to struct conversion
func TestTransMapToStruct(t *testing.T) {
	type TestStruct struct {
		Name  string `json:"name"`
		Age   int    `json:"age"`
		Email string `json:"email"`
	}

	tests := []struct {
		name     string
		input    map[string]interface{}
		wantErr  bool
		validate func(*testing.T, TestStruct)
	}{
		{
			name: "complete map",
			input: map[string]interface{}{
				"name":  "John Doe",
				"age":   30,
				"email": "john@example.com",
			},
			wantErr: false,
			validate: func(t *testing.T, result TestStruct) {
				assert.Equal(t, "John Doe", result.Name)
				assert.Equal(t, 30, result.Age)
				assert.Equal(t, "john@example.com", result.Email)
			},
		},
		{
			name: "partial map",
			input: map[string]interface{}{
				"name": "Jane Smith",
				"age":  25,
			},
			wantErr: false,
			validate: func(t *testing.T, result TestStruct) {
				assert.Equal(t, "Jane Smith", result.Name)
				assert.Equal(t, 25, result.Age)
				assert.Empty(t, result.Email)
			},
		},
		{
			name:    "empty map",
			input:   map[string]interface{}{},
			wantErr: false,
			validate: func(t *testing.T, result TestStruct) {
				assert.Empty(t, result.Name)
				assert.Equal(t, 0, result.Age)
				assert.Empty(t, result.Email)
			},
		},
		{
			name: "type coercion",
			input: map[string]interface{}{
				"name": "Test User",
				"age":  float64(40), // JSON numbers are float64
			},
			wantErr: false,
			validate: func(t *testing.T, result TestStruct) {
				assert.Equal(t, "Test User", result.Name)
				assert.Equal(t, 40, result.Age)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result TestStruct
			err := TransMapToStruct(tt.input, &result)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				tt.validate(t, result)
			}
		})
	}
}
