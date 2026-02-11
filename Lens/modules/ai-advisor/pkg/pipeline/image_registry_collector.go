// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/registry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	defaultCacheTTL = 7 * 24 * time.Hour // 7 days
)

// ImageRegistryCollector fetches image metadata from Harbor and caches it in
// the image_registry_cache table. On subsequent calls for the same image, it
// reads from cache instead of querying the registry again.
type ImageRegistryCollector struct {
	harborClient   *registry.HarborClient
	layerAnalyzer  *registry.LayerAnalyzer
	cacheFacade    database.ImageRegistryCacheFacadeInterface
}

// NewImageRegistryCollector creates a new collector with default configuration.
// Harbor URL and credentials are read from environment variables.
func NewImageRegistryCollector() *ImageRegistryCollector {
	harborURL := os.Getenv("HARBOR_URL")
	if harborURL == "" {
		harborURL = "https://harbor.primus-safe.amd.com"
	}

	client := registry.NewHarborClient(&registry.HarborClientConfig{
		BaseURL:  harborURL,
		Username: os.Getenv("HARBOR_USERNAME"),
		Password: os.Getenv("HARBOR_PASSWORD"),
	})

	return &ImageRegistryCollector{
		harborClient:  client,
		layerAnalyzer: registry.NewLayerAnalyzer(),
		cacheFacade:   database.NewImageRegistryCacheFacade(),
	}
}

// NewImageRegistryCollectorWithDeps creates a collector with injected dependencies
func NewImageRegistryCollectorWithDeps(
	client *registry.HarborClient,
	analyzer *registry.LayerAnalyzer,
	cacheFacade database.ImageRegistryCacheFacadeInterface,
) *ImageRegistryCollector {
	return &ImageRegistryCollector{
		harborClient:  client,
		layerAnalyzer: analyzer,
		cacheFacade:   cacheFacade,
	}
}

// Enrich fetches image metadata and enriches the IntentEvidence with registry data.
// It uses the cache when available to avoid redundant registry queries.
func (c *ImageRegistryCollector) Enrich(
	ctx context.Context,
	evidence *intent.IntentEvidence,
) {
	if evidence.Image == "" {
		return
	}

	imageRef := evidence.Image

	// Check cache first - parse image ref into components for lookup
	regHost, repo, tag := parseImageRef(imageRef)
	cached, err := c.cacheFacade.GetByTagRef(ctx, regHost, repo, tag)
	if err == nil && cached != nil && !c.isCacheExpired(cached) {
		log.Debugf("ImageRegistryCollector: cache hit for %s", imageRef)
		c.applyCache(cached, evidence)
		return
	}

	// Cache miss or expired: fetch from registry
	log.Infof("ImageRegistryCollector: fetching metadata for %s", imageRef)

	config, digest, err := c.harborClient.FetchImageMetadata(ctx, imageRef)
	if err != nil {
		log.Warnf("ImageRegistryCollector: failed to fetch %s: %v", imageRef, err)
		return
	}

	// Analyze layers
	analysis := c.layerAnalyzer.Analyze(config)

	// Build cache record
	cacheRecord := c.buildCacheRecord(imageRef, digest, analysis)

	// Store in cache (upsert)
	if err := c.cacheFacade.Upsert(ctx, cacheRecord); err != nil {
		log.Warnf("ImageRegistryCollector: failed to cache %s: %v", imageRef, err)
	}

	// Apply to evidence
	c.applyAnalysis(analysis, digest, evidence)
}

// buildCacheRecord creates an image_registry_cache record from the analysis
func (c *ImageRegistryCollector) buildCacheRecord(
	imageRef string,
	digest string,
	analysis *registry.AnalysisResult,
) *model.ImageRegistryCache {
	regHost, repo, tag := parseImageRef(imageRef)
	record := &model.ImageRegistryCache{
		ImageRef:   imageRef,
		Registry:   regHost,
		Repository: repo,
		Tag:        tag,
		Digest:     digest,
		BaseImage:  analysis.BaseImage,
	}

	// Serialize layer history (ExtJSON = json.RawMessage)
	if len(analysis.LayerHistory) > 0 {
		historyJSON, _ := json.Marshal(analysis.LayerHistory)
		record.LayerHistory = model.ExtJSON(historyJSON)
	}

	// Serialize installed packages (ExtJSON)
	if len(analysis.InstalledPackages) > 0 {
		pkgJSON, _ := json.Marshal(analysis.InstalledPackages)
		record.InstalledPackages = model.ExtJSON(pkgJSON)
	}

	// Serialize framework hints (ExtType = map[string]interface{})
	if len(analysis.FrameworkHints) > 0 {
		record.FrameworkHints = model.ExtType(analysis.FrameworkHints)
	}

	// Set expiration
	expiresAt := time.Now().Add(defaultCacheTTL)
	record.ExpiresAt = &expiresAt

	return record
}

// applyAnalysis applies fresh analysis results to the evidence
func (c *ImageRegistryCollector) applyAnalysis(
	analysis *registry.AnalysisResult,
	digest string,
	evidence *intent.IntentEvidence,
) {
	evidence.ImageRegistry = &intent.ImageRegistryEvidence{
		Digest:    digest,
		BaseImage: analysis.BaseImage,
	}

	// Convert layer history
	evidence.ImageRegistry.LayerHistory = analysis.LayerHistory

	// Convert installed packages
	evidence.ImageRegistry.InstalledPackages = analysis.InstalledPackages

	// Copy framework hints
	evidence.ImageRegistry.FrameworkHints = analysis.FrameworkHints
}

// applyCache applies cached data to the evidence
func (c *ImageRegistryCollector) applyCache(
	cached *model.ImageRegistryCache,
	evidence *intent.IntentEvidence,
) {
	evidence.ImageRegistry = &intent.ImageRegistryEvidence{
		Digest:    cached.Digest,
		BaseImage: cached.BaseImage,
	}

	// Parse layer history (ExtJSON = json.RawMessage)
	if len(cached.LayerHistory) > 0 {
		var layers []intent.LayerInfo
		if json.Unmarshal([]byte(cached.LayerHistory), &layers) == nil {
			evidence.ImageRegistry.LayerHistory = layers
		}
	}

	// Parse installed packages (ExtJSON)
	if len(cached.InstalledPackages) > 0 {
		var pkgs []intent.PackageInfo
		if json.Unmarshal([]byte(cached.InstalledPackages), &pkgs) == nil {
			evidence.ImageRegistry.InstalledPackages = pkgs
		}
	}

	// Parse framework hints (ExtType = map[string]interface{})
	if cached.FrameworkHints != nil {
		evidence.ImageRegistry.FrameworkHints = map[string]interface{}(cached.FrameworkHints)
	}
}

// isCacheExpired checks if a cache record has expired
func (c *ImageRegistryCollector) isCacheExpired(cached *model.ImageRegistryCache) bool {
	if cached.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*cached.ExpiresAt)
}

// parseImageRef splits "registry.example.com/repo/name:tag" into components
func parseImageRef(imageRef string) (registry, repository, tag string) {
	// Split off tag
	tag = "latest"
	ref := imageRef
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		// Make sure it's not part of the registry (e.g. localhost:5000)
		afterColon := ref[idx+1:]
		if !strings.Contains(afterColon, "/") {
			tag = afterColon
			ref = ref[:idx]
		}
	}

	// Split registry from repository
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
