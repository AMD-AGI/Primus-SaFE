/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"errors"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// PrimusApiError Define a unified Primus error response, including HTTP code, error code, and error message.
type PrimusApiError struct {
	HttpCode     int    `json:"-"`
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}

// Error returns the error message string.
func (err *PrimusApiError) Error() string {
	return err.ErrorMessage
}

// AbortWithApiError handles the error, convert it into a standardized error format, and return it to the Gin framework.
// It processes the error using handleErrors and converts it to a PrimusApiError response,
// then aborts the request with a JSON error response.
func AbortWithApiError(c *gin.Context, err error) {
	handleErrors(c, err)
	rsp := convertToErrResponse(err)
	c.AbortWithStatusJSON(rsp.HttpCode, rsp)
}

// convertToErrResponse converts an error into a standardized PrimusApiError format.
// It first checks if the error is already a PrimusApiError, otherwise converts
// standard Kubernetes API errors or other errors into appropriate error responses
// with HTTP codes, error codes, and error messages.
func convertToErrResponse(err error) PrimusApiError {
	var result *PrimusApiError
	if errors.As(err, &result) {
		return *result
	}
	var err2 *apierrors.StatusError
	if !errors.As(err, &err2) {
		switch {
		case apierrors.IsNotFound(err):
			err2 = commonerrors.NewNotFoundWithMessage(err.Error())
		case apierrors.IsBadRequest(err), apierrors.IsInvalid(err):
			err2 = commonerrors.NewBadRequest(err.Error())
		case apierrors.IsAlreadyExists(err):
			err2 = commonerrors.NewAlreadyExist(err.Error())
		case apierrors.IsForbidden(err):
			err2 = commonerrors.NewForbidden(err.Error())
		case apierrors.IsRequestEntityTooLargeError(err):
			err2 = commonerrors.NewRequestEntityTooLargeError(err.Error())
		default:
			err2 = commonerrors.NewInternalError(err.Error())
		}
	}
	return PrimusApiError{
		HttpCode:     int(err2.Status().Code),
		ErrorCode:    string(err2.Status().Reason),
		ErrorMessage: err2.Error(),
	}
}

// handleErrors processes single errors or error aggregates and adds them to the Gin context.
// If the error is an aggregate, it processes each individual error,
// otherwise it adds the single error to the context's error collection.
func handleErrors(c *gin.Context, err error) {
	var errs []error
	if aggregate, ok := err.(utilerrors.Aggregate); ok {
		errs = aggregate.Errors()
	} else {
		errs = []error{err}
	}
	for _, val := range errs {
		if val != nil {
			_ = c.Error(val)
		}
	}
}
