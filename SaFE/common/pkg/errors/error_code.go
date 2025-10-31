/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package errors

import (
	"fmt"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const PrimusPrefix = "Primus."

/*
   5-digit Error Code Convention: [xx][yyy]
   [xx] Business ID (00–99), used to distinguish errors from different business interfaces.
   00: General errors
   01: Workload-related errors
   02: Workspace-related errors
   03: Node-related errors
   [yyy] Error code range (000–999)
*/

// public: 00xxxx
const (
	InternalError         = PrimusPrefix + "00001"
	BadRequest            = PrimusPrefix + "00002"
	Forbidden             = PrimusPrefix + "00003"
	AlreadyExist          = PrimusPrefix + "00004"
	NotFound              = PrimusPrefix + "00005"
	RequestEntityTooLarge = PrimusPrefix + "00006"
	NotImplemented        = PrimusPrefix + "00007"
	QuotaInsufficient     = PrimusPrefix + "00008"
	Unauthorized          = PrimusPrefix + "00009"
	ResourceProcessing    = PrimusPrefix + "00010"
	UserNotRegistered     = PrimusPrefix + "00011"
)

// workload: 01xxx
const (
	WorkloadNotFound         = PrimusPrefix + "01001"
	ResourceTemplateNotFound = PrimusPrefix + "01002"
)

// workspace: 02xxx
const (
	WorkspaceNotFound = PrimusPrefix + "02001"
)

// node: 03xxx
const (
	NodeNotReady = PrimusPrefix + "03001"
	NodeNotFound = PrimusPrefix + "03002"
)

// IsPrimus returns true if the condition is met.
func IsPrimus(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(string(apierrors.ReasonForError(err)), PrimusPrefix)
}

// IsAlreadyExist checks if an error indicates a resource already exists.
func IsAlreadyExist(err error) bool {
	return apierrors.ReasonForError(err) == AlreadyExist
}

// IsBadRequest checks if an error indicates an invalid request.
func IsBadRequest(err error) bool {
	return apierrors.ReasonForError(err) == BadRequest
}

// IsInternal checks if an error is an internal server error.
func IsInternal(err error) bool {
	return apierrors.ReasonForError(err) == InternalError
}

// IsForbidden checks if an error indicates a forbidden operation.
func IsForbidden(err error) bool {
	return apierrors.ReasonForError(err) == Forbidden
}

// IsNotFound checks if an error indicates a resource was not found.
func IsNotFound(err error) bool {
	reason := apierrors.ReasonForError(err)
	if reason == NotFound || reason == WorkloadNotFound || reason == WorkspaceNotFound ||
		reason == NodeNotFound || reason == ResourceTemplateNotFound {
		return true
	}
	return false
}

// IgnoreFound returns nil if the error is a not-found error, otherwise returns the error.
func IgnoreFound(err error) error {
	if err == nil || IsNotFound(err) {
		return nil
	}
	return err
}

// GetErrorCode extracts the error code from a Primus error.
func GetErrorCode(err error) string {
	if err == nil || !IsPrimus(err) {
		return ""
	}
	return string(apierrors.ReasonForError(err))
}

// NewBadRequest creates a new bad request error with the given message.
func NewBadRequest(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusBadRequest,
		Reason:  BadRequest,
		Message: fmt.Sprintf("Bad request. %s", message),
	}}
}

// NewInternalError creates a new internal server error with the given message.
func NewInternalError(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusInternalServerError,
		Reason:  InternalError,
		Message: fmt.Sprintf("Internal error. %s", message),
	}}
}

// NewAlreadyExist creates a new "already exists" error with the given message.
func NewAlreadyExist(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusConflict,
		Reason:  AlreadyExist,
		Message: message,
	}}
}

// NewForbidden creates a new forbidden error with the given message.
func NewForbidden(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusForbidden,
		Reason:  Forbidden,
		Message: message,
	}}
}

// NotFoundErrorCode generates a not-found error code for the specified resource kind.
func NotFoundErrorCode(kind string) metav1.StatusReason {
	switch kind {
	case v1.WorkloadKind:
		return WorkloadNotFound
	case v1.ResourceTemplateKind:
		return ResourceTemplateNotFound
	case v1.WorkspaceKind:
		return WorkspaceNotFound
	case v1.NodeKind:
		return NodeNotFound
	default:
		return NotFound
	}
}

// NewNotFound creates a new not-found error for the specified resource.
func NewNotFound(kind, name string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure,
		Code:   http.StatusNotFound,
		Reason: NotFoundErrorCode(kind),
		Details: &metav1.StatusDetails{
			Kind: kind,
			Name: name,
		},
		Message: fmt.Sprintf("%s %s not found.", kind, name),
	}}
}

// NewNotFoundWithMessage creates a new not-found error with a custom message.
func NewNotFoundWithMessage(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusNotFound,
		Reason:  NotFound,
		Message: message,
	}}
}

// NewRequestEntityTooLargeError creates a new request entity too large error.
func NewRequestEntityTooLargeError(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusRequestEntityTooLarge,
		Reason:  RequestEntityTooLarge,
		Message: fmt.Sprintf("Request entity is too large: %s", message),
	}}
}

// NewQuotaInsufficient creates a new quota insufficient error.
func NewQuotaInsufficient(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusServiceUnavailable,
		Reason:  QuotaInsufficient,
		Message: message,
	}}
}

// NewUnauthorized creates a new unauthorized error.
func NewUnauthorized(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusUnauthorized,
		Reason:  Unauthorized,
		Message: message,
	}}
}

// NewUserNotRegistered creates a new user not registered error.
func NewUserNotRegistered(userId string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusUnauthorized,
		Reason:  UserNotRegistered,
		Message: fmt.Sprintf("the user(%s) is not registered or invalid userId", userId),
	}}
}

// NewResourceProcessing creates a new resource processing error.
func NewResourceProcessing(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusConflict,
		Reason:  ResourceProcessing,
		Message: message,
	}}
}

// NewNodeNotReady creates a new node not ready error.
func NewNodeNotReady(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusServiceUnavailable,
		Reason:  NodeNotReady,
		Message: message,
	}}
}

// NewNotImplemented creates a new not implemented error.
func NewNotImplemented(message string) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusNotImplemented,
		Reason:  NotImplemented,
		Message: message,
	}}
}
