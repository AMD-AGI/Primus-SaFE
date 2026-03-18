// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/service"
	"github.com/gin-gonic/gin"
)

// ErrorCode defines standard error codes for the API
type ErrorCode string

const (
	// Client errors (4xx)
	ErrCodeBadRequest       ErrorCode = "BAD_REQUEST"
	ErrCodeInvalidParameter ErrorCode = "INVALID_PARAMETER"
	ErrCodeUnauthorized     ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden        ErrorCode = "FORBIDDEN"
	ErrCodeNotFound         ErrorCode = "NOT_FOUND"
	ErrCodeConflict         ErrorCode = "CONFLICT"
	ErrCodeTooLarge         ErrorCode = "PAYLOAD_TOO_LARGE"

	// Server errors (5xx)
	ErrCodeInternalError      ErrorCode = "INTERNAL_ERROR"
	ErrCodeNotConfigured      ErrorCode = "SERVICE_NOT_CONFIGURED"
	ErrCodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"

	// Business logic errors
	ErrCodeToolNotFound      ErrorCode = "TOOL_NOT_FOUND"
	ErrCodeToolAlreadyLiked  ErrorCode = "TOOL_ALREADY_LIKED"
	ErrCodeAccessDenied      ErrorCode = "ACCESS_DENIED"
	ErrCodeInvalidFileType   ErrorCode = "INVALID_FILE_TYPE"
	ErrCodeFileTooLarge      ErrorCode = "FILE_TOO_LARGE"
	ErrCodeSkillNotSupported ErrorCode = "SKILL_NOT_SUPPORTED"
)

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}

// respondWithError sends a standardized error response
func respondWithError(c *gin.Context, statusCode int, errorCode ErrorCode, message string) {
	response := ErrorResponse{
		ErrorCode:    string(errorCode),
		ErrorMessage: message,
	}
	c.JSON(statusCode, response)
}

// respondBadRequest sends a 400 Bad Request error
func respondBadRequest(c *gin.Context, message string, detail ...string) {
	if len(detail) > 0 {
		message = message + ": " + detail[0]
	}
	respondWithError(c, http.StatusBadRequest, ErrCodeBadRequest, message)
}

// respondInvalidParameter sends a 400 Bad Request error for invalid parameters
func respondInvalidParameter(c *gin.Context, paramName string, detail ...string) {
	message := "Invalid parameter: " + paramName
	if len(detail) > 0 {
		message = message + ". " + detail[0]
	}
	respondWithError(c, http.StatusBadRequest, ErrCodeInvalidParameter, message)
}

// respondUnauthorized sends a 401 Unauthorized error
func respondUnauthorized(c *gin.Context, message string) {
	respondWithError(c, http.StatusUnauthorized, ErrCodeUnauthorized, message)
}

// respondForbidden sends a 403 Forbidden error
func respondForbidden(c *gin.Context, message string) {
	respondWithError(c, http.StatusForbidden, ErrCodeForbidden, message)
}

// respondNotFound sends a 404 Not Found error
func respondNotFound(c *gin.Context, resource string) {
	message := resource + " not found"
	respondWithError(c, http.StatusNotFound, ErrCodeNotFound, message)
}

// respondConflict sends a 409 Conflict error
func respondConflict(c *gin.Context, message string) {
	respondWithError(c, http.StatusConflict, ErrCodeConflict, message)
}

// respondInternalError sends a 500 Internal Server Error
func respondInternalError(c *gin.Context, detail ...string) {
	message := "Internal server error"
	if len(detail) > 0 {
		message = message + ": " + detail[0]
	}
	respondWithError(c, http.StatusInternalServerError, ErrCodeInternalError, message)
}

// respondServiceError maps service errors to standardized HTTP error responses
func respondServiceError(c *gin.Context, err error) {
	switch err {
	case service.ErrNotFound:
		respondWithError(c, http.StatusNotFound, ErrCodeToolNotFound, "Tool not found")
	case service.ErrAccessDenied:
		respondWithError(c, http.StatusForbidden, ErrCodeAccessDenied, "Access denied")
	case service.ErrNotConfigured:
		respondWithError(c, http.StatusServiceUnavailable, ErrCodeNotConfigured, "Service does not configured")
	case service.ErrAlreadyLiked:
		respondWithError(c, http.StatusConflict, ErrCodeToolAlreadyLiked, "Tool already liked")
	default:
		respondInternalError(c, err.Error())
	}
}
