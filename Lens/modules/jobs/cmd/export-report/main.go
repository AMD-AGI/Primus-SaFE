package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
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
	
	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("📊 GPU Usage Weekly Report - Export from Database")
		fmt.Println("==========================================")
		fmt.Println("Usage: go run main.go [options] <report_id>")
		fmt.Println("\nExample: go run main.go -dbHost=localhost -dbPass=yourpass rpt_20251125_x-flannel_abc12345")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		fmt.Println("\n💡 Tip: Run 'cd ../list-reports && go run main.go' to view all report IDs")
		os.Exit(1)
	}

	reportID := args[0]

	fmt.Println("📊 GPU Usage Weekly Report - Export from Database")
	fmt.Println("==========================================")
	fmt.Printf("📋 Report ID: %s\n\n", reportID)

	// Initialize database connection
	fmt.Println("💾 Connecting to database...")
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
		os.Exit(1)
	}
	fmt.Println("✅ Database connected successfully")

	// Query report
	fmt.Println("🔍 Querying report...")
	var report dbmodel.GpuUsageWeeklyReports
	result := db.Where("id = ?", reportID).First(&report)
	if result.Error != nil {
		fmt.Printf("❌ Query failed: %v\n", result.Error)
		fmt.Println("\n💡 Tip: Use 'cd ../list-reports && go run main.go' to view all available reports")
		os.Exit(1)
	}
	fmt.Println("✅ Report found")

	// Display report information
	fmt.Println("\n📊 Report information:")
	fmt.Printf("   - ID: %s\n", report.ID)
	fmt.Printf("   - Cluster: %s\n", report.ClusterName)
	fmt.Printf("   - Period: %s to %s\n", 
		report.PeriodStart.Format("2006-01-02"),
		report.PeriodEnd.Format("2006-01-02"))
	fmt.Printf("   - Generated at: %s\n", report.GeneratedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("   - Status: %s\n", report.Status)

	// Create output directory
	outputDir := fmt.Sprintf("exported_report_%s", reportID)
	fmt.Printf("\n📁 Creating output directory: %s\n", outputDir)
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		fmt.Printf("❌ Failed to create directory: %v\n", err)
		os.Exit(1)
	}

	filesExported := 0

	// Export JSON data
	if report.JSONContent != nil {
		fmt.Println("📄 Exporting JSON data...")
		jsonPath := filepath.Join(outputDir, "report_data.json")
		jsonBytes, err := json.MarshalIndent(report.JSONContent, "", "  ")
		if err != nil {
			fmt.Printf("⚠️  JSON serialization failed: %v\n", err)
		} else {
			err = os.WriteFile(jsonPath, jsonBytes, 0644)
			if err != nil {
				fmt.Printf("⚠️  Failed to save JSON: %v\n", err)
			} else {
				fmt.Printf("   ✅ %s (%d bytes)\n", jsonPath, len(jsonBytes))
				filesExported++
			}
		}
	}

	// Export HTML
	if len(report.HTMLContent) > 0 {
		fmt.Println("📄 Exporting HTML report...")
		htmlPath := filepath.Join(outputDir, "report.html")
		err = os.WriteFile(htmlPath, report.HTMLContent, 0644)
		if err != nil {
			fmt.Printf("⚠️  Failed to save HTML: %v\n", err)
		} else {
			fmt.Printf("   ✅ %s (%d bytes)\n", htmlPath, len(report.HTMLContent))
			filesExported++
		}
	}

	// Export PDF
	if len(report.PdfContent) > 0 {
		fmt.Println("📄 Exporting PDF report...")
		pdfPath := filepath.Join(outputDir, "report.pdf")
		err = os.WriteFile(pdfPath, report.PdfContent, 0644)
		if err != nil {
			fmt.Printf("⚠️  Failed to save PDF: %v\n", err)
		} else {
			fmt.Printf("   ✅ %s (%d bytes)\n", pdfPath, len(report.PdfContent))
			filesExported++
		}
	} else {
		fmt.Println("ℹ️  This report has no PDF content")
	}

	// Export metadata
	if report.Metadata != nil {
		fmt.Println("📄 Exporting metadata...")
		metadataPath := filepath.Join(outputDir, "metadata.json")
		metadataBytes, err := json.MarshalIndent(report.Metadata, "", "  ")
		if err != nil {
			fmt.Printf("⚠️  Metadata serialization failed: %v\n", err)
		} else {
			err = os.WriteFile(metadataPath, metadataBytes, 0644)
			if err != nil {
				fmt.Printf("⚠️  Failed to save metadata: %v\n", err)
			} else {
				fmt.Printf("   ✅ %s (%d bytes)\n", metadataPath, len(metadataBytes))
				filesExported++
			}
		}
	}

	// Create summary file
	fmt.Println("📄 Creating summary file...")
	summaryPath := filepath.Join(outputDir, "README.txt")
	summary := fmt.Sprintf(`GPU Usage Weekly Report Export
================================

Report ID:       %s
Cluster:         %s
Period:          %s to %s
Generated:       %s
Status:          %s
Exported:        %s

Files:
------
`, 
		report.ID,
		report.ClusterName,
		report.PeriodStart.Format("2006-01-02"),
		report.PeriodEnd.Format("2006-01-02"),
		report.GeneratedAt.Format("2006-01-02 15:04:05"),
		report.Status,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	if len(report.HTMLContent) > 0 {
		summary += "- report.html: HTML report\n"
	}
	if len(report.PdfContent) > 0 {
		summary += "- report.pdf: PDF report\n"
	}
	if report.JSONContent != nil {
		summary += "- report_data.json: Structured report data\n"
	}
	if report.Metadata != nil {
		summary += "- metadata.json: Report metadata and statistics\n"
	}

	err = os.WriteFile(summaryPath, []byte(summary), 0644)
	if err != nil {
		fmt.Printf("⚠️  Failed to save summary: %v\n", err)
	} else {
		fmt.Printf("   ✅ %s\n", summaryPath)
		filesExported++
	}

	fmt.Println("\n✨ Export complete!")
	fmt.Printf("   Exported %d files to: %s/\n", filesExported, outputDir)
	fmt.Println("\n💡 Tip: Open report.html in a browser to view the report")
}

