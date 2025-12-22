/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestCreateNodeFlavor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("create node flavor with all fields", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `
{
    "name": "amd-mi325x-256gb-hbm3e",
    "cpu": {
        "product": " AMD_EPYC_9575F",
        "quantity": "256"
    },
    "gpu": {
        "product": " AMD_Instinct_MI325X",
        "resourceName": "amd.com/gpu",
        "quantity": "8"
    },
    "memory": "128Gi",
    "rootDisk": {
        "type": "ssd",
        "quantity": "128Gi",
        "count": 1
    },
    "dataDisk": {
        "type": "nvme",
        "quantity": "1024Gi",
        "count": 8
    },
    "extendedResources": {
        "ephemeral-storage": "800Gi",
        "rdma/hca": "1k"
    }
}`
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/nodeflavors", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		h.CreateNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
		result := &types.CreateNodeFlavorResponse{}
		err := json.Unmarshal(rsp.Body.Bytes(), result)
		assert.NoError(t, err)
		time.Sleep(time.Millisecond * 200)

		nodeFlavor, err := h.getAdminNodeFlavor(context.Background(), result.FlavorId)
		assert.NoError(t, err)
		assert.Equal(t, int64(256), nodeFlavor.Spec.Cpu.Quantity.Value())
		assert.Equal(t, int64(8), nodeFlavor.Spec.Gpu.Quantity.Value())
		assert.Equal(t, int64(128*1024*1024*1024), nodeFlavor.Spec.Memory.Value())
		assert.Equal(t, int64(128*1024*1024*1024), nodeFlavor.Spec.RootDisk.Quantity.Value())
		assert.Equal(t, *resource.NewQuantity(800*1024*1024*1024, resource.BinarySI), nodeFlavor.Spec.ExtendResources[corev1.ResourceEphemeralStorage])
		quantity, _ := resource.ParseQuantity("1k")
		assert.Equal(t, quantity, nodeFlavor.Spec.ExtendResources["rdma/hca"])
	})

	t.Run("create node flavor without optional fields", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `{
    "name": "simple-flavor",
    "cpu": {
        "product": "AMD_EPYC",
        "quantity": "64"
    },
    "memory": "32Gi"
}`
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/nodeflavors", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		h.CreateNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		result := &types.CreateNodeFlavorResponse{}
		err := json.Unmarshal(rsp.Body.Bytes(), result)
		assert.NoError(t, err)
		assert.Equal(t, "simple-flavor", result.FlavorId)
	})

	t.Run("create node flavor with invalid body", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `{invalid json}`
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/nodeflavors", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		h.CreateNodeFlavor(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("create node flavor auto generates ephemeral storage from rootDisk", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `{
    "name": "auto-ephemeral-flavor",
    "cpu": {
        "product": "AMD_EPYC",
        "quantity": "64"
    },
    "memory": "32Gi",
    "rootDisk": {
        "type": "ssd",
        "quantity": "100Gi",
        "count": 2
    }
}`
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/nodeflavors", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		h.CreateNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		result := &types.CreateNodeFlavorResponse{}
		err := json.Unmarshal(rsp.Body.Bytes(), result)
		assert.NoError(t, err)

		nodeFlavor, err := h.getAdminNodeFlavor(context.Background(), result.FlavorId)
		assert.NoError(t, err)
		// ephemeral-storage = rootDisk.Quantity * rootDisk.Count = 100Gi * 2 = 200Gi
		expectedEphemeral := resource.NewQuantity(100*1024*1024*1024*2, resource.BinarySI)
		assert.Equal(t, *expectedEphemeral, nodeFlavor.Spec.ExtendResources[corev1.ResourceEphemeralStorage])
	})
}

func TestListNodeFlavor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("list empty node flavors", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodeflavors", nil)
		c.Set(common.UserId, mockUser.Name)
		h.ListNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		result := &types.ListNodeFlavorResponse{}
		err := json.Unmarshal(rsp.Body.Bytes(), result)
		assert.NoError(t, err)
		assert.Equal(t, 0, result.TotalCount)
		assert.Empty(t, result.Items)
	})

	t.Run("list multiple node flavors", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		// Create two node flavors first
		nf1 := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "flavor-a",
				Labels: map[string]string{
					v1.DisplayNameLabel: "flavor-a",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("32")},
				Memory: resource.MustParse("64Gi"),
			},
		}
		nf2 := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "flavor-b",
				Labels: map[string]string{
					v1.DisplayNameLabel: "flavor-b",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		err := fakeClient.Create(context.Background(), nf1)
		assert.NoError(t, err)
		err = fakeClient.Create(context.Background(), nf2)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodeflavors", nil)
		c.Set(common.UserId, mockUser.Name)
		h.ListNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		result := &types.ListNodeFlavorResponse{}
		err = json.Unmarshal(rsp.Body.Bytes(), result)
		assert.NoError(t, err)
		assert.Equal(t, 2, result.TotalCount)
		// Results should be sorted by FlavorId
		assert.Equal(t, "flavor-a", result.Items[0].FlavorId)
		assert.Equal(t, "flavor-b", result.Items[1].FlavorId)
	})
}

func TestGetNodeFlavor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("get existing node flavor", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-flavor",
				Labels: map[string]string{
					v1.DisplayNameLabel: "test-flavor",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Product: "AMD_EPYC", Quantity: resource.MustParse("128")},
				Memory: resource.MustParse("256Gi"),
				Gpu:    &v1.GpuChip{Product: "MI300X", ResourceName: "amd.com/gpu", Quantity: resource.MustParse("8")},
			},
		}
		err := fakeClient.Create(context.Background(), nf)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodeflavors/test-flavor", nil)
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "test-flavor")
		h.GetNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		result := &types.NodeFlavorResponseItem{}
		err = json.Unmarshal(rsp.Body.Bytes(), result)
		assert.NoError(t, err)
		assert.Equal(t, "test-flavor", result.FlavorId)
		assert.Equal(t, int64(128), result.Cpu.Quantity.Value())
		assert.Equal(t, int64(8), result.Gpu.Quantity.Value())
	})

	t.Run("get non-existing node flavor", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodeflavors/non-existing", nil)
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "non-existing")
		h.GetNodeFlavor(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("get node flavor with empty id", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodeflavors/", nil)
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "")
		h.GetNodeFlavor(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}

func TestPatchNodeFlavor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("patch cpu", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "patch-test-flavor",
				Labels: map[string]string{
					v1.DisplayNameLabel: "patch-test-flavor",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Product: "AMD_EPYC", Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		err := fakeClient.Create(context.Background(), nf)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `{"cpu": {"product": "AMD_EPYC_9554", "quantity": "128"}}`
		c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/nodeflavors/patch-test-flavor", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "patch-test-flavor")
		h.PatchNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		updated, err := h.getAdminNodeFlavor(context.Background(), "patch-test-flavor")
		assert.NoError(t, err)
		assert.Equal(t, int64(128), updated.Spec.Cpu.Quantity.Value())
		assert.Equal(t, "AMD_EPYC_9554", updated.Spec.Cpu.Product)
	})

	t.Run("patch memory", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "patch-memory-flavor",
				Labels: map[string]string{
					v1.DisplayNameLabel: "patch-memory-flavor",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("64Gi"),
			},
		}
		err := fakeClient.Create(context.Background(), nf)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `{"memory": "256Gi"}`
		c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/nodeflavors/patch-memory-flavor", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "patch-memory-flavor")
		h.PatchNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		updated, err := h.getAdminNodeFlavor(context.Background(), "patch-memory-flavor")
		assert.NoError(t, err)
		assert.Equal(t, int64(256*1024*1024*1024), updated.Spec.Memory.Value())
	})

	t.Run("patch gpu", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "patch-gpu-flavor",
				Labels: map[string]string{
					v1.DisplayNameLabel: "patch-gpu-flavor",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		err := fakeClient.Create(context.Background(), nf)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `{"gpu": {"product": "AMD_MI300X", "resourceName": "amd.com/gpu", "quantity": "4"}}`
		c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/nodeflavors/patch-gpu-flavor", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "patch-gpu-flavor")
		h.PatchNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		updated, err := h.getAdminNodeFlavor(context.Background(), "patch-gpu-flavor")
		assert.NoError(t, err)
		assert.NotNil(t, updated.Spec.Gpu)
		assert.Equal(t, int64(4), updated.Spec.Gpu.Quantity.Value())
	})

	t.Run("patch rootDisk", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "patch-rootdisk-flavor",
				Labels: map[string]string{
					v1.DisplayNameLabel: "patch-rootdisk-flavor",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		err := fakeClient.Create(context.Background(), nf)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `{"rootDisk": {"type": "ssd", "quantity": "500Gi", "count": 1}}`
		c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/nodeflavors/patch-rootdisk-flavor", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "patch-rootdisk-flavor")
		h.PatchNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		updated, err := h.getAdminNodeFlavor(context.Background(), "patch-rootdisk-flavor")
		assert.NoError(t, err)
		assert.NotNil(t, updated.Spec.RootDisk)
		assert.Equal(t, int64(500*1024*1024*1024), updated.Spec.RootDisk.Quantity.Value())
	})

	t.Run("patch dataDisk", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "patch-datadisk-flavor",
				Labels: map[string]string{
					v1.DisplayNameLabel: "patch-datadisk-flavor",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		err := fakeClient.Create(context.Background(), nf)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `{"dataDisk": {"type": "nvme", "quantity": "1Ti", "count": 4}}`
		c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/nodeflavors/patch-datadisk-flavor", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "patch-datadisk-flavor")
		h.PatchNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		updated, err := h.getAdminNodeFlavor(context.Background(), "patch-datadisk-flavor")
		assert.NoError(t, err)
		assert.NotNil(t, updated.Spec.DataDisk)
		assert.Equal(t, 4, updated.Spec.DataDisk.Count)
	})

	t.Run("patch extendResources", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "patch-extend-flavor",
				Labels: map[string]string{
					v1.DisplayNameLabel: "patch-extend-flavor",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		err := fakeClient.Create(context.Background(), nf)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `{"extendedResources": {"rdma/hca": "2", "ephemeral-storage": "500Gi"}}`
		c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/nodeflavors/patch-extend-flavor", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "patch-extend-flavor")
		h.PatchNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		updated, err := h.getAdminNodeFlavor(context.Background(), "patch-extend-flavor")
		assert.NoError(t, err)
		rdmaQuantity := updated.Spec.ExtendResources["rdma/hca"]
		assert.Equal(t, int64(2), rdmaQuantity.Value())
	})

	t.Run("patch with no changes", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "patch-nochange-flavor",
				Labels: map[string]string{
					v1.DisplayNameLabel: "patch-nochange-flavor",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Product: "AMD_EPYC", Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		err := fakeClient.Create(context.Background(), nf)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `{}`
		c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/nodeflavors/patch-nochange-flavor", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "patch-nochange-flavor")
		h.PatchNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})

	t.Run("patch non-existing node flavor", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		body := `{"cpu": {"product": "AMD_EPYC", "quantity": "128"}}`
		c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/nodeflavors/non-existing", strings.NewReader(body))
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "non-existing")
		h.PatchNodeFlavor(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}

func TestDeleteNodeFlavor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("delete existing node flavor", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "delete-test-flavor",
				Labels: map[string]string{
					v1.DisplayNameLabel: "delete-test-flavor",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		err := fakeClient.Create(context.Background(), nf)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/nodeflavors/delete-test-flavor", nil)
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "delete-test-flavor")
		h.DeleteNodeFlavor(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		// Verify deletion
		_, err = h.getAdminNodeFlavor(context.Background(), "delete-test-flavor")
		assert.Error(t, err)
	})

	t.Run("delete non-existing node flavor", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/nodeflavors/non-existing", nil)
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "non-existing")
		h.DeleteNodeFlavor(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}

func TestGetAdminNodeFlavor(t *testing.T) {
	t.Run("get existing node flavor", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "admin-get-flavor",
				Labels: map[string]string{
					v1.DisplayNameLabel: "admin-get-flavor",
					v1.UserIdLabel:      mockUser.Name,
				},
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		err := fakeClient.Create(context.Background(), nf)
		assert.NoError(t, err)

		result, err := h.getAdminNodeFlavor(context.Background(), "admin-get-flavor")
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "admin-get-flavor", result.Name)
	})

	t.Run("get non-existing node flavor", func(t *testing.T) {
		_, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		result, err := h.getAdminNodeFlavor(context.Background(), "non-existing")
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("get node flavor with empty id", func(t *testing.T) {
		_, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		result, err := h.getAdminNodeFlavor(context.Background(), "")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "nodeFlavorId is empty")
	})
}

func TestUpdateNodeFlavor(t *testing.T) {
	_, fakeClient := createMockUser()
	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

	t.Run("update cpu", func(t *testing.T) {
		nf := &v1.NodeFlavor{
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Product: "AMD_EPYC", Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		newCpu := v1.CpuChip{Product: "AMD_EPYC_9554", Quantity: resource.MustParse("128")}
		req := &types.PatchNodeFlavorRequest{CPU: &newCpu}
		shouldUpdate, err := h.updateNodeFlavor(nf, req)
		assert.NoError(t, err)
		assert.True(t, shouldUpdate)
		assert.Equal(t, int64(128), nf.Spec.Cpu.Quantity.Value())
	})

	t.Run("update memory", func(t *testing.T) {
		nf := &v1.NodeFlavor{
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		newMemory := resource.MustParse("256Gi")
		req := &types.PatchNodeFlavorRequest{Memory: &newMemory}
		shouldUpdate, err := h.updateNodeFlavor(nf, req)
		assert.NoError(t, err)
		assert.True(t, shouldUpdate)
		assert.Equal(t, int64(256*1024*1024*1024), nf.Spec.Memory.Value())
	})

	t.Run("update gpu on nil gpu", func(t *testing.T) {
		nf := &v1.NodeFlavor{
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
				Gpu:    nil,
			},
		}
		newGpu := &v1.GpuChip{Product: "MI300X", ResourceName: "amd.com/gpu", Quantity: resource.MustParse("8")}
		req := &types.PatchNodeFlavorRequest{Gpu: newGpu}
		shouldUpdate, err := h.updateNodeFlavor(nf, req)
		assert.NoError(t, err)
		assert.True(t, shouldUpdate)
		assert.NotNil(t, nf.Spec.Gpu)
		assert.Equal(t, int64(8), nf.Spec.Gpu.Quantity.Value())
	})

	t.Run("update rootDisk on nil rootDisk", func(t *testing.T) {
		nf := &v1.NodeFlavor{
			Spec: v1.NodeFlavorSpec{
				Cpu:      v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory:   resource.MustParse("128Gi"),
				RootDisk: nil,
			},
		}
		newRootDisk := &v1.DiskFlavor{Type: v1.SSD, Quantity: resource.MustParse("500Gi"), Count: 1}
		req := &types.PatchNodeFlavorRequest{RootDisk: newRootDisk}
		shouldUpdate, err := h.updateNodeFlavor(nf, req)
		assert.NoError(t, err)
		assert.True(t, shouldUpdate)
		assert.NotNil(t, nf.Spec.RootDisk)
		assert.Equal(t, int64(500*1024*1024*1024), nf.Spec.RootDisk.Quantity.Value())
	})

	t.Run("update dataDisk on nil dataDisk", func(t *testing.T) {
		nf := &v1.NodeFlavor{
			Spec: v1.NodeFlavorSpec{
				Cpu:      v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory:   resource.MustParse("128Gi"),
				DataDisk: nil,
			},
		}
		newDataDisk := &v1.DiskFlavor{Type: v1.NVME, Quantity: resource.MustParse("1Ti"), Count: 4}
		req := &types.PatchNodeFlavorRequest{DataDisk: newDataDisk}
		shouldUpdate, err := h.updateNodeFlavor(nf, req)
		assert.NoError(t, err)
		assert.True(t, shouldUpdate)
		assert.NotNil(t, nf.Spec.DataDisk)
		assert.Equal(t, 4, nf.Spec.DataDisk.Count)
	})

	t.Run("update extendResources", func(t *testing.T) {
		nf := &v1.NodeFlavor{
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		newExtend := corev1.ResourceList{"rdma/hca": resource.MustParse("2")}
		req := &types.PatchNodeFlavorRequest{ExtendResources: &newExtend}
		shouldUpdate, err := h.updateNodeFlavor(nf, req)
		assert.NoError(t, err)
		assert.True(t, shouldUpdate)
		rdmaQuantity := nf.Spec.ExtendResources["rdma/hca"]
		assert.Equal(t, int64(2), rdmaQuantity.Value())
	})

	t.Run("no update when same values", func(t *testing.T) {
		cpu := v1.CpuChip{Product: "AMD_EPYC", Quantity: resource.MustParse("64")}
		nf := &v1.NodeFlavor{
			Spec: v1.NodeFlavorSpec{
				Cpu:    cpu,
				Memory: resource.MustParse("128Gi"),
			},
		}
		req := &types.PatchNodeFlavorRequest{CPU: &cpu}
		shouldUpdate, err := h.updateNodeFlavor(nf, req)
		assert.NoError(t, err)
		assert.False(t, shouldUpdate)
	})

	t.Run("no update with empty request", func(t *testing.T) {
		nf := &v1.NodeFlavor{
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}
		req := &types.PatchNodeFlavorRequest{}
		shouldUpdate, err := h.updateNodeFlavor(nf, req)
		assert.NoError(t, err)
		assert.False(t, shouldUpdate)
	})
}

func TestGenerateNodeFlavor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("generate basic node flavor", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Set(common.UserId, "test-user")

		req := &types.CreateNodeFlavorRequest{
			Name: "test-flavor",
			NodeFlavorSpec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Product: "AMD_EPYC", Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}

		nf, err := generateNodeFlavor(c, req)
		assert.NoError(t, err)
		assert.NotNil(t, nf)
		assert.Equal(t, "test-flavor", nf.Name)
		assert.Equal(t, "test-user", nf.Labels[v1.UserIdLabel])
		assert.Equal(t, "test-flavor", nf.Labels[v1.DisplayNameLabel])
	})

	t.Run("generate node flavor with rootDisk auto ephemeral storage", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Set(common.UserId, "test-user")

		req := &types.CreateNodeFlavorRequest{
			Name: "test-flavor-disk",
			NodeFlavorSpec: v1.NodeFlavorSpec{
				Cpu:      v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory:   resource.MustParse("128Gi"),
				RootDisk: &v1.DiskFlavor{Quantity: resource.MustParse("100Gi"), Count: 2},
			},
		}

		nf, err := generateNodeFlavor(c, req)
		assert.NoError(t, err)
		assert.NotNil(t, nf)
		// ephemeral-storage should be auto calculated: 100Gi * 2 = 200Gi
		expectedStorage := resource.NewQuantity(100*1024*1024*1024*2, resource.BinarySI)
		assert.Equal(t, *expectedStorage, nf.Spec.ExtendResources[corev1.ResourceEphemeralStorage])
	})

	t.Run("generate node flavor with existing ephemeral storage", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Set(common.UserId, "test-user")

		existingEphemeral := resource.MustParse("500Gi")
		req := &types.CreateNodeFlavorRequest{
			Name: "test-flavor-existing-ephemeral",
			NodeFlavorSpec: v1.NodeFlavorSpec{
				Cpu:      v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory:   resource.MustParse("128Gi"),
				RootDisk: &v1.DiskFlavor{Quantity: resource.MustParse("100Gi"), Count: 2},
				ExtendResources: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: existingEphemeral,
				},
			},
		}

		nf, err := generateNodeFlavor(c, req)
		assert.NoError(t, err)
		assert.NotNil(t, nf)
		// Should keep existing ephemeral storage, not auto calculate
		assert.Equal(t, existingEphemeral, nf.Spec.ExtendResources[corev1.ResourceEphemeralStorage])
	})

	t.Run("generate node flavor without rootDisk", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Set(common.UserId, "test-user")

		req := &types.CreateNodeFlavorRequest{
			Name: "test-flavor-no-disk",
			NodeFlavorSpec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}

		nf, err := generateNodeFlavor(c, req)
		assert.NoError(t, err)
		assert.NotNil(t, nf)
		assert.Nil(t, nf.Spec.ExtendResources)
	})
}

func TestCvtToNodeFlavorResponseItem(t *testing.T) {
	t.Run("convert basic node flavor", func(t *testing.T) {
		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-flavor",
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:    v1.CpuChip{Product: "AMD_EPYC", Quantity: resource.MustParse("64")},
				Memory: resource.MustParse("128Gi"),
			},
		}

		result := cvtToNodeFlavorResponseItem(nf)
		assert.Equal(t, "test-flavor", result.FlavorId)
		assert.Equal(t, int64(64), result.Cpu.Quantity.Value())
		assert.Equal(t, int64(128*1024*1024*1024), result.Memory.Value())
	})

	t.Run("convert node flavor with all fields", func(t *testing.T) {
		nf := &v1.NodeFlavor{
			ObjectMeta: metav1.ObjectMeta{
				Name: "full-flavor",
			},
			Spec: v1.NodeFlavorSpec{
				Cpu:      v1.CpuChip{Product: "AMD_EPYC_9554", Quantity: resource.MustParse("128")},
				Memory:   resource.MustParse("256Gi"),
				Gpu:      &v1.GpuChip{Product: "MI300X", ResourceName: "amd.com/gpu", Quantity: resource.MustParse("8")},
				RootDisk: &v1.DiskFlavor{Type: v1.SSD, Quantity: resource.MustParse("500Gi"), Count: 1},
				DataDisk: &v1.DiskFlavor{Type: v1.NVME, Quantity: resource.MustParse("1Ti"), Count: 4},
				ExtendResources: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: resource.MustParse("500Gi"),
					"rdma/hca":                      resource.MustParse("2"),
				},
			},
		}

		result := cvtToNodeFlavorResponseItem(nf)
		assert.Equal(t, "full-flavor", result.FlavorId)
		assert.Equal(t, int64(128), result.Cpu.Quantity.Value())
		assert.Equal(t, "AMD_EPYC_9554", result.Cpu.Product)
		assert.NotNil(t, result.Gpu)
		assert.Equal(t, int64(8), result.Gpu.Quantity.Value())
		assert.NotNil(t, result.RootDisk)
		assert.NotNil(t, result.DataDisk)
		assert.Equal(t, 4, result.DataDisk.Count)
	})
}
