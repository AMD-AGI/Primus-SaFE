package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// GithubWorkflowMetricsFacadeInterface defines the database operation interface for github workflow metrics
type GithubWorkflowMetricsFacadeInterface interface {
	// Create creates a new metric record
	Create(ctx context.Context, metric *model.GithubWorkflowMetrics) error

	// BatchCreate creates multiple metric records
	BatchCreate(ctx context.Context, metrics []*model.GithubWorkflowMetrics) error

	// GetByID retrieves a metric by ID
	GetByID(ctx context.Context, id int64) (*model.GithubWorkflowMetrics, error)

	// List lists metrics with optional filtering
	List(ctx context.Context, filter *GithubWorkflowMetricsFilter) ([]*model.GithubWorkflowMetrics, int64, error)

	// ListByConfig lists metrics for a config within a time range
	ListByConfig(ctx context.Context, configID int64, start, end time.Time, limit int) ([]*model.GithubWorkflowMetrics, error)

	// ListByRun lists metrics for a specific run
	ListByRun(ctx context.Context, runID int64) ([]*model.GithubWorkflowMetrics, error)

	// CountByConfig counts metrics for a config
	CountByConfig(ctx context.Context, configID int64) (int64, error)

	// CountByRun counts metrics for a run
	CountByRun(ctx context.Context, runID int64) (int64, error)

	// Delete deletes a metric by ID
	Delete(ctx context.Context, id int64) error

	// DeleteByRun deletes all metrics for a run
	DeleteByRun(ctx context.Context, runID int64) error

	// DeleteByConfig deletes all metrics for a config
	DeleteByConfig(ctx context.Context, configID int64) error

	// DeleteByTimeRange deletes metrics older than the specified time
	DeleteByTimeRange(ctx context.Context, configID int64, before time.Time) (int64, error)

	// ========== Advanced Query Methods (Phase 4) ==========

	// QueryWithDimensions queries metrics with JSONB dimension filtering
	QueryWithDimensions(ctx context.Context, query *MetricsAdvancedQuery) ([]*model.GithubWorkflowMetrics, int64, error)

	// GetAggregatedMetrics returns aggregated metrics by time interval
	GetAggregatedMetrics(ctx context.Context, query *MetricsAggregationQuery) ([]*AggregatedMetricResult, error)

	// GetMetricsSummary returns summary statistics for a config
	GetMetricsSummary(ctx context.Context, configID int64, start, end *time.Time) (*MetricsSummary, error)

	// GetMetricsTrends returns time-series trends for specified metrics
	GetMetricsTrends(ctx context.Context, query *MetricsTrendsQuery) (*MetricsTrendsResult, error)

	// GetDistinctDimensionValues returns distinct values for a dimension field
	GetDistinctDimensionValues(ctx context.Context, configID int64, dimensionKey string, start, end *time.Time) ([]string, error)

	// GetAvailableDimensions returns all available dimension keys for a config
	GetAvailableDimensions(ctx context.Context, configID int64) ([]string, error)

	// GetAvailableMetricFields returns all available metric field keys for a config
	GetAvailableMetricFields(ctx context.Context, configID int64) ([]string, error)

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) GithubWorkflowMetricsFacadeInterface
}

// GithubWorkflowMetricsFilter defines filter options for listing metrics
type GithubWorkflowMetricsFilter struct {
	ConfigID   int64
	RunID      int64
	SchemaID   int64
	SourceFile string
	Start      *time.Time
	End        *time.Time
	Offset     int
	Limit      int
}

// MetricsAdvancedQuery defines advanced query options with dimension filtering
type MetricsAdvancedQuery struct {
	ConfigID      int64                  // Required: config ID
	Start         *time.Time             // Start time filter
	End           *time.Time             // End time filter
	Dimensions    map[string]interface{} // JSONB dimension filters (key=value)
	MetricFilters map[string]interface{} // JSONB metric filters (key op value)
	SortBy        string                 // Sort field: timestamp, or metric field
	SortOrder     string                 // asc or desc
	Offset        int
	Limit         int
}

// MetricsAggregationQuery defines query options for aggregation
type MetricsAggregationQuery struct {
	ConfigID    int64                  // Required: config ID
	Start       *time.Time             // Start time
	End         *time.Time             // End time
	Dimensions  map[string]interface{} // Filter by dimensions
	GroupBy     []string               // Dimension fields to group by
	MetricField string                 // Metric field to aggregate
	AggFunc     string                 // Aggregation function: avg, sum, min, max, count
	Interval    string                 // Time interval: 1h, 6h, 1d, 1w
}

// AggregatedMetricResult represents an aggregated metric result
type AggregatedMetricResult struct {
	Timestamp     time.Time              `json:"timestamp"`
	Dimensions    map[string]interface{} `json:"dimensions,omitempty"`
	Value         float64                `json:"value"`
	Count         int64                  `json:"count"`
	Min           float64                `json:"min,omitempty"`
	Max           float64                `json:"max,omitempty"`
	MetricField   string                 `json:"metric_field"`
	AggregateFunc string                 `json:"aggregate_func"`
}

// MetricsSummary provides summary statistics for metrics
type MetricsSummary struct {
	ConfigID          int64                  `json:"config_id"`
	TotalRecords      int64                  `json:"total_records"`
	TotalRuns         int64                  `json:"total_runs"`
	FirstTimestamp    *time.Time             `json:"first_timestamp,omitempty"`
	LastTimestamp     *time.Time             `json:"last_timestamp,omitempty"`
	TimeRangeDays     float64                `json:"time_range_days"`
	UniqueSourceFiles int64                  `json:"unique_source_files"`
	MetricStats       map[string]MetricStat  `json:"metric_stats"`
	DimensionCounts   map[string]int64       `json:"dimension_counts"`
}

// MetricStat provides statistics for a single metric field
type MetricStat struct {
	Field   string   `json:"field"`
	Count   int64    `json:"count"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Avg     *float64 `json:"avg,omitempty"`
	Sum     *float64 `json:"sum,omitempty"`
	StdDev  *float64 `json:"std_dev,omitempty"`
}

// MetricsTrendsQuery defines query for trends data
type MetricsTrendsQuery struct {
	ConfigID     int64                  // Required: config ID
	Start        *time.Time             // Start time
	End          *time.Time             // End time
	Dimensions   map[string]interface{} // Filter by dimensions
	MetricFields []string               // Metric fields to include
	Interval     string                 // Aggregation interval: 1h, 6h, 1d
	GroupBy      []string               // Optional dimension fields to group by
}

// MetricsTrendsResult contains trends data
type MetricsTrendsResult struct {
	Timestamps []time.Time              `json:"timestamps"`
	Series     []MetricSeriesData       `json:"series"`
	Interval   string                   `json:"interval"`
}

// MetricSeriesData represents a single time-series
type MetricSeriesData struct {
	Name       string                 `json:"name"`
	Field      string                 `json:"field"`
	Dimensions map[string]interface{} `json:"dimensions,omitempty"`
	Values     []float64              `json:"values"`
	Counts     []int64                `json:"counts,omitempty"`
}

// GithubWorkflowMetricsFacade implements GithubWorkflowMetricsFacadeInterface
type GithubWorkflowMetricsFacade struct {
	BaseFacade
}

// NewGithubWorkflowMetricsFacade creates a new GithubWorkflowMetricsFacade instance
func NewGithubWorkflowMetricsFacade() GithubWorkflowMetricsFacadeInterface {
	return &GithubWorkflowMetricsFacade{}
}

func (f *GithubWorkflowMetricsFacade) WithCluster(clusterName string) GithubWorkflowMetricsFacadeInterface {
	return &GithubWorkflowMetricsFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Create creates a new metric record
func (f *GithubWorkflowMetricsFacade) Create(ctx context.Context, metric *model.GithubWorkflowMetrics) error {
	now := time.Now()
	if metric.CreatedAt.IsZero() {
		metric.CreatedAt = now
	}
	return f.getDAL().GithubWorkflowMetrics.WithContext(ctx).Create(metric)
}

// BatchCreate creates multiple metric records
func (f *GithubWorkflowMetricsFacade) BatchCreate(ctx context.Context, metrics []*model.GithubWorkflowMetrics) error {
	if len(metrics) == 0 {
		return nil
	}
	now := time.Now()
	for _, m := range metrics {
		if m.CreatedAt.IsZero() {
			m.CreatedAt = now
		}
	}
	return f.getDAL().GithubWorkflowMetrics.WithContext(ctx).CreateInBatches(metrics, 100)
}

// GetByID retrieves a metric by ID
func (f *GithubWorkflowMetricsFacade) GetByID(ctx context.Context, id int64) (*model.GithubWorkflowMetrics, error) {
	q := f.getDAL().GithubWorkflowMetrics
	return q.WithContext(ctx).Where(q.ID.Eq(id)).First()
}

// List lists metrics with optional filtering
func (f *GithubWorkflowMetricsFacade) List(ctx context.Context, filter *GithubWorkflowMetricsFilter) ([]*model.GithubWorkflowMetrics, int64, error) {
	q := f.getDAL().GithubWorkflowMetrics
	query := q.WithContext(ctx)

	if filter != nil {
		if filter.ConfigID > 0 {
			query = query.Where(q.ConfigID.Eq(filter.ConfigID))
		}
		if filter.RunID > 0 {
			query = query.Where(q.RunID.Eq(filter.RunID))
		}
		if filter.SchemaID > 0 {
			query = query.Where(q.SchemaID.Eq(filter.SchemaID))
		}
		if filter.SourceFile != "" {
			query = query.Where(q.SourceFile.Eq(filter.SourceFile))
		}
		if filter.Start != nil {
			query = query.Where(q.Timestamp.Gte(*filter.Start))
		}
		if filter.End != nil {
			query = query.Where(q.Timestamp.Lte(*filter.End))
		}
	}

	total, err := query.Count()
	if err != nil {
		return nil, 0, err
	}

	if filter != nil {
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
	}

	results, err := query.Order(q.Timestamp.Desc()).Find()
	if err != nil {
		return nil, 0, err
	}

	return results, total, nil
}

// ListByConfig lists metrics for a config within a time range
func (f *GithubWorkflowMetricsFacade) ListByConfig(ctx context.Context, configID int64, start, end time.Time, limit int) ([]*model.GithubWorkflowMetrics, error) {
	q := f.getDAL().GithubWorkflowMetrics
	query := q.WithContext(ctx).
		Where(q.ConfigID.Eq(configID)).
		Where(q.Timestamp.Gte(start)).
		Where(q.Timestamp.Lte(end)).
		Order(q.Timestamp.Desc())

	if limit > 0 {
		query = query.Limit(limit)
	}

	return query.Find()
}

// ListByRun lists metrics for a specific run
func (f *GithubWorkflowMetricsFacade) ListByRun(ctx context.Context, runID int64) ([]*model.GithubWorkflowMetrics, error) {
	q := f.getDAL().GithubWorkflowMetrics
	return q.WithContext(ctx).
		Where(q.RunID.Eq(runID)).
		Order(q.Timestamp.Desc()).
		Find()
}

// CountByConfig counts metrics for a config
func (f *GithubWorkflowMetricsFacade) CountByConfig(ctx context.Context, configID int64) (int64, error) {
	q := f.getDAL().GithubWorkflowMetrics
	return q.WithContext(ctx).Where(q.ConfigID.Eq(configID)).Count()
}

// CountByRun counts metrics for a run
func (f *GithubWorkflowMetricsFacade) CountByRun(ctx context.Context, runID int64) (int64, error) {
	q := f.getDAL().GithubWorkflowMetrics
	return q.WithContext(ctx).Where(q.RunID.Eq(runID)).Count()
}

// Delete deletes a metric by ID
func (f *GithubWorkflowMetricsFacade) Delete(ctx context.Context, id int64) error {
	q := f.getDAL().GithubWorkflowMetrics
	_, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	return err
}

// DeleteByRun deletes all metrics for a run
func (f *GithubWorkflowMetricsFacade) DeleteByRun(ctx context.Context, runID int64) error {
	q := f.getDAL().GithubWorkflowMetrics
	_, err := q.WithContext(ctx).Where(q.RunID.Eq(runID)).Delete()
	return err
}

// DeleteByConfig deletes all metrics for a config
func (f *GithubWorkflowMetricsFacade) DeleteByConfig(ctx context.Context, configID int64) error {
	q := f.getDAL().GithubWorkflowMetrics
	_, err := q.WithContext(ctx).Where(q.ConfigID.Eq(configID)).Delete()
	return err
}

// DeleteByTimeRange deletes metrics older than the specified time
func (f *GithubWorkflowMetricsFacade) DeleteByTimeRange(ctx context.Context, configID int64, before time.Time) (int64, error) {
	q := f.getDAL().GithubWorkflowMetrics
	result, err := q.WithContext(ctx).
		Where(q.ConfigID.Eq(configID)).
		Where(q.Timestamp.Lt(before)).
		Delete()
	if err != nil {
		return 0, err
	}
	return result.RowsAffected, nil
}

// ========== Advanced Query Methods Implementation (Phase 4) ==========

// QueryWithDimensions queries metrics with JSONB dimension filtering
func (f *GithubWorkflowMetricsFacade) QueryWithDimensions(ctx context.Context, query *MetricsAdvancedQuery) ([]*model.GithubWorkflowMetrics, int64, error) {
	q := f.getDAL().GithubWorkflowMetrics
	db := q.WithContext(ctx).Where(q.ConfigID.Eq(query.ConfigID))

	// Time range filter
	if query.Start != nil {
		db = db.Where(q.Timestamp.Gte(*query.Start))
	}
	if query.End != nil {
		db = db.Where(q.Timestamp.Lte(*query.End))
	}

	// Get underlying gorm.DB for raw JSONB queries
	gormDB := db.UnderlyingDB()

	// Dimension filtering using JSONB containment
	if len(query.Dimensions) > 0 {
		dimJSON, err := json.Marshal(query.Dimensions)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal dimensions: %w", err)
		}
		gormDB = gormDB.Where("dimensions @> ?", string(dimJSON))
	}

	// Metric filtering using JSONB path queries
	for key, value := range query.MetricFilters {
		switch v := value.(type) {
		case map[string]interface{}:
			// Support for operator-based filters: {"gte": 100, "lte": 500}
			for op, opVal := range v {
				switch op {
				case "gte", ">=":
					gormDB = gormDB.Where(fmt.Sprintf("(metrics->>'%s')::float >= ?", key), opVal)
				case "gt", ">":
					gormDB = gormDB.Where(fmt.Sprintf("(metrics->>'%s')::float > ?", key), opVal)
				case "lte", "<=":
					gormDB = gormDB.Where(fmt.Sprintf("(metrics->>'%s')::float <= ?", key), opVal)
				case "lt", "<":
					gormDB = gormDB.Where(fmt.Sprintf("(metrics->>'%s')::float < ?", key), opVal)
				case "eq", "=":
					gormDB = gormDB.Where(fmt.Sprintf("(metrics->>'%s')::float = ?", key), opVal)
				}
			}
		default:
			// Simple equality filter
			gormDB = gormDB.Where(fmt.Sprintf("metrics->>'%s' = ?", key), fmt.Sprintf("%v", value))
		}
	}

	// Count total
	var total int64
	if err := gormDB.Model(&model.GithubWorkflowMetrics{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Sorting
	sortOrder := "DESC"
	if query.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	if query.SortBy != "" && query.SortBy != "timestamp" {
		// Sort by metric field
		gormDB = gormDB.Order(fmt.Sprintf("(metrics->>'%s')::float %s NULLS LAST", query.SortBy, sortOrder))
	} else {
		gormDB = gormDB.Order(fmt.Sprintf("timestamp %s", sortOrder))
	}

	// Pagination
	if query.Offset > 0 {
		gormDB = gormDB.Offset(query.Offset)
	}
	if query.Limit > 0 {
		gormDB = gormDB.Limit(query.Limit)
	}

	var results []*model.GithubWorkflowMetrics
	if err := gormDB.Find(&results).Error; err != nil {
		return nil, 0, err
	}

	return results, total, nil
}

// GetAggregatedMetrics returns aggregated metrics by time interval
func (f *GithubWorkflowMetricsFacade) GetAggregatedMetrics(ctx context.Context, query *MetricsAggregationQuery) ([]*AggregatedMetricResult, error) {
	db := f.getDAL().GithubWorkflowMetrics.WithContext(ctx).UnderlyingDB()

	// Build time bucket expression based on interval
	timeBucket := f.getTimeBucketSQL(query.Interval)

	// Build dimension filter
	if len(query.Dimensions) > 0 {
		dimJSON, err := json.Marshal(query.Dimensions)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal dimensions: %w", err)
		}
		db = db.Where("dimensions @> ?", string(dimJSON))
	}

	// Build base query
	db = db.Table("github_workflow_metrics").
		Where("config_id = ?", query.ConfigID)

	if query.Start != nil {
		db = db.Where("timestamp >= ?", *query.Start)
	}
	if query.End != nil {
		db = db.Where("timestamp <= ?", *query.End)
	}

	// Build aggregation function
	aggFunc := "AVG"
	switch query.AggFunc {
	case "sum":
		aggFunc = "SUM"
	case "min":
		aggFunc = "MIN"
	case "max":
		aggFunc = "MAX"
	case "count":
		aggFunc = "COUNT"
	}

	// Build SELECT clause
	selectCols := fmt.Sprintf(
		"%s as time_bucket, %s((metrics->>'%s')::float) as value, COUNT(*) as count, MIN((metrics->>'%s')::float) as min_val, MAX((metrics->>'%s')::float) as max_val",
		timeBucket, aggFunc, query.MetricField, query.MetricField, query.MetricField,
	)

	groupByClause := "time_bucket"

	// Add dimension grouping if requested
	for i, dim := range query.GroupBy {
		selectCols += fmt.Sprintf(", dimensions->>'%s' as dim_%d", dim, i)
		groupByClause += fmt.Sprintf(", dim_%d", i)
	}

	db = db.Select(selectCols).
		Group(groupByClause).
		Order("time_bucket ASC")

	rows, err := db.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*AggregatedMetricResult
	for rows.Next() {
		result := &AggregatedMetricResult{
			MetricField:   query.MetricField,
			AggregateFunc: query.AggFunc,
		}

		// Prepare scan destinations
		scanDest := []interface{}{&result.Timestamp, &result.Value, &result.Count, &result.Min, &result.Max}

		// Add dimension destinations
		dimValues := make([]string, len(query.GroupBy))
		for i := range query.GroupBy {
			scanDest = append(scanDest, &dimValues[i])
		}

		if err := rows.Scan(scanDest...); err != nil {
			return nil, err
		}

		// Build dimensions map
		if len(query.GroupBy) > 0 {
			result.Dimensions = make(map[string]interface{})
			for i, dim := range query.GroupBy {
				result.Dimensions[dim] = dimValues[i]
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// GetMetricsSummary returns summary statistics for a config
func (f *GithubWorkflowMetricsFacade) GetMetricsSummary(ctx context.Context, configID int64, start, end *time.Time) (*MetricsSummary, error) {
	db := f.getDAL().GithubWorkflowMetrics.WithContext(ctx).UnderlyingDB()

	// Base query
	baseQuery := db.Table("github_workflow_metrics").Where("config_id = ?", configID)
	if start != nil {
		baseQuery = baseQuery.Where("timestamp >= ?", *start)
	}
	if end != nil {
		baseQuery = baseQuery.Where("timestamp <= ?", *end)
	}

	summary := &MetricsSummary{
		ConfigID:        configID,
		MetricStats:     make(map[string]MetricStat),
		DimensionCounts: make(map[string]int64),
	}

	// Get basic counts
	var basicStats struct {
		TotalRecords      int64
		TotalRuns         int64
		UniqueSourceFiles int64
		FirstTimestamp    *time.Time
		LastTimestamp     *time.Time
	}

	err := baseQuery.Select(`
		COUNT(*) as total_records,
		COUNT(DISTINCT run_id) as total_runs,
		COUNT(DISTINCT source_file) as unique_source_files,
		MIN(timestamp) as first_timestamp,
		MAX(timestamp) as last_timestamp
	`).Scan(&basicStats).Error

	if err != nil {
		return nil, err
	}

	summary.TotalRecords = basicStats.TotalRecords
	summary.TotalRuns = basicStats.TotalRuns
	summary.UniqueSourceFiles = basicStats.UniqueSourceFiles
	summary.FirstTimestamp = basicStats.FirstTimestamp
	summary.LastTimestamp = basicStats.LastTimestamp

	if summary.FirstTimestamp != nil && summary.LastTimestamp != nil {
		summary.TimeRangeDays = summary.LastTimestamp.Sub(*summary.FirstTimestamp).Hours() / 24
	}

	// Get metric field statistics - need to get distinct metric fields first
	metricFields, err := f.GetAvailableMetricFields(ctx, configID)
	if err != nil {
		return nil, err
	}

	for _, field := range metricFields {
		var stat struct {
			Count  int64
			MinVal *float64
			MaxVal *float64
			AvgVal *float64
			SumVal *float64
		}

		query := db.Table("github_workflow_metrics").
			Where("config_id = ?", configID).
			Where(fmt.Sprintf("metrics ? '%s'", field))

		if start != nil {
			query = query.Where("timestamp >= ?", *start)
		}
		if end != nil {
			query = query.Where("timestamp <= ?", *end)
		}

		err := query.Select(fmt.Sprintf(`
			COUNT(*) as count,
			MIN((metrics->>'%s')::float) as min_val,
			MAX((metrics->>'%s')::float) as max_val,
			AVG((metrics->>'%s')::float) as avg_val,
			SUM((metrics->>'%s')::float) as sum_val
		`, field, field, field, field)).Scan(&stat).Error

		if err != nil {
			continue
		}

		summary.MetricStats[field] = MetricStat{
			Field: field,
			Count: stat.Count,
			Min:   stat.MinVal,
			Max:   stat.MaxVal,
			Avg:   stat.AvgVal,
			Sum:   stat.SumVal,
		}
	}

	// Get dimension distinct counts
	dimensions, err := f.GetAvailableDimensions(ctx, configID)
	if err != nil {
		return nil, err
	}

	for _, dim := range dimensions {
		var count int64
		query := db.Table("github_workflow_metrics").
			Where("config_id = ?", configID).
			Where(fmt.Sprintf("dimensions ? '%s'", dim))

		if start != nil {
			query = query.Where("timestamp >= ?", *start)
		}
		if end != nil {
			query = query.Where("timestamp <= ?", *end)
		}

		err := query.Select(fmt.Sprintf("COUNT(DISTINCT dimensions->>'%s')", dim)).Scan(&count).Error
		if err == nil {
			summary.DimensionCounts[dim] = count
		}
	}

	return summary, nil
}

// GetMetricsTrends returns time-series trends for specified metrics
func (f *GithubWorkflowMetricsFacade) GetMetricsTrends(ctx context.Context, query *MetricsTrendsQuery) (*MetricsTrendsResult, error) {
	db := f.getDAL().GithubWorkflowMetrics.WithContext(ctx).UnderlyingDB()

	timeBucket := f.getTimeBucketSQL(query.Interval)

	result := &MetricsTrendsResult{
		Interval: query.Interval,
		Series:   make([]MetricSeriesData, 0),
	}

	// Generate timestamp series first
	baseQuery := db.Table("github_workflow_metrics").
		Where("config_id = ?", query.ConfigID)

	if query.Start != nil {
		baseQuery = baseQuery.Where("timestamp >= ?", *query.Start)
	}
	if query.End != nil {
		baseQuery = baseQuery.Where("timestamp <= ?", *query.End)
	}

	// Dimension filter
	if len(query.Dimensions) > 0 {
		dimJSON, err := json.Marshal(query.Dimensions)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal dimensions: %w", err)
		}
		baseQuery = baseQuery.Where("dimensions @> ?", string(dimJSON))
	}

	// For each metric field, get time series
	for _, metricField := range query.MetricFields {
		if len(query.GroupBy) == 0 {
			// Simple aggregation without grouping
			series, err := f.getMetricTimeSeries(baseQuery, timeBucket, metricField, nil)
			if err != nil {
				continue
			}

			if len(series.Values) > 0 {
				result.Series = append(result.Series, series.MetricSeriesData)
				// Use first series timestamps as reference
				if len(result.Timestamps) == 0 {
					result.Timestamps = series.timestamps
				}
			}
		} else {
			// Get distinct dimension combinations
			dimCombinations, err := f.getDistinctDimensionCombinations(baseQuery, query.GroupBy)
			if err != nil {
				continue
			}

			for _, dims := range dimCombinations {
				series, err := f.getMetricTimeSeries(baseQuery, timeBucket, metricField, dims)
				if err != nil {
					continue
				}

				if len(series.Values) > 0 {
					series.Dimensions = dims
					result.Series = append(result.Series, series.MetricSeriesData)
					if len(result.Timestamps) == 0 {
						result.Timestamps = series.timestamps
					}
				}
			}
		}
	}

	return result, nil
}

// Internal helper for time series extraction
type metricSeriesInternal struct {
	MetricSeriesData
	timestamps []time.Time
}

func (f *GithubWorkflowMetricsFacade) getMetricTimeSeries(baseQuery *gorm.DB, timeBucket string, metricField string, dimensions map[string]interface{}) (metricSeriesInternal, error) {
	series := metricSeriesInternal{
		MetricSeriesData: MetricSeriesData{
			Name:       metricField,
			Field:      metricField,
			Dimensions: dimensions,
		},
	}

	query := baseQuery.Session(&gorm.Session{})
	query = query.Where(fmt.Sprintf("metrics ? '%s'", metricField))

	// Apply dimension filter if provided
	if len(dimensions) > 0 {
		dimJSON, _ := json.Marshal(dimensions)
		query = query.Where("dimensions @> ?", string(dimJSON))
	}

	query = query.Select(fmt.Sprintf(
		"%s as time_bucket, AVG((metrics->>'%s')::float) as value, COUNT(*) as count",
		timeBucket, metricField,
	)).Group("time_bucket").Order("time_bucket ASC")

	rows, err := query.Rows()
	if err != nil {
		return series, err
	}
	defer rows.Close()

	for rows.Next() {
		var ts time.Time
		var val float64
		var count int64
		if err := rows.Scan(&ts, &val, &count); err != nil {
			continue
		}
		series.timestamps = append(series.timestamps, ts)
		series.Values = append(series.Values, val)
		series.Counts = append(series.Counts, count)
	}

	return series, nil
}

func (f *GithubWorkflowMetricsFacade) getDistinctDimensionCombinations(baseQuery *gorm.DB, groupBy []string) ([]map[string]interface{}, error) {
	selectCols := ""
	for i, dim := range groupBy {
		if i > 0 {
			selectCols += ", "
		}
		selectCols += fmt.Sprintf("dimensions->>'%s' as dim_%d", dim, i)
	}

	rows, err := baseQuery.Session(&gorm.Session{}).Select(selectCols).Distinct().Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		values := make([]string, len(groupBy))
		ptrs := make([]interface{}, len(groupBy))
		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			continue
		}

		dims := make(map[string]interface{})
		for i, dim := range groupBy {
			dims[dim] = values[i]
		}
		result = append(result, dims)
	}

	return result, nil
}

// GetDistinctDimensionValues returns distinct values for a dimension field
func (f *GithubWorkflowMetricsFacade) GetDistinctDimensionValues(ctx context.Context, configID int64, dimensionKey string, start, end *time.Time) ([]string, error) {
	db := f.getDAL().GithubWorkflowMetrics.WithContext(ctx).UnderlyingDB()

	query := db.Table("github_workflow_metrics").
		Where("config_id = ?", configID).
		Where(fmt.Sprintf("dimensions ? '%s'", dimensionKey))

	if start != nil {
		query = query.Where("timestamp >= ?", *start)
	}
	if end != nil {
		query = query.Where("timestamp <= ?", *end)
	}

	rows, err := query.Select(fmt.Sprintf("DISTINCT dimensions->>'%s' as value", dimensionKey)).
		Where(fmt.Sprintf("dimensions->>'%s' IS NOT NULL", dimensionKey)).
		Order("value ASC").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var val string
		if err := rows.Scan(&val); err != nil {
			continue
		}
		values = append(values, val)
	}

	return values, nil
}

// GetAvailableDimensions returns all available dimension keys for a config
func (f *GithubWorkflowMetricsFacade) GetAvailableDimensions(ctx context.Context, configID int64) ([]string, error) {
	db := f.getDAL().GithubWorkflowMetrics.WithContext(ctx).UnderlyingDB()

	// Use PostgreSQL jsonb_object_keys to get all dimension keys
	rows, err := db.Table("github_workflow_metrics").
		Select("DISTINCT jsonb_object_keys(dimensions) as key").
		Where("config_id = ?", configID).
		Order("key ASC").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			continue
		}
		keys = append(keys, key)
	}

	return keys, nil
}

// GetAvailableMetricFields returns all available metric field keys for a config
func (f *GithubWorkflowMetricsFacade) GetAvailableMetricFields(ctx context.Context, configID int64) ([]string, error) {
	db := f.getDAL().GithubWorkflowMetrics.WithContext(ctx).UnderlyingDB()

	// Use PostgreSQL jsonb_object_keys to get all metric keys
	rows, err := db.Table("github_workflow_metrics").
		Select("DISTINCT jsonb_object_keys(metrics) as key").
		Where("config_id = ?", configID).
		Order("key ASC").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			continue
		}
		keys = append(keys, key)
	}

	return keys, nil
}

// getTimeBucketSQL returns SQL expression for time bucketing
func (f *GithubWorkflowMetricsFacade) getTimeBucketSQL(interval string) string {
	switch interval {
	case "1h":
		return "date_trunc('hour', timestamp)"
	case "6h":
		return "date_trunc('hour', timestamp) - (EXTRACT(HOUR FROM timestamp)::int % 6) * interval '1 hour'"
	case "1d":
		return "date_trunc('day', timestamp)"
	case "1w":
		return "date_trunc('week', timestamp)"
	default:
		return "date_trunc('day', timestamp)"
	}
}

