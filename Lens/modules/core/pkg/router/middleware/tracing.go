package middleware

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

func HandleTracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// 尝试从 HTTP header 中提取 span context
		operationName := c.Request.Method + " " + c.Request.URL.Path
		var span opentracing.Span

		spanCtx, err := trace.ExtractHeader(ctx, c.Request.Header, operationName)
		if err != nil {
			// 如果没有 parent span，创建新的 root span
			span, ctx = trace.StartSpanFromContext(ctx, operationName)
		} else {
			// 有 parent span，使用提取的 context
			ctx = spanCtx
			if s, ok := trace.SpanFromContext(ctx); ok {
				span = s
			}
		}

		// 设置 HTTP 相关的 tags
		if span != nil {
			ext.HTTPMethod.Set(span, c.Request.Method)
			ext.HTTPUrl.Set(span, c.Request.URL.String())
			ext.Component.Set(span, "gin-http")
			span.SetTag("http.path", c.Request.URL.Path)

			// 在请求完成后设置状态码
			defer func() {
				ext.HTTPStatusCode.Set(span, uint16(c.Writer.Status()))
				if c.Writer.Status() >= 400 {
					ext.Error.Set(span, true)
				}
				trace.FinishSpan(span)
			}()
		}

		// 将更新后的 context 放回 request
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
