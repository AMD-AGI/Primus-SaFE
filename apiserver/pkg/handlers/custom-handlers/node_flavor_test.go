/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gotest.tools/assert"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
)

func TestCreateNodeFlavor(t *testing.T) {
	adminClient := fake.NewClientBuilder().WithObjects().WithScheme(scheme.Scheme).Build()

	h := Handler{Client: adminClient}
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	body := `{"name":"amd-mi300x-192gb-hbm3","flavorType":"BareMetal","cpu":256,"cpuProduct":"AMD_EPYC_9554","gpu":8,"gpuName":"amd.com/gpu","gpuProduct":"AMD_Instinct_MI300X_OAM","memory":1622959652864,"rootDisk":{"type":"nvme","quantity":"7864Gi","count":1},"dataDisk":{"type":"nvme","quantity":"7864Gi","count":9},"extends":{"ephemeral-storage":"7440663780Ki","rdma/hca":"1k"}}`
	c.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/nodeflavors"), strings.NewReader(body))
	h.CreateNodeFlavor(c)
	assert.Equal(t, rsp.Code, http.StatusOK)
	time.Sleep(time.Millisecond * 200)

	nodeflavor, err := h.getAdminNodeFlavor(context.Background(), "amd-mi300x-192gb-hbm3")
	assert.NilError(t, err)
	assert.Equal(t, nodeflavor.Spec.Cpu.Quantity.Value(), int64(256))
	assert.Equal(t, nodeflavor.Spec.Gpu.Quantity.Value(), int64(8))
}
