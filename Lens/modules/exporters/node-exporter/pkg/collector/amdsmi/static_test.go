package amdsmi

import (
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestParseGPUInfoArray(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    []byte
		expectedLen int
		expectError bool
	}{
		{
			name: "Valid single GPU JSON",
			jsonData: []byte(`[{
				"asic": {
					"market_name": "AMD Instinct MI300X",
					"vendor_id": "0x1002",
					"vendor_name": "Advanced Micro Devices Inc. [AMD/ATI]",
					"device_id": "0x74a0",
					"rev_id": "0x00",
					"asic_serial": "0x12345678",
					"oam_id": "0"
				},
				"bus": {
					"bdf": "0000:0c:00.0",
					"max_pcie_width": 16,
					"max_pcie_speed": {"value": 5.0, "unit": "GT/s"},
					"pcie_interface_version": "GEN 4",
					"slot_type": "PCIE"
				},
				"vbios": {
					"name": "VBIOS Name",
					"build_date": "2024/01/01",
					"part_number": "123-456-789",
					"version": "1.0.0"
				},
				"gpu": 0
			}]`),
			expectedLen: 1,
			expectError: false,
		},
		{
			name: "Valid multiple GPUs JSON",
			jsonData: []byte(`[
				{
					"asic": {"market_name": "GPU 1", "asic_serial": "111"},
					"bus": {"bdf": "0000:01:00.0"},
					"gpu": 0
				},
				{
					"asic": {"market_name": "GPU 2", "asic_serial": "222"},
					"bus": {"bdf": "0000:02:00.0"},
					"gpu": 1
				},
				{
					"asic": {"market_name": "GPU 3", "asic_serial": "333"},
					"bus": {"bdf": "0000:03:00.0"},
					"gpu": 2
				}
			]`),
			expectedLen: 3,
			expectError: false,
		},
		{
			name:        "Empty JSON array",
			jsonData:    []byte(`[]`),
			expectedLen: 0,
			expectError: false,
		},
		{
			name:        "Invalid JSON - missing bracket",
			jsonData:    []byte(`[{"gpu": 0}`),
			expectedLen: 0,
			expectError: true,
		},
		{
			name:        "Invalid JSON - malformed",
			jsonData:    []byte(`{invalid json}`),
			expectedLen: 0,
			expectError: true,
		},
		{
			name:        "Empty input",
			jsonData:    []byte(``),
			expectedLen: 0,
			expectError: true,
		},
		{
			name:        "Null JSON",
			jsonData:    []byte(`null`),
			expectedLen: 0,
			expectError: false,
		},
		{
			name: "JSON with extra fields",
			jsonData: []byte(`[{
				"asic": {"market_name": "Test GPU"},
				"bus": {"bdf": "0000:01:00.0"},
				"gpu": 0,
				"extra_field": "should be ignored"
			}]`),
			expectedLen: 1,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseGPUInfoArray(tt.jsonData)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedLen, len(result))
			}
		})
	}
}

func TestParseGPUInfoArray_ValidateFields(t *testing.T) {
	jsonData := []byte(`[{
		"asic": {
			"market_name": "AMD Instinct MI300X",
			"vendor_id": "0x1002",
			"device_id": "0x74a0",
			"asic_serial": "ABC123"
		},
		"bus": {
			"bdf": "0000:0c:00.0",
			"max_pcie_width": 16,
			"max_pcie_speed": {"value": 5.0, "unit": "GT/s"}
		},
		"vbios": {
			"version": "1.2.3"
		},
		"gpu": 0
	}]`)

	result, err := ParseGPUInfoArray(jsonData)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result))

	gpu := result[0]
	assert.Equal(t, "AMD Instinct MI300X", gpu.Asic.MarketName)
	assert.Equal(t, "0x1002", gpu.Asic.VendorID)
	assert.Equal(t, "0x74a0", gpu.Asic.DeviceID)
	assert.Equal(t, "ABC123", gpu.Asic.AsicSerial)
	assert.Equal(t, "0000:0c:00.0", gpu.Bus.BDF)
	assert.Equal(t, 16, gpu.Bus.MaxPCIeWidth)
	assert.Equal(t, 5.0, gpu.Bus.MaxPCIeSpeed.Value)
	assert.Equal(t, "GT/s", gpu.Bus.MaxPCIeSpeed.Unit)
	assert.Equal(t, "1.2.3", gpu.VBIOS.Version)
	assert.Equal(t, 0, gpu.GPU)
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple JSON array",
			input:    `[{"key": "value"}]`,
			expected: `[{"key": "value"}]`,
		},
		{
			name:     "JSON array with prefix",
			input:    `Some text before [{"key": "value"}]`,
			expected: `[{"key": "value"}]`,
		},
		{
			name:     "JSON array with suffix",
			input:    `[{"key": "value"}] and some text after`,
			expected: `[{"key": "value"}]`,
		},
		{
			name:     "JSON array with prefix and suffix",
			input:    `Before [{"key": "value"}] After`,
			expected: `[{"key": "value"}]`,
		},
		{
			name:     "Multi-line JSON with text",
			input:    "Debug info\n[{\"gpu\": 0}, {\"gpu\": 1}]\nMore debug",
			expected: `[{"gpu": 0}, {"gpu": 1}]`,
		},
		{
			name:     "Empty JSON array",
			input:    `[]`,
			expected: `[]`,
		},
		{
			name:     "Empty JSON array with text",
			input:    `Some text [] more text`,
			expected: `[]`,
		},
		{
			name:     "No brackets",
			input:    `This is just text without brackets`,
			expected: ``,
		},
		{
			name:     "Only opening bracket",
			input:    `[{"incomplete": true`,
			expected: ``,
		},
		{
			name:     "Only closing bracket",
			input:    `"incomplete": true}]`,
			expected: ``,
		},
		{
			name:     "Brackets in wrong order",
			input:    `]wrong order[`,
			expected: ``,
		},
		{
			name:     "Empty string",
			input:    ``,
			expected: ``,
		},
		{
			name:     "Multiple JSON arrays - extracts first to last",
			input:    `[{"first": 1}] and [{"second": 2}]`,
			expected: `[{"first": 1}] and [{"second": 2}]`,
		},
		{
			name:     "Nested brackets",
			input:    `[{"nested": [1, 2, 3]}]`,
			expected: `[{"nested": [1, 2, 3]}]`,
		},
		{
			name:     "Real amd-smi output example",
			input:    "AMD SMI version: 1.0.0\nDriver version: 6.1.0\n[{\"gpu\": 0, \"asic\": {\"market_name\": \"MI300X\"}}]\nEnd of output",
			expected: `[{"gpu": 0, "asic": {"market_name": "MI300X"}}]`,
		},
		{
			name:     "Single bracket character",
			input:    `[`,
			expected: ``,
		},
		{
			name:     "Brackets with spaces",
			input:    `   [  {"key": "value"}  ]   `,
			expected: `[  {"key": "value"}  ]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseGPUInfoArray_EdgeCases(t *testing.T) {
	t.Run("Very large JSON array", func(t *testing.T) {
		// Create JSON with 100 GPUs
		json := `[`
		for i := 0; i < 100; i++ {
			if i > 0 {
				json += `,`
			}
			json += `{"gpu": ` + string(rune('0'+i%10)) + `, "asic": {"market_name": "GPU"}}`
		}
		json += `]`

		result, err := ParseGPUInfoArray([]byte(json))
		assert.NoError(t, err)
		assert.Equal(t, 100, len(result))
	})

	t.Run("JSON with unicode characters", func(t *testing.T) {
		jsonData := []byte(`[{
			"asic": {"market_name": "GPU-test🚀"},
			"gpu": 0
		}]`)

		result, err := ParseGPUInfoArray(jsonData)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
	})

	t.Run("JSON with escaped characters", func(t *testing.T) {
		jsonData := []byte(`[{
			"asic": {"market_name": "GPU\"Test\""},
			"gpu": 0
		}]`)

		result, err := ParseGPUInfoArray(jsonData)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
		assert.Equal(t, `GPU"Test"`, result[0].Asic.MarketName)
	})
}

func TestExtractJSON_Performance(t *testing.T) {
	t.Run("Large input string", func(t *testing.T) {
		// Create a large string with JSON at the end
		largePrefix := ""
		for i := 0; i < 10000; i++ {
			largePrefix += "Debug line " + string(rune('0'+i%10)) + "\n"
		}
		input := largePrefix + `[{"gpu": 0}]`

		result := extractJSON(input)
		assert.Equal(t, `[{"gpu": 0}]`, result)
	})
}

func TestParseGPUInfoArray_Integration(t *testing.T) {
	t.Run("Parse and extract combined", func(t *testing.T) {
		rawOutput := `AMD SMI Output:
[{"gpu": 0, "asic": {"market_name": "MI300X"}}, {"gpu": 1, "asic": {"market_name": "MI300A"}}]
End of output`

		extracted := extractJSON(rawOutput)
		assert.NotEmpty(t, extracted)

		gpus, err := ParseGPUInfoArray([]byte(extracted))
		assert.NoError(t, err)
		assert.Equal(t, 2, len(gpus))
		assert.Equal(t, 0, gpus[0].GPU)
		assert.Equal(t, 1, gpus[1].GPU)
	})
}

func TestParseGPUInfoArray_TypeSafety(t *testing.T) {
	t.Run("Return type is correct", func(t *testing.T) {
		jsonData := []byte(`[{"gpu": 0}]`)
		result, err := ParseGPUInfoArray(jsonData)

		assert.NoError(t, err)
		assert.IsType(t, []model.GPUInfo{}, result)
	})

	t.Run("Nil check", func(t *testing.T) {
		result, err := ParseGPUInfoArray(nil)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}
