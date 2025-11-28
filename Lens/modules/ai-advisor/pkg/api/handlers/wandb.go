package handlers

import (
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
	var req detection.WandBDetectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warnf("Failed to parse WandB detection request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request",
			"details": err.Error(),
		})
		return
	}

	// Process detection
	err := h.wandbDetector.ProcessWandBDetection(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to process WandB detection: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to process detection",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Detection processed successfully",
	})
}

