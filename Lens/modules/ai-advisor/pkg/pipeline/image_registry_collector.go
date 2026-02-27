// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pipeline

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// hybridWaitTimeout is how long Enrich() will poll for the image-analyzer to complete
	hybridWaitTimeout = 30 * time.Second
	// hybridPollInterval is the interval between cache checks during hybrid wait
	hybridPollInterval = 2 * time.Second
)

// ImageRegistryCollector enriches IntentEvidence with image registry metadata.
//
// It operates in hybrid async mode:
//  1. Check cache for completed result -> apply and return
//  2. If no result, write a pending request for the image-analyzer service
//  3. Poll cache for up to 30s waiting for completion
//  4. If timeout, return without image evidence (will be picked up in monitoring cycle)
type ImageRegistryCollector struct {
	cacheFacade database.ImageRegistryCacheFacadeInterface
}

// NewImageRegistryCollector creates a new collector with default configuration.
func NewImageRegistryCollector() *ImageRegistryCollector {
	return &ImageRegistryCollector{
		cacheFacade: database.NewImageRegistryCacheFacade(),
	}
}

// NewImageRegistryCollectorWithDeps creates a collector with injected dependencies
func NewImageRegistryCollectorWithDeps(
	cacheFacade database.ImageRegistryCacheFacadeInterface,
) *ImageRegistryCollector {
	return &ImageRegistryCollector{
		cacheFacade: cacheFacade,
	}
}

// Enrich checks the image registry cache for completed analysis results.
// If not available, it writes a pending request for the image-analyzer service
// and waits briefly for completion (hybrid async mode).
func (c *ImageRegistryCollector) Enrich(
	ctx context.Context,
	evidence *intent.IntentEvidence,
) {
	if evidence.Image == "" {
		return
	}

	imageRef := evidence.Image

	// 1. Check cache for completed result
	regHost, repo, tag := parseImageRef(imageRef)
	cached, err := c.cacheFacade.GetByTagRef(ctx, regHost, repo, tag)
	if err == nil && cached != nil && cached.Status == "completed" && !c.isCacheExpired(cached) {
		log.Debugf("ImageRegistryCollector: cache hit for %s (completed)", imageRef)
		c.applyCache(cached, evidence)
		return
	}

	// 2. Write a pending request if no entry or previously failed
	if cached == nil || cached.Status == "failed" || cached.Status == "" {
		namespace := evidence.WorkloadNamespace
		if namespace == "" {
			namespace = "primus-lens"
		}
		log.Infof("ImageRegistryCollector: writing pending request for %s (namespace=%s)", imageRef, namespace)
		_, err := c.cacheFacade.UpsertPending(ctx, imageRef, namespace)
		if err != nil {
			log.Warnf("ImageRegistryCollector: failed to write pending request for %s: %v", imageRef, err)
			return
		}
	}

	// If already processing, just wait
	if cached != nil && cached.Status == "processing" {
		log.Debugf("ImageRegistryCollector: %s is being processed, waiting...", imageRef)
	}

	// 3. Hybrid wait: poll cache for up to hybridWaitTimeout
	deadline := time.After(hybridWaitTimeout)
	ticker := time.NewTicker(hybridPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Debugf("ImageRegistryCollector: context cancelled while waiting for %s", imageRef)
			return
		case <-deadline:
			log.Debugf("ImageRegistryCollector: timeout waiting for %s, will retry in monitoring cycle", imageRef)
			return
		case <-ticker.C:
			cached, err = c.cacheFacade.GetByTagRef(ctx, regHost, repo, tag)
			if err != nil {
				continue
			}
			if cached != nil && cached.Status == "completed" {
				log.Infof("ImageRegistryCollector: analysis completed for %s during wait", imageRef)
				c.applyCache(cached, evidence)
				return
			}
			if cached != nil && cached.Status == "failed" {
				log.Warnf("ImageRegistryCollector: analysis failed for %s: %s", imageRef, cached.ErrorMessage)
				return
			}
		}
	}
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
