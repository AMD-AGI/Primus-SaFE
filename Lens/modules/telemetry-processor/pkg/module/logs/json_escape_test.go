package logs

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// TestJSONEscaping 测试 JSON 转义对正则表达式的影响
func TestJSONEscaping(t *testing.T) {
	log := `[20251202 09:12:08][rank-7/8][INFO ] [--------trainer.py:2560] : iteration 126/ 5000 | consumed samples: 16128 | elapsed time per iteration (ms): 13372.8/13364.7 | hip mem usage/free/total/usage_ratio: 153.81GB/102.17GB/255.98GB/60.09% | throughput per GPU (TFLOP/s/GPU): 567.6/568.0 | tokens per GPU (tokens/s/GPU): 9801.4/9807.3 | learning rate: 9.984820E-06 | global batch size: 128 | lm loss: 6.548988E-03 | loss scale: 1.0 | grad norm: 0.061 | number of skipped iterations: 0 | number of nan iterations: 0 |`

	// 测试不同级别的转义
	tests := []struct {
		name        string
		jsonPattern string
		desc        string
	}{
		{
			name:        "直接字符串（无转义）",
			jsonPattern: `.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)`,
			desc:        "这是我们期望的最终正则表达式",
		},
		{
			name:        "JSON中的单层转义",
			jsonPattern: `.*iteration\\s+(?P<CurrentIteration>\\d+)\\s*/\\s*(?P<TargetIteration>\\d+)`,
			desc:        "JSON字符串字面量中需要转义反斜杠一次",
		},
		{
			name:        "JSON中的双层转义（SQL中的形式）",
			jsonPattern: `.*iteration\\\\s+(?P<CurrentIteration>\\\\d+)\\\\s*/\\\\s*(?P<TargetIteration>\\\\d+)`,
			desc:        "当JSON作为SQL字符串字面量时需要双重转义",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Description: %s", tt.desc)
			t.Logf("Pattern string: %s", tt.jsonPattern)
			t.Logf("Pattern length: %d", len(tt.jsonPattern))

			// 显示转义字符
			showEscapes := strings.ReplaceAll(tt.jsonPattern, `\`, `[BACKSLASH]`)
			t.Logf("Pattern with escapes visible: %s", showEscapes[:min(200, len(showEscapes))])

			// 尝试编译
			re, err := regexp.Compile(tt.jsonPattern)
			if err != nil {
				t.Errorf("❌ Failed to compile: %v", err)
				return
			}
			t.Logf("✓ Compiled successfully")

			// 尝试匹配
			matches := re.FindStringSubmatch(log)
			if matches == nil {
				t.Errorf("❌ Pattern did not match")
				return
			}
			t.Logf("✓ Pattern matched, got %d groups", len(matches))

			// 提取命名组
			groups := make(map[string]string)
			names := re.SubexpNames()
			for i, name := range names {
				if i > 0 && i < len(matches) && name != "" {
					groups[name] = matches[i]
				}
			}

			t.Logf("Extracted %d named groups", len(groups))
			for name, value := range groups {
				t.Logf("  %s = %q", name, value)
			}

			// 验证值
			if groups["CurrentIteration"] != "126" {
				t.Errorf("❌ CurrentIteration: got %q, want \"126\"", groups["CurrentIteration"])
			} else {
				t.Logf("✓ CurrentIteration = 126")
			}
			if groups["TargetIteration"] != "5000" {
				t.Errorf("❌ TargetIteration: got %q, want \"5000\"", groups["TargetIteration"])
			} else {
				t.Logf("✓ TargetIteration = 5000")
			}
		})
	}

	// 现在测试完整的 JSON 解析流程
	t.Run("完整JSON解析流程", func(t *testing.T) {
		// 这是数据库中存储的 JSON（带双重转义）
		jsonStr := `{
			"pattern": ".*iteration\\\\s+(?P<CurrentIteration>\\\\d+)\\\\s*/\\\\s*(?P<TargetIteration>\\\\d+)"
		}`

		var config struct {
			Pattern string `json:"pattern"`
		}

		if err := json.Unmarshal([]byte(jsonStr), &config); err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		t.Logf("After JSON parsing: %s", config.Pattern)
		showEscapes := strings.ReplaceAll(config.Pattern, `\`, `[BACKSLASH]`)
		t.Logf("With escapes visible: %s", showEscapes)

		// 编译并测试
		re, err := regexp.Compile(config.Pattern)
		if err != nil {
			t.Fatalf("Failed to compile: %v", err)
		}

		matches := re.FindStringSubmatch(log)
		if matches == nil {
			t.Fatal("Pattern did not match")
		}

		groups := make(map[string]string)
		names := re.SubexpNames()
		for i, name := range names {
			if i > 0 && i < len(matches) && name != "" {
				groups[name] = matches[i]
			}
		}

		t.Logf("Groups: %+v", groups)

		if groups["CurrentIteration"] != "126" {
			t.Errorf("❌ CurrentIteration: got %q, want \"126\"", groups["CurrentIteration"])
		} else {
			t.Logf("✓ CurrentIteration = 126")
		}
	})
}
