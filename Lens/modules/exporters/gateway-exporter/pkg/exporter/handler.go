package exporter

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// handleListCollectors handles the list collectors request
func (e *Exporter) handleListCollectors(c *gin.Context) {
	collectors := e.manager.ListCollectors()
	c.JSON(http.StatusOK, rest.SuccessResp(c, map[string]interface{}{
		"collectors": collectors,
	}))
}

// handleHealthCheck handles the health check request
func (e *Exporter) handleHealthCheck(c *gin.Context) {
	healthResults := e.manager.HealthCheck(c.Request.Context())

	allHealthy := true
	results := make(map[string]string)
	for name, err := range healthResults {
		if err != nil {
			allHealthy = false
			results[name] = err.Error()
		} else {
			results[name] = "healthy"
		}
	}

	status := http.StatusOK
	if !allHealthy {
		status = http.StatusServiceUnavailable
	}

	c.JSON(status, rest.SuccessResp(c, map[string]interface{}{
		"healthy":    allHealthy,
		"collectors": results,
	}))
}

// handleCacheStats handles the cache stats request
func (e *Exporter) handleCacheStats(c *gin.Context) {
	size, oldest := e.enricher.CacheStats()

	e.mu.RLock()
	lastCollected := e.lastCollected
	metricsCount := len(e.metricsCache)
	e.mu.RUnlock()

	c.JSON(http.StatusOK, rest.SuccessResp(c, map[string]interface{}{
		"cache_size":     size,
		"oldest_entry":   oldest,
		"last_collected": lastCollected,
		"metrics_count":  metricsCount,
	}))
}
