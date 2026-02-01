/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestParseListPersistentVolumeQuery(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantErr     bool
		workspaceID string
	}{
		{
			name:        "valid workspaceId",
			url:         "/api/v1/persistentvolumes?workspaceId=test-workspace",
			wantErr:     false,
			workspaceID: "test-workspace",
		},
		{
			name:    "missing workspaceId",
			url:     "/api/v1/persistentvolumes",
			wantErr: true,
		},
		{
			name:    "empty workspaceId",
			url:     "/api/v1/persistentvolumes?workspaceId=",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsp := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rsp)
			c.Request = httptest.NewRequest(http.MethodGet, tt.url, nil)

			query, err := parseListPersistentVolumeQuery(c)
			if tt.wantErr {
				assert.Assert(t, err != nil)
			} else {
				assert.NilError(t, err)
				assert.Equal(t, query.WorkspaceID, tt.workspaceID)
			}
		})
	}
}

func TestBuildListPersistentVolumeSelector(t *testing.T) {
	tests := []struct {
		name        string
		query       *view.ListPersistentVolumeRequest
		wantEmpty   bool
		shouldMatch labels.Set
	}{
		{
			name:        "empty workspaceId",
			query:       &view.ListPersistentVolumeRequest{WorkspaceID: ""},
			wantEmpty:   true,
			shouldMatch: nil,
		},
		{
			name:        "with workspaceId",
			query:       &view.ListPersistentVolumeRequest{WorkspaceID: "test-workspace"},
			wantEmpty:   false,
			shouldMatch: labels.Set{v1.WorkspaceIdLabel: "test-workspace"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector, err := buildListPersistentVolumeSelector(tt.query)
			assert.NilError(t, err)

			if tt.wantEmpty {
				assert.Equal(t, selector.Empty(), true)
			} else {
				assert.Equal(t, selector.Matches(tt.shouldMatch), true)
			}
		})
	}
}

func TestCvtToPersistentVolumeItem(t *testing.T) {
	volumeMode := corev1.PersistentVolumeFilesystem
	storageClass := "standard"

	pv := corev1.PersistentVolume{
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("100Gi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			ClaimRef: &corev1.ObjectReference{
				Name:      "test-pvc",
				Namespace: "default",
			},
			VolumeMode:                    &volumeMode,
			StorageClassName:              storageClass,
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
		},
		Status: corev1.PersistentVolumeStatus{
			Phase:   corev1.VolumeBound,
			Message: "test message",
		},
	}
	pv.Labels = map[string]string{
		common.PfsSelectorKey: "pfs-test-value",
		"other-label":         "other-value",
	}

	result := cvtToPersistentVolumeItem(pv)

	// Verify capacity
	assert.Equal(t, result.Capacity.Storage().String(), "100Gi")

	// Verify access modes
	assert.Equal(t, len(result.AccessModes), 1)
	assert.Equal(t, result.AccessModes[0], corev1.ReadWriteMany)

	// Verify claim ref
	assert.Assert(t, result.ClaimRef != nil)
	assert.Equal(t, result.ClaimRef.Name, "test-pvc")
	assert.Equal(t, result.ClaimRef.Namespace, "default")

	// Verify volume mode
	assert.Assert(t, result.VolumeMode != nil)
	assert.Equal(t, *result.VolumeMode, corev1.PersistentVolumeFilesystem)

	// Verify storage class
	assert.Equal(t, result.StorageClassName, storageClass)

	// Verify reclaim policy
	assert.Equal(t, result.PersistentVolumeReclaimPolicy, corev1.PersistentVolumeReclaimRetain)

	// Verify status
	assert.Equal(t, result.Phase, corev1.VolumeBound)
	assert.Equal(t, result.Message, "test message")

	// Verify labels - only PfsSelectorKey should be included
	assert.Equal(t, result.Labels[common.PfsSelectorKey], "pfs-test-value")
	_, hasOtherLabel := result.Labels["other-label"]
	assert.Equal(t, hasOtherLabel, false)
}
