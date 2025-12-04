package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// AiWorkloadMetadataFacadeInterface defines the database operation interface for AI Workload Metadata
type AiWorkloadMetadataFacadeInterface interface {
	// GetAiWorkloadMetadata retrieves workload metadata by workload UID
	GetAiWorkloadMetadata(ctx context.Context, workloadUID string) (*model.AiWorkloadMetadata, error)

	// CreateAiWorkloadMetadata creates a new AI workload metadata record
	CreateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error

	// UpdateAiWorkloadMetadata updates an existing AI workload metadata record
	UpdateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error

	// FindCandidateWorkloads finds candidate workloads for reuse based on image prefix and time window
	FindCandidateWorkloads(ctx context.Context, imagePrefix string, timeWindow time.Time, minConfidence float64, limit int) ([]*model.AiWorkloadMetadata, error)

	// ListAiWorkloadMetadataByUIDs retrieves multiple metadata records by workload UIDs
	ListAiWorkloadMetadataByUIDs(ctx context.Context, workloadUIDs []string) ([]*model.AiWorkloadMetadata, error)

	// DeleteAiWorkloadMetadata deletes workload metadata by workload UID
	DeleteAiWorkloadMetadata(ctx context.Context, workloadUID string) error

	// WithCluster method
	WithCluster(clusterName string) AiWorkloadMetadataFacadeInterface
}

// AiWorkloadMetadataFacade implements AiWorkloadMetadataFacadeInterface
type AiWorkloadMetadataFacade struct {
	BaseFacade
}

// NewAiWorkloadMetadataFacade creates a new AiWorkloadMetadataFacade instance
func NewAiWorkloadMetadataFacade() AiWorkloadMetadataFacadeInterface {
	return &AiWorkloadMetadataFacade{}
}

func (f *AiWorkloadMetadataFacade) WithCluster(clusterName string) AiWorkloadMetadataFacadeInterface {
	return &AiWorkloadMetadataFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// GetAiWorkloadMetadata retrieves workload metadata by workload UID
func (f *AiWorkloadMetadataFacade) GetAiWorkloadMetadata(ctx context.Context, workloadUID string) (*model.AiWorkloadMetadata, error) {
	q := f.getDAL().AiWorkloadMetadata
	result, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUID)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if result.ID == 0 {
		return nil, nil
	}
	return result, nil
}

// CreateAiWorkloadMetadata creates a new AI workload metadata record
// It automatically populates the image_prefix from metadata for reuse matching
func (f *AiWorkloadMetadataFacade) CreateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error {
	// Extract and set image_prefix from workload_signature if available
	f.ensureImagePrefix(metadata)
	return f.getDAL().AiWorkloadMetadata.WithContext(ctx).Create(metadata)
}

// UpdateAiWorkloadMetadata updates an existing AI workload metadata record
// It automatically updates the image_prefix if workload_signature changed
func (f *AiWorkloadMetadataFacade) UpdateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error {
	// Extract and set image_prefix from workload_signature if available
	f.ensureImagePrefix(metadata)
	return f.getDAL().AiWorkloadMetadata.WithContext(ctx).Save(metadata)
}

// ensureImagePrefix extracts and sets image_prefix from workload_signature in metadata
func (f *AiWorkloadMetadataFacade) ensureImagePrefix(metadata *model.AiWorkloadMetadata) {
	if metadata.Metadata == nil {
		return
	}

	// metadata.Metadata is already ExtType (map[string]interface{})
	metadataMap := metadata.Metadata

	if signatureData, ok := metadataMap["workload_signature"]; ok {
		if signature, ok := signatureData.(map[string]interface{}); ok {
			if image, ok := signature["image"].(string); ok && image != "" {
				// Use the existing ExtractImageRepo function from framework package
				// or implement inline to avoid circular dependency
				imagePrefix := extractImageRepo(image)

				// Update metadata using raw SQL to avoid model update
				db := f.getDB()
				db.WithContext(context.Background()).
					Table(model.TableNameAiWorkloadMetadata).
					Where("workload_uid = ?", metadata.WorkloadUID).
					Update("image_prefix", imagePrefix)
			}
		}
	}
}

// extractImageRepo extracts the image repository address (without tag)
// Example: registry.example.com/primus:v1.2.3 -> registry.example.com/primus
func extractImageRepo(image string) string {
	// Find the colon separator for tag
	for i := len(image) - 1; i >= 0; i-- {
		if image[i] == ':' {
			return image[:i]
		}
		// Stop at first slash from the end (for images without tag)
		if image[i] == '/' {
			break
		}
	}
	return image
}

// FindCandidateWorkloads finds candidate workloads for reuse
// This method queries workloads with:
// - Same image prefix (extracted from image name without tag)
// - Created within the time window
// - Status is 'verified' or 'confirmed'
// - Confidence >= minConfidence
func (f *AiWorkloadMetadataFacade) FindCandidateWorkloads(
	ctx context.Context,
	imagePrefix string,
	timeWindow time.Time,
	minConfidence float64,
	limit int,
) ([]*model.AiWorkloadMetadata, error) {
	db := f.getDB()

	var results []*model.AiWorkloadMetadata

	// Build the query with JSONB operations
	// Note: This uses PostgreSQL-specific JSONB operators
	err := db.WithContext(ctx).
		Table(model.TableNameAiWorkloadMetadata).
		Where("image_prefix = ?", imagePrefix).
		Where("created_at > ?", timeWindow).
		Where("(metadata->'framework_detection'->>'status')::text IN ?", []string{"verified", "confirmed"}).
		Where("(metadata->'framework_detection'->>'confidence')::float >= ?", minConfidence).
		Order("created_at DESC").
		Limit(limit).
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListAiWorkloadMetadataByUIDs retrieves multiple metadata records by workload UIDs
func (f *AiWorkloadMetadataFacade) ListAiWorkloadMetadataByUIDs(ctx context.Context, workloadUIDs []string) ([]*model.AiWorkloadMetadata, error) {
	if len(workloadUIDs) == 0 {
		return []*model.AiWorkloadMetadata{}, nil
	}

	q := f.getDAL().AiWorkloadMetadata
	results, err := q.WithContext(ctx).Where(q.WorkloadUID.In(workloadUIDs...)).Find()
	if err != nil {
		return nil, err
	}

	return results, nil
}

// DeleteAiWorkloadMetadata deletes workload metadata by workload UID
func (f *AiWorkloadMetadataFacade) DeleteAiWorkloadMetadata(ctx context.Context, workloadUID string) error {
	q := f.getDAL().AiWorkloadMetadata
	_, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUID)).Delete()
	return err
}
