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
	// File path relative to jobs directory
	baseDir := filepath.Join("..", "..")
	
	// Read report_data.json
	inputPath := filepath.Join(baseDir, "report_data.json")
	fmt.Printf("📖 Reading %s...\n", inputPath)
	jsonData, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Printf("❌ Failed to read file: %v\n", err)
		os.Exit(1)
	}

	// Parse JSON into ReportData structure
	var reportData gpu_usage_weekly_report.ReportData
	err = json.Unmarshal(jsonData, &reportData)
	if err != nil {
		fmt.Printf("❌ Failed to parse JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ JSON parsed successfully")

	// Display summary info
	fmt.Printf("📊 Cluster: %s\n", reportData.ClusterName)
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
	fmt.Println("\n🎨 Initializing renderer...")
	renderer := gpu_usage_weekly_report.NewReportRenderer(cfg)

	// Render HTML
	fmt.Println("🖼️  Rendering HTML...")
	ctx := context.Background()
	htmlContent, err := renderer.RenderHTML(ctx, &reportData)
	if err != nil {
		fmt.Printf("❌ HTML rendering failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ HTML rendered successfully")

	// Save HTML to file
	htmlOutputPath := filepath.Join(baseDir, "report_output.html")
	err = os.WriteFile(htmlOutputPath, htmlContent, 0644)
	if err != nil {
		fmt.Printf("❌ Failed to save HTML file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ HTML saved to: %s\n", htmlOutputPath)

	fmt.Println("\n✨ Rendering test complete!")
	fmt.Println("\n💡 Tip: Open report_output.html in a browser to view the rendered result")
	if reportData.Summary != nil {
		fmt.Printf("    Total GPUs should display as: %d\n", reportData.Summary.TotalGPUs)
	}
}

