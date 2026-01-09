// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package prom

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promModel "github.com/prometheus/common/model"
)

func QueryInstant(ctx context.Context, clientSets *clientsets.StorageClientSet, query string) ([]*promModel.Sample, error) {
	now := time.Now()
	promAPI, err := getPromApi(clientSets)
	if err != nil {
		return nil, err
	}
	result, warnings, err := promAPI.Query(ctx, query, now)
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		log.Warnf("Prometheus query warnings for %s: %v\n", query, warnings)
	}
	vectorVal, ok := result.(promModel.Vector)
	if !ok || len(vectorVal) == 0 {
		log.Warnf("Query result for %s: %v\n", query, result)
		return []*promModel.Sample{}, nil
	}

	return vectorVal, nil
}

func getPromApi(clientSets *clientsets.StorageClientSet) (v1.API, error) {
	promClient := clientSets.PrometheusRead
	if promClient == nil {
		return nil, fmt.Errorf("Prometheus client is not initialized")
	}

	return v1.NewAPI(promClient), nil
}

func QueryRange(ctx context.Context, clientSets *clientsets.StorageClientSet, query string, start, end time.Time, step int, labelFilters map[string]struct{}) ([]model.MetricsSeries, error) {

	promAPI, err := getPromApi(clientSets)
	if err != nil {
		return nil, err
	}

	rangeQuery := v1.Range{
		Start: start,
		End:   end,
		Step:  time.Duration(step) * time.Second,
	}

	result, warnings, err := promAPI.QueryRange(ctx, query, rangeQuery)
	if err != nil {
		return nil, fmt.Errorf("prometheus query range failed: %w", err)
	}
	if len(warnings) > 0 {
		log.Warnf("Prometheus query range warnings: %v\n", warnings)
	}

	matrixVal, ok := result.(promModel.Matrix)
	if !ok || len(matrixVal) == 0 {
		log.Warnf("No data returned for query: %s", query)
		return []model.MetricsSeries{}, nil
	}

	results := []model.MetricsSeries{}
	for _, stream := range matrixVal {
		var timeSeries []model.TimePoint
		for _, point := range stream.Values {
			timeSeries = append(timeSeries, model.TimePoint{
				Timestamp: point.Timestamp.Unix(),
				Value:     float64(point.Value),
			})
		}
		label := promModel.Metric{}
		if len(labelFilters) > 0 {
			for name, value := range stream.Metric {
				if _, ok := labelFilters[string(name)]; ok {
					label[name] = value
				}
			}
		} else {
			label = stream.Metric
		}
		results = append(results, model.MetricsSeries{
			Labels: label,
			Values: timeSeries,
		})
	}

	return results, nil
}

func QueryPrometheusInstant(ctx context.Context, query string, clientSets *clientsets.StorageClientSet) (float64, error) {
	promAPI := v1.NewAPI(clientSets.PrometheusRead)
	now := time.Now()
	result, warnings, err := promAPI.Query(ctx, query, now)
	if err != nil {
		return 0, err
	}
	if len(warnings) > 0 {
		log.Warnf("Prometheus query warnings for %s: %v\n", query, warnings)
	}

	vectorVal, ok := result.(promModel.Vector)
	if !ok || len(vectorVal) == 0 {
		log.Warnf("No data returned for query: %s", query)
		return 0, nil
	}

	return float64(vectorVal[0].Value), nil
}
