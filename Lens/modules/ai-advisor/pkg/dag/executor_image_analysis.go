// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/pipeline"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/registry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ImageAnalysisExecutor is the T1 executor that resolves the container image
// reference and performs inline OCI image analysis.
type ImageAnalysisExecutor struct {
	specCollector *pipeline.SpecCollector
	analyzer      *registry.InlineImageAnalyzer
	podFacade     database.PodFacadeInterface
}

// NewImageAnalysisExecutor creates a T1 executor.
func NewImageAnalysisExecutor() *ImageAnalysisExecutor {
	return &ImageAnalysisExecutor{
		specCollector: pipeline.NewSpecCollector(),
		analyzer:      registry.NewInlineImageAnalyzer(),
		podFacade:     database.NewPodFacade(),
	}
}

// Execute collects the image reference from the workload spec, then runs
// inline image analysis and stores the result.
func (e *ImageAnalysisExecutor) Execute(ctx context.Context, master *MasterTask, sub *SubTask) error {
	evidence, err := e.specCollector.Collect(ctx, master.WorkloadUID)
	if err != nil {
		return fmt.Errorf("spec collection failed: %w", err)
	}

	imageRef := evidence.Image
	if imageRef == "" {
		return fmt.Errorf("no image reference found for workload %s", master.WorkloadUID)
	}

	namespace := evidence.WorkloadNamespace
	result, err := e.analyzer.AnalyzeOrCache(ctx, imageRef, namespace)
	if err != nil {
		return fmt.Errorf("image analysis failed for %s: %w", imageRef, err)
	}

	sub.Result = map[string]interface{}{
		"image_ref":          imageRef,
		"digest":             result.Digest,
		"base_image":         result.BaseImage,
		"layer_count":        result.LayerCount,
		"installed_packages": result.InstalledPackages,
		"framework_hints":    result.FrameworkHints,
	}

	log.Infof("ImageAnalysisExecutor: completed analysis for %s (digest=%s, packages=%d)",
		imageRef, result.Digest, len(result.InstalledPackages))
	return nil
}
