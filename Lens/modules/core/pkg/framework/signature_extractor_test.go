package framework

import (
	"testing"

	"github.com/stretchr/testify/assert"

	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func TestSignatureExtractor_ExtractSignature(t *testing.T) {
	extractor := NewSignatureExtractor()

	tests := []struct {
		name     string
		workload *Workload
		validate func(t *testing.T, sig *coreModel.WorkloadSignature)
	}{
		{
			name: "basic workload",
			workload: &Workload{
				UID:       "test-uid-1",
				Image:     "registry.example.com/primus:v1.2.3",
				Command:   []string{"/usr/bin/python3", "train.py"},
				Args:      []string{"--epochs", "100"},
				Env:       map[string]string{"FRAMEWORK": "PyTorch"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
			validate: func(t *testing.T, sig *coreModel.WorkloadSignature) {
				assert.Equal(t, "registry.example.com/primus:v1.2.3", sig.Image)
				assert.Equal(t, []string{"python", "train.py"}, sig.Command) // python3 normalized to python
				assert.Equal(t, []string{"--epochs", "100"}, sig.Args)
				assert.Equal(t, map[string]string{"FRAMEWORK": "PyTorch"}, sig.Env)
				assert.Equal(t, map[string]string{"app": "training"}, sig.Labels)
				assert.Equal(t, "default", sig.Namespace)
				assert.NotEmpty(t, sig.ImageHash)
				assert.NotEmpty(t, sig.CommandHash)
				assert.NotEmpty(t, sig.EnvHash)
			},
		},
		{
			name: "workload with sensitive env",
			workload: &Workload{
				UID:   "test-uid-2",
				Image: "registry.example.com/primus:v1.0.0",
				Env: map[string]string{
					"FRAMEWORK":    "PyTorch",
					"API_PASSWORD": "secret123",
					"API_TOKEN":    "token456",
					"DATABASE_URL": "postgresql://...",
				},
				Namespace: "default",
			},
			validate: func(t *testing.T, sig *coreModel.WorkloadSignature) {
				// Sensitive env vars should be filtered out
				assert.Contains(t, sig.Env, "FRAMEWORK")
				assert.Contains(t, sig.Env, "DATABASE_URL")
				assert.NotContains(t, sig.Env, "API_PASSWORD")
				assert.NotContains(t, sig.Env, "API_TOKEN")
			},
		},
		{
			name: "workload with dynamic labels",
			workload: &Workload{
				UID:   "test-uid-3",
				Image: "registry.example.com/primus:latest",
				Labels: map[string]string{
					"app":                       "training",
					"pod-template-hash":         "abc123",
					"controller-revision-hash":  "def456",
				},
				Namespace: "default",
			},
			validate: func(t *testing.T, sig *coreModel.WorkloadSignature) {
				// Dynamic labels should be filtered out
				assert.Contains(t, sig.Labels, "app")
				assert.NotContains(t, sig.Labels, "pod-template-hash")
				assert.NotContains(t, sig.Labels, "controller-revision-hash")
			},
		},
		{
			name: "empty workload",
			workload: &Workload{
				UID:       "test-uid-4",
				Image:     "",
				Command:   []string{},
				Args:      []string{},
				Env:       map[string]string{},
				Labels:    map[string]string{},
				Namespace: "default",
			},
			validate: func(t *testing.T, sig *coreModel.WorkloadSignature) {
				assert.Empty(t, sig.Image)
				assert.Empty(t, sig.Command)
				assert.Empty(t, sig.Args)
				assert.Empty(t, sig.Env)
				assert.Empty(t, sig.Labels)
				assert.Equal(t, "default", sig.Namespace)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := extractor.ExtractSignature(tt.workload)
			assert.NotNil(t, sig)
			tt.validate(t, sig)
		})
	}
}

func TestSignatureExtractor_NormalizeCommand(t *testing.T) {
	extractor := NewSignatureExtractor()

	tests := []struct {
		name     string
		command  []string
		expected []string
	}{
		{
			name:     "full path command",
			command:  []string{"/usr/bin/python3", "train.py"},
			expected: []string{"python", "train.py"}, // python3 normalized to python
		},
		{
			name:     "relative path command",
			command:  []string{"./bin/python", "script.py"},
			expected: []string{"python", "script.py"},
		},
		{
			name:     "simple command",
			command:  []string{"python", "train.py"},
			expected: []string{"python", "train.py"},
		},
		{
			name:     "empty command",
			command:  []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.normalizeCommand(tt.command)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSignatureExtractor_CalculateImageHash(t *testing.T) {
	extractor := NewSignatureExtractor()

	tests := []struct {
		name     string
		image1   string
		image2   string
		samehash bool
	}{
		{
			name:     "same image",
			image1:   "registry.example.com/primus:v1.2.3",
			image2:   "registry.example.com/primus:v1.2.3",
			samehash: true,
		},
		{
			name:     "case insensitive",
			image1:   "REGISTRY.EXAMPLE.COM/PRIMUS:V1.2.3",
			image2:   "registry.example.com/primus:v1.2.3",
			samehash: true,
		},
		{
			name:     "different images",
			image1:   "registry.example.com/primus:v1.2.3",
			image2:   "registry.example.com/primus:v1.2.4",
			samehash: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := extractor.calculateImageHash(tt.image1)
			hash2 := extractor.calculateImageHash(tt.image2)

			if tt.samehash {
				assert.Equal(t, hash1, hash2)
			} else {
				assert.NotEqual(t, hash1, hash2)
			}
		})
	}
}

func TestSignatureExtractor_CalculateCommandHash(t *testing.T) {
	extractor := NewSignatureExtractor()

	tests := []struct {
		name     string
		cmd1     []string
		cmd2     []string
		samehash bool
	}{
		{
			name:     "same command",
			cmd1:     []string{"python", "train.py"},
			cmd2:     []string{"python", "train.py"},
			samehash: true,
		},
		{
			name:     "different order - same hash (sorted before hashing)",
			cmd1:     []string{"python", "train.py"},
			cmd2:     []string{"train.py", "python"},
			samehash: true,
		},
		{
			name:     "different command",
			cmd1:     []string{"python", "train.py"},
			cmd2:     []string{"python", "test.py"},
			samehash: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := extractor.calculateCommandHash(tt.cmd1)
			hash2 := extractor.calculateCommandHash(tt.cmd2)

			if tt.samehash {
				assert.Equal(t, hash1, hash2)
			} else {
				assert.NotEqual(t, hash1, hash2)
			}
		})
	}
}

func TestSignatureExtractor_CalculateEnvHash(t *testing.T) {
	extractor := NewSignatureExtractor()

	tests := []struct {
		name     string
		env1     map[string]string
		env2     map[string]string
		samehash bool
	}{
		{
			name: "same key env vars",
			env1: map[string]string{
				"FRAMEWORK":   "PyTorch",
				"WORLD_SIZE":  "8",
				"OTHER_VAR":   "value1",
			},
			env2: map[string]string{
				"FRAMEWORK":   "PyTorch",
				"WORLD_SIZE":  "8",
				"OTHER_VAR":   "value2",
			},
			samehash: true, // Only key vars affect hash
		},
		{
			name: "different key env vars",
			env1: map[string]string{
				"FRAMEWORK":   "PyTorch",
			},
			env2: map[string]string{
				"FRAMEWORK":   "TensorFlow",
			},
			samehash: false,
		},
		{
			name: "missing key env var",
			env1: map[string]string{
				"FRAMEWORK":   "PyTorch",
				"WORLD_SIZE":  "8",
			},
			env2: map[string]string{
				"FRAMEWORK":   "PyTorch",
			},
			samehash: false,
		},
		{
			name:     "empty env",
			env1:     map[string]string{},
			env2:     map[string]string{},
			samehash: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := extractor.calculateEnvHash(tt.env1)
			hash2 := extractor.calculateEnvHash(tt.env2)

			if tt.samehash {
				assert.Equal(t, hash1, hash2)
			} else {
				assert.NotEqual(t, hash1, hash2)
			}
		})
	}
}

func TestExtractImageRepo(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "image with tag",
			image:    "registry.example.com/primus:v1.2.3",
			expected: "registry.example.com/primus",
		},
		{
			name:     "image without tag",
			image:    "registry.example.com/primus",
			expected: "registry.example.com/primus",
		},
		{
			name:     "docker hub image",
			image:    "pytorch/pytorch:1.9.0",
			expected: "pytorch/pytorch",
		},
		{
			name:     "latest tag",
			image:    "registry.example.com/primus:latest",
			expected: "registry.example.com/primus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractImageRepo(tt.image)
			assert.Equal(t, tt.expected, result)
		})
	}
}

