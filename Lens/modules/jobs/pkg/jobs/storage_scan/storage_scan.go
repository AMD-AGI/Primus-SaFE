// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package storage_scan

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type StorageScanJob struct {
}

func (s *StorageScanJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	// Create main trace span
	span, ctx := trace.StartSpanFromContext(ctx, "storage_scan_job.Run")
	defer trace.FinishSpan(span)

	// Record total job start time
	jobStartTime := time.Now()

	stats := common.NewExecutionStats()

	span.SetAttributes(
		attribute.String("job.name", "storage_scan"),
		attribute.String("cluster.name", clientsets.GetClusterManager().GetCurrentClusterName()),
	)

	scanner := &Scanner{Targets: []ClusterTarget{
		{
			Name:       "K8S",
			ClientSets: clientSets,
			Extra:      nil,
		},
	}}

	// Execute storage scan
	scanSpan, scanCtx := trace.StartSpanFromContext(ctx, "scanStorageBackends")
	scanSpan.SetAttributes(attribute.Int("targets_count", len(scanner.Targets)))

	scanStart := time.Now()
	result, err := scanner.Run(scanCtx)
	stats.QueryDuration = time.Since(scanStart).Seconds()
	duration := time.Since(scanStart)

	if err != nil {
		scanSpan.RecordError(err)
		scanSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		scanSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(scanSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to scan storage backends")
		return stats, err
	}

	// Count scan results
	totalBackendItems := 0
	for _, report := range result {
		totalBackendItems += len(report.BackendItems)
	}

	scanSpan.SetAttributes(
		attribute.Int("reports_count", len(result)),
		attribute.Int("backend_items_count", totalBackendItems),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	scanSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(scanSpan)

	// Process scan results
	processSpan, processCtx := trace.StartSpanFromContext(ctx, "processStorageItems")
	processSpan.SetAttributes(attribute.Int("items_count", totalBackendItems))

	processStart := time.Now()
	for _, report := range result {
		for _, item := range report.BackendItems {
			itemSpan, itemCtx := trace.StartSpanFromContext(processCtx, "processStorageItem")
			itemSpan.SetAttributes(
				attribute.String("storage.name", item.BackendName),
				attribute.String("storage.kind", string(item.BackendKind)),
				attribute.String("storage.health", string(item.Health)),
			)

			dbItem := &dbModel.Storage{
				Name: item.BackendName,
				Kind: string(item.BackendKind),
				Config: map[string]interface{}{
					"meta_secret": item.MetaSecret,
				},
				Source:    "scan",
				Status:    string(item.Health),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			// Check if storage already exists
			getSpan, getCtx := trace.StartSpanFromContext(itemCtx, "getStorageByKindAndName")
			getSpan.SetAttributes(
				attribute.String("storage.kind", dbItem.Kind),
				attribute.String("storage.name", dbItem.Name),
			)

			getStart := time.Now()
			existDbItem, err := database.GetFacade().GetStorage().GetStorageByKindAndName(getCtx, dbItem.Kind, dbItem.Name)
			getDuration := time.Since(getStart)

			if err != nil {
				getSpan.RecordError(err)
				getSpan.SetAttributes(
					attribute.String("error.message", err.Error()),
					attribute.Float64("duration_ms", float64(getDuration.Milliseconds())),
				)
				getSpan.SetStatus(codes.Error, err.Error())
				trace.FinishSpan(getSpan)

				itemSpan.RecordError(err)
				itemSpan.SetAttributes(attribute.String("error.message", err.Error()))
				itemSpan.SetStatus(codes.Error, "Failed to get storage from database")
				trace.FinishSpan(itemSpan)

				stats.ErrorCount++
				log.Errorf("Fail to get storage %s/%s: %v", dbItem.Kind, dbItem.Name, err)
				continue
			}

			getSpan.SetAttributes(
				attribute.Bool("storage.exists", existDbItem != nil),
				attribute.Float64("duration_ms", float64(getDuration.Milliseconds())),
			)
			getSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(getSpan)

			if existDbItem != nil {
				// Update existing storage
				updateSpan, updateCtx := trace.StartSpanFromContext(itemCtx, "updateStorage")
				updateSpan.SetAttributes(
					attribute.String("storage.kind", dbItem.Kind),
					attribute.String("storage.name", dbItem.Name),
					attribute.String("storage.status", dbItem.Status),
					attribute.Int64("storage.id", int64(existDbItem.ID)),
				)

				dbItem.ID = existDbItem.ID
				dbItem.CreatedAt = existDbItem.CreatedAt

				updateStart := time.Now()
				err = database.GetFacade().GetStorage().UpdateStorage(updateCtx, dbItem)
				updateDuration := time.Since(updateStart)

				if err != nil {
					updateSpan.RecordError(err)
					updateSpan.SetAttributes(
						attribute.String("error.message", err.Error()),
						attribute.Float64("duration_ms", float64(updateDuration.Milliseconds())),
					)
					updateSpan.SetStatus(codes.Error, err.Error())
					trace.FinishSpan(updateSpan)

					itemSpan.RecordError(err)
					itemSpan.SetAttributes(attribute.String("error.message", err.Error()))
					itemSpan.SetStatus(codes.Error, "Failed to update storage")
					trace.FinishSpan(itemSpan)

					stats.ErrorCount++
					log.Errorf("Fail to update storage %s/%s: %v", dbItem.Kind, dbItem.Name, err)
					continue
				}

				updateSpan.SetAttributes(attribute.Float64("duration_ms", float64(updateDuration.Milliseconds())))
				updateSpan.SetStatus(codes.Ok, "")
				trace.FinishSpan(updateSpan)

				stats.ItemsUpdated++
				log.Infof("Storage %s/%s updated", dbItem.Kind, dbItem.Name)

				itemSpan.SetAttributes(
					attribute.String("operation", "update"),
					attribute.Bool("success", true),
				)
			} else {
				// Create new storage
				createSpan, createCtx := trace.StartSpanFromContext(itemCtx, "createStorage")
				createSpan.SetAttributes(
					attribute.String("storage.kind", dbItem.Kind),
					attribute.String("storage.name", dbItem.Name),
					attribute.String("storage.status", dbItem.Status),
				)

				createStart := time.Now()
				err = database.GetFacade().GetStorage().CreateStorage(createCtx, dbItem)
				createDuration := time.Since(createStart)

				if err != nil {
					createSpan.RecordError(err)
					createSpan.SetAttributes(
						attribute.String("error.message", err.Error()),
						attribute.Float64("duration_ms", float64(createDuration.Milliseconds())),
					)
					createSpan.SetStatus(codes.Error, err.Error())
					trace.FinishSpan(createSpan)

					itemSpan.RecordError(err)
					itemSpan.SetAttributes(attribute.String("error.message", err.Error()))
					itemSpan.SetStatus(codes.Error, "Failed to create storage")
					trace.FinishSpan(itemSpan)

					stats.ErrorCount++
					log.Errorf("Fail to create storage %s/%s: %v", dbItem.Kind, dbItem.Name, err)
					continue
				}

				createSpan.SetAttributes(attribute.Float64("duration_ms", float64(createDuration.Milliseconds())))
				createSpan.SetStatus(codes.Ok, "")
				trace.FinishSpan(createSpan)

				stats.ItemsCreated++
				log.Infof("Storage %s/%s created", dbItem.Kind, dbItem.Name)

				itemSpan.SetAttributes(
					attribute.String("operation", "create"),
					attribute.Bool("success", true),
				)
			}

			stats.RecordsProcessed++
			itemSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(itemSpan)
		}
	}

	processDuration := time.Since(processStart)
	processSpan.SetAttributes(
		attribute.Float64("duration_ms", float64(processDuration.Milliseconds())),
		attribute.Int64("records_processed", stats.RecordsProcessed),
		attribute.Int64("items_created", stats.ItemsCreated),
		attribute.Int64("items_updated", stats.ItemsUpdated),
		attribute.Int64("error_count", stats.ErrorCount),
	)
	processSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(processSpan)

	stats.AddMessage("Storage scan completed successfully")

	// Record total job duration
	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(
		attribute.Int("backend_items_scanned", totalBackendItems),
		attribute.Int64("records_processed", stats.RecordsProcessed),
		attribute.Int64("items_created", stats.ItemsCreated),
		attribute.Int64("items_updated", stats.ItemsUpdated),
		attribute.Int64("error_count", stats.ErrorCount),
		attribute.Float64("scan_duration_seconds", stats.QueryDuration),
		attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())),
	)

	if stats.ErrorCount > 0 {
		span.SetStatus(codes.Error, "Some storage items failed to process")
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return stats, nil
}

func (s *StorageScanJob) Schedule() string {
	return "@every 1m"
}
