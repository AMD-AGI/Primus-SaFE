package main

import (
	"flag"
	"fmt"
	"os"

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

	fmt.Println("📊 GPU Usage Weekly Reports - Query Database")
	fmt.Println("==========================================")

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
		fmt.Println("\n💡 Tip: Usage: go run main.go -dbHost=localhost -dbPort=5432 -dbUser=postgres -dbPass=yourpass -dbName=primus_lens")
		os.Exit(1)
	}
	fmt.Println("✅ Database connected successfully")

	// Query all reports (excluding large fields)
	var reports []dbmodel.GpuUsageWeeklyReports
	result := db.Select("id, cluster_name, period_start, period_end, generated_at, status, error_message, created_at, updated_at").
		Order("generated_at DESC").
		Limit(20).
		Find(&reports)

	if result.Error != nil {
		fmt.Printf("❌ Query failed: %v\n", result.Error)
		os.Exit(1)
	}

	if len(reports) == 0 {
		fmt.Println("📭 No reports found")
		return
	}

	fmt.Printf("📋 Found %d report records (showing last 20):\n\n", len(reports))

	// Display report list
	for i, report := range reports {
		fmt.Printf("═══════════════════════════════════════════════════════════\n")
		fmt.Printf("Report #%d\n", i+1)
		fmt.Printf("───────────────────────────────────────────────────────────\n")
		fmt.Printf("  ID:               %s\n", report.ID)
		fmt.Printf("  Cluster name:     %s\n", report.ClusterName)
		fmt.Printf("  Period start:     %s\n", report.PeriodStart.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Period end:       %s\n", report.PeriodEnd.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Generated at:     %s\n", report.GeneratedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Status:           %s\n", report.Status)

		// Display statistics from metadata
		if report.Metadata != nil {
			if totalGPUs, ok := report.Metadata["total_gpus"].(float64); ok {
				fmt.Printf("  Total GPUs:       %.0f\n", totalGPUs)
			}
			if avgUtil, ok := report.Metadata["avg_utilization"].(float64); ok {
				fmt.Printf("  Avg utilization:  %.2f%%\n", avgUtil)
			}
			if avgAlloc, ok := report.Metadata["avg_allocation"].(float64); ok {
				fmt.Printf("  Avg allocation:   %.2f%%\n", avgAlloc)
			}
			if lowUtilCount, ok := report.Metadata["low_util_count"].(float64); ok {
				fmt.Printf("  Low util users:   %.0f\n", lowUtilCount)
			}
		}

		if report.ErrorMessage != "" {
			fmt.Printf("  Error message:    %s\n", report.ErrorMessage)
		}
		fmt.Println()
	}

	fmt.Println("═══════════════════════════════════════════════════════════")

	// Provide export option
	if len(reports) > 0 {
		fmt.Println("💡 Tip: To export a report, run:")
		fmt.Printf("   cd cmd/export-report && go run main.go %s\n", reports[0].ID)
	}
}
