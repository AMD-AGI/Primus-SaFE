package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// httpRequestsTotal counts total HTTP requests
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// httpRequestErrorsTotal counts HTTP requests that resulted in errors (4xx and 5xx)
	httpRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_errors_total",
			Help: "Total number of HTTP request errors (4xx and 5xx status codes)",
		},
		[]string{"method", "path", "status"},
	)

	// httpRequestDuration measures HTTP request duration in seconds
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	// httpRequestsInFlight tracks the number of in-flight HTTP requests
	httpRequestsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed",
		},
		[]string{"method"},
	)
)

// HandleMetrics returns a gin middleware that records HTTP metrics
func HandleMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip metrics endpoint to avoid self-referential metrics
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		startTime := time.Now()
		method := c.Request.Method

		// Use FullPath for better grouping (e.g., /api/users/:id instead of /api/users/123)
		// Fall back to URL.Path if FullPath is empty (for unmatched routes)
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		// Track in-flight requests
		httpRequestsInFlight.WithLabelValues(method).Inc()
		defer httpRequestsInFlight.WithLabelValues(method).Dec()

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(startTime).Seconds()

		// Get status code
		statusCode := c.Writer.Status()
		statusStr := strconv.Itoa(statusCode)

		// Record request count
		httpRequestsTotal.WithLabelValues(method, path, statusStr).Inc()

		// Record error count for 4xx and 5xx status codes
		if statusCode >= 400 {
			httpRequestErrorsTotal.WithLabelValues(method, path, statusStr).Inc()
		}

		// Record request duration
		httpRequestDuration.WithLabelValues(method, path).Observe(duration)
	}
}

