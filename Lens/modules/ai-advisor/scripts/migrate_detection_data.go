package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OldAiWorkloadMetadata represents the old ai_workload_metadata table
type OldAiWorkloadMetadata struct {
	ID          int32                  `gorm:"column:id;primaryKey"`
	WorkloadUID string                 `gorm:"column:workload_uid"`
	Type        string                 `gorm:"column:type"`
	Framework   string                 `gorm:"column:framework"`
	Metadata    map[string]interface{} `gorm:"column:metadata;type:jsonb"`
	CreatedAt   time.Time              `gorm:"column:created_at"`
	ImagePrefix string                 `gorm:"column:image_prefix"`
}

func (OldAiWorkloadMetadata) TableName() string {
	return "ai_workload_metadata"
}

// NewWorkloadDetection represents the new workload_detection table
type NewWorkloadDetection struct {
	ID               int64                  `gorm:"column:id;primaryKey;autoIncrement:true"`
	WorkloadUID      string                 `gorm:"column:workload_uid;not null;uniqueIndex"`
	Status           string                 `gorm:"column:status;not null;default:unknown"`
	Framework        string                 `gorm:"column:framework"`
	Frameworks       []byte                 `gorm:"column:frameworks;type:jsonb"`
	WorkloadType     string                 `gorm:"column:workload_type"`
	Confidence       float64                `gorm:"column:confidence;default:0.0"`
	FrameworkLayer   string                 `gorm:"column:framework_layer"`
	WrapperFramework string                 `gorm:"column:wrapper_framework"`
	BaseFramework    string                 `gorm:"column:base_framework"`
	DetectionState   string                 `gorm:"column:detection_state;not null;default:completed"`
	AttemptCount     int32                  `gorm:"column:attempt_count;default:0"`
	MaxAttempts      int32                  `gorm:"column:max_attempts;default:5"`
	LastAttemptAt    *time.Time             `gorm:"column:last_attempt_at"`
	NextAttemptAt    *time.Time             `gorm:"column:next_attempt_at"`
	Context          map[string]interface{} `gorm:"column:context;type:jsonb"`
	EvidenceCount    int32                  `gorm:"column:evidence_count;default:0"`
	EvidenceSources  []byte                 `gorm:"column:evidence_sources;type:jsonb"`
	Conflicts        []byte                 `gorm:"column:conflicts;type:jsonb"`
	CreatedAt        time.Time              `gorm:"column:created_at;not null"`
	UpdatedAt        time.Time              `gorm:"column:updated_at;not null"`
	ConfirmedAt      *time.Time             `gorm:"column:confirmed_at"`
}

func (NewWorkloadDetection) TableName() string {
	return "workload_detection"
}

// NewWorkloadDetectionEvidence represents the new workload_detection_evidence table
type NewWorkloadDetectionEvidence struct {
	ID               int64                  `gorm:"column:id;primaryKey;autoIncrement:true"`
	WorkloadUID      string                 `gorm:"column:workload_uid;not null"`
	Source           string                 `gorm:"column:source;not null"`
	SourceType       string                 `gorm:"column:source_type;default:passive"`
	Framework        string                 `gorm:"column:framework"`
	Frameworks       []byte                 `gorm:"column:frameworks;type:jsonb"`
	WorkloadType     string                 `gorm:"column:workload_type"`
	Confidence       float64                `gorm:"column:confidence;not null;default:0.0"`
	FrameworkLayer   string                 `gorm:"column:framework_layer"`
	WrapperFramework string                 `gorm:"column:wrapper_framework"`
	BaseFramework    string                 `gorm:"column:base_framework"`
	Evidence         map[string]interface{} `gorm:"column:evidence;type:jsonb;not null"`
	Processed        bool                   `gorm:"column:processed;not null;default:true"`
	ProcessedAt      *time.Time             `gorm:"column:processed_at"`
	DetectedAt       time.Time              `gorm:"column:detected_at;not null"`
	ExpiresAt        *time.Time             `gorm:"column:expires_at"`
	CreatedAt        time.Time              `gorm:"column:created_at;not null"`
}

func (NewWorkloadDetectionEvidence) TableName() string {
	return "workload_detection_evidence"
}

// MigrationStats tracks migration progress
type MigrationStats struct {
	TotalRecords     int
	DetectionsMigrated int
	EvidencesMigrated  int
	Skipped          int
	Errors           int
}

func main() {
	// Parse command line flags
	dsn := flag.String("dsn", "", "PostgreSQL DSN (e.g., host=localhost user=postgres password=xxx dbname=primus_lens)")
	dryRun := flag.Bool("dry-run", false, "Dry run mode - don't actually write data")
	batchSize := flag.Int("batch-size", 100, "Batch size for processing")
	flag.Parse()

	if *dsn == "" {
		fmt.Println("Usage: migrate_detection_data -dsn <postgresql_dsn> [-dry-run] [-batch-size N]")
		fmt.Println("")
		fmt.Println("Example:")
		fmt.Println("  migrate_detection_data -dsn 'host=localhost user=postgres password=xxx dbname=primus_lens sslmode=disable'")
		os.Exit(1)
	}

	// Connect to database
	db, err := gorm.Open(postgres.Open(*dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		fmt.Printf("Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	stats := &MigrationStats{}

	fmt.Println("=== Detection Data Migration ===")
	fmt.Printf("Dry Run: %v\n", *dryRun)
	fmt.Printf("Batch Size: %d\n", *batchSize)
	fmt.Println("")

	// Step 1: Check if new tables exist
	if !db.Migrator().HasTable(&NewWorkloadDetection{}) {
		fmt.Println("ERROR: workload_detection table does not exist. Please run the migration SQL first.")
		os.Exit(1)
	}
	if !db.Migrator().HasTable(&NewWorkloadDetectionEvidence{}) {
		fmt.Println("ERROR: workload_detection_evidence table does not exist. Please run the migration SQL first.")
		os.Exit(1)
	}

	// Step 2: Count records to migrate
	var totalCount int64
	if err := db.WithContext(ctx).Model(&OldAiWorkloadMetadata{}).Count(&totalCount).Error; err != nil {
		fmt.Printf("Failed to count records: %v\n", err)
		os.Exit(1)
	}
	stats.TotalRecords = int(totalCount)
	fmt.Printf("Found %d records in ai_workload_metadata to migrate\n\n", totalCount)

	// Step 3: Process in batches
	offset := 0
	for {
		var oldRecords []OldAiWorkloadMetadata
		if err := db.WithContext(ctx).
			Order("id ASC").
			Offset(offset).
			Limit(*batchSize).
			Find(&oldRecords).Error; err != nil {
			fmt.Printf("Failed to fetch records: %v\n", err)
			break
		}

		if len(oldRecords) == 0 {
			break
		}

		for _, old := range oldRecords {
			if err := migrateRecord(ctx, db, &old, *dryRun, stats); err != nil {
				fmt.Printf("  ERROR migrating workload %s: %v\n", old.WorkloadUID, err)
				stats.Errors++
			}
		}

		offset += len(oldRecords)
		fmt.Printf("Progress: %d/%d records processed\n", offset, totalCount)
	}

	// Step 4: Print summary
	fmt.Println("")
	fmt.Println("=== Migration Summary ===")
	fmt.Printf("Total Records:        %d\n", stats.TotalRecords)
	fmt.Printf("Detections Migrated:  %d\n", stats.DetectionsMigrated)
	fmt.Printf("Evidences Migrated:   %d\n", stats.EvidencesMigrated)
	fmt.Printf("Skipped:              %d\n", stats.Skipped)
	fmt.Printf("Errors:               %d\n", stats.Errors)

	if *dryRun {
		fmt.Println("\n[DRY RUN] No data was actually written to the database.")
	}
}

func migrateRecord(ctx context.Context, db *gorm.DB, old *OldAiWorkloadMetadata, dryRun bool, stats *MigrationStats) error {
	// Skip if already migrated
	var existingCount int64
	db.WithContext(ctx).Model(&NewWorkloadDetection{}).Where("workload_uid = ?", old.WorkloadUID).Count(&existingCount)
	if existingCount > 0 {
		fmt.Printf("  SKIP: workload %s already migrated\n", old.WorkloadUID)
		stats.Skipped++
		return nil
	}

	// Extract detection info from metadata
	detectionInfo := extractDetectionInfo(old)

	// Create workload_detection record
	now := time.Now()
	detection := &NewWorkloadDetection{
		WorkloadUID:      old.WorkloadUID,
		Status:           detectionInfo.Status,
		Framework:        old.Framework,
		WorkloadType:     old.Type,
		Confidence:       detectionInfo.Confidence,
		FrameworkLayer:   detectionInfo.FrameworkLayer,
		WrapperFramework: detectionInfo.WrapperFramework,
		BaseFramework:    detectionInfo.BaseFramework,
		DetectionState:   "completed", // Migrated data is already completed
		AttemptCount:     0,
		MaxAttempts:      5,
		EvidenceCount:    int32(len(detectionInfo.Sources)),
		CreatedAt:        old.CreatedAt,
		UpdatedAt:        now,
	}

	// Set frameworks JSON
	if old.Framework != "" {
		frameworks := []string{old.Framework}
		if detectionInfo.WrapperFramework != "" && detectionInfo.WrapperFramework != old.Framework {
			frameworks = append([]string{detectionInfo.WrapperFramework}, frameworks...)
		}
		if detectionInfo.BaseFramework != "" && detectionInfo.BaseFramework != old.Framework {
			frameworks = append(frameworks, detectionInfo.BaseFramework)
		}
		detection.Frameworks, _ = json.Marshal(frameworks)
	}

	// Set evidence sources JSON
	if len(detectionInfo.Sources) > 0 {
		detection.EvidenceSources, _ = json.Marshal(detectionInfo.Sources)
	}

	// Set confirmed_at if status is confirmed or verified
	if detectionInfo.Status == "confirmed" || detectionInfo.Status == "verified" {
		detection.ConfirmedAt = &old.CreatedAt
	}

	if !dryRun {
		if err := db.WithContext(ctx).Create(detection).Error; err != nil {
			return fmt.Errorf("failed to create detection: %w", err)
		}
	}
	stats.DetectionsMigrated++

	// Create evidence records from sources in metadata
	evidences := extractEvidences(old, detectionInfo)
	for _, ev := range evidences {
		if !dryRun {
			if err := db.WithContext(ctx).Create(ev).Error; err != nil {
				return fmt.Errorf("failed to create evidence: %w", err)
			}
		}
		stats.EvidencesMigrated++
	}

	fmt.Printf("  OK: workload %s (framework=%s, status=%s, evidences=%d)\n",
		old.WorkloadUID, old.Framework, detectionInfo.Status, len(evidences))

	return nil
}

// DetectionInfo holds extracted detection information
type DetectionInfo struct {
	Status           string
	Confidence       float64
	FrameworkLayer   string
	WrapperFramework string
	BaseFramework    string
	Sources          []string
}

func extractDetectionInfo(old *OldAiWorkloadMetadata) *DetectionInfo {
	info := &DetectionInfo{
		Status:     "confirmed", // Default to confirmed for existing data
		Confidence: 0.8,         // Default confidence for migrated data
		Sources:    []string{},
	}

	if old.Metadata == nil {
		return info
	}

	// Extract framework_detection info
	if fwDetection, ok := old.Metadata["framework_detection"].(map[string]interface{}); ok {
		if status, ok := fwDetection["status"].(string); ok {
			info.Status = status
		}
		if confidence, ok := fwDetection["confidence"].(float64); ok {
			info.Confidence = confidence
		}
	}

	// Extract framework layer info
	if layer, ok := old.Metadata["framework_layer"].(string); ok {
		info.FrameworkLayer = layer
	}
	if wrapper, ok := old.Metadata["wrapper_framework"].(string); ok {
		info.WrapperFramework = wrapper
	}
	if base, ok := old.Metadata["base_framework"].(string); ok {
		info.BaseFramework = base
	}

	// Extract sources
	if sources, ok := old.Metadata["detection_sources"].([]interface{}); ok {
		for _, s := range sources {
			if src, ok := s.(string); ok {
				info.Sources = append(info.Sources, src)
			}
		}
	}

	// Also check for source in evidence
	if source, ok := old.Metadata["source"].(string); ok {
		if !contains(info.Sources, source) {
			info.Sources = append(info.Sources, source)
		}
	}

	// If no sources found, add "migration" as source
	if len(info.Sources) == 0 {
		info.Sources = []string{"migration"}
	}

	return info
}

func extractEvidences(old *OldAiWorkloadMetadata, info *DetectionInfo) []*NewWorkloadDetectionEvidence {
	var evidences []*NewWorkloadDetectionEvidence
	now := time.Now()

	// Create one evidence record per source
	for _, source := range info.Sources {
		ev := &NewWorkloadDetectionEvidence{
			WorkloadUID:      old.WorkloadUID,
			Source:           source,
			SourceType:       "passive",
			Framework:        old.Framework,
			WorkloadType:     old.Type,
			Confidence:       info.Confidence,
			FrameworkLayer:   info.FrameworkLayer,
			WrapperFramework: info.WrapperFramework,
			BaseFramework:    info.BaseFramework,
			Evidence: map[string]interface{}{
				"migrated_from": "ai_workload_metadata",
				"migrated_at":   now.Format(time.RFC3339),
				"original_id":   old.ID,
			},
			Processed:   true,
			ProcessedAt: &now,
			DetectedAt:  old.CreatedAt,
			CreatedAt:   now,
		}

		// Add source-specific evidence if available
		if source == "wandb" {
			if wandbData, ok := old.Metadata["wandb"].(map[string]interface{}); ok {
				ev.Evidence["wandb"] = wandbData
			}
		}

		evidences = append(evidences, ev)
	}

	// If no evidences created, create a default one
	if len(evidences) == 0 && old.Framework != "" {
		ev := &NewWorkloadDetectionEvidence{
			WorkloadUID:      old.WorkloadUID,
			Source:           "migration",
			SourceType:       "passive",
			Framework:        old.Framework,
			WorkloadType:     old.Type,
			Confidence:       info.Confidence,
			FrameworkLayer:   info.FrameworkLayer,
			WrapperFramework: info.WrapperFramework,
			BaseFramework:    info.BaseFramework,
			Evidence: map[string]interface{}{
				"migrated_from": "ai_workload_metadata",
				"migrated_at":   now.Format(time.RFC3339),
				"original_id":   old.ID,
			},
			Processed:   true,
			ProcessedAt: &now,
			DetectedAt:  old.CreatedAt,
			CreatedAt:   now,
		}
		evidences = append(evidences, ev)
	}

	return evidences
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

