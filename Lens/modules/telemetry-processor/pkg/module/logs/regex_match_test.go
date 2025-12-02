package logs

import (
	"regexp"
	"testing"
)

// TestRegexMatchRealLog 测试正则表达式是否能匹配实际日志
func TestRegexMatchRealLog(t *testing.T) {
	// 实际的日志（带 ANSI 颜色代码）
	rawLog := `[[32m20251202 09:12:08[0m][[36mrank-7/8[0m][[1mINFO [0m] [1m[--------trainer.py:2560] : iteration 126/ 5000 | consumed samples: 16128 | elapsed time per iteration (ms): 13372.8/13364.7 | hip mem usage/free/total/usage_ratio: 153.81GB/102.17GB/255.98GB/60.09% | throughput per GPU (TFLOP/s/GPU): 567.6/568.0 | tokens per GPU (tokens/s/GPU): 9801.4/9807.3 | learning rate: 9.984820E-06 | global batch size: 128 | lm loss: 6.548988E-03 | loss scale: 1.0 | grad norm: 0.061 | number of skipped iterations: 0 | number of nan iterations: 0 |[0m`

	// 清理 ANSI 代码
	cleanLog := stripAnsiCodes(rawLog)
	t.Logf("Raw log length: %d", len(rawLog))
	t.Logf("Clean log length: %d", len(cleanLog))
	t.Logf("Clean log:\n%s", cleanLog)

	// 你的正则表达式
	pattern := `\.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|` +
		`\s*consumed samples:\s+(?P<ConsumedSamples>\d+)\s*\|` +
		`\s*elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
		`\s+hip\s+mem\s+usage/free/total/usage_ratio:\s+` +
		`(?P<MemUsage>\d+\.\d+)GB/` +
		`(?P<MemFree>\d+\.\d+)GB/` +
		`(?P<MemTotal>\d+\.\d+)GB/` +
		`(?P<MemUsageRatio>\d+\.\d+)%\s+\|` +
		`\s+throughput\s+per\s+GPU\s+\(TFLOP/s/GPU\):\s+(?P<TFLOPS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
		`\s+tokens\s+per\s+GPU\s+\(tokens/s/GPU\):\s+(?P<TokensPerGPU>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
		`\s*learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s*\|` +
		`\s+global\s+batch\s+size:\s+(?P<GlobalBatchSize>\d+(?:\.\d+)*)\s+\|` +
		`\s+lm\s+loss:\s+(?P<LmLoss>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|` +
		`\s+loss\s+scale:\s+(?P<LossScale>\d+(?:\.\d+)*)\s+\|` +
		`\s+grad\s+norm:\s+(?P<GradNorm>\d+(?:\.\d+)*)\s+\|` +
		`\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|` +
		`\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\s*\|.*`

	// 编译正则表达式
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("Failed to compile regex: %v", err)
	}

	// 测试原始日志（不应该匹配）
	if re.MatchString(rawLog) {
		t.Log("✓ Raw log matched (unexpected but OK)")
	} else {
		t.Log("✗ Raw log did NOT match (expected, needs stripAnsiCodes)")
	}

	// 测试清理后的日志（应该匹配）
	if !re.MatchString(cleanLog) {
		t.Errorf("❌ Clean log did NOT match! This is the problem!")
		t.Logf("Pattern: %s", pattern)
		t.Logf("Log: %s", cleanLog)
		
		// 尝试找出哪里不匹配
		testSimplePattern := `iteration\s+\d+\s*/\s*\d+`
		simpleRe := regexp.MustCompile(testSimplePattern)
		if simpleRe.MatchString(cleanLog) {
			t.Logf("✓ Simple pattern 'iteration X / Y' matches")
		} else {
			t.Logf("✗ Even simple pattern doesn't match")
		}
		
		return
	}

	t.Log("✓ Clean log matched!")

	// 提取所有 groups
	matches := re.FindStringSubmatch(cleanLog)
	if matches == nil {
		t.Fatal("No matches found")
	}

	groups := make(map[string]string)
	names := re.SubexpNames()
	for i, name := range names {
		if i > 0 && i < len(matches) && name != "" {
			groups[name] = matches[i]
		}
	}

	// 打印所有捕获的组
	t.Logf("\n=== Captured Groups ===")
	t.Logf("Total groups: %d", len(groups))
	for name, value := range groups {
		t.Logf("  %s = %s", name, value)
	}

	// 验证关键字段
	expectedGroups := map[string]string{
		"CurrentIteration":         "126",
		"TargetIteration":          "5000",
		"ConsumedSamples":          "16128",
		"ElapsedTimePerIterationMS": "13372.8",
		"MemUsage":                 "153.81",
		"MemFree":                  "102.17",
		"MemTotal":                 "255.98",
		"MemUsageRatio":            "60.09",
		"TFLOPS":                   "567.6",
		"TokensPerGPU":             "9801.4",
		"LearningRate":             "9.984820E-06",
		"GlobalBatchSize":          "128",
		"LmLoss":                   "6.548988E-03",
		"LossScale":                "1.0",
		"GradNorm":                 "0.061",
		"SkippedIterationsNumber":  "0",
		"NanIterationsNumber":      "0",
	}

	for name, expected := range expectedGroups {
		if got, ok := groups[name]; !ok {
			t.Errorf("❌ Group %s not captured", name)
		} else if got != expected {
			t.Errorf("❌ Group %s: got %s, want %s", name, got, expected)
		} else {
			t.Logf("✓ Group %s = %s", name, got)
		}
	}

	// 测试转换
	t.Log("\n=== Testing Conversion ===")
	perf, err := ConvertGroupsToPerformance(groups)
	if err != nil {
		t.Fatalf("ConvertGroupsToPerformance failed: %v", err)
	}
	t.Logf("CurrentIteration: %d (expected 126)", perf.CurrentIteration)
	t.Logf("MemUsages: %.2f (expected 153.81)", perf.MemUsages)
	t.Logf("LearningRate: %.10f (expected 0.000009984820)", perf.LearningRate)
	t.Logf("LmLoss: %.10f (expected 0.006548988)", perf.LmLoss)

	if perf.CurrentIteration != 126 {
		t.Errorf("CurrentIteration conversion failed: got %d, want 126", perf.CurrentIteration)
	}
}

