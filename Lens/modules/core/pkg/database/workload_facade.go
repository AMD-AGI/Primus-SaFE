// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"gorm.io/gorm"
)

// WorkloadFacadeInterface defines the database operation interface for Workload
type WorkloadFacadeInterface interface {
	// GpuWorkload operations
	GetGpuWorkloadByUid(ctx context.Context, uid string) (*model.GpuWorkload, error)
	GetGpuWorkloadByName(ctx context.Context, name string) (*model.GpuWorkload, error)
	CreateGpuWorkload(ctx context.Context, gpuWorkload *model.GpuWorkload) error
	UpdateGpuWorkload(ctx context.Context, gpuWorkload *model.GpuWorkload) error
	QueryWorkload(ctx context.Context, f *filter.WorkloadFilter) ([]*model.GpuWorkload, int, error)
	GetWorkloadsNamespaceList(ctx context.Context) ([]string, error)
	GetWorkloadKindList(ctx context.Context) ([]string, error)
	GetWorkloadNotEnd(ctx context.Context) ([]*model.GpuWorkload, error)
	ListRunningWorkload(ctx context.Context) ([]*model.GpuWorkload, error)
	ListWorkloadsByUids(ctx context.Context, uids []string) ([]*model.GpuWorkload, error)
	GetNearestWorkloadByPodUid(ctx context.Context, podUid string) (*model.GpuWorkload, error)
	ListTopLevelWorkloadByUids(ctx context.Context, uids []string) ([]*model.GpuWorkload, error)
	ListChildrenWorkloadByParentUid(ctx context.Context, parentUid string) ([]*model.GpuWorkload, error)
	ListWorkloadByLabelValue(ctx context.Context, labelKey, labelValue string) ([]*model.GpuWorkload, error)
	ListWorkloadNotEndByKind(ctx context.Context, kind string) ([]*model.GpuWorkload, error)
	// ListCompletedWorkloadsByKindAndParent lists completed workloads of a specific kind under a parent
	// Used for scanning completed EphemeralRunners under an AutoscalingRunnerSet
	ListCompletedWorkloadsByKindAndParent(ctx context.Context, kind, parentUID string, since time.Time, limit int) ([]*model.GpuWorkload, error)
	// ListCompletedWorkloadsByKindAndNamespace lists completed workloads of a specific kind in a namespace
	// A workload is completed if EndAt is not null/zero
	ListCompletedWorkloadsByKindAndNamespace(ctx context.Context, kind, namespace string, since time.Time, limit int) ([]*model.GpuWorkload, error)
	// ListActiveTopLevelWorkloads queries top-level workloads active within a time range
	// A workload is considered active if: CreatedAt <= endTime AND (EndAt is null/zero OR EndAt >= startTime)
	// Only returns workloads where ParentUID == "" (top-level)
	// If namespace is empty, returns workloads from all namespaces
	ListActiveTopLevelWorkloads(ctx context.Context, startTime, endTime time.Time, namespace string) ([]*model.GpuWorkload, error)

	// GpuWorkloadSnapshot operations
	CreateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *model.GpuWorkloadSnapshot) error
	UpdateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *model.GpuWorkloadSnapshot) error
	GetLatestGpuWorkloadSnapshotByUid(ctx context.Context, uid string, resourceVersion int) (*model.GpuWorkloadSnapshot, error)

	// WorkloadPodReference operations
	CreateWorkloadPodReference(ctx context.Context, workloadUid, podUid string) error
	ListWorkloadPodReferencesByPodUids(ctx context.Context, podUids []string) ([]*model.WorkloadPodReference, error)
	ListWorkloadPodReferenceByWorkloadUid(ctx context.Context, workloadUid string) ([]*model.WorkloadPodReference, error)
	GetAllWorkloadPodReferences(ctx context.Context) ([]*model.WorkloadPodReference, error)
	// ListWorkloadUidsByPodUids returns workload UIDs for the given pod UIDs
	// This is used to find workloads from their associated pods
	ListWorkloadUidsByPodUids(ctx context.Context, podUids []string) ([]string, error)

	// WorkloadEvent operations
	GetWorkloadEventByWorkloadUidAndNearestWorkloadIdAndType(ctx context.Context, workloadUid, nearestWorkloadId, typ string) (*model.WorkloadEvent, error)
	CreateWorkloadEvent(ctx context.Context, workloadEvent *model.WorkloadEvent) error
	UpdateWorkloadEvent(ctx context.Context, workloadEvent *model.WorkloadEvent) error
	GetLatestEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*model.WorkloadEvent, error)
	GetLatestOtherWorkloadEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*model.WorkloadEvent, error)

	// WithCluster method
	WithCluster(clusterName string) WorkloadFacadeInterface
}

// WorkloadFacade implements WorkloadFacadeInterface
type WorkloadFacade struct {
	BaseFacade
}

// NewWorkloadFacade creates a new WorkloadFacade instance
func NewWorkloadFacade() WorkloadFacadeInterface {
	return &WorkloadFacade{}
}

func (f *WorkloadFacade) WithCluster(clusterName string) WorkloadFacadeInterface {
	return &WorkloadFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// GpuWorkload operation implementations
func (f *WorkloadFacade) GetGpuWorkloadByUid(ctx context.Context, uid string) (*model.GpuWorkload, error) {
	q := f.getDAL().GpuWorkload
	result, err := q.WithContext(ctx).Where(q.UID.Eq(uid)).First()
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

func (f *WorkloadFacade) GetGpuWorkloadByName(ctx context.Context, name string) (*model.GpuWorkload, error) {
	db := f.getDB()
	if db == nil {
		return nil, nil
	}
	var result model.GpuWorkload
	// Prefer active workloads (end_at is null or zero) over completed ones.
	// Within the same active/inactive group, pick the most recently created.
	err := db.WithContext(ctx).
		Where("name = ?", name).
		Order("CASE WHEN end_at IS NULL OR end_at = '0001-01-01 00:00:00+00' THEN 0 ELSE 1 END ASC, created_at DESC").
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if result.ID == 0 {
		return nil, nil
	}
	return &result, nil
}

func (f *WorkloadFacade) CreateGpuWorkload(ctx context.Context, gpuWorkload *model.GpuWorkload) error {
	return f.getDAL().GpuWorkload.WithContext(ctx).Create(gpuWorkload)
}

func (f *WorkloadFacade) UpdateGpuWorkload(ctx context.Context, gpuWorkload *model.GpuWorkload) error {
	return f.getDAL().GpuWorkload.WithContext(ctx).Save(gpuWorkload)
}

func (f *WorkloadFacade) QueryWorkload(ctx context.Context, filter *filter.WorkloadFilter) ([]*model.GpuWorkload, int, error) {
	q := f.getDAL().GpuWorkload
	query := q.WithContext(ctx)
	if filter.Kind != nil {
		query = query.Where(q.Kind.Eq(*filter.Kind))
	}
	if filter.Namespace != nil {
		query = query.Where(q.Namespace.Eq(*filter.Namespace))
	}
	if filter.Name != nil {
		query = query.Where(q.Name.Like(fmt.Sprintf("%%%s%%", *filter.Name)))
	}
	if filter.Uid != nil {
		query = query.Where(q.UID.Eq(*filter.Uid))
	}
	if filter.ParentUid != nil {
		query = query.Where(q.ParentUID.Eq(*filter.ParentUid))
	} else {
		query = query.Where(q.ParentUID.Eq(""))
	}
	if filter.Status != nil {
		query = query.Where(q.Status.Eq(*filter.Status))
	}
	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}
	gormDB := query.UnderlyingDB()
	if filter.OrderBy != "" {
		order := filter.Order
		if order == "" {
			order = "DESC"
		}
		gormDB = gormDB.Order(fmt.Sprintf("%s %s", filter.OrderBy, order))
	} else {
		gormDB = gormDB.Order("created_at desc")
	}

	if filter.Limit > 0 {
		gormDB = gormDB.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		gormDB = gormDB.Offset(filter.Offset)
	}
	var workloads []*model.GpuWorkload
	err = gormDB.Find(&workloads).Error
	if err != nil {
		return nil, 0, err
	}
	return workloads, int(count), nil
}

func (f *WorkloadFacade) GetWorkloadsNamespaceList(ctx context.Context) ([]string, error) {
	q := f.getDAL().GpuWorkload
	var namespaces []string
	err := q.WithContext(ctx).
		Distinct(q.Namespace).
		Pluck(q.Namespace, &namespaces)
	if err != nil {
		return nil, err
	}
	return namespaces, nil
}

func (f *WorkloadFacade) GetWorkloadKindList(ctx context.Context) ([]string, error) {
	q := f.getDAL().GpuWorkload
	var kinds []string
	err := q.WithContext(ctx).
		Distinct(q.Kind).
		Where(q.ParentUID.Eq("")).
		Pluck(q.Kind, &kinds)
	if err != nil {
		return nil, err
	}
	return kinds, nil
}

func (f *WorkloadFacade) GetWorkloadNotEnd(ctx context.Context) ([]*model.GpuWorkload, error) {
	q := f.getDAL().GpuWorkload
	result, err := q.WithContext(ctx).Where(q.EndAt.IsNull()).Or(q.EndAt.Eq(time.Time{})).Find()
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (f *WorkloadFacade) ListRunningWorkload(ctx context.Context) ([]*model.GpuWorkload, error) {
	q := f.getDAL().GpuWorkload
	workloads, err := q.WithContext(ctx).Where(q.Status.In(metadata.WorkloadStatusRunning)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return workloads, nil
}

func (f *WorkloadFacade) ListWorkloadsByUids(ctx context.Context, uids []string) ([]*model.GpuWorkload, error) {
	q := f.getDAL().GpuWorkload
	results, err := q.WithContext(ctx).Where(q.UID.In(uids...)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

func (f *WorkloadFacade) GetNearestWorkloadByPodUid(ctx context.Context, podUid string) (*model.GpuWorkload, error) {
	workloadRefs, err := f.ListWorkloadPodReferencesByPodUids(ctx, []string{podUid})
	if err != nil {
		return nil, err
	}
	if len(workloadRefs) == 0 {
		return nil, nil
	}
	workloadUids := []string{}
	for _, ref := range workloadRefs {
		workloadUids = append(workloadUids, ref.WorkloadUID)
	}
	workloads, err := f.ListWorkloadsByUids(ctx, workloadUids)
	if err != nil {
		return nil, err
	}
	leaves := findLeafWorkloads(workloads)
	if len(leaves) == 0 {
		return nil, nil
	}
	return leaves[0], nil
}

func (f *WorkloadFacade) ListTopLevelWorkloadByUids(ctx context.Context, uids []string) ([]*model.GpuWorkload, error) {
	q := f.getDAL().GpuWorkload
	workloads, err := q.WithContext(ctx).Where(q.UID.In(uids...)).Where(q.ParentUID.Eq("")).Find()
	if err != nil {
		return nil, err
	}
	return workloads, nil
}

func (f *WorkloadFacade) ListChildrenWorkloadByParentUid(ctx context.Context, parentUid string) ([]*model.GpuWorkload, error) {
	q := f.getDAL().GpuWorkload
	results, err := q.WithContext(ctx).Where(q.ParentUID.Eq(parentUid)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

func (f *WorkloadFacade) ListWorkloadByLabelValue(ctx context.Context, labelKey, labelValue string) ([]*model.GpuWorkload, error) {
	result := []*model.GpuWorkload{}
	err := f.getDB().Raw(fmt.Sprintf(`SELECT * FROM gpu_workload WHERE labels @> '{"%s": "%s"}'`, labelKey, labelValue)).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (f *WorkloadFacade) ListWorkloadNotEndByKind(ctx context.Context, kind string) ([]*model.GpuWorkload, error) {
	q := f.getDAL().GpuWorkload
	results, err := q.WithContext(ctx).
		Where(q.Kind.Eq(kind)).
		Where(q.EndAt.IsNull()).
		Or(q.EndAt.Eq(time.Time{})).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

// ListCompletedWorkloadsByKindAndParent lists completed workloads of a specific kind under a parent
func (f *WorkloadFacade) ListCompletedWorkloadsByKindAndParent(ctx context.Context, kind, parentUID string, since time.Time, limit int) ([]*model.GpuWorkload, error) {
	db := f.getDB()
	if db == nil {
		return nil, nil
	}

	query := db.WithContext(ctx).Model(&model.GpuWorkload{}).
		Where("kind = ?", kind).
		Where("parent_uid = ?", parentUID).
		Where("end_at IS NOT NULL AND end_at != ?", time.Time{})

	if !since.IsZero() {
		query = query.Where("end_at >= ?", since)
	}

	query = query.Order("end_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	var workloads []*model.GpuWorkload
	if err := query.Find(&workloads).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.GpuWorkload{}, nil
		}
		return nil, err
	}

	return workloads, nil
}

// ListCompletedWorkloadsByKindAndNamespace lists completed workloads of a specific kind in a namespace
func (f *WorkloadFacade) ListCompletedWorkloadsByKindAndNamespace(ctx context.Context, kind, namespace string, since time.Time, limit int) ([]*model.GpuWorkload, error) {
	db := f.getDB()
	if db == nil {
		return nil, nil
	}

	query := db.WithContext(ctx).Model(&model.GpuWorkload{}).
		Where("kind = ?", kind).
		Where("namespace = ?", namespace).
		Where("end_at IS NOT NULL AND end_at != ?", time.Time{})

	if !since.IsZero() {
		query = query.Where("end_at >= ?", since)
	}

	query = query.Order("end_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	var workloads []*model.GpuWorkload
	if err := query.Find(&workloads).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.GpuWorkload{}, nil
		}
		return nil, err
	}

	return workloads, nil
}

// ListActiveTopLevelWorkloads queries top-level workloads active within a time range
func (f *WorkloadFacade) ListActiveTopLevelWorkloads(ctx context.Context, startTime, endTime time.Time, namespace string) ([]*model.GpuWorkload, error) {
	db := f.getDB()
	if db == nil {
		return nil, nil
	}

	query := db.WithContext(ctx).Model(&model.GpuWorkload{}).
		Where("parent_uid = ?", "").                                                    // Top-level workloads only
		Where("created_at <= ?", endTime).                                              // Created before or at end time
		Where("(end_at IS NULL OR end_at = ? OR end_at >= ?)", time.Time{}, startTime). // Still active or ended after start time
		Where("(deleted_at IS NULL OR deleted_at >= ?)", endTime)                       // Not deleted before or at end time (use >= to include boundary case)

	if namespace != "" {
		query = query.Where("namespace = ?", namespace)
	}

	var workloads []*model.GpuWorkload
	if err := query.Find(&workloads).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.GpuWorkload{}, nil
		}
		return nil, err
	}

	return workloads, nil
}

// GpuWorkloadSnapshot operation implementations
func (f *WorkloadFacade) CreateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *model.GpuWorkloadSnapshot) error {
	return f.getDAL().GpuWorkloadSnapshot.WithContext(ctx).Create(gpuWorkloadSnapshot)
}

func (f *WorkloadFacade) UpdateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *model.GpuWorkloadSnapshot) error {
	return f.getDAL().GpuWorkloadSnapshot.WithContext(ctx).Save(gpuWorkloadSnapshot)
}

func (f *WorkloadFacade) GetLatestGpuWorkloadSnapshotByUid(ctx context.Context, uid string, resourceVersion int) (*model.GpuWorkloadSnapshot, error) {
	q := f.getDAL().GpuWorkloadSnapshot
	result, err := q.
		WithContext(ctx).
		Where(q.UID.Eq(uid)).
		Where(q.ResourceVersion.Lt(int32(resourceVersion))).
		Order(q.ResourceVersion.Desc()).
		First()
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

// WorkloadPodReference operation implementations
func (f *WorkloadFacade) CreateWorkloadPodReference(ctx context.Context, workloadUid, podUid string) error {
	// Check if reference already exists to prevent duplicates
	q := f.getDAL().WorkloadPodReference
	existingRef, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUid)).
		Where(q.PodUID.Eq(podUid)).
		First()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// If reference already exists (either found or ID > 0), skip creation
	if existingRef != nil && existingRef.ID > 0 {
		return nil
	}

	ref := &model.WorkloadPodReference{
		WorkloadUID: workloadUid,
		PodUID:      podUid,
		CreatedAt:   time.Now(),
	}
	return f.getDAL().WorkloadPodReference.WithContext(ctx).Create(ref)
}

func (f *WorkloadFacade) ListWorkloadPodReferencesByPodUids(ctx context.Context, podUids []string) ([]*model.WorkloadPodReference, error) {
	q := f.getDAL().WorkloadPodReference
	refs, err := q.WithContext(ctx).Where(q.PodUID.In(podUids...)).Find()
	if err != nil {
		return nil, err
	}
	return refs, nil
}

func (f *WorkloadFacade) ListWorkloadPodReferenceByWorkloadUid(ctx context.Context, workloadUid string) ([]*model.WorkloadPodReference, error) {
	q := f.getDAL().WorkloadPodReference
	refs, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUid)).Find()
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// GetAllWorkloadPodReferences retrieves all workload-pod references for cache building
func (f *WorkloadFacade) GetAllWorkloadPodReferences(ctx context.Context) ([]*model.WorkloadPodReference, error) {
	q := f.getDAL().WorkloadPodReference
	refs, err := q.WithContext(ctx).Find()
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// ListWorkloadUidsByPodUids returns distinct workload UIDs for the given pod UIDs
func (f *WorkloadFacade) ListWorkloadUidsByPodUids(ctx context.Context, podUids []string) ([]string, error) {
	if len(podUids) == 0 {
		return []string{}, nil
	}

	q := f.getDAL().WorkloadPodReference
	refs, err := q.WithContext(ctx).Where(q.PodUID.In(podUids...)).Find()
	if err != nil {
		return nil, err
	}

	// Deduplicate workload UIDs
	uidSet := make(map[string]struct{})
	for _, ref := range refs {
		uidSet[ref.WorkloadUID] = struct{}{}
	}

	result := make([]string, 0, len(uidSet))
	for uid := range uidSet {
		result = append(result, uid)
	}

	return result, nil
}

// WorkloadEvent operation implementations
func (f *WorkloadFacade) GetWorkloadEventByWorkloadUidAndNearestWorkloadIdAndType(ctx context.Context, workloadUid, nearestWorkloadId, typ string) (*model.WorkloadEvent, error) {
	q := f.getDAL().WorkloadEvent
	result, err := q.
		WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUid)).
		Where(q.NearestWorkloadUID.Eq(nearestWorkloadId)).
		Where(q.Type.Eq(typ)).
		First()
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

func (f *WorkloadFacade) CreateWorkloadEvent(ctx context.Context, workloadEvent *model.WorkloadEvent) error {
	return f.getDAL().WorkloadEvent.WithContext(ctx).Create(workloadEvent)
}

func (f *WorkloadFacade) UpdateWorkloadEvent(ctx context.Context, workloadEvent *model.WorkloadEvent) error {
	return f.getDAL().WorkloadEvent.WithContext(ctx).Create(workloadEvent)
}

func (f *WorkloadFacade) GetLatestEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*model.WorkloadEvent, error) {
	q := f.getDAL().WorkloadEvent
	result, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUid)).Where(q.NearestWorkloadUID.Eq(nearestWorkloadId)).Order(q.CreatedAt.Desc()).First()
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

func (f *WorkloadFacade) GetLatestOtherWorkloadEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*model.WorkloadEvent, error) {
	q := f.getDAL().WorkloadEvent
	result, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUid)).Where(q.NearestWorkloadUID.Neq(nearestWorkloadId)).Order(q.CreatedAt.Desc()).First()
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

// findLeafWorkloads is a helper function to find leaf workloads
func findLeafWorkloads(workloads []*model.GpuWorkload) []*model.GpuWorkload {
	parentSet := make(map[string]bool)
	for _, w := range workloads {
		if w.ParentUID != "" {
			parentSet[w.ParentUID] = true
		}
	}

	var leaves []*model.GpuWorkload
	for _, w := range workloads {
		if !parentSet[w.UID] {
			leaves = append(leaves, w)
		}
	}
	return leaves
}
