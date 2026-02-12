// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package analyzer

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
	pollInterval    = 5 * time.Second
	batchSize       = 1
	defaultCacheTTL = 7 * 24 * time.Hour // 7 days
)

// Worker is the main image analysis worker that polls pending requests
// from image_registry_cache and processes them.
type Worker struct {
	cacheFacade  database.ImageRegistryCacheFacadeInterface
	layerFacade  database.ImageLayerCacheFacadeInterface
	ociClient    *OCIClient
	layerScanner *LayerScanner
	authResolver *AuthResolver
}

// NewWorker creates a new image analysis worker
func NewWorker() (*Worker, error) {
	// Get K8s client from cluster manager
	var k8sClient kubernetes.Interface
	cm := clientsets.GetClusterManager()
	if cm != nil {
		current := cm.GetCurrentClusterClients()
		if current != nil && current.K8SClientSet != nil && current.K8SClientSet.Clientsets != nil {
			k8sClient = current.K8SClientSet.Clientsets
		}
	}

	authResolver := NewAuthResolver(k8sClient)
	ociClient := NewOCIClient(authResolver)

	return &Worker{
		cacheFacade:  database.NewImageRegistryCacheFacade(),
		layerFacade:  database.NewImageLayerCacheFacade(),
		ociClient:    ociClient,
		layerScanner: NewLayerScanner(),
		authResolver: authResolver,
	}, nil
}

// Run starts the main worker loop. It blocks until the context is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Worker: context cancelled, stopping")
			return
		case <-ticker.C:
			w.processNext(ctx)
		}
	}
}

// processNext picks up one pending request and processes it.
func (w *Worker) processNext(ctx context.Context) {
	pending, err := w.cacheFacade.GetPending(ctx, batchSize)
	if err != nil {
		log.Errorf("Worker: failed to get pending tasks: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}

	for _, entry := range pending {
		w.processEntry(ctx, entry)
	}
}

// processEntry processes a single pending image analysis request.
func (w *Worker) processEntry(ctx context.Context, entry *model.ImageRegistryCache) {
	imageRef := entry.ImageRef
	log.Infof("Worker: processing image %s (id=%d)", imageRef, entry.ID)

	// Update status to processing
	if err := w.cacheFacade.UpdateStatus(ctx, entry.ID, "processing", ""); err != nil {
		log.Errorf("Worker: failed to update status to processing for %s: %v", imageRef, err)
		return
	}

	// Parse image reference
	ref := ParseImageRef(imageRef)

	// Resolve auth for this registry
	namespace := entry.Namespace
	if namespace == "" {
		namespace = "primus-lens" // Default namespace
	}

	auth, err := w.authResolver.ResolveAuth(ctx, namespace, ref.Registry)
	if err != nil {
		log.Warnf("Worker: failed to resolve auth for %s in namespace %s: %v", ref.Registry, namespace, err)
		// Continue with anonymous access
	}

	// Fetch manifest
	manifest, err := w.ociClient.FetchManifest(ctx, ref, auth)
	if err != nil {
		errMsg := fmt.Sprintf("failed to fetch manifest: %v", err)
		log.Warnf("Worker: %s for %s", errMsg, imageRef)
		_ = w.cacheFacade.UpdateStatus(ctx, entry.ID, "failed", errMsg)
		return
	}

	// Fetch image config
	config, err := w.ociClient.FetchConfig(ctx, ref, manifest.Config.Digest, auth)
	if err != nil {
		errMsg := fmt.Sprintf("failed to fetch config: %v", err)
		log.Warnf("Worker: %s for %s", errMsg, imageRef)
		_ = w.cacheFacade.UpdateStatus(ctx, entry.ID, "failed", errMsg)
		return
	}

	// Analyze layers with deduplication
	aggregated, err := w.analyzeLayers(ctx, ref, manifest, auth)
	if err != nil {
		errMsg := fmt.Sprintf("layer analysis failed: %v", err)
		log.Warnf("Worker: %s for %s", errMsg, imageRef)
		_ = w.cacheFacade.UpdateStatus(ctx, entry.ID, "failed", errMsg)
		return
	}

	// Build the completed cache record
	w.updateCacheRecord(ctx, entry, manifest, config, aggregated)

	log.Infof("Worker: completed analysis for %s (layers=%d, packages=%d)",
		imageRef, len(manifest.Layers), len(aggregated.Packages))
}

// analyzeLayers processes each layer in the manifest, using the layer cache
// for deduplication.
func (w *Worker) analyzeLayers(ctx context.Context, ref ImageRef, manifest *OCIManifest, auth *RegistryAuth) (*LayerResult, error) {
	aggregated := &LayerResult{
		FrameworkHints: make(map[string]interface{}),
	}

	// Collect all layer digests for batch lookup
	digests := make([]string, len(manifest.Layers))
	for i, layer := range manifest.Layers {
		digests[i] = layer.Digest
	}

	// Batch check which layers are already cached
	cachedLayers, err := w.layerFacade.GetByDigests(ctx, digests)
	if err != nil {
		log.Warnf("Worker: failed to batch lookup layer cache: %v", err)
		// Continue - will fetch all layers
	}

	cachedMap := make(map[string]*model.ImageLayerCache)
	for _, cached := range cachedLayers {
		cachedMap[cached.LayerDigest] = cached
	}

	for _, layer := range manifest.Layers {
		// Check cache first
		if cached, ok := cachedMap[layer.Digest]; ok {
			log.Debugf("Worker: layer %s already cached, skipping download", layer.Digest[:16])
			w.mergeLayerFromCache(aggregated, cached)
			continue
		}

		// Download and analyze the layer
		layerResult, err := w.downloadAndAnalyzeLayer(ctx, ref, layer, auth)
		if err != nil {
			log.Warnf("Worker: failed to analyze layer %s: %v", layer.Digest[:16], err)
			continue
		}

		// Store in layer cache
		w.storeLayerCache(ctx, layer, layerResult)

		// Merge into aggregated result
		w.mergeLayerResult(aggregated, layerResult)
	}

	return aggregated, nil
}

// downloadAndAnalyzeLayer downloads a single layer blob and performs streaming analysis.
func (w *Worker) downloadAndAnalyzeLayer(ctx context.Context, ref ImageRef, layer OCIDescriptor, auth *RegistryAuth) (*LayerResult, error) {
	log.Debugf("Worker: downloading layer %s (%d bytes)", layer.Digest[:16], layer.Size)

	reader, _, err := w.ociClient.StreamLayerBlob(ctx, ref, layer.Digest, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to stream layer: %w", err)
	}
	defer reader.Close()

	result, err := w.layerScanner.ScanLayer(reader)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	return result, nil
}

// storeLayerCache stores a layer analysis result in the image_layer_cache table.
func (w *Worker) storeLayerCache(ctx context.Context, layer OCIDescriptor, result *LayerResult) {
	pkgJSON, _ := json.Marshal(result.Packages)
	pathsJSON, _ := json.Marshal(result.NotablePaths)

	entry := &model.ImageLayerCache{
		LayerDigest:    layer.Digest,
		CompressedSize: layer.Size,
		MediaType:      layer.MediaType,
		FileCount:      result.FileCount,
		Packages:       model.ExtJSON(pkgJSON),
		FrameworkHints: model.ExtType(result.FrameworkHints),
		NotablePaths:   model.ExtJSON(pathsJSON),
		AnalyzedAt:     time.Now(),
		CreatedAt:      time.Now(),
	}

	if err := w.layerFacade.Upsert(ctx, entry); err != nil {
		log.Warnf("Worker: failed to cache layer %s: %v", layer.Digest[:16], err)
	}
}

// mergeLayerFromCache merges cached layer data into the aggregated result.
func (w *Worker) mergeLayerFromCache(aggregated *LayerResult, cached *model.ImageLayerCache) {
	aggregated.FileCount += cached.FileCount

	// Parse packages from cache
	if len(cached.Packages) > 0 {
		var pkgs []PackageInfo
		if json.Unmarshal([]byte(cached.Packages), &pkgs) == nil {
			aggregated.Packages = append(aggregated.Packages, pkgs...)
		}
	}

	// Parse notable paths from cache
	if len(cached.NotablePaths) > 0 {
		var paths []string
		if json.Unmarshal([]byte(cached.NotablePaths), &paths) == nil {
			aggregated.NotablePaths = append(aggregated.NotablePaths, paths...)
		}
	}

	// Merge framework hints (later layers override earlier ones)
	if cached.FrameworkHints != nil {
		for k, v := range cached.FrameworkHints {
			aggregated.FrameworkHints[k] = v
		}
	}
}

// mergeLayerResult merges a fresh layer result into the aggregated result.
func (w *Worker) mergeLayerResult(aggregated *LayerResult, result *LayerResult) {
	aggregated.FileCount += result.FileCount
	aggregated.Packages = append(aggregated.Packages, result.Packages...)
	aggregated.NotablePaths = append(aggregated.NotablePaths, result.NotablePaths...)

	for k, v := range result.FrameworkHints {
		aggregated.FrameworkHints[k] = v
	}
}

// updateCacheRecord updates the image_registry_cache entry with completed analysis.
func (w *Worker) updateCacheRecord(ctx context.Context, entry *model.ImageRegistryCache, manifest *OCIManifest, config *OCIImageConfig, aggregated *LayerResult) {
	now := time.Now()
	expiresAt := now.Add(defaultCacheTTL)

	entry.Status = "completed"
	entry.Digest = manifest.Config.Digest
	entry.LayerCount = int32(len(manifest.Layers))
	entry.AnalyzedAt = &now
	entry.ExpiresAt = &expiresAt

	// Extract base image from config history
	if len(config.History) > 0 {
		entry.BaseImage = extractBaseImageFromHistory(config.History[0].CreatedBy)
	}

	// Calculate total size
	var totalSize int64
	for _, layer := range manifest.Layers {
		totalSize += layer.Size
	}
	entry.TotalSize = totalSize

	// Build layer history from config
	type layerHistoryEntry struct {
		CreatedBy string `json:"created_by,omitempty"`
		Comment   string `json:"comment,omitempty"`
	}
	var history []layerHistoryEntry
	for _, h := range config.History {
		history = append(history, layerHistoryEntry{
			CreatedBy: h.CreatedBy,
			Comment:   h.Comment,
		})
	}
	historyJSON, _ := json.Marshal(history)
	entry.LayerHistory = model.ExtJSON(historyJSON)

	// Serialize packages
	if len(aggregated.Packages) > 0 {
		pkgJSON, _ := json.Marshal(aggregated.Packages)
		entry.InstalledPackages = model.ExtJSON(pkgJSON)
	}

	// Serialize framework hints
	if len(aggregated.FrameworkHints) > 0 {
		entry.FrameworkHints = model.ExtType(aggregated.FrameworkHints)
	}

	// Serialize env vars and entrypoint from config
	if len(config.Config.Env) > 0 {
		envMap := make(model.ExtType)
		for _, envStr := range config.Config.Env {
			parts := strings.SplitN(envStr, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}
		entry.ImageEnv = envMap
	}

	if len(config.Config.Entrypoint) > 0 {
		entry.ImageEntrypoint = strings.Join(config.Config.Entrypoint, " ")
	}

	if len(config.Config.Labels) > 0 {
		labelMap := make(model.ExtType)
		for k, v := range config.Config.Labels {
			labelMap[k] = v
		}
		entry.ImageLabels = labelMap
	}

	// Save the updated record by ID (not Upsert, which conflicts on digest)
	if err := w.cacheFacade.UpdateAnalysisResult(ctx, entry); err != nil {
		log.Errorf("Worker: failed to update cache record for %s: %v", entry.ImageRef, err)
		_ = w.cacheFacade.UpdateStatus(ctx, entry.ID, "failed", fmt.Sprintf("update failed: %v", err))
		return
	}

	log.Infof("Worker: analysis completed for %s (digest=%s, layers=%d)", entry.ImageRef, entry.Digest[:min(12, len(entry.Digest))], entry.LayerCount)
}

// extractBaseImageFromHistory extracts the base image from the first history entry
func extractBaseImageFromHistory(instruction string) string {
	instruction = strings.TrimSpace(instruction)
	if idx := strings.Index(instruction, "FROM "); idx != -1 {
		rest := instruction[idx+5:]
		parts := strings.Fields(rest)
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}
