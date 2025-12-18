package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
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
	// Command line flags
	action := flag.String("action", "stats", "Action to perform: stats, migrate-all, migrate-batch, migrate-one")
	workloadUID := flag.String("workload", "", "Workload UID (for migrate-one)")
	batchSize := flag.Int("batch-size", 100, "Batch size for batch migration")
	dryRun := flag.Bool("dry-run", false, "Dry run mode (show what would be migrated)")
	autoYes := flag.Bool("yes", false, "Automatically answer yes to confirmation prompts")

	flag.Parse()

	// Initialize database connection
	fmt.Println("🔧 Detection Version Migration Tool")
	fmt.Println("===================================")
	fmt.Println()
	fmt.Println("💾 Connecting to database...")
	fmt.Printf("   - Host: %s:%s\n", *dbHost, *dbPort)
	fmt.Printf("   - Database: %s\n", *dbName)
	fmt.Printf("   - User: %s\n", *dbUser)
	fmt.Println()

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
		fmt.Println("   Usage: detection-migrate --action stats -dbHost=localhost -dbPort=5432 -dbUser=postgres -dbPass=yourpass -dbName=primus_lens")
		return
	}
	fmt.Println("✅ Database connected successfully")
	fmt.Println()

	sqlDB, err := db.DB()
	if err != nil {
		fmt.Printf("❌ Failed to get sql.DB: %v\n", err)
		return
	}
	defer sqlDB.Close()

	// Create V1 migrator
	migrator := NewV1Migrator(db)

	ctx := context.Background()

	// Execute action
	switch *action {
	case "stats":
		showStats(ctx, migrator)

	case "migrate-all":
		migrateAll(ctx, migrator, *dryRun, *autoYes)

	case "migrate-batch":
		migrateBatch(ctx, migrator, *batchSize, *dryRun)

	case "migrate-one":
		if *workloadUID == "" {
			fmt.Println("❌ Error: --workload flag is required for migrate-one action")
			return
		}
		migrateOne(ctx, migrator, *workloadUID, *dryRun, *autoYes)

	default:
		fmt.Printf("❌ Unknown action: %s\n", *action)
		fmt.Println("\n💡 Available actions: stats, migrate-all, migrate-batch, migrate-one")
	}
}

// showStats displays version distribution statistics
func showStats(ctx context.Context, migrator *V1Migrator) {
	fmt.Println("📊 Fetching version statistics from ai_workload_metadata...")

	stats, err := migrator.GetVersionStats(ctx)
	if err != nil {
		fmt.Printf("❌ Failed to get version stats: %v\n", err)
		return
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
	migrator *V1Migrator,
	dryRun bool,
	autoYes bool,
) {
	// Get current stats
	stats, err := migrator.GetVersionStats(ctx)
	if err != nil {
		fmt.Printf("❌ Failed to get version stats: %v\n", err)
		return
	}

	v1Count := stats["1.0"]
	if v1Count == 0 {
		fmt.Println("✅ No v1 records to migrate")
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
	if !autoYes {
		fmt.Print("Continue with migration? (yes/no): ")
		var confirm string
		fmt.Scanln(&confirm)

		if confirm != "yes" {
			fmt.Println("❌ Migration cancelled")
			return
		}
	} else {
		fmt.Println("Auto-confirmed (--yes flag)")
	}

	// Execute migration
	fmt.Println("⚙️  Starting migration...")
	startTime := time.Now()

	successCount, err := migrator.MigrateAll(ctx)
	if err != nil {
		fmt.Printf("❌ Migration failed: %v\n", err)
		return
	}

	duration := time.Since(startTime)

	fmt.Printf("\n=== Migration Complete ===\n")
	fmt.Printf("Successfully migrated: %d/%d records\n", successCount, v1Count)
	fmt.Printf("Duration: %s\n", duration)
	if duration.Seconds() > 0 {
		fmt.Printf("Average: %.2f records/second\n", float64(successCount)/duration.Seconds())
	}

	// Show final stats
	fmt.Println("\nFinal statistics:")
	showStats(ctx, migrator)
}

// migrateBatch migrates records in batches
func migrateBatch(
	ctx context.Context,
	migrator *V1Migrator,
	batchSize int,
	dryRun bool,
) {
	// Get v1 records
	stats, _ := migrator.GetVersionStats(ctx)
	v1Count := stats["1.0"]

	if v1Count == 0 {
		fmt.Println("✅ No v1 records to migrate")
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
	fmt.Println("⚠️  Batch migration with specific UIDs not yet implemented, using migrate-all")
	migrateAll(ctx, migrator, false, true) // Auto-yes for batch mode
}

// migrateOne migrates a single workload
func migrateOne(
	ctx context.Context,
	migrator *V1Migrator,
	workloadUID string,
	dryRun bool,
	autoYes bool,
) {
	fmt.Printf("\n=== Migrate Single Workload ===\n")
	fmt.Printf("Workload UID: %s\n", workloadUID)
	fmt.Printf("Table: ai_workload_metadata\n")
	fmt.Printf("Field: metadata.framework_detection\n\n")

	// Load current version
	v2, v1, err := migrator.LoadDetection(ctx, workloadUID)
	if err != nil {
		fmt.Printf("❌ Failed to load workload: %v\n", err)
		return
	}

	if v2 == nil && v1 == nil {
		fmt.Printf("❌ No detection data found for workload: %s\n", workloadUID)
		return
	}

	if v2 != nil {
		fmt.Printf("Current version: %s\n", v2.Version)
		fmt.Printf("Confidence: %.2f\n", v2.Confidence)
		fmt.Printf("Status: %s\n", v2.Status)
		fmt.Printf("Dimensions: %d\n", len(v2.Dimensions))
		fmt.Println("\n✓ Workload is already on v2.0")

		// Show dimension details
		if len(v2.Dimensions) > 0 {
			fmt.Println("\nDimension details:")
			for dim, values := range v2.Dimensions {
				fmt.Printf("  - %s: %d values\n", dim, len(values))
				for _, v := range values {
					fmt.Printf("      • %s (confidence: %.2f, source: %s)\n",
						v.Value, v.Confidence, v.Source)
				}
			}
		}
		return
	}

	// V1 workload
	fmt.Println("⚠️  This workload is on V1.0 format")
	fmt.Printf("Frameworks: %v\n", v1.Frameworks)
	fmt.Printf("Type: %s\n", v1.Type)
	fmt.Printf("Confidence: %.2f\n", v1.Confidence)
	fmt.Printf("Status: %s\n", v1.Status)

	if dryRun {
		fmt.Println("\nDRY RUN MODE - Would migrate this workload to v2.0")
		v2Preview := ConvertV1ToV2(v1)
		fmt.Println("After migration, workload will have:")
		for dim, values := range v2Preview.Dimensions {
			fmt.Printf("  - %s: %d values\n", dim, len(values))
		}
		return
	}

	// Migrate
	if !autoYes {
		fmt.Print("\nContinue with migration? (yes/no): ")
		var confirm string
		fmt.Scanln(&confirm)

		if confirm != "yes" {
			fmt.Println("❌ Migration cancelled")
			return
		}
	} else {
		fmt.Println("\nAuto-confirmed (--yes flag)")
	}

	if err := migrator.MigrateWorkload(ctx, workloadUID); err != nil {
		fmt.Printf("❌ Migration failed: %v\n", err)
		return
	}

	fmt.Println("✓ Migration successful!")

	// Load and show updated version
	v2, _, _ = migrator.LoadDetection(ctx, workloadUID)
	if v2 != nil {
		fmt.Printf("\nNew version: %s\n", v2.Version)
		fmt.Printf("Dimensions: %d\n", len(v2.Dimensions))
		if len(v2.Dimensions) > 0 {
			fmt.Println("\nDimension details:")
			for dim, values := range v2.Dimensions {
				fmt.Printf("  - %s: %d values\n", dim, len(values))
			}
		}
	}
}

// === V1 Migration Logic ===
// These functions handle V1 (FrameworkDetection) to V2 (MultiDimensionalDetection) migration

// V1Migrator handles migration from V1 to V2
type V1Migrator struct {
	db *gorm.DB
}

func NewV1Migrator(db *gorm.DB) *V1Migrator {
	return &V1Migrator{
		db: db,
	}
}

// GetVersionStats returns version distribution
func (m *V1Migrator) GetVersionStats(ctx context.Context) (map[string]int, error) {
	var results []struct {
		Version string
		Count   int64
	}

	query := `
		SELECT 
			CASE 
				WHEN metadata->'framework_detection'->>'version' = '2.0' THEN '2.0'
				WHEN metadata->'framework_detection'->'dimensions' IS NOT NULL THEN '2.0'
				WHEN metadata->'framework_detection'->'frameworks' IS NOT NULL THEN '1.0'
				WHEN metadata->'framework_detection' IS NOT NULL THEN 'unknown'
				ELSE 'no_data'
			END as version,
			COUNT(*) as count
		FROM ai_workload_metadata
		WHERE metadata->'framework_detection' IS NOT NULL
		GROUP BY version
	`

	err := m.db.WithContext(ctx).Raw(query).Scan(&results).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get version stats: %w", err)
	}

	stats := make(map[string]int)
	for _, r := range results {
		stats[r.Version] = int(r.Count)
	}

	return stats, nil
}

// LoadDetection loads detection (V1 or V2) directly from database
func (m *V1Migrator) LoadDetection(
	ctx context.Context,
	workloadUID string,
) (*model.MultiDimensionalDetection, *model.FrameworkDetection, error) {
	var result struct {
		WorkloadUID string
		Metadata    []byte // Scan as raw bytes
	}

	err := m.db.WithContext(ctx).
		Table("ai_workload_metadata").
		Select("workload_uid, metadata").
		Where("workload_uid = ?", workloadUID).
		First(&result).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	// Parse JSONB data
	var metadata map[string]interface{}
	if err := json.Unmarshal(result.Metadata, &metadata); err != nil {
		return nil, nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	detectionData, ok := metadata["framework_detection"]
	if !ok {
		return nil, nil, nil
	}

	detectionJSON, err := json.Marshal(detectionData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal detection: %w", err)
	}

	// Try V2 first
	var v2Detection model.MultiDimensionalDetection
	if err := json.Unmarshal(detectionJSON, &v2Detection); err == nil {
		if v2Detection.Version == "2.0" && v2Detection.Dimensions != nil {
			return &v2Detection, nil, nil
		}
	}

	// Parse as V1
	var v1Detection model.FrameworkDetection
	if err := json.Unmarshal(detectionJSON, &v1Detection); err != nil {
		return nil, nil, fmt.Errorf("failed to parse detection: %w", err)
	}

	return nil, &v1Detection, nil
}

// MigrateWorkload migrates a single workload from V1 to V2
func (m *V1Migrator) MigrateWorkload(ctx context.Context, workloadUID string) error {
	v2, v1, err := m.LoadDetection(ctx, workloadUID)
	if err != nil {
		return err
	}

	if v2 != nil {
		//fmt.Printf("  ℹ️  Workload %s already on V2\n", workloadUID)
		return nil
	}

	if v1 == nil {
		return fmt.Errorf("no detection found for workload %s", workloadUID)
	}

	// Convert V1 to V2
	v2Detection := ConvertV1ToV2(v1)
	v2Detection.WorkloadUID = workloadUID

	// Save as V2 - directly update the metadata field
	err = m.db.WithContext(ctx).
		Table("ai_workload_metadata").
		Where("workload_uid = ?", workloadUID).
		Update("metadata", gorm.Expr("jsonb_set(metadata, '{framework_detection}', ?::jsonb)",
			toJSON(v2Detection))).Error

	if err != nil {
		return fmt.Errorf("failed to save V2 detection: %w", err)
	}

	fmt.Printf("  ✅ Migrated %s\n", workloadUID)
	return nil
}

// toJSON converts a value to JSON string
func toJSON(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// MigrateAll migrates all V1 workloads to V2
func (m *V1Migrator) MigrateAll(ctx context.Context) (int, error) {
	// Find all V1 workloads
	query := `
		SELECT workload_uid
		FROM ai_workload_metadata
		WHERE metadata->'framework_detection' IS NOT NULL
		  AND metadata->'framework_detection'->'frameworks' IS NOT NULL
		  AND (
		      metadata->'framework_detection'->>'version' IS NULL
		      OR metadata->'framework_detection'->>'version' != '2.0'
		  )
	`

	var workloadUIDs []string
	err := m.db.WithContext(ctx).Raw(query).Scan(&workloadUIDs).Error
	if err != nil {
		return 0, fmt.Errorf("failed to find V1 workloads: %w", err)
	}

	successCount := 0
	for i, uid := range workloadUIDs {
		if err := m.MigrateWorkload(ctx, uid); err != nil {
			fmt.Printf("  ❌ Failed to migrate %s: %v\n", uid, err)
			continue
		}

		successCount++

		if (i+1)%10 == 0 {
			fmt.Printf("Progress: %d/%d workloads\n", i+1, len(workloadUIDs))
		}
	}

	return successCount, nil
}

// ConvertV1ToV2 converts V1 FrameworkDetection to V2 MultiDimensionalDetection
func ConvertV1ToV2(v1 *model.FrameworkDetection) *model.MultiDimensionalDetection {
	v2 := &model.MultiDimensionalDetection{
		Version:    "2.0",
		Dimensions: make(map[model.DetectionDimension][]model.DimensionValue),
		Conflicts:  make(map[model.DetectionDimension][]model.DetectionConflict),
		Confidence: v1.Confidence,
		Status:     v1.Status,
		UpdatedAt:  time.Now(),
	}

	// Convert behavior (type)
	if v1.Type != "" {
		v2.Dimensions[model.DimensionBehavior] = []model.DimensionValue{
			{
				Value:      v1.Type,
				Confidence: v1.Confidence,
				Source:     "v1_migration",
				DetectedAt: v1.UpdatedAt,
				Evidence:   map[string]interface{}{"method": "v1_type_field"},
			},
		}
	}

	// Convert wrapper framework
	if v1.WrapperFramework != "" {
		v2.Dimensions[model.DimensionWrapperFramework] = []model.DimensionValue{
			{
				Value:      v1.WrapperFramework,
				Confidence: v1.Confidence,
				Source:     "v1_migration",
				DetectedAt: v1.UpdatedAt,
				Evidence:   map[string]interface{}{"method": "v1_wrapper_framework_field"},
			},
		}
	}

	// Convert base framework
	if v1.BaseFramework != "" {
		v2.Dimensions[model.DimensionBaseFramework] = []model.DimensionValue{
			{
				Value:      v1.BaseFramework,
				Confidence: v1.Confidence,
				Source:     "v1_migration",
				DetectedAt: v1.UpdatedAt,
				Evidence:   map[string]interface{}{"method": "v1_base_framework_field"},
			},
		}

		// Infer runtime from base framework
		runtime := inferRuntimeFromFramework(v1.BaseFramework)
		if runtime != "" {
			v2.Dimensions[model.DimensionRuntime] = []model.DimensionValue{
				{
					Value:      runtime,
					Confidence: v1.Confidence * 0.9,
					Source:     "v1_migration_inference",
					DetectedAt: v1.UpdatedAt,
					Evidence: map[string]interface{}{
						"method":         "inferred_from_base_framework",
						"base_framework": v1.BaseFramework,
					},
				},
			}
		}
	}

	// Infer language from sources
	language := inferLanguageFromSources(v1.Sources)
	if language != "" {
		v2.Dimensions[model.DimensionLanguage] = []model.DimensionValue{
			{
				Value:      language,
				Confidence: v1.Confidence * 0.8,
				Source:     "v1_migration_inference",
				DetectedAt: v1.UpdatedAt,
				Evidence:   map[string]interface{}{"method": "inferred_from_sources"},
			},
		}
	}

	return v2
}

func inferRuntimeFromFramework(framework string) string {
	fw := strings.ToLower(framework)

	if fw == "megatron" || fw == "deepspeed" || fw == "fairscale" ||
		strings.Contains(fw, "pytorch") || strings.Contains(fw, "torch") {
		return "pytorch"
	}

	if strings.Contains(fw, "tensorflow") || strings.Contains(fw, "keras") {
		return "tensorflow"
	}

	if strings.Contains(fw, "jax") || strings.Contains(fw, "flax") {
		return "jax"
	}

	return ""
}

func inferLanguageFromSources(sources []model.DetectionSource) string {
	for _, source := range sources {
		if source.Source == "wandb" {
			return "python" // WandB is typically Python
		}
		// Check evidence for python version
		if source.Evidence != nil {
			if system, ok := source.Evidence["system"].(map[string]interface{}); ok {
				if _, ok := system["python_version"]; ok {
					return "python"
				}
			}
		}
	}
	return ""
}
