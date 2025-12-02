package logs

import (
	"reflect"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func TestGroupsToPerformance(t *testing.T) {
	// 模拟实际的正则捕获组数据
	groups := map[string]string{
		"CurrentIteration":          "4273",
		"TargetIteration":           "5000",
		"ConsumedSamples":           "546944",
		"ElapsedTimePerIterationMS": "13107.0",
		"MemUsage":                  "153.81",
		"MemFree":                   "102.17",
		"MemTotal":                  "255.98",
		"MemUsageRatio":             "60.09",
		"TFLOPS":                    "579.1",
		"TokensPerGPU":              "10000.2",
		"LearningRate":              "5.130331E-07",
		"GlobalBatchSize":           "128",
		"LmLoss":                    "4.092303E-03",
		"LossScale":                 "1.0",
		"GradNorm":                  "0.003",
		"SkippedIterationsNumber":   "0",
		"NanIterationsNumber":       "0",
	}

	// 调用转换方法
	perf, err := ConvertGroupsToPerformance(groups)
	if err != nil {
		t.Fatalf("ConvertGroupsToPerformance failed: %v", err)
	}

	// 验证转换结果
	tests := []struct {
		name      string
		got       interface{}
		want      interface{}
		fieldName string
	}{
		{"CurrentIteration", perf.CurrentIteration, 4273, "CurrentIteration"},
		{"TargetIteration", perf.TargetIteration, 5000, "TargetIteration"},
		{"ConsumedSamples", perf.ConsumedSamples, int64(546944), "ConsumedSamples"},
		{"ElapsedTimePerIterationMS", perf.ElapsedTimePerIterationMS, 13107.0, "ElapsedTimePerIterationMS"},
		{"MemUsages", perf.MemUsages, 153.81, "MemUsages (from MemUsage)"},
		{"MemFree", perf.MemFree, 102.17, "MemFree"},
		{"MemTotal", perf.MemTotal, 255.98, "MemTotal"},
		{"MemUsageRatio", perf.MemUsageRatio, 60.09, "MemUsageRatio"},
		{"TFLOPS", perf.TFLOPS, 579.1, "TFLOPS"},
		{"TokensPerGPU", perf.TokensPerGPU, 10000.2, "TokensPerGPU"},
		{"LearningRate", perf.LearningRate, 5.130331e-07, "LearningRate"},
		{"GlobalBatchSize", perf.GlobalBatchSize, 128, "GlobalBatchSize"},
		{"LmLoss", perf.LmLoss, 4.092303e-03, "LmLoss"},
		{"LossScale", perf.LossScale, 1.0, "LossScale"},
		{"GradNorm", perf.GradNorm, 0.003, "GradNorm"},
		{"SkippedIterationsNumber", perf.SkippedIterationsNumber, 0, "SkippedIterationsNumber"},
		{"NanIterationsNumber", perf.NanIterationsNumber, 0, "NanIterationsNumber"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch expected := tt.want.(type) {
			case int:
				if got, ok := tt.got.(int); !ok || got != expected {
					t.Errorf("Field %s: got %v (type %T), want %v (type %T)",
						tt.fieldName, tt.got, tt.got, tt.want, tt.want)
				}
			case int64:
				if got, ok := tt.got.(int64); !ok || got != expected {
					t.Errorf("Field %s: got %v (type %T), want %v (type %T)",
						tt.fieldName, tt.got, tt.got, tt.want, tt.want)
				}
			case float64:
				if got, ok := tt.got.(float64); !ok || got != expected {
					t.Errorf("Field %s: got %v (type %T), want %v (type %T)",
						tt.fieldName, tt.got, tt.got, tt.want, tt.want)
				}
			default:
				t.Errorf("Unsupported type in test: %T", tt.want)
			}
		})
	}

	// 打印所有字段值用于调试
	t.Logf("\n=== Converted Performance Data ===")
	t.Logf("CurrentIteration: %d", perf.CurrentIteration)
	t.Logf("TargetIteration: %d", perf.TargetIteration)
	t.Logf("ConsumedSamples: %d", perf.ConsumedSamples)
	t.Logf("ElapsedTimePerIterationMS: %.2f", perf.ElapsedTimePerIterationMS)
	t.Logf("MemUsages: %.2f", perf.MemUsages)
	t.Logf("MemFree: %.2f", perf.MemFree)
	t.Logf("MemTotal: %.2f", perf.MemTotal)
	t.Logf("MemUsageRatio: %.2f", perf.MemUsageRatio)
	t.Logf("TFLOPS: %.2f", perf.TFLOPS)
	t.Logf("TokensPerGPU: %.2f", perf.TokensPerGPU)
	t.Logf("LearningRate: %.10f", perf.LearningRate)
	t.Logf("GlobalBatchSize: %d", perf.GlobalBatchSize)
	t.Logf("LmLoss: %.10f", perf.LmLoss)
	t.Logf("LossScale: %.2f", perf.LossScale)
	t.Logf("GradNorm: %.6f", perf.GradNorm)
	t.Logf("SkippedIterationsNumber: %d", perf.SkippedIterationsNumber)
	t.Logf("NanIterationsNumber: %d", perf.NanIterationsNumber)
}

// 测试 tryAlternativeNames 方法
func TestTryAlternativeNames(t *testing.T) {
	groups := map[string]string{
		"MemUsage": "153.81",
	}

	// 测试 MemUsages 字段应该能从 MemUsage 获取值
	result := tryAlternativeNames(groups, "MemUsages")
	if result != "153.81" {
		t.Errorf("tryAlternativeNames(groups, 'MemUsages') = %v, want '153.81'", result)
	}

	// 测试不存在的字段
	result = tryAlternativeNames(groups, "NonExistentField")
	if result != "" {
		t.Errorf("tryAlternativeNames(groups, 'NonExistentField') = %v, want ''", result)
	}
}

// 测试 setFieldValue 科学计数法解析
func TestSetFieldValueScientificNotation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  float64
		expectErr bool
	}{
		{"科学计数法小写e", "5.130331e-07", 5.130331e-07, false},
		{"科学计数法大写E", "5.130331E-07", 5.130331e-07, false},
		{"科学计数法正数", "4.092303E-03", 4.092303e-03, false},
		{"普通浮点数", "123.45", 123.45, false},
		{"整数", "12345", 12345.0, false},
		{"零", "0", 0.0, false},
		{"负数", "-123.45", -123.45, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perf := &model.TrainingPerformance{}

			// 使用 reflection 获取 LearningRate 字段
			perfValue := reflect.ValueOf(perf).Elem()
			field := perfValue.FieldByName("LearningRate")

			if !field.IsValid() {
				t.Fatal("Field LearningRate not found")
			}

			err := setFieldValue(field, tt.input)
			if (err != nil) != tt.expectErr {
				t.Errorf("setFieldValue() error = %v, expectErr %v", err, tt.expectErr)
			}

			if err == nil {
				got := field.Float()
				if got != tt.expected {
					t.Errorf("setFieldValue() got = %v, want %v (input: %s)", got, tt.expected, tt.input)
				} else {
					t.Logf("✓ Successfully parsed %s = %v (input: %s)", tt.name, got, tt.input)
				}
			}
		})
	}
}
