/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	corev1 "k8s.io/api/core/v1"
)

type ListPersistentVolumeRequest struct {
	// Filter results by workspace ID
	WorkspaceID string `form:"workspaceId" binding:"required,max=64"`
}

type ListPersistentVolumeResponse struct {
	// The total number of pv, not limited by pagination
	TotalCount int                    `json:"totalCount"`
	Items      []PersistentVolumeItem `json:"items"`
}

type PersistentVolumeItem struct {
	Labels                        map[string]string                    `json:"labels"`
	Capacity                      corev1.ResourceList                  `json:"capacity,omitempty"`
	AccessModes                   []corev1.PersistentVolumeAccessMode  `json:"accessModes"`
	ClaimRef                      *corev1.ObjectReference              `json:"claimRef,omitempty"`
	VolumeMode                    *corev1.PersistentVolumeMode         `json:"volumeMode,omitempty"`
	StorageClassName              string                               `json:"storageClassName,omitempty"`
	PersistentVolumeReclaimPolicy corev1.PersistentVolumeReclaimPolicy `json:"persistentVolumeReclaimPolicy,omitempty"`
	Phase                         corev1.PersistentVolumePhase         `json:"phase"`
	Message                       string                               `json:"message,omitempty"`
}
