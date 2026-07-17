// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests processed, by method, route and status code.",
	}, []string{"method", "code", "handler"})

	httpRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds, by method, route and status code.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "code", "handler"})

	httpRequestsInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Number of HTTP requests currently being served.",
	})
)

func init() {
	// Register on the controller-runtime registry so the series are exposed on
	// the apiserver's existing /metrics endpoint and scraped by the monitoring
	// infrastructure.
	ctrlmetrics.Registry.MustRegister(httpRequestsTotal, httpRequestDuration, httpRequestsInFlight)
}

// HandleMetrics records Prometheus-native HTTP server metrics (RED method).
// The matched route template (c.FullPath) is used as the handler label to keep
// cardinality bounded. Reverse-proxy/SSE routes are excluded (same as tracing)
// so long-lived streams do not skew the latency histogram.
func HandleMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isProxyRoute(c.Request.URL.Path) {
			c.Next()
			return
		}

		start := time.Now()
		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		c.Next()

		handler := c.FullPath()
		if handler == "" {
			handler = "unmatched"
		}
		code := strconv.Itoa(c.Writer.Status())
		httpRequestsTotal.WithLabelValues(c.Request.Method, code, handler).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, code, handler).Observe(time.Since(start).Seconds())
	}
}
