package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/detection"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// WandBHandler handles WandB detection requests
type WandBHandler struct {
	wandbDetector *detection.WandBFrameworkDetector
}

// NewWandBHandler creates a new WandB handler
func NewWandBHandler(detector *detection.WandBFrameworkDetector) *WandBHandler {
	return &WandBHandler{
		wandbDetector: detector,
	}
}

// ReceiveDetection handles WandB detection data upload
//
// POST /api/v1/wandb/detection
func (h *WandBHandler) ReceiveDetection(c *gin.Context) {
	log.Info("====== [AI-Advisor WandB Detection API] Received request ======")

	var req detection.WandBDetectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warnf("[AI-Advisor WandB Detection API] Failed to parse request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request",
			"details": err.Error(),
		})
		return
	}

	// Print request body details (Debug level)
	if reqJSON, err := json.MarshalIndent(req, "", "  "); err == nil {
		log.Debugf("[AI-Advisor WandB Detection API] Request body:\n%s", string(reqJSON))
	}

	// Log output supporting dual-layer framework
	if len(req.Hints.WrapperFrameworks) > 0 || len(req.Hints.BaseFrameworks) > 0 {
		log.Infof("[AI-Advisor WandB Detection API] Detection request (dual-layer) - WorkloadUID: %s, PodName: %s, RunID: %s, Wrapper: %v, Base: %v",
			req.WorkloadUID, req.PodName, req.Evidence.WandB.ID, req.Hints.WrapperFrameworks, req.Hints.BaseFrameworks)
	} else {
		// Backward compatible: old format
		log.Infof("[AI-Advisor WandB Detection API] Detection request - WorkloadUID: %s, PodName: %s, RunID: %s, PossibleFrameworks: %v",
			req.WorkloadUID, req.PodName, req.Evidence.WandB.ID, req.Hints.PossibleFrameworks)
	}

	// Print key fields information
	log.Debugf("[AI-Advisor WandB Detection API] Key fields - Source: %s, Type: %s, Version: %s, Namespace: %s, Confidence: %s",
		req.Source, req.Type, req.Version, req.Namespace, req.Hints.Confidence)

	// Process detection
	log.Infof("[AI-Advisor WandB Detection API] Starting detection processing...")
	err := h.wandbDetector.ProcessWandBDetection(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("[AI-Advisor WandB Detection API] Failed to process detection: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to process detection",
			"details": err.Error(),
		})
		return
	}

	// Success log supporting dual-layer framework
	if len(req.Hints.WrapperFrameworks) > 0 || len(req.Hints.BaseFrameworks) > 0 {
		log.Infof("[AI-Advisor WandB Detection API] ✓ Detection processed successfully (dual-layer) - Wrapper: %v, Base: %v, WorkloadUID: %s",
			req.Hints.WrapperFrameworks, req.Hints.BaseFrameworks, req.WorkloadUID)
	} else {
		log.Infof("[AI-Advisor WandB Detection API] ✓ Detection processed successfully - PossibleFrameworks: %v, WorkloadUID: %s",
			req.Hints.PossibleFrameworks, req.WorkloadUID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Detection processed successfully",
	})
}
