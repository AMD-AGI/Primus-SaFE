package logs

import (
	"encoding/json"
	"regexp"
	"testing"
)

// TestFullMatchingFlow 完整测试从 JSON 配置到日志匹配的整个流程
func TestFullMatchingFlow(t *testing.T) {
	// 1. 模拟从数据库加载的 JSON 配置（与 SQL 文件中的 JSON 一致）
	jsonConfig := `{
      "name": "primus-hip-memory-v2",
      "pattern": "\\\\..*iteration\\\\s+(?P<CurrentIteration>\\\\d+)\\\\s*/\\\\s*(?P<TargetIteration>\\\\d+)\\\\s*\\\\|\\\\s*consumed samples:\\\\s+(?P<ConsumedSamples>\\\\d+)\\\\s*\\\\|\\\\s*elapsed\\\\stime\\\\sper\\\\siteration\\\\s\\\\(ms\\\\):\\\\s+(?P<ElapsedTimePerIterationMS>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s+hip\\\\s+mem\\\\s+usage/free/total/usage_ratio:\\\\s+(?P<MemUsage>\\\\d+\\\\.\\\\d+)GB/(?P<MemFree>\\\\d+\\\\.\\\\d+)GB/(?P<MemTotal>\\\\d+\\\\.\\\\d+)GB/(?P<MemUsageRatio>\\\\d+\\\\.\\\\d+)%\\\\s+\\\\|\\\\s+throughput\\\\s+per\\\\s+GPU\\\\s+\\\\(TFLOP/s/GPU\\\\):\\\\s+(?P<TFLOPS>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s+tokens\\\\s+per\\\\s+GPU\\\\s+\\\\(tokens/s/GPU\\\\):\\\\s+(?P<TokensPerGPU>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s*learning\\\\s+rate:\\\\s+(?P<LearningRate>[+-]?\\\\d+(?:\\\\.\\\\d+)?(?:[Ee][+-]?\\\\d+)?)\\\\s*\\\\|\\\\s+global\\\\s+batch\\\\s+size:\\\\s+(?P<GlobalBatchSize>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+lm\\\\s+loss:\\\\s+(?P<LmLoss>[+-]?\\\\d+(?:\\\\.\\\\d+)?(?:[Ee][+-]?\\\\d+)?)\\\\s+\\\\|\\\\s+loss\\\\s+scale:\\\\s+(?P<LossScale>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+grad\\\\s+norm:\\\\s+(?P<GradNorm>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+number\\\\s+of\\\\s+skipped\\\\s+iterations:\\\\s+(?P<SkippedIterationsNumber>\\\\d+)\\\\s+\\\\|\\\\s+number\\\\s+of\\\\s+nan\\\\s+iterations:\\\\s+(?P<NanIterationsNumber>\\\\d+)\\\\s*\\\\|.*",
      "description": "Primus performance log with HIP memory metrics (v2 - without num zeros field)",
      "enabled": true,
      "tags": ["performance", "hip", "memory"],
      "confidence": 1.0
    }`

	// 修复后的配置（只使用两个反斜杠，JSON解析后会变成一个）
	jsonConfigFixed := `{
      "name": "primus-hip-memory-v2-FIXED",
      "pattern": ".*iteration\\s+(?P<CurrentIteration>\\d+)\\s*/\\s*(?P<TargetIteration>\\d+)\\s*\\|\\s*consumed samples:\\s+(?P<ConsumedSamples>\\d+)\\s*\\|\\s*elapsed\\stime\\sper\\siteration\\s\\(ms\\):\\s+(?P<ElapsedTimePerIterationMS>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s+hip\\s+mem\\s+usage/free/total/usage_ratio:\\s+(?P<MemUsage>\\d+\\.\\d+)GB/(?P<MemFree>\\d+\\.\\d+)GB/(?P<MemTotal>\\d+\\.\\d+)GB/(?P<MemUsageRatio>\\d+\\.\\d+)%\\s+\\|\\s+throughput\\s+per\\s+GPU\\s+\\(TFLOP/s/GPU\\):\\s+(?P<TFLOPS>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s+tokens\\s+per\\s+GPU\\s+\\(tokens/s/GPU\\):\\s+(?P<TokensPerGPU>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s*learning\\s+rate:\\s+(?P<LearningRate>[+-]?\\d+(?:\\.\\d+)?(?:[Ee][+-]?\\d+)?)\\s*\\|\\s+global\\s+batch\\s+size:\\s+(?P<GlobalBatchSize>\\d+(?:\\.\\d+)*)\\s+\\|\\s+lm\\s+loss:\\s+(?P<LmLoss>[+-]?\\d+(?:\\.\\d+)?(?:[Ee][+-]?\\d+)?)\\s+\\|\\s+loss\\s+scale:\\s+(?P<LossScale>\\d+(?:\\.\\d+)*)\\s+\\|\\s+grad\\s+norm:\\s+(?P<GradNorm>\\d+(?:\\.\\d+)*)\\s+\\|\\s+number\\s+of\\s+skipped\\s+iterations:\\s+(?P<SkippedIterationsNumber>\\d+)\\s+\\|\\s+number\\s+of\\s+nan\\s+iterations:\\s+(?P<NanIterationsNumber>\\d+)\\s*\\|.*",
      "description": "Primus performance log with HIP memory metrics (v2 - FIXED with correct escaping)",
      "enabled": true,
      "tags": ["performance", "hip", "memory"],
      "confidence": 1.0
    }`

	// 2. 原始日志（带 ANSI 代码）
	rawLog := `[[32m20251202 09:12:08[0m][[36mrank-7/8[0m][[1mINFO [0m] [1m[--------trainer.py:2560] : iteration 126/ 5000 | consumed samples: 16128 | elapsed time per iteration (ms): 13372.8/13364.7 | hip mem usage/free/total/usage_ratio: 153.81GB/102.17GB/255.98GB/60.09% | throughput per GPU (TFLOP/s/GPU): 567.6/568.0 | tokens per GPU (tokens/s/GPU): 9801.4/9807.3 | learning rate: 9.984820E-06 | global batch size: 128 | lm loss: 6.548988E-03 | loss scale: 1.0 | grad norm: 0.061 | number of skipped iterations: 0 | number of nan iterations: 0 |[0m`

	// 3. 清理 ANSI 代码
	cleanedLog := stripAnsiCodes(rawLog)
	t.Logf("\n===== Step 1: Clean ANSI Codes =====")
	t.Logf("Raw log length: %d", len(rawLog))
	t.Logf("Cleaned log length: %d", len(cleanedLog))
	t.Logf("Cleaned log: %s", cleanedLog)

	// 测试原始配置（有问题的）
	t.Run("Original Config (with bug)", func(t *testing.T) {
		testPatternConfig(t, jsonConfig, cleanedLog, false)
	})

	// 测试修复后的配置
	t.Run("Fixed Config", func(t *testing.T) {
		testPatternConfig(t, jsonConfigFixed, cleanedLog, true)
	})
}

// testPatternConfig 测试单个配置
func testPatternConfig(t *testing.T, jsonConfig, cleanedLog string, expectSuccess bool) {
	// 1. 解析 JSON 配置
	t.Logf("\n===== Step 2: Parse JSON Config =====")
	var config PatternConfig
	if err := json.Unmarshal([]byte(jsonConfig), &config); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}
	t.Logf("Pattern name: %s", config.Name)
	t.Logf("Pattern (first 100 chars): %s...", config.Pattern[:min(100, len(config.Pattern))])
	t.Logf("Pattern (last 100 chars): ...%s", config.Pattern[max(0, len(config.Pattern)-100):])

	// 2. 编译正则表达式
	t.Logf("\n===== Step 3: Compile Regex =====")
	re, err := regexp.Compile(config.Pattern)
	if err != nil {
		t.Fatalf("Failed to compile regex: %v", err)
	}
	t.Logf("✓ Regex compiled successfully")
	t.Logf("Regex pattern (first 100 chars): %s...", re.String()[:min(100, len(re.String()))])

	// 3. 测试匹配
	t.Logf("\n===== Step 4: Test Match =====")
	if !re.MatchString(cleanedLog) {
		t.Logf("❌ Pattern does NOT match the log")
		if expectSuccess {
			t.Errorf("Expected pattern to match but it didn't")
		}
		return
	}
	t.Logf("✓ Pattern matches the log")

	// 4. 提取所有匹配
	t.Logf("\n===== Step 5: Extract Groups =====")
	matches := re.FindStringSubmatch(cleanedLog)
	if matches == nil {
		t.Fatal("FindStringSubmatch returned nil (should not happen if MatchString was true)")
	}
	t.Logf("Total matches: %d", len(matches))
	t.Logf("Match[0] (full match, first 150 chars): %s...", matches[0][:min(150, len(matches[0]))])

	// 5. 提取命名组
	groups := make(map[string]string)
	names := re.SubexpNames()
	t.Logf("Subexp names count: %d", len(names))

	for i, name := range names {
		if i > 0 && i < len(matches) && name != "" {
			groups[name] = matches[i]
			t.Logf("  Group[%d] %s = %q", i, name, matches[i])
		}
	}

	t.Logf("\n===== Step 6: Validate Groups =====")
	t.Logf("Extracted %d groups", len(groups))

	// 检查是否有空值
	emptyCount := 0
	for name, value := range groups {
		if value == "" {
			emptyCount++
			t.Logf("  ⚠️  %s is EMPTY", name)
		}
	}

	if emptyCount > 0 {
		t.Logf("❌ Found %d empty groups out of %d total groups", emptyCount, len(groups))
		if expectSuccess {
			t.Errorf("Expected all groups to have values but found %d empty groups", emptyCount)
		}
	} else {
		t.Logf("✓ All %d groups have non-empty values", len(groups))
	}

	// 验证关键字段
	expectedGroups := map[string]string{
		"CurrentIteration":          "126",
		"TargetIteration":           "5000",
		"ConsumedSamples":           "16128",
		"ElapsedTimePerIterationMS": "13372.8",
		"MemUsage":                  "153.81",
		"LearningRate":              "9.984820E-06",
		"LmLoss":                    "6.548988E-03",
	}

	t.Logf("\n===== Step 7: Validate Key Fields =====")
	for name, expected := range expectedGroups {
		got, ok := groups[name]
		if !ok {
			t.Logf("  ❌ %s: NOT FOUND", name)
		} else if got != expected {
			t.Logf("  ❌ %s: got %q, want %q", name, got, expected)
		} else {
			t.Logf("  ✓ %s = %s", name, got)
		}
	}

	// 6. 转换为 Performance
	if len(groups) > 0 && emptyCount == 0 {
		t.Logf("\n===== Step 8: Convert to Performance =====")
		perf, err := ConvertGroupsToPerformance(groups)
		if err != nil {
			t.Errorf("ConvertGroupsToPerformance failed: %v", err)
		} else {
			t.Logf("✓ Conversion successful")
			if perf.CurrentIteration != nil {
				t.Logf("  CurrentIteration: %d (expected 126)", *perf.CurrentIteration)
			}
			if perf.TargetIteration != nil {
				t.Logf("  TargetIteration: %d (expected 5000)", *perf.TargetIteration)
			}
			if perf.MemUsages != nil {
				t.Logf("  MemUsages: %.2f (expected 153.81)", *perf.MemUsages)
			}
			if perf.LearningRate != nil {
				t.Logf("  LearningRate: %.10f", *perf.LearningRate)
			}
			if perf.LmLoss != nil {
				t.Logf("  LmLoss: %.10f", *perf.LmLoss)
			}

			if perf.CurrentIteration == nil || *perf.CurrentIteration != 126 {
				t.Errorf("CurrentIteration: got %v, want 126", perf.CurrentIteration)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
