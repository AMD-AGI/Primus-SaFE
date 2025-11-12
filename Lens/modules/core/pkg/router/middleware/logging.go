package middleware

import (
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

func HandleLogging() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Record request start time
		startTime := time.Now()

		// Process request
		c.Next()

		// Calculate response time
		duration := time.Since(startTime)

		// Get request information
		statusCode := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()

		// Print log
		log.GlobalLogger().WithContext(c).Infof(
			"Request: Method=%s | Path=%s | Status=%d | IP=%s | Duration=%v | UserAgent=%s",
			method,
			path,
			statusCode,
			clientIP,
			duration,
			userAgent,
		)
	}
}
