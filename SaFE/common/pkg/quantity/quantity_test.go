/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package quantity

import (
	"testing"

	"github.com/spf13/viper"
	testifyassert "github.com/stretchr/testify/assert"

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
	assert.Equal(t, result.Cpu().Value(), int64(0))
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

// --- merged from quantity_extra_test.go ---

func qval(list corev1.ResourceList, name corev1.ResourceName) int64 {
	q := list[name]
	return q.Value()
}

func qsign(list corev1.ResourceList, name corev1.ResourceName) int {
	q := list[name]
	return q.Sign()
}

func rl(cpu, mem string) corev1.ResourceList {
	out := corev1.ResourceList{}
	if cpu != "" {
		out[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if mem != "" {
		out[corev1.ResourceMemory] = resource.MustParse(mem)
	}
	return out
}

func TestCopyAndNormalizeAndEqual(t *testing.T) {
	testifyassert.Empty(t, Copy(nil))
	src := rl("2", "4Gi")
	cp := Copy(src)
	testifyassert.True(t, Equal(src, cp))

	testifyassert.Nil(t, Normalize(nil))
	// non-concerned resource is dropped by Normalize
	withExtra := rl("1", "")
	withExtra["example.com/foo"] = resource.MustParse("3")
	norm := Normalize(withExtra)
	_, ok := norm["example.com/foo"]
	testifyassert.False(t, ok)

	testifyassert.False(t, Equal(rl("1", ""), rl("2", "")))
	testifyassert.False(t, Equal(rl("1", ""), rl("1", "2Gi")))
}

func TestGetConcernedResources(t *testing.T) {
	in := rl("2", "4Gi")
	in["example.com/foo"] = resource.MustParse("1")
	in[corev1.ResourceStorage] = resource.MustParse("0") // zero dropped
	out := GetConcernedResources(in)
	testifyassert.Contains(t, out, corev1.ResourceCPU)
	testifyassert.Contains(t, out, corev1.ResourceMemory)
	testifyassert.NotContains(t, out, corev1.ResourceName("example.com/foo"))
	testifyassert.NotContains(t, out, corev1.ResourceStorage)
}

func TestIsConcernedResourceRdma(t *testing.T) {
	viper.Reset()
	testifyassert.False(t, IsConcernedResource("net.x/rdma"))
	viper.Set("net.rdma_name", "net.x/rdma")
	testifyassert.True(t, IsConcernedResource("net.x/rdma"))
	testifyassert.True(t, IsConcernedResource(corev1.ResourceEphemeralStorage))
}

func TestNegativeAndSubMissing(t *testing.T) {
	neg := Negative(rl("2", ""))
	assert.Equal(t, int64(-2), qval(neg, corev1.ResourceCPU))

	// list2 has a key not in list1 -> negated in result
	res := SubResource(rl("2", ""), rl("1", "1Gi"))
	assert.Equal(t, int64(1), qval(res, corev1.ResourceCPU))
	testifyassert.True(t, qsign(res, corev1.ResourceMemory) < 0)

	// equal lists -> nil
	testifyassert.Nil(t, SubResource(rl("2", ""), rl("2", "")))
	// empty list2 -> returns list1
	testifyassert.NotNil(t, SubResource(rl("2", ""), rl("", "")))
}

func TestNonNegative(t *testing.T) {
	// nil/empty input is returned as-is.
	testifyassert.Nil(t, NonNegative(nil))

	in := corev1.ResourceList{
		corev1.ResourceCPU:              resource.MustParse("-4"),  // negative -> clamped to 0
		corev1.ResourceMemory:           resource.MustParse("8Gi"), // positive -> kept
		corev1.ResourceEphemeralStorage: resource.MustParse("0"),   // zero -> kept as 0
	}
	out := NonNegative(in)
	assert.Equal(t, int64(0), qval(out, corev1.ResourceCPU))
	assert.Equal(t, int64(8*1024*1024*1024), qval(out, corev1.ResourceMemory))
	assert.Equal(t, int64(0), qval(out, corev1.ResourceEphemeralStorage))

	// input is not mutated.
	assert.Equal(t, int64(-4), qval(in, corev1.ResourceCPU))
}

func TestFormat(t *testing.T) {
	assert.Equal(t, "2 Gi", Format(string(corev1.ResourceMemory), resource.MustParse("2Gi")))
	assert.Equal(t, "4 Gi", Format(string(corev1.ResourceEphemeralStorage), resource.MustParse("4Gi")))
	assert.Equal(t, "3", Format(string(corev1.ResourceCPU), resource.MustParse("3")))
}

func TestToString(t *testing.T) {
	assert.Equal(t, "2Gi", ToString(resource.MustParse("2Gi")))
	assert.Equal(t, "512Mi", ToString(resource.MustParse("512Mi")))
	assert.Equal(t, "", ToString(resource.MustParse("100Ki")))
}

func TestGetAvailableResource(t *testing.T) {
	viper.Reset()
	// no reserves -> returns input unchanged
	in := rl("10", "10Gi")
	testifyassert.True(t, Equal(in, GetAvailableResource(in)))
	testifyassert.Empty(t, GetAvailableResource(corev1.ResourceList{}))

	viper.Set("workspace.cpu_reserve_percent", 0.1)
	viper.Set("workspace.mem_reserve_percent", 0.1)
	viper.Set("workspace.ephemeral_store_reserve_percent", 0.1)
	in2 := corev1.ResourceList{
		corev1.ResourceCPU:              resource.MustParse("10"),
		corev1.ResourceMemory:           resource.MustParse("10Gi"),
		corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
	}
	out := GetAvailableResource(in2)
	testifyassert.Less(t, qval(out, corev1.ResourceCPU), int64(10))
}

func TestGetMaxEphemeralStoreQuantity(t *testing.T) {
	viper.Reset()
	_, err := GetMaxEphemeralStoreQuantity(rl("1", ""))
	testifyassert.Error(t, err)

	hundredGi := resource.MustParse("100Gi")
	in := corev1.ResourceList{corev1.ResourceEphemeralStorage: hundredGi}
	// no reserve and no max -> maxPercent 1 -> returns original
	q, err := GetMaxEphemeralStoreQuantity(in)
	testifyassert.NoError(t, err)
	assert.Equal(t, hundredGi.Value(), q.Value())

	viper.Set("workload.max_ephemeral_store_percent", 0.5)
	q2, err := GetMaxEphemeralStoreQuantity(in)
	testifyassert.NoError(t, err)
	testifyassert.Less(t, q2.Value(), hundredGi.Value())
}

func TestCvtToResourceListBranches(t *testing.T) {
	viper.Reset()
	viper.Set("net.rdma_name", "net.x/rdma")

	// replica <= 0 -> nil,nil
	out, err := CvtToResourceList("1", "", "", "", "", "", 0)
	testifyassert.NoError(t, err)
	testifyassert.Nil(t, out)

	// full happy path with gpu + ephemeral + rdma
	out, err = CvtToResourceList("4", "8Gi", "2", "amd.com/gpu", "50Gi", "1", 1)
	testifyassert.NoError(t, err)
	assert.Equal(t, int64(4), qval(out, corev1.ResourceCPU))
	testifyassert.Contains(t, out, corev1.ResourceName("amd.com/gpu"))
	testifyassert.Contains(t, out, corev1.ResourceName("net.x/rdma"))

	// invalid cpu value
	_, err = CvtToResourceList("abc", "", "", "", "", "", 1)
	testifyassert.Error(t, err)
	// zero cpu
	_, err = CvtToResourceList("0", "", "", "", "", "", 1)
	testifyassert.Error(t, err)
	// invalid memory
	_, err = CvtToResourceList("", "xx", "", "", "", "", 1)
	testifyassert.Error(t, err)
}
