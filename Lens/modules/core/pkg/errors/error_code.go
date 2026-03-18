// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package errors

const (
	RequestParameterInvalid int = 4001
	RequestDataExists       int = 4002
	AuthFailed              int = 4003
	RequestDataNotExisted   int = 4004
	PermissionDeny          int = 4005
	InvalidOperation        int = 4016
	InvalidArgument         int = 4017

	InternalError     int = 5000
	InvalidDataError  int = 5001
	CodeDatabaseError     = 5002

	ClientError       int = 6001
	K8SOperationError int = 6002
	OpensearchError   int = 6003

	CodeInitializeError = 7001
	CodeLackOfConfig    = 7002

	CodeRemoteServiceError = 8001
	CodeInvalidArgument    = 8002

	ServiceUnavailable = 5003
)
