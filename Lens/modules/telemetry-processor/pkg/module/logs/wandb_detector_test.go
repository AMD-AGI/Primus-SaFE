package logs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWandBFrameworkDetector_DetectFromEnvVars(t *testing.T) {
	detector := &WandBFrameworkDetector{}

	tests := []struct {
		name          string
		env           map[string]string
		expectedFw    string
		expectedConf  float64
		expectedNil   bool
	}{
		{
			name: "Primus environment variables",
			env: map[string]string{
				"PRIMUS_CONFIG":  "/config.yaml",
				"PRIMUS_VERSION": "1.2.3",
			},
			expectedFw:   "primus",
			expectedConf: 0.80,
			expectedNil:  false,
		},
		{
			name: "DeepSpeed environment variables",
			env: map[string]string{
				"DEEPSPEED_CONFIG": "/ds_config.json",
			},
			expectedFw:   "deepspeed",
			expectedConf: 0.80,
			expectedNil:  false,
		},
		{
			name: "Megatron environment variables",
			env: map[string]string{
				"MEGATRON_CONFIG": "/megatron_config.json",
			},
			expectedFw:   "megatron",
			expectedConf: 0.80,
			expectedNil:  false,
		},
		{
			name: "Generic FRAMEWORK environment variable",
			env: map[string]string{
				"FRAMEWORK": "PyTorch",
			},
			expectedFw:   "pytorch",
			expectedConf: 0.75,
			expectedNil:  false,
		},
		{
			name:        "No matching environment variables",
			env:         map[string]string{},
			expectedNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.detectFromEnvVars(tt.env)
			if tt.expectedNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedFw, result.Framework)
				assert.Equal(t, tt.expectedConf, result.Confidence)
				assert.Equal(t, "env_vars", result.Method)
			}
		})
	}
}

func TestWandBFrameworkDetector_DetectFromWandBConfig(t *testing.T) {
	detector := &WandBFrameworkDetector{}

	tests := []struct {
		name         string
		config       map[string]interface{}
		expectedFw   string
		expectedConf float64
		expectedNil  bool
	}{
		{
			name: "config.framework field",
			config: map[string]interface{}{
				"framework": "primus",
			},
			expectedFw:   "primus",
			expectedConf: 0.70,
			expectedNil:  false,
		},
		{
			name: "config.trainer field with deepspeed",
			config: map[string]interface{}{
				"trainer": "deepspeed",
			},
			expectedFw:   "deepspeed",
			expectedConf: 0.65,
			expectedNil:  false,
		},
		{
			name: "primus_config key",
			config: map[string]interface{}{
				"primus_config": "/path/to/config",
			},
			expectedFw:   "primus",
			expectedConf: 0.65,
			expectedNil:  false,
		},
		{
			name:        "No matching config",
			config:      map[string]interface{}{},
			expectedNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.detectFromWandBConfig(tt.config)
			if tt.expectedNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedFw, result.Framework)
				assert.Equal(t, tt.expectedConf, result.Confidence)
			}
		})
	}
}

func TestWandBFrameworkDetector_DetectFromPyTorchModules(t *testing.T) {
	detector := &WandBFrameworkDetector{}

	tests := []struct {
		name         string
		pytorch      *PyTorchInfo
		expectedFw   string
		expectedConf float64
		expectedNil  bool
	}{
		{
			name: "DeepSpeed module detected",
			pytorch: &PyTorchInfo{
				DetectedModules: map[string]bool{
					"deepspeed": true,
					"megatron":  false,
				},
			},
			expectedFw:   "deepspeed",
			expectedConf: 0.60,
			expectedNil:  false,
		},
		{
			name: "Megatron module detected",
			pytorch: &PyTorchInfo{
				DetectedModules: map[string]bool{
					"deepspeed": false,
					"megatron":  true,
				},
			},
			expectedFw:   "megatron",
			expectedConf: 0.60,
			expectedNil:  false,
		},
		{
			name: "No modules detected",
			pytorch: &PyTorchInfo{
				DetectedModules: map[string]bool{},
			},
			expectedNil: true,
		},
		{
			name:        "Nil modules",
			pytorch:     &PyTorchInfo{},
			expectedNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.detectFromPyTorchModules(tt.pytorch)
			if tt.expectedNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedFw, result.Framework)
				assert.Equal(t, tt.expectedConf, result.Confidence)
			}
		})
	}
}

func TestWandBFrameworkDetector_DetectFromWandBProject(t *testing.T) {
	detector := &WandBFrameworkDetector{}

	tests := []struct {
		name         string
		project      string
		expectedFw   string
		expectedConf float64
		expectedNil  bool
	}{
		{
			name:         "Primus project name",
			project:      "primus-training-exp",
			expectedFw:   "primus",
			expectedConf: 0.50,
			expectedNil:  false,
		},
		{
			name:         "DeepSpeed project name",
			project:      "my-deepspeed-experiment",
			expectedFw:   "deepspeed",
			expectedConf: 0.50,
			expectedNil:  false,
		},
		{
			name:         "Megatron project name",
			project:      "megatron-lm-training",
			expectedFw:   "megatron",
			expectedConf: 0.50,
			expectedNil:  false,
		},
		{
			name:        "Unknown project name",
			project:     "my-generic-project",
			expectedNil: true,
		},
		{
			name:        "Empty project name",
			project:     "",
			expectedNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.detectFromWandBProject(tt.project)
			if tt.expectedNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedFw, result.Framework)
				assert.Equal(t, tt.expectedConf, result.Confidence)
			}
		})
	}
}

func TestHasAnyKey(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]string
		keys     []string
		expected []string
	}{
		{
			name: "Multiple matches",
			m: map[string]string{
				"PRIMUS_CONFIG":  "/config.yaml",
				"PRIMUS_VERSION": "1.2.3",
				"OTHER_VAR":      "value",
			},
			keys:     []string{"PRIMUS_CONFIG", "PRIMUS_VERSION", "PRIMUS_BACKEND"},
			expected: []string{"PRIMUS_CONFIG", "PRIMUS_VERSION"},
		},
		{
			name: "Single match",
			m: map[string]string{
				"DEEPSPEED_CONFIG": "/ds_config.json",
			},
			keys:     []string{"DEEPSPEED_CONFIG", "DS_CONFIG"},
			expected: []string{"DEEPSPEED_CONFIG"},
		},
		{
			name: "No matches",
			m: map[string]string{
				"OTHER_VAR": "value",
			},
			keys:     []string{"PRIMUS_CONFIG", "DEEPSPEED_CONFIG"},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasAnyKey(tt.m, tt.keys)
			assert.Equal(t, len(tt.expected), len(result))
			for _, expectedKey := range tt.expected {
				assert.Contains(t, result, expectedKey)
			}
		})
	}
}

func TestWandBFrameworkDetector_ProcessWandBDetection_ValidationErrors(t *testing.T) {
	// Skip this test because it requires a real DetectionManager
	// Will be covered in integration tests
	t.Skip("Requires full DetectionManager setup - tested in integration tests")
	
	tests := []struct {
		name    string
		req     *WandBDetectionRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Missing workload_uid",
			req: &WandBDetectionRequest{
				WorkloadUID: "",
			},
			wantErr: true,
			errMsg:  "workload_uid is required",
		},
		{
			name: "Valid request",
			req: &WandBDetectionRequest{
				WorkloadUID: "workload-123",
				Evidence: WandBEvidence{
					Environment: map[string]string{
						"PRIMUS_CONFIG": "/config.yaml",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test implementation would go here
			_ = tt
		})
	}
}

