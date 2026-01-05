/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package quantity

import (
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

func TestAddResource(t *testing.T) {
	resource1 := corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewMilliQuantity(1000, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(1024, resource.BinarySI),
	}
	resource2 := corev1.ResourceList{
		corev1.ResourceCPU:     *resource.NewQuantity(1, resource.DecimalSI),
		corev1.ResourceStorage: *resource.NewQuantity(1024*1024, resource.BinarySI),
	}
	result := AddResource(resource1, resource2)
	assert.Equal(t, result.Cpu().Value(), int64(2))
	assert.Equal(t, result.Memory().Value(), int64(1024))
	assert.Equal(t, result.Storage().String(), "1Mi")

	result = AddResource(nil, resource1)
	assert.Equal(t, result.Cpu().Value(), int64(1))
	assert.Equal(t, result.Memory().Value(), int64(1024))
	assert.Equal(t, result.Storage().Value(), int64(0))
}

func TestSubResource(t *testing.T) {
	resource1 := corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewMilliQuantity(1000, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(512, resource.BinarySI),
		common.AmdGpu:         *resource.NewQuantity(8, resource.DecimalSI),
	}
	resource2 := corev1.ResourceList{
		corev1.ResourceCPU:     *resource.NewQuantity(1, resource.DecimalSI),
		corev1.ResourceMemory:  *resource.NewQuantity(1024, resource.BinarySI),
		corev1.ResourceStorage: *resource.NewQuantity(1024, resource.BinarySI),
	}
	result := SubResource(resource1, resource2)
	_, ok := result[corev1.ResourceCPU]
	assert.Equal(t, ok, false)
	assert.Equal(t, result.Memory().Value(), int64(-512))
	assert.Equal(t, result.StorageEphemeral().Value(), int64(0))
	assert.Equal(t, result.Storage().Value(), int64(-1024))
	gpu, ok := result[common.AmdGpu]
	assert.Equal(t, ok, true)
	assert.Equal(t, gpu.Value(), int64(8))

	resource1 = corev1.ResourceList{
		corev1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
	}
	resource2 = corev1.ResourceList{
		corev1.ResourceCPU: *resource.NewQuantity(1, resource.DecimalSI),
	}
	result = SubResource(resource1, resource2)
	assert.Equal(t, len(result), 0)
}

func TestNegative(t *testing.T) {
	resource1 := corev1.ResourceList{
		corev1.ResourceCPU:     *resource.NewMilliQuantity(1000, resource.DecimalSI),
		corev1.ResourceMemory:  *resource.NewQuantity(-512, resource.BinarySI),
		corev1.ResourceStorage: *resource.NewQuantity(1024, resource.BinarySI),
	}
	result := Negative(resource1)
	assert.Equal(t, result.Cpu().Value(), int64(-1))
	assert.Equal(t, result.Memory().Value(), int64(512))
	assert.Equal(t, result.Storage().Value(), int64(-1024))
}

func TestIsSubResource(t *testing.T) {
	resource2 := corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewQuantity(128, resource.DecimalSI),
		corev1.ResourceMemory: resource.MustParse("128Mi"),
		common.NvidiaGpu:      *resource.NewQuantity(8, resource.DecimalSI),
	}

	tests := []struct {
		name      string
		resource1 corev1.ResourceList
		result    bool
	}{
		{
			"success",
			corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(100, resource.DecimalSI),
				corev1.ResourceMemory: resource.MustParse("128Ki"),
				common.NvidiaGpu:      *resource.NewQuantity(4, resource.DecimalSI),
			},
			true,
		},
		{
			"one less",
			corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(128, resource.DecimalSI),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			true,
		},
		{
			"one more",
			corev1.ResourceList{
				corev1.ResourceCPU:              *resource.NewQuantity(128, resource.DecimalSI),
				corev1.ResourceMemory:           resource.MustParse("128Mi"),
				common.NvidiaGpu:                *resource.NewQuantity(8, resource.DecimalSI),
				corev1.ResourceEphemeralStorage: resource.MustParse("128Mi"),
			},
			false,
		},
		{
			"one passed",
			corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(128, resource.DecimalSI),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
				common.NvidiaGpu:      *resource.NewQuantity(8, resource.DecimalSI),
			},
			false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, _ := IsSubResource(test.resource1, resource2)
			assert.Equal(t, result, test.result)
		})
	}
}

func TestMultiResource(t *testing.T) {
	resource1 := corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewMilliQuantity(1000, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(512, resource.BinarySI),
		common.NvidiaGpu:      *resource.NewQuantity(8, resource.DecimalSI),
	}
	resource2 := MultiResource(resource1, 2)
	assert.Equal(t, resource2.Cpu().Value(), int64(2))
	assert.Equal(t, resource2.Memory().Value(), int64(1024))
	gpu, ok := resource2[common.NvidiaGpu]
	assert.Equal(t, ok, true)
	assert.Equal(t, gpu.Value(), int64(16))
}

func TestCvtToResourceList(t *testing.T) {
	rdmaName := "net.rdma_name"
	commonconfig.SetValue(rdmaName, "rdma/hca")
	defer commonconfig.SetValue(rdmaName, "")

	res, err := CvtToResourceList("1000m", "512", "8", common.NvidiaGpu, "", "1k", 2)
	assert.NilError(t, err)
	assert.Equal(t, res.Cpu().Value(), int64(2))
	assert.Equal(t, res.Memory().Value(), int64(1024))
	gpu, ok := res[common.NvidiaGpu]
	assert.Equal(t, ok, true)
	assert.Equal(t, gpu.Value(), int64(16))
	rdma, ok := res["rdma/hca"]
	assert.Equal(t, ok, true)
	assert.Equal(t, rdma.Value(), int64(2000))
}

func TestParseFloatQuantity(t *testing.T) {
	memQuantity, err := resource.ParseQuantity("1Gi")
	assert.NilError(t, err)
	shareMemQuantity := resource.NewQuantity(memQuantity.Value()/2, memQuantity.Format)
	assert.Equal(t, shareMemQuantity != nil, true)
	assert.Equal(t, shareMemQuantity.Value(), int64(536870912))
	assert.Equal(t, shareMemQuantity.String(), "512Mi")
}

func TestToGiString(t *testing.T) {
	q1 := resource.MustParse("2Gi")
	assert.Equal(t, ToString(q1), "2Gi")
	q2 := resource.MustParse("1024Mi")
	assert.Equal(t, ToString(q2), "1Gi")
	q3 := resource.MustParse("500Mi")
	assert.Equal(t, ToString(q3), "500Mi")
}
