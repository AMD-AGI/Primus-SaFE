/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestCreateNodeFlavor(t *testing.T) {
	mockUser, fakeClient := createMockUser()
	h := Handler{Client: fakeClient, auth: authority.NewAuthorizer(fakeClient)}
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
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/nodeflavors"), strings.NewReader(body))
	c.Set(common.UserId, mockUser.Name)
	h.CreateNodeFlavor(c)
	assert.Equal(t, rsp.Code, http.StatusOK)
	result := &types.CreateNodeFlavorResponse{}
	err := json.Unmarshal(rsp.Body.Bytes(), result)
	assert.NilError(t, err)
	time.Sleep(time.Millisecond * 200)

	nodeflavor, err := h.getAdminNodeFlavor(context.Background(), result.FlavorId)
	assert.NilError(t, err)
	assert.Equal(t, nodeflavor.Spec.Cpu.Quantity.Value(), int64(256))
	assert.Equal(t, nodeflavor.Spec.Gpu.Quantity.Value(), int64(8))
	assert.Equal(t, nodeflavor.Spec.Memory.Value(), int64(128*1024*1024*1024))
	assert.Equal(t, nodeflavor.Spec.RootDisk.Quantity.Value(), int64(128*1024*1024*1024))
	assert.Equal(t, nodeflavor.Spec.ExtendResources[corev1.ResourceEphemeralStorage], *resource.NewQuantity(800*1024*1024*1024, resource.BinarySI))
	quantity, _ := resource.ParseQuantity("1k")
	assert.Equal(t, nodeflavor.Spec.ExtendResources["rdma/hca"], quantity)
}
