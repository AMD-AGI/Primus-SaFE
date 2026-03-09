// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pipeline

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/registry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ImageRegistryCollector enriches IntentEvidence with image registry metadata
// by performing inline OCI analysis (manifest + layer scanning) directly in-process.
type ImageRegistryCollector struct {
	analyzer    *registry.InlineImageAnalyzer
	cacheFacade database.ImageRegistryCacheFacadeInterface
}

// NewImageRegistryCollector creates a new collector backed by an InlineImageAnalyzer.
func NewImageRegistryCollector() *ImageRegistryCollector {
	return &ImageRegistryCollector{
		analyzer:    registry.NewInlineImageAnalyzer(),
		cacheFacade: database.NewImageRegistryCacheFacade(),
	}
}

// Enrich performs inline image analysis (or returns a cached result) and applies
// the findings to the evidence.
func (c *ImageRegistryCollector) Enrich(
	ctx context.Context,
	evidence *intent.IntentEvidence,
) {
	if evidence.Image == "" {
		return
	}

	namespace := evidence.WorkloadNamespace
	if namespace == "" {
		namespace = "primus-lens"
	}

	result, err := c.analyzer.AnalyzeOrCache(ctx, evidence.Image, namespace)
	if err != nil {
		log.Warnf("ImageRegistryCollector: inline analysis failed for %s: %v", evidence.Image, err)
		return
	}

	c.applyResult(result, evidence)
}

// EnrichFromCache checks the image registry cache without triggering a new
// analysis. Use this for terminated workloads to avoid unnecessary work.
func (c *ImageRegistryCollector) EnrichFromCache(
	ctx context.Context,
	evidence *intent.IntentEvidence,
) {
	if evidence.Image == "" {
		return
	}

	regHost, repo, tag := parseImageRef(evidence.Image)
	cached, err := c.cacheFacade.GetByTagRef(ctx, regHost, repo, tag)
	if err == nil && cached != nil && cached.Status == "completed" {
		log.Debugf("ImageRegistryCollector: cache hit for %s (completed)", evidence.Image)
		c.applyCache(cached, evidence)
	}
}

// applyResult converts an InlineAnalysisResult into IntentEvidence fields.
func (c *ImageRegistryCollector) applyResult(
	result *registry.InlineAnalysisResult,
	evidence *intent.IntentEvidence,
) {
	evidence.ImageRegistry = &intent.ImageRegistryEvidence{
		Digest:    result.Digest,
		BaseImage: result.BaseImage,
	}

	if len(result.LayerHistory) > 0 {
		layers := make([]intent.LayerInfo, len(result.LayerHistory))
		for i, h := range result.LayerHistory {
			layers[i] = intent.LayerInfo{
				CreatedBy: h.CreatedBy,
				Comment:   h.Comment,
			}
		}
		evidence.ImageRegistry.LayerHistory = layers
	}

	if len(result.InstalledPackages) > 0 {
		pkgs := make([]intent.PackageInfo, len(result.InstalledPackages))
		for i, p := range result.InstalledPackages {
			pkgs[i] = intent.PackageInfo{
				Manager: p.Manager,
				Name:    p.Name,
				Version: p.Version,
			}
		}
		evidence.ImageRegistry.InstalledPackages = pkgs
	}

	if result.FrameworkHints != nil {
		evidence.ImageRegistry.FrameworkHints = result.FrameworkHints
	}
}

// applyCache applies cached image registry data to the evidence.
func (c *ImageRegistryCollector) applyCache(
	cached *model.ImageRegistryCache,
	evidence *intent.IntentEvidence,
) {
	evidence.ImageRegistry = &intent.ImageRegistryEvidence{
		Digest:    cached.Digest,
		BaseImage: cached.BaseImage,
	}

	if len(cached.LayerHistory) > 0 {
		var layers []intent.LayerInfo
		if json.Unmarshal([]byte(cached.LayerHistory), &layers) == nil {
			evidence.ImageRegistry.LayerHistory = layers
		}
	}

	if len(cached.InstalledPackages) > 0 {
		var pkgs []intent.PackageInfo
		if json.Unmarshal([]byte(cached.InstalledPackages), &pkgs) == nil {
			evidence.ImageRegistry.InstalledPackages = pkgs
		}
	}

	if cached.FrameworkHints != nil {
		evidence.ImageRegistry.FrameworkHints = map[string]interface{}(cached.FrameworkHints)
	}
}

// parseImageRef splits "registry.example.com/repo/name:tag" into components
func parseImageRef(imageRef string) (registry, repository, tag string) {
	tag = "latest"
	ref := imageRef
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		afterColon := ref[idx+1:]
		if !strings.Contains(afterColon, "/") {
			tag = afterColon
			ref = ref[:idx]
		}
	}

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		registry = parts[0]
		repository = parts[1]
	} else {
		registry = ""
		repository = ref
	}
	return
}
