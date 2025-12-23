package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// WorkloadDetectionEvidenceFacadeInterface defines the database operation interface for detection evidence
type WorkloadDetectionEvidenceFacadeInterface interface {
	// CreateEvidence creates a new detection evidence record
	CreateEvidence(ctx context.Context, evidence *model.WorkloadDetectionEvidence) error

	// UpsertEvidence creates or updates an evidence record
	// Updates are matched by workload_uid + source + framework
	UpsertEvidence(ctx context.Context, evidence *model.WorkloadDetectionEvidence) error

	// BatchCreateEvidence creates multiple evidence records in a single transaction
	BatchCreateEvidence(ctx context.Context, evidences []*model.WorkloadDetectionEvidence) error

	// GetEvidenceByID retrieves an evidence record by ID
	GetEvidenceByID(ctx context.Context, id int64) (*model.WorkloadDetectionEvidence, error)

	// ListEvidenceByWorkload retrieves all evidence records for a workload
	ListEvidenceByWorkload(ctx context.Context, workloadUID string) ([]*model.WorkloadDetectionEvidence, error)

	// ListUnprocessedEvidence retrieves unprocessed evidence for a workload
	ListUnprocessedEvidence(ctx context.Context, workloadUID string) ([]*model.WorkloadDetectionEvidence, error)

	// ListEvidenceBySource retrieves evidence by workload and source
	ListEvidenceBySource(ctx context.Context, workloadUID string, source string) ([]*model.WorkloadDetectionEvidence, error)

	// ListEvidenceBySourceType retrieves evidence by source type (passive/active)
	ListEvidenceBySourceType(ctx context.Context, workloadUID string, sourceType string) ([]*model.WorkloadDetectionEvidence, error)

	// MarkEvidenceProcessed marks multiple evidence records as processed
	MarkEvidenceProcessed(ctx context.Context, evidenceIDs []int64) error

	// MarkEvidenceProcessedByWorkload marks all evidence for a workload as processed
	MarkEvidenceProcessedByWorkload(ctx context.Context, workloadUID string) error

	// CountEvidenceByWorkload counts evidence records for a workload
	CountEvidenceByWorkload(ctx context.Context, workloadUID string) (int64, error)

	// CountUnprocessedEvidence counts unprocessed evidence for a workload
	CountUnprocessedEvidence(ctx context.Context, workloadUID string) (int64, error)

	// GetDistinctSourcesByWorkload gets distinct sources that have contributed evidence for a workload
	GetDistinctSourcesByWorkload(ctx context.Context, workloadUID string) ([]string, error)

	// DeleteEvidenceByWorkload deletes all evidence for a workload
	DeleteEvidenceByWorkload(ctx context.Context, workloadUID string) error

	// DeleteExpiredEvidence deletes evidence records that have expired
	DeleteExpiredEvidence(ctx context.Context) (int64, error)

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) WorkloadDetectionEvidenceFacadeInterface
}

// WorkloadDetectionEvidenceFacade implements WorkloadDetectionEvidenceFacadeInterface
type WorkloadDetectionEvidenceFacade struct {
	BaseFacade
}

// NewWorkloadDetectionEvidenceFacade creates a new WorkloadDetectionEvidenceFacade instance
func NewWorkloadDetectionEvidenceFacade() WorkloadDetectionEvidenceFacadeInterface {
	return &WorkloadDetectionEvidenceFacade{}
}

func (f *WorkloadDetectionEvidenceFacade) WithCluster(clusterName string) WorkloadDetectionEvidenceFacadeInterface {
	return &WorkloadDetectionEvidenceFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// CreateEvidence creates a new detection evidence record
func (f *WorkloadDetectionEvidenceFacade) CreateEvidence(ctx context.Context, evidence *model.WorkloadDetectionEvidence) error {
	if evidence.DetectedAt.IsZero() {
		evidence.DetectedAt = time.Now()
	}
	if evidence.CreatedAt.IsZero() {
		evidence.CreatedAt = time.Now()
	}
	return f.getDAL().WorkloadDetectionEvidence.WithContext(ctx).Create(evidence)
}

// UpsertEvidence creates or updates an evidence record
// Updates are matched by workload_uid + source + framework
func (f *WorkloadDetectionEvidenceFacade) UpsertEvidence(ctx context.Context, evidence *model.WorkloadDetectionEvidence) error {
	if evidence.DetectedAt.IsZero() {
		evidence.DetectedAt = time.Now()
	}
	if evidence.CreatedAt.IsZero() {
		evidence.CreatedAt = time.Now()
	}

	q := f.getDAL().WorkloadDetectionEvidence

	// Try to find existing evidence with same workload_uid + source + framework
	existing, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(evidence.WorkloadUID)).
		Where(q.Source.Eq(evidence.Source)).
		Where(q.Framework.Eq(evidence.Framework)).
		First()

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if existing != nil {
		// Update existing record if new confidence is higher or same
		if evidence.Confidence >= existing.Confidence {
			_, err = q.WithContext(ctx).
				Where(q.ID.Eq(existing.ID)).
				UpdateSimple(
					q.Confidence.Value(evidence.Confidence),
					q.DetectedAt.Value(evidence.DetectedAt),
					q.Evidence.Value(evidence.Evidence),
					q.SourceType.Value(evidence.SourceType),
				)
			return err
		}
		// Skip update if existing has higher confidence
		return nil
	}

	// Create new record
	return q.WithContext(ctx).Create(evidence)
}

// BatchCreateEvidence creates multiple evidence records in a single transaction
func (f *WorkloadDetectionEvidenceFacade) BatchCreateEvidence(ctx context.Context, evidences []*model.WorkloadDetectionEvidence) error {
	if len(evidences) == 0 {
		return nil
	}

	now := time.Now()
	for _, evidence := range evidences {
		if evidence.DetectedAt.IsZero() {
			evidence.DetectedAt = now
		}
		if evidence.CreatedAt.IsZero() {
			evidence.CreatedAt = now
		}
	}

	return f.getDAL().WorkloadDetectionEvidence.WithContext(ctx).CreateInBatches(evidences, 100)
}

// GetEvidenceByID retrieves an evidence record by ID
func (f *WorkloadDetectionEvidenceFacade) GetEvidenceByID(ctx context.Context, id int64) (*model.WorkloadDetectionEvidence, error) {
	q := f.getDAL().WorkloadDetectionEvidence
	result, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// ListEvidenceByWorkload retrieves all evidence records for a workload
func (f *WorkloadDetectionEvidenceFacade) ListEvidenceByWorkload(ctx context.Context, workloadUID string) ([]*model.WorkloadDetectionEvidence, error) {
	q := f.getDAL().WorkloadDetectionEvidence
	results, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Order(q.DetectedAt.Desc()).
		Find()
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ListUnprocessedEvidence retrieves unprocessed evidence for a workload
func (f *WorkloadDetectionEvidenceFacade) ListUnprocessedEvidence(ctx context.Context, workloadUID string) ([]*model.WorkloadDetectionEvidence, error) {
	q := f.getDAL().WorkloadDetectionEvidence
	results, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.Processed.Is(false)).
		Order(q.DetectedAt.Asc()). // Process in order of detection
		Find()
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ListEvidenceBySource retrieves evidence by workload and source
func (f *WorkloadDetectionEvidenceFacade) ListEvidenceBySource(ctx context.Context, workloadUID string, source string) ([]*model.WorkloadDetectionEvidence, error) {
	q := f.getDAL().WorkloadDetectionEvidence
	results, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.Source.Eq(source)).
		Order(q.DetectedAt.Desc()).
		Find()
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ListEvidenceBySourceType retrieves evidence by source type (passive/active)
func (f *WorkloadDetectionEvidenceFacade) ListEvidenceBySourceType(ctx context.Context, workloadUID string, sourceType string) ([]*model.WorkloadDetectionEvidence, error) {
	q := f.getDAL().WorkloadDetectionEvidence
	results, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.SourceType.Eq(sourceType)).
		Order(q.DetectedAt.Desc()).
		Find()
	if err != nil {
		return nil, err
	}
	return results, nil
}

// MarkEvidenceProcessed marks multiple evidence records as processed
func (f *WorkloadDetectionEvidenceFacade) MarkEvidenceProcessed(ctx context.Context, evidenceIDs []int64) error {
	if len(evidenceIDs) == 0 {
		return nil
	}

	q := f.getDAL().WorkloadDetectionEvidence
	now := time.Now()
	_, err := q.WithContext(ctx).
		Where(q.ID.In(evidenceIDs...)).
		UpdateSimple(q.Processed.Value(true), q.ProcessedAt.Value(now))
	return err
}

// MarkEvidenceProcessedByWorkload marks all evidence for a workload as processed
func (f *WorkloadDetectionEvidenceFacade) MarkEvidenceProcessedByWorkload(ctx context.Context, workloadUID string) error {
	q := f.getDAL().WorkloadDetectionEvidence
	now := time.Now()
	_, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.Processed.Is(false)).
		UpdateSimple(q.Processed.Value(true), q.ProcessedAt.Value(now))
	return err
}

// CountEvidenceByWorkload counts evidence records for a workload
func (f *WorkloadDetectionEvidenceFacade) CountEvidenceByWorkload(ctx context.Context, workloadUID string) (int64, error) {
	q := f.getDAL().WorkloadDetectionEvidence
	return q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUID)).Count()
}

// CountUnprocessedEvidence counts unprocessed evidence for a workload
func (f *WorkloadDetectionEvidenceFacade) CountUnprocessedEvidence(ctx context.Context, workloadUID string) (int64, error) {
	q := f.getDAL().WorkloadDetectionEvidence
	return q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.Processed.Is(false)).
		Count()
}

// GetDistinctSourcesByWorkload gets distinct sources that have contributed evidence for a workload
func (f *WorkloadDetectionEvidenceFacade) GetDistinctSourcesByWorkload(ctx context.Context, workloadUID string) ([]string, error) {
	db := f.getDB()

	var sources []string
	err := db.WithContext(ctx).
		Table(model.TableNameWorkloadDetectionEvidence).
		Where("workload_uid = ?", workloadUID).
		Distinct("source").
		Pluck("source", &sources).Error

	if err != nil {
		return nil, err
	}
	return sources, nil
}

// DeleteEvidenceByWorkload deletes all evidence for a workload
func (f *WorkloadDetectionEvidenceFacade) DeleteEvidenceByWorkload(ctx context.Context, workloadUID string) error {
	q := f.getDAL().WorkloadDetectionEvidence
	_, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUID)).Delete()
	return err
}

// DeleteExpiredEvidence deletes evidence records that have expired
func (f *WorkloadDetectionEvidenceFacade) DeleteExpiredEvidence(ctx context.Context) (int64, error) {
	q := f.getDAL().WorkloadDetectionEvidence
	now := time.Now()

	result, err := q.WithContext(ctx).
		Where(q.ExpiresAt.IsNotNull()).
		Where(q.ExpiresAt.Lt(now)).
		Delete()
	if err != nil {
		return 0, err
	}
	return result.RowsAffected, nil
}
