package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

func main() {
	// Command line flags
	action := flag.String("action", "stats", "Action to perform: stats, migrate-all, migrate-batch, migrate-one")
	workloadUID := flag.String("workload", "", "Workload UID (for migrate-one)")
	batchSize := flag.Int("batch-size", 100, "Batch size for batch migration")
	dryRun := flag.Bool("dry-run", false, "Dry run mode (show what would be migrated)")
	
	flag.Parse()
	
	// Initialize database
	log.Info("Initializing database connection...")
	db := database.GetFacade().GetSystemConfig().GetDB()
	if db == nil {
		log.Fatal("Failed to get database connection")
	}
	
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get sql.DB: %v", err)
	}
	
	// Create versioned manager
	manager := framework.NewMultiDimensionalDetectionManagerWithVersioning(db, nil)
	
	// Ensure schema
	ctx := context.Background()
	if err := manager.EnsureSchema(ctx); err != nil {
		log.Fatalf("Failed to ensure schema: %v", err)
	}
	
	// Execute action
	switch *action {
	case "stats":
		showStats(ctx, manager)
	
	case "migrate-all":
		migrateAll(ctx, manager, *dryRun)
	
	case "migrate-batch":
		migrateBatch(ctx, manager, *batchSize, *dryRun)
	
	case "migrate-one":
		if *workloadUID == "" {
			log.Fatal("--workload flag is required for migrate-one action")
		}
		migrateOne(ctx, manager, *workloadUID, *dryRun)
	
	default:
		log.Fatalf("Unknown action: %s", *action)
	}
	
	sqlDB.Close()
}

// showStats displays version distribution statistics
func showStats(ctx context.Context, manager *framework.MultiDimensionalDetectionManagerWithVersioning) {
	log.Info("Fetching version statistics...")
	
	stats, err := manager.GetVersionStats(ctx)
	if err != nil {
		log.Fatalf("Failed to get version stats: %v", err)
	}
	
	total := 0
	for _, count := range stats {
		total += count
	}
	
	fmt.Println("\n=== Detection Version Statistics ===")
	fmt.Printf("Total records: %d\n\n", total)
	
	for version, count := range stats {
		percentage := float64(count) / float64(total) * 100
		fmt.Printf("Version %s: %d records (%.1f%%)\n", version, count, percentage)
	}
	
	if stats["1.0"] > 0 {
		fmt.Printf("\n⚠️  %d records need migration to v2.0\n", stats["1.0"])
		fmt.Println("\nTo migrate:")
		fmt.Println("  All records:     detection-migrate --action migrate-all")
		fmt.Println("  Batch migration: detection-migrate --action migrate-batch --batch-size 100")
	} else {
		fmt.Println("\n✓ All records are on v2.0!")
	}
	
	fmt.Println()
}

// migrateAll migrates all v1 records to v2
func migrateAll(
	ctx context.Context,
	manager *framework.MultiDimensionalDetectionManagerWithVersioning,
	dryRun bool,
) {
	// Get current stats
	stats, err := manager.GetVersionStats(ctx)
	if err != nil {
		log.Fatalf("Failed to get version stats: %v", err)
	}
	
	v1Count := stats["1.0"]
	if v1Count == 0 {
		log.Info("No v1 records to migrate")
		return
	}
	
	fmt.Printf("\n=== Migrate All ===\n")
	fmt.Printf("Found %d v1 records to migrate\n\n", v1Count)
	
	if dryRun {
		fmt.Println("DRY RUN MODE - No changes will be made")
		fmt.Printf("Would migrate %d records to v2.0\n", v1Count)
		return
	}
	
	// Confirm
	fmt.Print("Continue with migration? (yes/no): ")
	var confirm string
	fmt.Scanln(&confirm)
	
	if confirm != "yes" {
		log.Info("Migration cancelled")
		return
	}
	
	// Execute migration
	log.Info("Starting migration...")
	startTime := time.Now()
	
	successCount, err := manager.MigrateAll(ctx)
	if err != nil {
		log.Errorf("Migration failed: %v", err)
		return
	}
	
	duration := time.Since(startTime)
	
	fmt.Printf("\n=== Migration Complete ===\n")
	fmt.Printf("Successfully migrated: %d/%d records\n", successCount, v1Count)
	fmt.Printf("Duration: %s\n", duration)
	fmt.Printf("Average: %.2f records/second\n", float64(successCount)/duration.Seconds())
	
	// Show final stats
	fmt.Println("\nFinal statistics:")
	showStats(ctx, manager)
}

// migrateBatch migrates records in batches
func migrateBatch(
	ctx context.Context,
	manager *framework.MultiDimensionalDetectionManagerWithVersioning,
	batchSize int,
	dryRun bool,
) {
	// Get v1 records
	stats, _ := manager.GetVersionStats(ctx)
	v1Count := stats["1.0"]
	
	if v1Count == 0 {
		log.Info("No v1 records to migrate")
		return
	}
	
	fmt.Printf("\n=== Batch Migration ===\n")
	fmt.Printf("Total v1 records: %d\n", v1Count)
	fmt.Printf("Batch size: %d\n", batchSize)
	fmt.Printf("Estimated batches: %d\n\n", (v1Count+batchSize-1)/batchSize)
	
	if dryRun {
		fmt.Println("DRY RUN MODE - No changes will be made")
		return
	}
	
	// TODO: Implement batch migration with workload UID list
	// For now, use migrate-all
	log.Warn("Batch migration with specific UIDs not yet implemented, using migrate-all")
	migrateAll(ctx, manager, false)
}

// migrateOne migrates a single workload
func migrateOne(
	ctx context.Context,
	manager *framework.MultiDimensionalDetectionManagerWithVersioning,
	workloadUID string,
	dryRun bool,
) {
	fmt.Printf("\n=== Migrate Single Workload ===\n")
	fmt.Printf("Workload UID: %s\n\n", workloadUID)
	
	// Load current version
	detection, err := manager.LoadDetection(ctx, workloadUID)
	if err != nil {
		log.Fatalf("Failed to load workload: %v", err)
	}
	
	if detection == nil {
		log.Fatalf("Workload not found: %s", workloadUID)
	}
	
	fmt.Printf("Current version: %s\n", detection.Version)
	fmt.Printf("Confidence: %.2f\n", detection.Confidence)
	fmt.Printf("Status: %s\n", detection.Status)
	fmt.Printf("Dimensions: %d\n", len(detection.Dimensions))
	
	if detection.Version == "2.0" {
		fmt.Println("\n✓ Workload is already on v2.0")
		return
	}
	
	if dryRun {
		fmt.Println("\nDRY RUN MODE - Would migrate this workload to v2.0")
		return
	}
	
	// Migrate
	fmt.Print("\nContinue with migration? (yes/no): ")
	var confirm string
	fmt.Scanln(&confirm)
	
	if confirm != "yes" {
		log.Info("Migration cancelled")
		return
	}
	
	if err := manager.MigrateWorkload(ctx, workloadUID); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
	
	fmt.Println("\n✓ Migration successful!")
	
	// Load and show updated version
	detection, _ = manager.LoadDetection(ctx, workloadUID)
	fmt.Printf("New version: %s\n", detection.Version)
	fmt.Printf("Dimensions:\n")
	for dim, values := range detection.Dimensions {
		fmt.Printf("  - %s: %d values\n", dim, len(values))
	}
}

