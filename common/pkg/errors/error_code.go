/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package errors

import (
	"fmt"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const SafePrefix = "Safe."

// public: 00xxxx
const (
	InternalError         = SafePrefix + "000001"
	BadRequest            = SafePrefix + "000002"
	Forbidden             = SafePrefix + "000003"
	AlreadyExist          = SafePrefix + "000004"
	NotFound              = SafePrefix + "000005"
	RequestEntityTooLarge = SafePrefix + "000006"
	UnsupportedMediaType  = SafePrefix + "000007"
	QuotaInsufficient     = SafePrefix + "000008"
	Unauthorized          = SafePrefix + "000009"
	StatusGone            = SafePrefix + "000010"
	ResourceProcessing    = SafePrefix + "000011"
	Timeout               = SafePrefix + "000012"
	UserNotRegistered     = SafePrefix + "000013"
	Success               = SafePrefix + "000000"
)

// workload: 01xxxx
const (
	WorkloadNotFound         = SafePrefix + "010001"
	ResourceTemplateNotFound = SafePrefix + "010002"
)

// workspace: 02xxxx
const (
	WorkspaceNotFound = SafePrefix + "020001"
)

// node: 03xxxx
const (
	NodeIsNotReady = SafePrefix + "030001"
)

// IsSafeError returns true if the specified error reason is safe error.
func IsSafeError(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(string(apierrors.ReasonForError(err)), SafePrefix)
}

func IgnoreSafeError(err error) error {
	if IsSafeError(err) {
		return nil
	}
	return err
}

func IsAlreadyExist(err error) bool {
	return apierrors.ReasonForError(err) == AlreadyExist
}

func IsBadRequest(err error) bool {
	return apierrors.ReasonForError(err) == BadRequest
}

func GetErrorCode(err error) string {
	if err == nil || !IsSafeError(err) {
		return ""
	}
	return string(apierrors.ReasonForError(err))
}

func NewBadRequest(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusBadRequest,
		Reason:  BadRequest,
		Message: fmt.Sprintf("Bad request. %s", message),
	}}
}

func NewBadRequestWithRawMsg(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusBadRequest,
		Reason:  BadRequest,
		Message: message,
	}}
}

func NewInternalError(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusInternalServerError,
		Reason:  InternalError,
		Message: fmt.Sprintf("Internal error. %s", message),
	}}
}

func NewAlreadyExist(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusConflict,
		Reason:  AlreadyExist,
		Message: message,
	}}
}

func NewForbidden(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusForbidden,
		Reason:  Forbidden,
		Message: message,
	}}
}

func NotFoundErrorCode(resource string) metav1.StatusReason {
	switch strings.ToLower(resource) {
	case "workloads":
		return WorkloadNotFound
	case "workspaces":
		return WorkspaceNotFound
	default:
		return NotFound
	}
}

func NewNotFound(resource, name string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure,
		Code:   http.StatusNotFound,
		Reason: NotFoundErrorCode(resource),
		Details: &metav1.StatusDetails{
			Kind: resource,
			Name: name,
		},
		Message: fmt.Sprintf("%s %s not found.", resource, name),
	}}
}

func NewNotFoundWithMessage(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusNotFound,
		Reason:  NotFound,
		Message: message,
	}}
}

func NewRequestEntityTooLargeError(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusRequestEntityTooLarge,
		Reason:  RequestEntityTooLarge,
		Message: fmt.Sprintf("Request entity is too large: %s", message),
	}}
}

func NewUnsupportedMediaType(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusUnsupportedMediaType,
		Reason:  UnsupportedMediaType,
		Message: fmt.Sprintf("UnsupportedMediaType: %s", message),
	}}
}

func NewQuotaInsufficient(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusInternalServerError,
		Reason:  QuotaInsufficient,
		Message: message,
	}}
}

func NewUnauthorized(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusUnauthorized,
		Reason:  Unauthorized,
		Message: message,
	}}
}

func NewUserNotRegistered() *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusUnauthorized,
		Reason:  UserNotRegistered,
		Message: "the user is not registered",
	}}
}

func NewStatusGone(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusGone,
		Reason:  StatusGone,
		Message: message,
	}}
}

func NewResourceProcessing(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusConflict,
		Reason:  ResourceProcessing,
		Message: message,
	}}
}

func NewNodeIsNotReady(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusServiceUnavailable,
		Reason:  NodeIsNotReady,
		Message: message,
	}}
}

func NewTimeout(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusRequestTimeout,
		Reason:  Timeout,
		Message: message,
	}}
}
