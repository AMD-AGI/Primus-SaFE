// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_usage_weekly_report"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var (
	dbName  = flag.String("dbName", "primus_lens", "The name of the database")
	dbUser  = flag.String("dbUser", "postgres", "The user of the database")
	dbPass  = flag.String("dbPass", "", "The password of the database")
	dbHost  = flag.String("dbHost", "localhost", "The host of the database")
	dbPort  = flag.String("dbPort", "5432", "The port of the database")
	sslMode = flag.String("sslMode", "disable", "The ssl mode of the database")
)

func main() {
	flag.Parse()

	fmt.Println("📊 GPU Usage Weekly Report - Save to Database")
	fmt.Println("==========================================")
	fmt.Println()

	// File path relative to jobs directory
	baseDir := ""

	// 1. Read report_data.json
	inputPath := filepath.Join(baseDir, "report_data.json")
	fmt.Printf("📖 Reading %s...\n", inputPath)
	jsonData, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Printf("❌ Failed to read report_data.json: %v\n", err)
		os.Exit(1)
	}

	var reportData gpu_usage_weekly_report.ReportData
	err = json.Unmarshal(jsonData, &reportData)
	if err != nil {
		fmt.Printf("❌ Failed to parse JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ JSON parsed successfully - Cluster: %s\n", reportData.ClusterName)

	// 2. Read report_output.html
	htmlPath := filepath.Join(baseDir, "report_output.html")
	fmt.Printf("📄 Reading %s...\n", htmlPath)
	htmlContent, err := os.ReadFile(htmlPath)
	if err != nil {
		fmt.Printf("❌ Failed to read HTML file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ HTML read successfully - Size: %d bytes\n", len(htmlContent))

	// 3. Read report_output.pdf (optional)
	var pdfContent []byte
	pdfPath := filepath.Join(baseDir, "report_output.pdf")
	fmt.Printf("📄 Reading %s...\n", pdfPath)
	pdfContent, err = os.ReadFile(pdfPath)
	if err != nil {
		fmt.Printf("⚠️  PDF file not found or failed to read: %v\n", err)
		fmt.Println("   Will continue saving without PDF content")
		pdfContent = nil
	} else {
		fmt.Printf("✅ PDF read successfully - Size: %d bytes\n", len(pdfContent))
	}

	// 4. Initialize database connection
	fmt.Println("\n💾 Connecting to database...")
	fmt.Printf("   - Host: %s:%s\n", *dbHost, *dbPort)
	fmt.Printf("   - Database: %s\n", *dbName)
	fmt.Printf("   - User: %s\n", *dbUser)

	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=%s",
		*dbHost, *dbPort, *dbUser, *dbName, *dbPass, *sslMode)

	db, err := gorm.Open(postgres.Dialector{
		Config: &postgres.Config{
			DSN: dsn,
		},
	}, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	})

	if err != nil {
		fmt.Printf("❌ Database connection failed: %v\n", err)
		fmt.Println("\n💡 Tip: Please check if database parameters are correct")
		fmt.Println("   Usage: go run main.go -dbHost=localhost -dbPort=5432 -dbUser=postgres -dbPass=yourpass -dbName=primus_lens")
		os.Exit(1)
	}
	fmt.Println("✅ Database connected successfully")

	// 5. Prepare database record
	fmt.Println("\n📝 Preparing database record...")

	// Generate unique ID
	reportID := generateReportID(reportData.ClusterName)

	// Parse time range
	var periodStart, periodEnd time.Time
	if reportData.Metadata != nil {
		if params, ok := reportData.Metadata["parameters"].(map[string]interface{}); ok {
			if startTimeStr, ok := params["start_time"].(string); ok {
				periodStart, _ = time.Parse(time.RFC3339, startTimeStr)
			}
			if endTimeStr, ok := params["end_time"].(string); ok {
				periodEnd, _ = time.Parse(time.RFC3339, endTimeStr)
			}
		}
	}

	// If unable to get from metadata, use Period field
	if periodStart.IsZero() {
		periodStart = reportData.Period.StartTime
	}
	if periodEnd.IsZero() {
		periodEnd = reportData.Period.EndTime
	}

	// If still zero, use last 7 days to now
	if periodStart.IsZero() {
		periodEnd = time.Now()
		periodStart = periodEnd.AddDate(0, 0, -7)
	}

	// Prepare json_content
	jsonContent := reportData.ToExtType()

	// Prepare metadata
	metadata := reportData.GenerateMetadata()

	// Create database record
	record := &dbmodel.GpuUsageWeeklyReports{
		ID:           reportID,
		ClusterName:  reportData.ClusterName,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
		GeneratedAt:  time.Now(),
		Status:       "generated",
		HTMLContent:  htmlContent,
		PdfContent:   pdfContent,
		JSONContent:  jsonContent,
		Metadata:     metadata,
		ErrorMessage: "",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	fmt.Printf("   - Report ID: %s\n", reportID)
	fmt.Printf("   - Cluster name: %s\n", record.ClusterName)
	fmt.Printf("   - Period: %s to %s\n",
		periodStart.Format("2006-01-02"),
		periodEnd.Format("2006-01-02"))
	fmt.Printf("   - Status: %s\n", record.Status)

	// 6. Save to database
	fmt.Println("\n💾 Saving to database...")

	// Check if record with same ID already exists
	var existingRecord dbmodel.GpuUsageWeeklyReports
	result := db.Where("id = ?", reportID).First(&existingRecord)

	if result.Error == nil {
		// Record exists, ask if overwrite
		fmt.Printf("⚠️  Warning: Report with ID %s already exists\n", reportID)
		fmt.Println("   Overwrite existing record? (y/n)")

		var response string
		fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			fmt.Println("❌ Operation cancelled")
			os.Exit(0)
		}

		// Update existing record
		result = db.Model(&existingRecord).Updates(record)
		if result.Error != nil {
			fmt.Printf("❌ Failed to update record: %v\n", result.Error)
			os.Exit(1)
		}
		fmt.Println("✅ Record updated successfully")
	} else if result.Error == gorm.ErrRecordNotFound {
		// Record doesn't exist, create new record
		result = db.Create(record)
		if result.Error != nil {
			fmt.Printf("❌ Failed to create record: %v\n", result.Error)
			os.Exit(1)
		}
		fmt.Println("✅ Record created successfully")
	} else {
		fmt.Printf("❌ Database query failed: %v\n", result.Error)
		os.Exit(1)
	}

	// 7. Display summary
	fmt.Println("\n✨ Save complete!")
	fmt.Println("\n📊 Report summary:")
	fmt.Printf("   - Report ID: %s\n", reportID)
	fmt.Printf("   - Cluster name: %s\n", record.ClusterName)
	fmt.Printf("   - HTML size: %d bytes\n", len(htmlContent))
	if len(pdfContent) > 0 {
		fmt.Printf("   - PDF size: %d bytes\n", len(pdfContent))
	}

	if reportData.Summary != nil {
		fmt.Println("\n📈 Statistics:")
		fmt.Printf("   - Total GPUs: %d\n", reportData.Summary.TotalGPUs)
		fmt.Printf("   - Avg Utilization: %.2f%%\n", reportData.Summary.AvgUtilization)
		fmt.Printf("   - Avg Allocation: %.2f%%\n", reportData.Summary.AvgAllocation)
		fmt.Printf("   - Low Util Users: %d\n", reportData.Summary.LowUtilCount)
		fmt.Printf("   - Wasted GPU Days: %.1f\n", reportData.Summary.WastedGpuDays)
	}

	fmt.Println("\n💡 You can query the report using:")
	fmt.Printf("   SELECT * FROM gpu_usage_weekly_reports WHERE id = '%s';\n", reportID)
}

// generateReportID generates unique identifier for report
// Format: rpt_YYYYMMDD_clustername_uuid
func generateReportID(clusterName string) string {
	now := time.Now()
	dateStr := now.Format("20060102")
	shortUUID := uuid.New().String()[:8]
	return fmt.Sprintf("rpt_%s_%s_%s", dateStr, clusterName, shortUUID)
}
