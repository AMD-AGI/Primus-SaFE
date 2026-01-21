// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package types

import (
	coretypes "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/types"
)

// Re-export all types from core for backward compatibility
// This allows existing code to continue using this package while the types are defined in core

// Process Tree Types
type ProcessTreeRequest = coretypes.ProcessTreeRequest
type ProcessInfo = coretypes.ProcessInfo
type ContainerProcessTree = coretypes.ContainerProcessTree
type PodProcessTree = coretypes.PodProcessTree

// Container Filesystem Types
type ContainerFileInfo = coretypes.ContainerFileInfo
type ContainerFileReadRequest = coretypes.ContainerFileReadRequest
type ContainerFileReadResponse = coretypes.ContainerFileReadResponse
type ContainerDirectoryListRequest = coretypes.ContainerDirectoryListRequest
type ContainerDirectoryListResponse = coretypes.ContainerDirectoryListResponse
type TensorBoardLogInfo = coretypes.TensorBoardLogInfo

// TensorBoard Detection Types
type TensorboardFileInfo = coretypes.TensorboardFileInfo
type TensorboardFilesResponse = coretypes.TensorboardFilesResponse

// Process Environment & Arguments Types
type ProcessEnvRequest = coretypes.ProcessEnvRequest
type ProcessEnvResponse = coretypes.ProcessEnvResponse
type ProcessEnvInfo = coretypes.ProcessEnvInfo
type ProcessArgsRequest = coretypes.ProcessArgsRequest
type ProcessArgsResponse = coretypes.ProcessArgsResponse
type ProcessArgInfo = coretypes.ProcessArgInfo
