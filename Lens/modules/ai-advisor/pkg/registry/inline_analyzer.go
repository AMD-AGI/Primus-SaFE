// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"k8s.io/client-go/kubernetes"
)

const (
	cacheExpireDuration = 24 * time.Hour
)

// InlineImageAnalyzer performs full image analysis in-process by pulling
// manifests, configs, and layer blobs directly from the OCI registry.
type InlineImageAnalyzer struct {
	cacheFacade  database.ImageRegistryCacheFacadeInterface
	layerFacade  database.ImageLayerCacheFacadeInterface
	ociClient    *OCIClient
	layerScanner *OCILayerScanner
	authResolver *AuthResolver
}

// InlineAnalysisResult holds the complete analysis output for an image.
type InlineAnalysisResult struct {
	Digest            string                 `json:"digest"`
	BaseImage         string                 `json:"base_image"`
	LayerCount        int                    `json:"layer_count"`
	LayerHistory      []OCIHistory           `json:"layer_history"`
	InstalledPackages []OCIPackageInfo       `json:"installed_packages"`
	FrameworkHints    map[string]interface{} `json:"framework_hints"`
	ImageEnv          map[string]string      `json:"image_env"`
	ImageEntrypoint   string                 `json:"image_entrypoint"`
	ImageLabels       map[string]string      `json:"image_labels"`
}

// NewInlineImageAnalyzer creates a new analyzer using the cluster's K8s client
// for imagePullSecret resolution.
func NewInlineImageAnalyzer() *InlineImageAnalyzer {
	var k8sClient kubernetes.Interface
	cm := clientsets.GetClusterManager()
	if cs := cm.GetCurrentClusterClients(); cs != nil && cs.K8SClientSet != nil {
		k8sClient = cs.K8SClientSet.Clientsets
	}

	authResolver := NewAuthResolver(k8sClient)
	return &InlineImageAnalyzer{
		cacheFacade:  database.NewImageRegistryCacheFacade(),
		layerFacade:  database.NewImageLayerCacheFacade(),
		ociClient:    NewOCIClient(authResolver),
		layerScanner: NewOCILayerScanner(),
		authResolver: authResolver,
	}
}

// AnalyzeImage fetches the manifest + config, downloads and scans each layer,
// deduplicates via the layer cache, and writes the full result to the image cache.
func (a *InlineImageAnalyzer) AnalyzeImage(ctx context.Context, imageRef, namespace string) (*InlineAnalysisResult, error) {
	ref := ParseImageRef(imageRef)

	auth, err := a.authResolver.ResolveAuth(ctx, namespace, ref.Registry)
	if err != nil {
		log.Warnf("InlineImageAnalyzer: auth resolution failed for %s: %v", ref.Registry, err)
	}

	manifest, err := a.ociClient.FetchManifest(ctx, ref, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest for %s: %w", imageRef, err)
	}

	config, err := a.ociClient.FetchConfig(ctx, ref, manifest.Config.Digest, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config for %s: %w", imageRef, err)
	}

	result := &InlineAnalysisResult{
		Digest:            manifest.Config.Digest,
		LayerCount:        len(manifest.Layers),
		LayerHistory:      config.History,
		InstalledPackages: make([]OCIPackageInfo, 0),
		FrameworkHints:    make(map[string]interface{}),
		ImageEnv:          make(map[string]string),
		ImageLabels:       config.Config.Labels,
	}

	// Parse env vars
	for _, env := range config.Config.Env {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			result.ImageEnv[parts[0]] = parts[1]
		}
	}

	if len(config.Config.Entrypoint) > 0 {
		result.ImageEntrypoint = strings.Join(config.Config.Entrypoint, " ")
	}

	result.BaseImage = extractOCIBaseImage(config.History)

	// Download and scan layers
	layerDigests := make([]string, len(manifest.Layers))
	for i, l := range manifest.Layers {
		layerDigests[i] = l.Digest
	}

	layerResults, err := a.analyzeLayers(ctx, ref, auth, manifest.Layers)
	if err != nil {
		log.Warnf("InlineImageAnalyzer: layer analysis partial failure for %s: %v", imageRef, err)
	}

	for _, lr := range layerResults {
		result.InstalledPackages = append(result.InstalledPackages, lr.Packages...)
		for k, v := range lr.FrameworkHints {
			result.FrameworkHints[k] = v
		}
	}

	a.writeCacheRecord(ctx, imageRef, ref, result)

	return result, nil
}

// AnalyzeOrCache checks the image cache first. On cache miss or expiry, it
// performs a full analysis.
func (a *InlineImageAnalyzer) AnalyzeOrCache(ctx context.Context, imageRef, namespace string) (*InlineAnalysisResult, error) {
	cached, err := a.cacheFacade.GetByImageRef(ctx, imageRef)
	if err != nil {
		log.Warnf("InlineImageAnalyzer: cache lookup failed for %s: %v", imageRef, err)
	}

	if cached != nil && cached.Status == "completed" && !isCacheExpired(cached) {
		log.Debugf("InlineImageAnalyzer: cache hit for %s (digest=%s)", imageRef, shortDigest(cached.Digest))
		return buildResultFromCache(cached), nil
	}

	log.Infof("InlineImageAnalyzer: cache miss for %s, performing full analysis", imageRef)
	return a.AnalyzeImage(ctx, imageRef, namespace)
}

// analyzeLayers batch-checks the layer cache and only downloads uncached layers.
func (a *InlineImageAnalyzer) analyzeLayers(ctx context.Context, ref ImageRef, auth *RegistryAuth, layers []OCIDescriptor) ([]*OCILayerResult, error) {
	digests := make([]string, len(layers))
	for i, l := range layers {
		digests[i] = l.Digest
	}

	cachedLayers, err := a.layerFacade.GetByDigests(ctx, digests)
	if err != nil {
		log.Warnf("InlineImageAnalyzer: batch layer cache lookup failed: %v", err)
	}

	cachedMap := make(map[string]*model.ImageLayerCache)
	for _, cl := range cachedLayers {
		cachedMap[cl.LayerDigest] = cl
	}

	results := make([]*OCILayerResult, 0, len(layers))
	for _, layer := range layers {
		if cached, ok := cachedMap[layer.Digest]; ok {
			log.Debugf("InlineImageAnalyzer: layer cache hit for %s", shortDigest(layer.Digest))
			results = append(results, mergeLayerFromCache(cached))
			continue
		}

		lr, err := a.downloadAndScanLayer(ctx, ref, auth, layer)
		if err != nil {
			log.Warnf("InlineImageAnalyzer: failed to scan layer %s: %v", shortDigest(layer.Digest), err)
			continue
		}
		results = append(results, lr)

		a.storeLayerCache(ctx, layer, lr)
	}

	return results, nil
}

func (a *InlineImageAnalyzer) downloadAndScanLayer(ctx context.Context, ref ImageRef, auth *RegistryAuth, layer OCIDescriptor) (*OCILayerResult, error) {
	body, _, err := a.ociClient.StreamLayerBlob(ctx, ref, layer.Digest, auth)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	result, err := a.layerScanner.ScanLayer(body)
	if err != nil {
		// Retry as plain tar
		body2, _, err2 := a.ociClient.StreamLayerBlob(ctx, ref, layer.Digest, auth)
		if err2 != nil {
			return nil, fmt.Errorf("retry stream failed: %w", err2)
		}
		defer body2.Close()
		result, err = a.layerScanner.ScanLayerPlain(body2)
		if err != nil {
			return nil, fmt.Errorf("plain tar scan also failed: %w", err)
		}
	}
	return result, nil
}

// storeLayerCache writes a single layer's analysis result to the image_layer_cache table.
func (a *InlineImageAnalyzer) storeLayerCache(ctx context.Context, layer OCIDescriptor, lr *OCILayerResult) {
	pkgJSON, _ := json.Marshal(lr.Packages)
	pathsJSON, _ := json.Marshal(lr.NotablePaths)

	hintsMap := model.ExtType(lr.FrameworkHints)

	entry := &model.ImageLayerCache{
		LayerDigest:    layer.Digest,
		CompressedSize: layer.Size,
		MediaType:      layer.MediaType,
		FileCount:      lr.FileCount,
		Packages:       model.ExtJSON(pkgJSON),
		FrameworkHints: hintsMap,
		NotablePaths:   model.ExtJSON(pathsJSON),
		AnalyzedAt:     time.Now(),
		CreatedAt:      time.Now(),
	}

	if err := a.layerFacade.Upsert(ctx, entry); err != nil {
		log.Warnf("InlineImageAnalyzer: failed to cache layer %s: %v", shortDigest(layer.Digest), err)
	}
}

// writeCacheRecord creates or updates the image_registry_cache entry with full analysis results.
func (a *InlineImageAnalyzer) writeCacheRecord(ctx context.Context, imageRef string, ref ImageRef, result *InlineAnalysisResult) {
	historyJSON, _ := json.Marshal(result.LayerHistory)
	pkgJSON, _ := json.Marshal(result.InstalledPackages)

	labelsMap := make(model.ExtType)
	for k, v := range result.ImageLabels {
		labelsMap[k] = v
	}
	envMap := make(model.ExtType)
	for k, v := range result.ImageEnv {
		envMap[k] = v
	}
	hintsMap := model.ExtType(result.FrameworkHints)

	now := time.Now()

	entry := &model.ImageRegistryCache{
		ImageRef:          imageRef,
		Digest:            result.Digest,
		Registry:          ref.Registry,
		Repository:        ref.Repository,
		Tag:               ref.Tag,
		BaseImage:         result.BaseImage,
		LayerCount:        int32(result.LayerCount),
		LayerHistory:      model.ExtJSON(historyJSON),
		ImageLabels:       labelsMap,
		ImageEnv:          envMap,
		ImageEntrypoint:   result.ImageEntrypoint,
		InstalledPackages: model.ExtJSON(pkgJSON),
		FrameworkHints:    hintsMap,
		TotalSize:         0,
		Status:            "completed",
		CachedAt:          now,
	}

	if err := a.cacheFacade.Upsert(ctx, entry); err != nil {
		log.Warnf("InlineImageAnalyzer: failed to write cache for %s: %v", imageRef, err)
	}
}

// mergeLayerFromCache reconstructs an OCILayerResult from a cached layer entry.
func mergeLayerFromCache(cached *model.ImageLayerCache) *OCILayerResult {
	result := &OCILayerResult{
		FileCount:      cached.FileCount,
		Packages:       make([]OCIPackageInfo, 0),
		FrameworkHints: make(map[string]interface{}),
		NotablePaths:   make([]string, 0),
	}
	if len(cached.Packages) > 0 {
		_ = json.Unmarshal([]byte(cached.Packages), &result.Packages)
	}
	if len(cached.FrameworkHints) > 0 {
		for k, v := range cached.FrameworkHints {
			result.FrameworkHints[k] = v
		}
	}
	if len(cached.NotablePaths) > 0 {
		_ = json.Unmarshal([]byte(cached.NotablePaths), &result.NotablePaths)
	}
	return result
}

// mergeLayerResult combines a layer result into the overall analysis result.
func mergeLayerResult(target *InlineAnalysisResult, lr *OCILayerResult) {
	target.InstalledPackages = append(target.InstalledPackages, lr.Packages...)
	for k, v := range lr.FrameworkHints {
		target.FrameworkHints[k] = v
	}
}

// buildResultFromCache constructs an InlineAnalysisResult from a cache entry.
func buildResultFromCache(cached *model.ImageRegistryCache) *InlineAnalysisResult {
	result := &InlineAnalysisResult{
		Digest:            cached.Digest,
		BaseImage:         cached.BaseImage,
		LayerCount:        int(cached.LayerCount),
		InstalledPackages: make([]OCIPackageInfo, 0),
		FrameworkHints:    make(map[string]interface{}),
		ImageEnv:          make(map[string]string),
		ImageLabels:       make(map[string]string),
		ImageEntrypoint:   cached.ImageEntrypoint,
	}

	if len(cached.LayerHistory) > 0 {
		_ = json.Unmarshal([]byte(cached.LayerHistory), &result.LayerHistory)
	}
	if len(cached.InstalledPackages) > 0 {
		_ = json.Unmarshal([]byte(cached.InstalledPackages), &result.InstalledPackages)
	}
	for k, v := range cached.FrameworkHints {
		result.FrameworkHints[k] = v
	}
	for k, v := range cached.ImageEnv {
		if s, ok := v.(string); ok {
			result.ImageEnv[k] = s
		}
	}
	for k, v := range cached.ImageLabels {
		if s, ok := v.(string); ok {
			result.ImageLabels[k] = s
		}
	}

	return result
}

// parseImageRefComponents splits an image reference into registry, repository, and tag.
func parseImageRefComponents(imageRef string) (registry, repository, tag string) {
	ref := ParseImageRef(imageRef)
	return ref.Registry, ref.Repository, ref.Tag
}

// isCacheExpired checks whether a cache entry has passed its expiration time.
func isCacheExpired(entry *model.ImageRegistryCache) bool {
	if entry.ExpiresAt != nil {
		return time.Now().After(*entry.ExpiresAt)
	}
	return time.Since(entry.CachedAt) > cacheExpireDuration
}

// shortDigest returns the first 12 characters of a digest for logging.
func shortDigest(digest string) string {
	d := strings.TrimPrefix(digest, "sha256:")
	if len(d) > 12 {
		return d[:12]
	}
	return d
}

// extractOCIBaseImage attempts to identify the base image from layer history.
// It looks for the last FROM instruction in the layer history.
func extractOCIBaseImage(history []OCIHistory) string {
	for i := len(history) - 1; i >= 0; i-- {
		cmd := history[i].CreatedBy
		if strings.Contains(cmd, "FROM") {
			parts := strings.Fields(cmd)
			for j, p := range parts {
				if strings.ToUpper(p) == "FROM" && j+1 < len(parts) {
					return parts[j+1]
				}
			}
		}
	}

	// Fallback: use the first non-empty history entry
	for _, h := range history {
		if h.CreatedBy != "" && !h.EmptyLayer {
			return h.CreatedBy
		}
	}
	return ""
}
