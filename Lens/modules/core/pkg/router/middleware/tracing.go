package middleware

import (
	"context"
	"github.com/AMD-AGI/primus-lens/core/pkg/trace"
	"github.com/gin-gonic/gin"
)

func HandleTracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Context(c)
		ctx, err := trace.ExtractHeader(ctx, c.Request.Header, c.Request.Method+" "+c.Request.URL.Path)
		if err != nil {
			_, ctx = trace.StartSpanFromContext(ctx, c.Request.Method+" "+c.Request.URL.Path)
		}
		c.Next()
	}
}
