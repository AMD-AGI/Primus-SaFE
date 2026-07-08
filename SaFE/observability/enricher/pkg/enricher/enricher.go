/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package enricher

import (
	"context"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Enricher ties together mapping, scraping, transform and write into a single
// periodic pass.
type Enricher struct {
	cfg     Config
	mapper  *Mapper
	scraper *Scraper
	writer  *Writer
}

func New(c client.Client, cfg Config) *Enricher {
	return &Enricher{
		cfg:     cfg,
		mapper:  NewMapper(c, cfg.WorkloadPodLabel),
		scraper: NewScraper(c, cfg),
		writer:  NewWriter(cfg.VMImportURL, cfg.HTTPTimeout),
	}
}

// Run executes enrich passes on cfg.Interval until ctx is cancelled.
func (e *Enricher) Run(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.Interval)
	defer ticker.Stop()
	e.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.runOnce(ctx)
		}
	}
}

func (e *Enricher) runOnce(ctx context.Context) {
	mapping, err := e.mapper.Build(ctx)
	if err != nil {
		klog.Warningf("[enricher] build mapping: %v", err)
		return
	}
	targets, err := e.scraper.Targets(ctx)
	if err != nil {
		klog.Warningf("[enricher] resolve exporter targets: %v", err)
		return
	}
	if len(targets) == 0 {
		klog.V(2).Info("[enricher] no exporter endpoints found")
		return
	}
	records := e.scraper.ScrapeAll(ctx, targets)

	batch := e.build(records, mapping)
	if len(batch) == 0 {
		klog.V(3).Infof("[enricher] no workload-owned GPU series this pass (records=%d, byPod=%d, byNode=%d)",
			len(records), len(mapping.ByPod), len(mapping.ByNode))
		return
	}
	if err := e.writer.Write(ctx, batch); err != nil {
		klog.Warningf("[enricher] write %d series: %v", len(batch), err)
		return
	}
	klog.V(2).Infof("[enricher] wrote %d workload_gpu_* series (records=%d, byPod=%d, byNode=%d)",
		len(batch), len(records), len(mapping.ByPod), len(mapping.ByNode))
}

// build converts scraped GPU records into robust-compatible workload_gpu_*
// series, keeping only GPUs currently owned by a resolvable workload.
//
// Attribution order per GPU: exact (namespace,pod) label from the exporter
// first; otherwise fall back to the GPU's node (hostname) when that node runs
// exactly one GPU workload.
func (e *Enricher) build(records map[string]*gpuRecord, mapping *Mapping) []series {
	var batch []series
	for _, rec := range records {
		var info WorkloadInfo
		var resolved bool

		ns := rec.labels["namespace"]
		pod := rec.labels["pod"]
		if ns != "" && pod != "" {
			if i, ok := mapping.ByPod[podKey(ns, pod)]; ok && i.UID != "" {
				info, resolved = i, true
			}
		}
		if !resolved {
			if node := recNode(rec.labels); node != "" {
				if i, ok := mapping.ByNode[node]; ok && i.UID != "" {
					info, resolved = i, true
				}
			}
		}
		if !resolved {
			continue // GPU not attributable to a SaFE workload this pass
		}

		base := map[string]string{
			"workload_uid":       info.UID,
			"workload_name":      info.Name,
			"workload_namespace": info.Namespace,
			"cluster":            e.cfg.ClusterName,
		}
		if info.User != "" {
			base["workload_user"] = info.User
		}
		if node := recNode(rec.labels); node != "" {
			base["node"] = node
		}
		if gpu := rec.labels["gpu_id"]; gpu != "" {
			base["gpu_id"] = gpu
		}

		for _, om := range outputMetrics {
			value, ok := computeMetric(om, rec.values)
			if !ok {
				continue
			}
			labels := make(map[string]string, len(base))
			for k, v := range base {
				labels[k] = v
			}
			batch = append(batch, series{
				name:   relabelPrefix + om.Name,
				labels: labels,
				value:  value,
			})
		}
	}

	// workload_pod_info is a join metric (value 1) that lets dashboards filter
	// the existing cAdvisor container_* series (pod CPU/memory/storage/IO) by
	// workload_uid via `* on(namespace,pod) group_left(workload_uid)`.
	for _, p := range mapping.Pods {
		labels := map[string]string{
			"workload_uid":       p.Info.UID,
			"workload_name":      p.Info.Name,
			"workload_namespace": p.Info.Namespace,
			"namespace":          p.Namespace,
			"pod":                p.Name,
			"cluster":            e.cfg.ClusterName,
		}
		if p.Node != "" {
			labels["node"] = p.Node
		}
		if p.Info.User != "" {
			labels["workload_user"] = p.Info.User
		}
		batch = append(batch, series{name: "workload_pod_info", labels: labels, value: 1})
	}
	return batch
}

// computeMetric derives one output value from a GPU record's raw exporter
// values, supporting direct, ratio, and auto (direct-then-ratio) derivations.
func computeMetric(om outputMetric, values map[string]float64) (float64, bool) {
	scale := om.Scale
	if scale == 0 {
		scale = 1
	}
	ratio := func() (float64, bool) {
		num, ok := firstPresent(values, om.Numerator)
		if !ok {
			return 0, false
		}
		den, ok := firstPresent(values, om.Denominator)
		if !ok || den == 0 {
			return 0, false
		}
		return num / den * 100, true
	}
	direct := func() (float64, bool) {
		v, ok := firstPresent(values, om.Sources)
		if !ok {
			return 0, false
		}
		return v * scale, true
	}

	switch om.Kind {
	case "ratio":
		return ratio()
	case "auto":
		if v, ok := direct(); ok {
			return v, true
		}
		return ratio()
	default: // direct
		return direct()
	}
}

func firstPresent(values map[string]float64, candidates []string) (float64, bool) {
	for _, c := range candidates {
		if v, ok := values[c]; ok {
			return v, true
		}
	}
	return 0, false
}
