package callbacks

import (
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const (
	tracingSpanKey      = "_tracing_span"
	tracingStartTimeKey = "_tracing_start_time"
)

// beforeQuery is called before query execution
func beforeQuery(db *gorm.DB) {
	startSpan(db, "gorm:query")
}

// afterQuery is called after query execution
func afterQuery(db *gorm.DB) {
	finishSpan(db, "SELECT")
}

// beforeCreate is called before create execution
func beforeCreate(db *gorm.DB) {
	startSpan(db, "gorm:create")
}

// afterCreate is called after create execution
func afterCreate(db *gorm.DB) {
	finishSpan(db, "INSERT")
}

// beforeUpdate is called before update execution
func beforeUpdate(db *gorm.DB) {
	startSpan(db, "gorm:update")
}

// afterUpdate is called after update execution
func afterUpdate(db *gorm.DB) {
	finishSpan(db, "UPDATE")
}

// beforeDelete is called before delete execution
func beforeDelete(db *gorm.DB) {
	startSpan(db, "gorm:delete")
}

// afterDelete is called after delete execution
func afterDelete(db *gorm.DB) {
	finishSpan(db, "DELETE")
}

// startSpan creates a new tracing span for the database operation
func startSpan(db *gorm.DB, operationName string) {
	if db.Statement == nil || db.Statement.Context == nil {
		return
	}

	// Record start time for duration calculation
	startTime := time.Now()

	ctx := db.Statement.Context
	span, newCtx := trace.StartSpanFromContext(ctx, operationName,
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
	)

	// Set database attributes
	span.SetAttributes(
		semconv.DBSystemPostgreSQL,
	)

	// Store span and start time in db instance for later retrieval
	db.InstanceSet(tracingSpanKey, span)
	db.InstanceSet(tracingStartTimeKey, startTime)

	// Update context with new span
	db.Statement.Context = newCtx
}

// finishSpan completes the tracing span and adds relevant tags
func finishSpan(db *gorm.DB, sqlType string) {
	spanInterface, exists := db.InstanceGet(tracingSpanKey)
	if !exists {
		return
	}

	span, ok := spanInterface.(oteltrace.Span)
	if !ok {
		return
	}

	// Calculate duration if start time exists
	var duration time.Duration
	if startTimeInterface, exists := db.InstanceGet(tracingStartTimeKey); exists {
		if startTime, ok := startTimeInterface.(time.Time); ok {
			duration = time.Since(startTime)
		}
	}

	// Add database operation details
	attrs := []attribute.KeyValue{
		attribute.String("db.type", "postgres"),
		attribute.String("db.sql_type", sqlType),
	}

	// Add duration attributes if calculated
	if duration > 0 {
		attrs = append(attrs,
			attribute.Float64("db.duration_ms", float64(duration.Milliseconds())),
			attribute.Int64("db.duration_ns", duration.Nanoseconds()),
		)
	}

	if db.Statement != nil {
		// Add table name
		if db.Statement.Table != "" {
			attrs = append(attrs, semconv.DBSQLTable(db.Statement.Table))
		}

		// Add SQL statement (truncated for large queries)
		sql := db.Statement.SQL.String()
		if len(sql) > 500 {
			sql = sql[:500] + "..."
		}
		if sql != "" {
			attrs = append(attrs, semconv.DBStatement(sql))
		}

		// Add rows affected
		attrs = append(attrs, attribute.Int64("db.rows_affected", db.Statement.RowsAffected))
	}

	span.SetAttributes(attrs...)

	// Mark error if exists
	if db.Error != nil && db.Error != gorm.ErrRecordNotFound {
		span.RecordError(db.Error)
		span.SetStatus(codes.Error, db.Error.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	// Finish the span
	trace.FinishSpan(span)
}

// RegisterTracingCallbacks registers all tracing callbacks for GORM
func RegisterTracingCallbacks(db *gorm.DB) error {
	// Register before callbacks
	if err := db.Callback().Query().Before("gorm:query").Register("tracing:before_query", beforeQuery); err != nil {
		return fmt.Errorf("failed to register tracing:before_query: %w", err)
	}
	if err := db.Callback().Create().Before("gorm:create").Register("tracing:before_create", beforeCreate); err != nil {
		return fmt.Errorf("failed to register tracing:before_create: %w", err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("tracing:before_update", beforeUpdate); err != nil {
		return fmt.Errorf("failed to register tracing:before_update: %w", err)
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("tracing:before_delete", beforeDelete); err != nil {
		return fmt.Errorf("failed to register tracing:before_delete: %w", err)
	}

	// Register after callbacks
	if err := db.Callback().Query().After("gorm:query").Register("tracing:after_query", afterQuery); err != nil {
		return fmt.Errorf("failed to register tracing:after_query: %w", err)
	}
	if err := db.Callback().Create().After("gorm:create").Register("tracing:after_create", afterCreate); err != nil {
		return fmt.Errorf("failed to register tracing:after_create: %w", err)
	}
	if err := db.Callback().Update().After("gorm:update").Register("tracing:after_update", afterUpdate); err != nil {
		return fmt.Errorf("failed to register tracing:after_update: %w", err)
	}
	if err := db.Callback().Delete().After("gorm:delete").Register("tracing:after_delete", afterDelete); err != nil {
		return fmt.Errorf("failed to register tracing:after_delete: %w", err)
	}

	return nil
}
