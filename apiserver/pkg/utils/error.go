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

type PrimusApiError struct {
	HttpCode     int    `json:"-"`
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}

func (err *PrimusApiError) Error() string {
	return err.ErrorMessage
}

func AbortWithApiError(c *gin.Context, err error) {
	handleErrors(c, err)
	rsp := cvtToErrResponse(err)
	c.AbortWithStatusJSON(rsp.HttpCode, rsp)
}

func cvtToErrResponse(err error) PrimusApiError {
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

func handleErrors(c *gin.Context, err error) {
	var errs []error
	if aggregate, ok := err.(utilerrors.Aggregate); ok {
		errs = aggregate.Errors()
	} else {
		errs = []error{err}
	}
	for _, val := range errs {
		if val != nil {
			// 在 Gin 框架中，c.Error() 用于将错误信息与请求关联起来，并传递给日志记录中间件或其他处理错误的中间件
			_ = c.Error(val)
		}
	}
}
