package database

import (
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetBuiltinTemplates tests getting all builtin templates
func TestGetBuiltinTemplates(t *testing.T) {
	templates := getBuiltinTemplates()
	
	require.NotNil(t, templates)
	assert.Greater(t, len(templates), 0, "Should have at least one template")
	
	// Verify all templates have required fields
	for _, tpl := range templates {
		assert.NotEmpty(t, tpl.Name, "Template name should not be empty")
		assert.NotEmpty(t, tpl.Category, "Template category should not be empty")
		assert.NotEmpty(t, tpl.Description, "Template description should not be empty")
		assert.NotNil(t, tpl.TemplateConfig, "Template config should not be nil")
		assert.True(t, tpl.IsBuiltin, "Template should be marked as builtin")
		assert.Equal(t, "system", tpl.CreatedBy, "Template should be created by system")
	}
}

// TestGetBuiltinTemplates_Categories tests template categorization
func TestGetBuiltinTemplates_Categories(t *testing.T) {
	templates := getBuiltinTemplates()
	
	categories := make(map[string]int)
	for _, tpl := range templates {
		categories[tpl.Category]++
	}
	
	// Should have multiple categories
	assert.Greater(t, len(categories), 1, "Should have multiple categories")
	
	// Known categories should exist
	expectedCategories := []string{"basic", "gpu", "network", "training", "performance"}
	for _, cat := range expectedCategories {
		assert.Contains(t, categories, cat, "Should contain category: %s", cat)
	}
}

// TestGetBuiltinTemplates_SpecificTemplates tests specific template existence
func TestGetBuiltinTemplates_SpecificTemplates(t *testing.T) {
	templates := getBuiltinTemplates()
	
	templateMap := make(map[string]*model.LogAlertRuleTemplates)
	for _, tpl := range templates {
		templateMap[tpl.Name] = tpl
	}
	
	tests := []struct {
		name     string
		category string
	}{
		{"Generic-Error-Detection", "basic"},
		{"GPU-OOM-Detection", "gpu"},
		{"GPU-OOM-Frequency", "gpu"},
		{"NCCL-Error-Detection", "network"},
		{"InfiniBand-Error-Detection", "network"},
		{"Training-Loss-NaN", "training"},
		{"Training-Checkpoint-Failed", "training"},
		{"Training-Throughput-Degradation", "performance"},
		{"Pod-Restart-Detection", "basic"},
		{"Production-Critical-Error", "basic"},
		{"Disk-Space-Warning", "basic"},
		{"Connection-Timeout", "network"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpl, exists := templateMap[tt.name]
			assert.True(t, exists, "Template %s should exist", tt.name)
			if exists {
				assert.Equal(t, tt.category, tpl.Category, "Template %s should be in category %s", tt.name, tt.category)
			}
		})
	}
}

// TestBuildTemplateConfig tests template config building
func TestBuildTemplateConfig(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]interface{}
	}{
		{
			name: "Simple config",
			config: map[string]interface{}{
				"severity": "warning",
				"enabled":  true,
			},
		},
		{
			name: "Complex config with nested structures",
			config: map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "namespace",
						"key":      "namespace",
						"operator": "exists",
					},
				},
				"match_type": "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "ERROR",
					"ignore_case": true,
				},
			},
		},
		{
			name:   "Empty config",
			config: map[string]interface{}{},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTemplateConfig(tt.config)
			
			assert.NotNil(t, result)
			
			// Verify all keys are present
			for key := range tt.config {
				assert.Contains(t, result, key, "Result should contain key: %s", key)
			}
		})
	}
}

// TestInitBuiltinLogAlertRuleTemplates tests template initialization
func TestInitBuiltinLogAlertRuleTemplates(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	ctx := helper.CreateTestContext()
	
	// Mock the facade to use our test DB
	// Since InitBuiltinLogAlertRuleTemplates uses GetFacade(), we need to set up the global facade
	// For this test, we'll test the function behavior indirectly by creating templates manually
	
	templates := getBuiltinTemplates()
	require.Greater(t, len(templates), 0)
	
	// Create a log alert rule facade with test DB
	facade := newTestLogAlertRuleFacade(helper.DB)
	
	// Create one of the templates
	template := templates[0]
	err := facade.CreateLogAlertRuleTemplate(ctx, template)
	require.NoError(t, err)
	
	// Verify it was created
	result, err := facade.GetLogAlertRuleTemplateByID(ctx, template.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, template.Name, result.Name)
	assert.Equal(t, template.Category, result.Category)
	assert.True(t, result.IsBuiltin)
}

// TestBuiltinTemplates_ConfigStructure tests the structure of template configs
func TestBuiltinTemplates_ConfigStructure(t *testing.T) {
	templates := getBuiltinTemplates()
	
	for _, tpl := range templates {
		t.Run(tpl.Name, func(t *testing.T) {
			config := tpl.TemplateConfig
			
			// All templates should have these basic fields
			if _, ok := config["match_type"]; ok {
				assert.Contains(t, []interface{}{"pattern", "threshold"}, config["match_type"])
			}
			
			if _, ok := config["severity"]; ok {
				assert.Contains(t, []interface{}{"critical", "warning", "info"}, config["severity"])
			}
			
			// Templates with match_config should have pattern
			if matchConfig, ok := config["match_config"].(map[string]interface{}); ok {
				if tpl.Category != "performance" { // Performance templates might not have pattern
					_, hasPattern := matchConfig["pattern"]
					assert.True(t, hasPattern || matchConfig["threshold"] != nil, 
						"Template %s should have pattern or threshold", tpl.Name)
				}
			}
		})
	}
}

// TestBuiltinTemplates_Tags tests template tagging
func TestBuiltinTemplates_Tags(t *testing.T) {
	templates := getBuiltinTemplates()
	
	for _, tpl := range templates {
		t.Run(tpl.Name, func(t *testing.T) {
			assert.NotEmpty(t, tpl.Tags, "Template %s should have tags", tpl.Name)
		})
	}
}

// TestBuiltinTemplates_GPU tests GPU-related templates
func TestBuiltinTemplates_GPU(t *testing.T) {
	templates := getBuiltinTemplates()
	
	var gpuTemplates []*model.LogAlertRuleTemplates
	for _, tpl := range templates {
		if tpl.Category == "gpu" {
			gpuTemplates = append(gpuTemplates, tpl)
		}
	}
	
	assert.Greater(t, len(gpuTemplates), 0, "Should have GPU templates")
	
	// GPU OOM template should exist
	var oomTemplate *model.LogAlertRuleTemplates
	for _, tpl := range gpuTemplates {
		if tpl.Name == "GPU-OOM-Detection" {
			oomTemplate = tpl
			break
		}
	}
	
	require.NotNil(t, oomTemplate, "GPU OOM template should exist")
	assert.Equal(t, "critical", oomTemplate.TemplateConfig["severity"])
}

// TestBuiltinTemplates_Network tests network-related templates
func TestBuiltinTemplates_Network(t *testing.T) {
	templates := getBuiltinTemplates()
	
	var networkTemplates []*model.LogAlertRuleTemplates
	for _, tpl := range templates {
		if tpl.Category == "network" {
			networkTemplates = append(networkTemplates, tpl)
		}
	}
	
	assert.Greater(t, len(networkTemplates), 0, "Should have network templates")
	
	// Should include NCCL and InfiniBand templates
	templateNames := make([]string, len(networkTemplates))
	for i, tpl := range networkTemplates {
		templateNames[i] = tpl.Name
	}
	
	assert.Contains(t, templateNames, "NCCL-Error-Detection")
	assert.Contains(t, templateNames, "InfiniBand-Error-Detection")
}

// TestBuiltinTemplates_Training tests training-related templates
func TestBuiltinTemplates_Training(t *testing.T) {
	templates := getBuiltinTemplates()
	
	var trainingTemplates []*model.LogAlertRuleTemplates
	for _, tpl := range templates {
		if tpl.Category == "training" {
			trainingTemplates = append(trainingTemplates, tpl)
		}
	}
	
	assert.Greater(t, len(trainingTemplates), 0, "Should have training templates")
	
	// Should include NaN and checkpoint templates
	templateNames := make([]string, len(trainingTemplates))
	for i, tpl := range trainingTemplates {
		templateNames[i] = tpl.Name
	}
	
	assert.Contains(t, templateNames, "Training-Loss-NaN")
	assert.Contains(t, templateNames, "Training-Checkpoint-Failed")
}

// ==================== Benchmark Tests ====================

func BenchmarkGetBuiltinTemplates(b *testing.B) {
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_ = getBuiltinTemplates()
	}
}

func BenchmarkBuildTemplateConfig(b *testing.B) {
	config := map[string]interface{}{
		"label_selectors": []map[string]interface{}{
			{
				"type":     "namespace",
				"key":      "namespace",
				"operator": "exists",
			},
		},
		"match_type": "pattern",
		"match_config": map[string]interface{}{
			"pattern":     "ERROR",
			"ignore_case": true,
		},
		"severity": "warning",
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_ = buildTemplateConfig(config)
	}
}

