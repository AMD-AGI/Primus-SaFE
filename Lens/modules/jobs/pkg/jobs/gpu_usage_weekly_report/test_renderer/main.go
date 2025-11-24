package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_usage_weekly_report"
)

// ConductorAPIResponse matches the structure of report-example.json
type ConductorAPIResponse struct {
	Status    string                 `json:"status"`
	Report    string                 `json:"report"`
	ChartData map[string]interface{} `json:"chart_data"`
	Metadata  map[string]interface{} `json:"metadata"`
	Error     interface{}            `json:"error"`
	Timestamp string                 `json:"timestamp"`
}

func main() {
	// Read report-example.json
	fmt.Println("📖 读取 report-example.json...")
	jsonData, err := os.ReadFile("report-example.json")
	if err != nil {
		fmt.Printf("❌ 读取文件失败: %v\n", err)
		os.Exit(1)
	}

	// Parse JSON
	var apiResp ConductorAPIResponse
	err = json.Unmarshal(jsonData, &apiResp)
	if err != nil {
		fmt.Printf("❌ 解析 JSON 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ JSON 解析成功")

	// Extract parameters from metadata
	params := apiResp.Metadata["parameters"].(map[string]interface{})
	cluster := params["cluster"].(string)
	startTimeStr := params["start_time"].(string)
	endTimeStr := params["end_time"].(string)

	startTime, _ := time.Parse(time.RFC3339, startTimeStr)
	endTime, _ := time.Parse(time.RFC3339, endTimeStr)

	fmt.Printf("📊 集群: %s\n", cluster)
	fmt.Printf("📅 时间范围: %s 到 %s\n", startTime.Format("2006-01-02"), endTime.Format("2006-01-02"))

	// Extract summary data from markdown report
	// Note: In production, summary data (including total_gpu_count) comes from API response
	summary := &gpu_usage_weekly_report.ReportSummary{
		TotalGPUs:      1004, // Should be populated from API response's summary.total_gpu_count
		AvgUtilization: 65.85,
		AvgAllocation:  65.81,
		TotalGpuHours:  0,
		LowUtilCount:   13,
		WastedGpuDays:  400,
	}

	// Parse chart data
	var chartData *gpu_usage_weekly_report.ChartData
	if apiResp.ChartData != nil {
		chartDataJSON, _ := json.Marshal(apiResp.ChartData)
		chartData = &gpu_usage_weekly_report.ChartData{}
		err = json.Unmarshal(chartDataJSON, chartData)
		if err != nil {
			fmt.Printf("⚠️  解析 chart_data 失败: %v\n", err)
			chartData = &gpu_usage_weekly_report.ChartData{}
		} else {
			fmt.Println("✅ Chart data 解析成功")
			if chartData.ClusterUsageTrend != nil {
				fmt.Printf("   - cluster_usage_trend: %d data points, %d series\n",
					len(chartData.ClusterUsageTrend.XAxis),
					len(chartData.ClusterUsageTrend.Series))
			}
		}
	} else {
		chartData = &gpu_usage_weekly_report.ChartData{}
	}

	// Create ReportData structure
	reportData := &gpu_usage_weekly_report.ReportData{
		ClusterName:    cluster,
		MarkdownReport: apiResp.Report,
		Period: gpu_usage_weekly_report.ReportPeriod{
			StartTime: startTime,
			EndTime:   endTime,
		},
		ChartData: chartData,
		Summary:   summary,
		Metadata:  apiResp.Metadata,
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
	htmlContent, err := renderer.RenderHTML(ctx, reportData)
	if err != nil {
		fmt.Printf("❌ HTML 渲染失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ HTML 渲染成功")

	// Save HTML to file
	htmlOutputPath := "report_output.html"
	err = os.WriteFile(htmlOutputPath, htmlContent, 0644)
	if err != nil {
		fmt.Printf("❌ 保存 HTML 文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ HTML 已保存到: %s\n", htmlOutputPath)

	// Render PDF (if supported)
	fmt.Println("\n📄 渲染 PDF...")
	pdfContent, err := renderer.RenderPDF(ctx, htmlContent)
	if err != nil {
		fmt.Printf("⚠️  PDF 渲染失败: %v\n", err)
	} else if len(pdfContent) > 0 {
		pdfOutputPath := "report_output.pdf"
		err = os.WriteFile(pdfOutputPath, pdfContent, 0644)
		if err != nil {
			fmt.Printf("❌ 保存 PDF 文件失败: %v\n", err)
		} else {
			fmt.Printf("✅ PDF 已保存到: %s\n", pdfOutputPath)
		}
	} else {
		fmt.Println("ℹ️  PDF 渲染未实现（这是预期的）")
	}

	// Save full report data as JSON for inspection
	jsonOutputPath := "report_data.json"
	reportDataJSON, _ := json.MarshalIndent(reportData, "", "  ")
	err = os.WriteFile(jsonOutputPath, reportDataJSON, 0644)
	if err != nil {
		fmt.Printf("⚠️  保存 report_data.json 失败: %v\n", err)
	} else {
		fmt.Printf("✅ 报告数据已保存到: %s\n", jsonOutputPath)
	}

	fmt.Println("\n✨ 渲染测试完成！")
	fmt.Println("\n📁 生成的文件:")
	fmt.Printf("   - %s (HTML 报告)\n", htmlOutputPath)
	fmt.Printf("   - %s (报告数据)\n", jsonOutputPath)
	fmt.Println("\n💡 提示: 在浏览器中打开 report_output.html 查看渲染结果")
}
