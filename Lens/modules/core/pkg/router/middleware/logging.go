package middleware

import (
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

func HandleLogging() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 记录请求开始时间
		startTime := time.Now()

		// 处理请求
		c.Next()

		// 计算响应时间
		duration := time.Since(startTime)

		// 获取请求信息
		statusCode := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()

		// 打印日志
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

