/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// Logger: returns a Gin middleware function that logs HTTP request and response information.
// It captures request details, response status, latency, and any errors that occurred during processing.
// The log entries are formatted and written using klog, with automatic flushing after each request.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer klog.Flush()
		start := time.Now().UTC()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		param := gin.LogFormatterParams{
			Request: c.Request,
			Keys:    c.Keys,
		}
		param.TimeStamp = time.Now().UTC()
		param.Latency = param.TimeStamp.Sub(start)
		param.Method = c.Request.Method
		param.ClientIP = c.ClientIP()
		param.StatusCode = c.Writer.Status()
		param.ErrorMessage = errorWrapper(c.Errors.ByType(gin.ErrorTypePrivate))
		param.BodySize = c.Writer.Size()
		if raw != "" {
			path = path + "?" + raw
		}
		param.Path = path
		klog.Info(formatter(param))
	}
}

// formatter: formats the Gin log parameters into a standardized log message string.
// It creates a structured log entry containing timestamp, status code, latency, client IP,
// HTTP method, path, response body size, and any error messages.
func formatter(param gin.LogFormatterParams) string {
	return fmt.Sprintf("[GIN] %v | %d | %v | %s | %s %#v | %d |\n%s",
		param.TimeStamp.Format(time.DateTime),
		param.StatusCode,
		param.Latency,
		param.ClientIP,
		param.Method,
		param.Path,
		param.BodySize,
		param.ErrorMessage,
	)
}

// errorWrapper: processes a slice of Gin errors and formats them into a readable string.
// It handles different types of errors, including commonerrors.Error with special formatting
// for message, code, and stack trace, as well as standard fmt.Formatter errors.
// Returns a formatted string containing all error information or empty string if no errors.
func errorWrapper(errs []*gin.Error) string {
	if len(errs) == 0 {
		return ""
	}
	var buffer strings.Builder
	for i, err := range errs {
		var innerErr *commonerrors.Error
		if errors.As(err.Err, &innerErr) {
			_, _ = fmt.Fprintf(&buffer, "Error #%02d:Message %s.Code %s. Stack %s\n",
				i+1, innerErr.Message, innerErr.Code, innerErr.GetTopStackString())

		} else if _, ok := err.Err.(fmt.Formatter); ok {
			_, _ = fmt.Fprintf(&buffer, "Error #%02d: %+v\n", i+1, err.Err)
		} else {
			_, _ = fmt.Fprintf(&buffer, "Error #%02d: %q\n", i+1, err.Err)
		}
	}
	return buffer.String()
}
