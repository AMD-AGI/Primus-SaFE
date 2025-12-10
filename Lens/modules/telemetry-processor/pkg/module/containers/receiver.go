package containers

import (
	"net/http"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ReceiveContainerEvent handles single container event HTTP requests
func ReceiveContainerEvent(c *gin.Context) {
	var req ContainerEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Errorf("Failed to bind container event request: %v", err)
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Validate required fields
	if req.Type == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "type is required", nil))
		return
	}
	if req.Source == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "source is required", nil))
		return
	}
	if req.Node == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "node is required", nil))
		return
	}
	if req.ContainerID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "container_id is required", nil))
		return
	}
	if req.Data == nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "data is required", nil))
		return
	}

	// Process event
	if err := ProcessContainerEvent(c.Request.Context(), &req); err != nil {
		log.Errorf("Failed to process container event: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"message": "Container event processed successfully",
	}))
}

// ReceiveBatchContainerEvents handles batch container events HTTP requests
func ReceiveBatchContainerEvents(c *gin.Context) {
	var req BatchContainerEventsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Errorf("Failed to bind batch container events request: %v", err)
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	if len(req.Events) == 0 {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "events cannot be empty", nil))
		return
	}

	// Record batch size
	containerEventBatchSize.Observe(float64(len(req.Events)))

	// Process events concurrently with a semaphore to limit parallelism
	successCount := 0
	errorCount := 0
	var firstError error
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Limit concurrent processing to 10 events at a time
	semaphore := make(chan struct{}, 10)

	for _, event := range req.Events {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		go func(evt ContainerEventRequest) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			if err := ProcessContainerEvent(c.Request.Context(), &evt); err != nil {
				mu.Lock()
				errorCount++
				if firstError == nil {
					firstError = err
				}
				mu.Unlock()
				log.Errorf("Failed to process container event in batch: container=%s, error=%v", evt.ContainerID, err)
			} else {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(event)
	}

	// Wait for all events to be processed
	wg.Wait()

	// Return response
	if errorCount > 0 {
		log.Warnf("Batch container event processing completed with errors: success=%d, error=%d", successCount, errorCount)
		c.JSON(http.StatusPartialContent, rest.ErrorResp(c.Request.Context(), http.StatusPartialContent, firstError.Error(), gin.H{
			"total":   len(req.Events),
			"success": successCount,
			"error":   errorCount,
		}))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"message": "All container events processed successfully",
		"total":   successCount,
	}))
}
