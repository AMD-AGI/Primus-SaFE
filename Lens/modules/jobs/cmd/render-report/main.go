package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_usage_weekly_report"
)

func main() {
	// 文件路径相对于 jobs 目录
	baseDir := filepath.Join("..", "..")
	
	// Read report_data.json
	inputPath := filepath.Join(baseDir, "report_data.json")
	fmt.Printf("📖 读取 %s...\n", inputPath)
	jsonData, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Printf("❌ 读取文件失败: %v\n", err)
		os.Exit(1)
	}

	// Parse JSON into ReportData structure
	var reportData gpu_usage_weekly_report.ReportData
	err = json.Unmarshal(jsonData, &reportData)
	if err != nil {
		fmt.Printf("❌ 解析 JSON 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ JSON 解析成功")

	// Display summary info
	fmt.Printf("📊 集群: %s\n", reportData.ClusterName)
	if reportData.Summary != nil {
		fmt.Printf("   - Total GPUs: %d\n", reportData.Summary.TotalGPUs)
		fmt.Printf("   - Avg Utilization: %.2f%%\n", reportData.Summary.AvgUtilization)
		fmt.Printf("   - Avg Allocation: %.2f%%\n", reportData.Summary.AvgAllocation)
		fmt.Printf("   - Low Util Users: %d\n", reportData.Summary.LowUtilCount)
	}
	if reportData.ChartData != nil && reportData.ChartData.ClusterUsageTrend != nil {
		fmt.Printf("   - Chart data points: %d\n", len(reportData.ChartData.ClusterUsageTrend.XAxis))
	}

	// Create renderer configuration
	cfg := &config.WeeklyReportConfig{
		Enabled:       true,
		OutputFormats: []string{"html", "pdf"},
		Brand: config.BrandConfig{
			PrimaryColor: "#ED1C24", // AMD Red
			CompanyName:  "AMD AGI",
		},
	}

	// Initialize renderer
	fmt.Println("\n🎨 初始化渲染器...")
	renderer := gpu_usage_weekly_report.NewReportRenderer(cfg)

	// Render HTML
	fmt.Println("🖼️  渲染 HTML...")
	ctx := context.Background()
	htmlContent, err := renderer.RenderHTML(ctx, &reportData)
	if err != nil {
		fmt.Printf("❌ HTML 渲染失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ HTML 渲染成功")

	// Save HTML to file
	htmlOutputPath := filepath.Join(baseDir, "report_output.html")
	err = os.WriteFile(htmlOutputPath, htmlContent, 0644)
	if err != nil {
		fmt.Printf("❌ 保存 HTML 文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ HTML 已保存到: %s\n", htmlOutputPath)

	fmt.Println("\n✨ 渲染测试完成！")
	fmt.Println("\n💡 提示: 在浏览器中打开 report_output.html 查看渲染结果")
	if reportData.Summary != nil {
		fmt.Printf("    Total GPUs 应该显示为: %d\n", reportData.Summary.TotalGPUs)
	}
}

