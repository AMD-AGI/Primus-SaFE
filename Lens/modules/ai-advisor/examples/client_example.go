package main

import (
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/client"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
)

func main() {
	// 创建客户端
	aiAdvisor := client.NewClientWithDefaults("http://localhost:8080").
		SetTimeout(30 * time.Second).
		SetDebug(true)

	workloadUID := "example-workload-123"

	// 1. 健康检查
	fmt.Println("=== Health Check ===")
	healthy, err := aiAdvisor.HealthCheck()
	if err != nil {
		fmt.Printf("Health check failed: %v\n", err)
	} else {
		fmt.Printf("AI Advisor is healthy: %v\n", healthy)
	}

	// 2. 上报框架检测
	fmt.Println("\n=== Report Detection ===")
	detection, err := aiAdvisor.ReportDetection(&common.DetectionRequest{
		WorkloadUID: workloadUID,
		Source:      "wandb",
		Frameworks:  []string{"primus"},
		Type:        "training",
		Confidence:  0.95,
		Evidence: map[string]interface{}{
			"method":  "import_detection",
			"version": "1.0.0",
			"modules": []string{"primus", "megatron"},
		},
	})
	if err != nil {
		fmt.Printf("Failed to report detection: %v\n", err)
	} else {
		fmt.Printf("Detection reported: %v (confidence: %.2f, status: %s)\n",
			detection.Frameworks, detection.Confidence, detection.Status)
	}

	// 3. 查询框架检测结果
	fmt.Println("\n=== Get Detection ===")
	result, err := aiAdvisor.GetDetection(workloadUID)
	if err != nil {
		fmt.Printf("Failed to get detection: %v\n", err)
	} else if result == nil {
		fmt.Println("Detection not found")
	} else {
		fmt.Printf("Framework: %v\n", result.Frameworks)
		fmt.Printf("Confidence: %.2f\n", result.Confidence)
		fmt.Printf("Status: %s\n", result.Status)
		fmt.Printf("Sources: %d\n", len(result.Sources))
	}

	// 4. 批量查询
	fmt.Println("\n=== Batch Get Detection ===")
	workloadUIDs := []string{workloadUID, "workload-2", "workload-3"}
	results, err := aiAdvisor.BatchGetDetection(workloadUIDs)
	if err != nil {
		fmt.Printf("Failed to batch get detections: %v\n", err)
	} else {
		for uid, det := range results {
			if det == nil {
				fmt.Printf("%s: not found\n", uid)
			} else {
				fmt.Printf("%s: %v (%.2f)\n", uid, det.Frameworks, det.Confidence)
			}
		}
	}

	// 5. 性能分析
	fmt.Println("\n=== Analyze Performance ===")
	analysis, err := aiAdvisor.AnalyzePerformance(workloadUID)
	if err != nil {
		fmt.Printf("Failed to analyze performance: %v\n", err)
	} else {
		fmt.Printf("Overall Score: %.2f\n", analysis.OverallScore)
		if analysis.GPUUtilization != nil {
			fmt.Printf("GPU Utilization: %.2f%%\n", analysis.GPUUtilization.AvgUtilization*100)
		}
	}

	// 6. 异常检测
	fmt.Println("\n=== Detect Anomalies ===")
	anomalies, err := aiAdvisor.DetectAnomalies(workloadUID)
	if err != nil {
		fmt.Printf("Failed to detect anomalies: %v\n", err)
	} else {
		fmt.Printf("Detected %d anomalies\n", len(anomalies))
		for _, anomaly := range anomalies {
			fmt.Printf("  - [%s] %s: %s\n",
				anomaly.Severity, anomaly.Type, anomaly.Description)
		}
	}

	// 7. 获取建议
	fmt.Println("\n=== Get Recommendations ===")
	recommendations, err := aiAdvisor.GetRecommendations(workloadUID)
	if err != nil {
		fmt.Printf("Failed to get recommendations: %v\n", err)
	} else {
		fmt.Printf("Found %d recommendations\n", len(recommendations))
		for _, rec := range recommendations {
			fmt.Printf("  - [%s] %s\n", rec.Priority, rec.Title)
		}
	}

	// 8. 故障诊断
	fmt.Println("\n=== Analyze Workload (Diagnostics) ===")
	diagnostic, err := aiAdvisor.AnalyzeWorkload(workloadUID)
	if err != nil {
		fmt.Printf("Failed to analyze workload: %v\n", err)
	} else {
		fmt.Printf("Status: %s\n", diagnostic.Status)
		fmt.Printf("Summary: %s\n", diagnostic.Summary)
		fmt.Printf("Root Causes: %d\n", len(diagnostic.RootCauses))
	}

	// 9. 模型洞察
	fmt.Println("\n=== Analyze Model ===")
	insight, err := aiAdvisor.AnalyzeModel(workloadUID, map[string]interface{}{
		"architecture":        "transformer",
		"num_layers":          96,
		"hidden_size":         12288,
		"num_attention_heads": 96,
	})
	if err != nil {
		fmt.Printf("Failed to analyze model: %v\n", err)
	} else {
		fmt.Printf("Model Name: %s\n", insight.ModelName)
		fmt.Printf("Total Parameters: %d\n", insight.TotalParameters)
		if insight.MemoryEstimate != nil {
			fmt.Printf("Recommended GPUs: %d\n", insight.MemoryEstimate.RecommendedGPUs)
		}
	}

	// 10. 统计信息
	fmt.Println("\n=== Get Statistics ===")
	stats, err := aiAdvisor.GetDetectionStats("", "", "") // No filters
	if err != nil {
		fmt.Printf("Failed to get stats: %v\n", err)
	} else {
		fmt.Printf("Total Workloads: %d\n", stats.TotalWorkloads)
		fmt.Printf("Average Confidence: %.2f\n", stats.AverageConfidence)
		fmt.Println("Framework Distribution:")
		for framework, count := range stats.ByFramework {
			fmt.Printf("  - %s: %d\n", framework, count)
		}
	}

	fmt.Println("\n=== Example Completed ===")
}
