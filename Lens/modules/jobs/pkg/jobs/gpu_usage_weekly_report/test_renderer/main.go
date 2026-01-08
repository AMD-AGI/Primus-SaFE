// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

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
	fmt.Println("📖 Reading report-example.json...")
	jsonData, err := os.ReadFile("report-example.json")
	if err != nil {
		fmt.Printf("❌ Failed to read file: %v\n", err)
		os.Exit(1)
	}

	// Parse JSON
	var apiResp ConductorAPIResponse
	err = json.Unmarshal(jsonData, &apiResp)
	if err != nil {
		fmt.Printf("❌ Failed to parse JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ JSON parsed successfully")

	// Extract parameters from metadata
	params := apiResp.Metadata["parameters"].(map[string]interface{})
	cluster := params["cluster"].(string)
	startTimeStr := params["start_time"].(string)
	endTimeStr := params["end_time"].(string)

	startTime, _ := time.Parse(time.RFC3339, startTimeStr)
	endTime, _ := time.Parse(time.RFC3339, endTimeStr)

	fmt.Printf("📊 Cluster: %s\n", cluster)
	fmt.Printf("📅 Time range: %s to %s\n", startTime.Format("2006-01-02"), endTime.Format("2006-01-02"))

	// Extract summary data from markdown report
	// Note: In production, summary data (including total_gpu_count) comes from API response
	summary := &gpu_usage_weekly_report.ReportSummary{
		TotalGPUs:      1456, // Should be populated from API response's summary.total_gpu_count
		AvgUtilization: 40.06,
		AvgAllocation:  40.39,
		TotalGpuHours:  0,
		LowUtilCount:   20,
		WastedGpuDays:  275,
	}

	// Parse chart data
	var chartData *gpu_usage_weekly_report.ChartData
	if apiResp.ChartData != nil {
		chartDataJSON, _ := json.Marshal(apiResp.ChartData)
		chartData = &gpu_usage_weekly_report.ChartData{}
		err = json.Unmarshal(chartDataJSON, chartData)
		if err != nil {
			fmt.Printf("⚠️  Failed to parse chart_data: %v\n", err)
			chartData = &gpu_usage_weekly_report.ChartData{}
		} else {
			fmt.Println("✅ Chart data parsed successfully")
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
	fmt.Println("\n🎨 Initializing renderer...")
	renderer := gpu_usage_weekly_report.NewReportRenderer(cfg)

	// Render HTML
	fmt.Println("🖼️  Rendering HTML...")
	ctx := context.Background()
	htmlContent, err := renderer.RenderHTML(ctx, reportData)
	if err != nil {
		fmt.Printf("❌ HTML rendering failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ HTML rendered successfully")

	// Save HTML to file
	htmlOutputPath := "report_output.html"
	err = os.WriteFile(htmlOutputPath, htmlContent, 0644)
	if err != nil {
		fmt.Printf("❌ Failed to save HTML file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ HTML saved to: %s\n", htmlOutputPath)

	// Render PDF (if supported)
	fmt.Println("\n📄 Rendering PDF...")
	pdfContent, err := renderer.RenderPDF(ctx, htmlContent)
	if err != nil {
		fmt.Printf("⚠️  PDF rendering failed: %v\n", err)
	} else if len(pdfContent) > 0 {
		pdfOutputPath := "report_output.pdf"
		err = os.WriteFile(pdfOutputPath, pdfContent, 0644)
		if err != nil {
			fmt.Printf("❌ Failed to save PDF file: %v\n", err)
		} else {
			fmt.Printf("✅ PDF saved to: %s\n", pdfOutputPath)
		}
	} else {
		fmt.Println("ℹ️  PDF rendering not implemented (this is expected)")
	}

	// Save full report data as JSON for inspection
	jsonOutputPath := "report_data.json"
	reportDataJSON, _ := json.MarshalIndent(reportData, "", "  ")
	err = os.WriteFile(jsonOutputPath, reportDataJSON, 0644)
	if err != nil {
		fmt.Printf("⚠️  Failed to save report_data.json: %v\n", err)
	} else {
		fmt.Printf("✅ Report data saved to: %s\n", jsonOutputPath)
	}

	fmt.Println("\n✨ Rendering test complete!")
	fmt.Println("\n📁 Generated files:")
	fmt.Printf("   - %s (HTML report)\n", htmlOutputPath)
	fmt.Printf("   - %s (report data)\n", jsonOutputPath)
	fmt.Println("\n💡 Tip: Open report_output.html in a browser to view the rendered result")
}
