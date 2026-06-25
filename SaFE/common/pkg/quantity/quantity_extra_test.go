/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package quantity

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

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
	assert.Empty(t, Copy(nil))
	src := rl("2", "4Gi")
	cp := Copy(src)
	assert.True(t, Equal(src, cp))

	assert.Nil(t, Normalize(nil))
	// non-concerned resource is dropped by Normalize
	withExtra := rl("1", "")
	withExtra["example.com/foo"] = resource.MustParse("3")
	norm := Normalize(withExtra)
	_, ok := norm["example.com/foo"]
	assert.False(t, ok)

	assert.False(t, Equal(rl("1", ""), rl("2", "")))
	assert.False(t, Equal(rl("1", ""), rl("1", "2Gi")))
}

func TestGetConcernedResources(t *testing.T) {
	in := rl("2", "4Gi")
	in["example.com/foo"] = resource.MustParse("1")
	in[corev1.ResourceStorage] = resource.MustParse("0") // zero dropped
	out := GetConcernedResources(in)
	assert.Contains(t, out, corev1.ResourceCPU)
	assert.Contains(t, out, corev1.ResourceMemory)
	assert.NotContains(t, out, corev1.ResourceName("example.com/foo"))
	assert.NotContains(t, out, corev1.ResourceStorage)
}

func TestIsConcernedResourceRdma(t *testing.T) {
	viper.Reset()
	assert.False(t, IsConcernedResource("net.x/rdma"))
	viper.Set("net.rdma_name", "net.x/rdma")
	assert.True(t, IsConcernedResource("net.x/rdma"))
	assert.True(t, IsConcernedResource(corev1.ResourceEphemeralStorage))
}

func TestNegativeAndSubMissing(t *testing.T) {
	neg := Negative(rl("2", ""))
	assert.Equal(t, int64(-2), qval(neg, corev1.ResourceCPU))

	// list2 has a key not in list1 -> negated in result
	res := SubResource(rl("2", ""), rl("1", "1Gi"))
	assert.Equal(t, int64(1), qval(res, corev1.ResourceCPU))
	assert.True(t, qsign(res, corev1.ResourceMemory) < 0)

	// equal lists -> nil
	assert.Nil(t, SubResource(rl("2", ""), rl("2", "")))
	// empty list2 -> returns list1
	assert.NotNil(t, SubResource(rl("2", ""), rl("", "")))
}

func TestNonNegative(t *testing.T) {
	// nil/empty input is returned as-is.
	assert.Nil(t, NonNegative(nil))

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
	assert.True(t, Equal(in, GetAvailableResource(in)))
	assert.Empty(t, GetAvailableResource(corev1.ResourceList{}))

	viper.Set("workspace.cpu_reserve_percent", 0.1)
	viper.Set("workspace.mem_reserve_percent", 0.1)
	viper.Set("workspace.ephemeral_store_reserve_percent", 0.1)
	in2 := corev1.ResourceList{
		corev1.ResourceCPU:              resource.MustParse("10"),
		corev1.ResourceMemory:           resource.MustParse("10Gi"),
		corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
	}
	out := GetAvailableResource(in2)
	assert.Less(t, qval(out, corev1.ResourceCPU), int64(10))
}

func TestGetMaxEphemeralStoreQuantity(t *testing.T) {
	viper.Reset()
	_, err := GetMaxEphemeralStoreQuantity(rl("1", ""))
	assert.Error(t, err)

	hundredGi := resource.MustParse("100Gi")
	in := corev1.ResourceList{corev1.ResourceEphemeralStorage: hundredGi}
	// no reserve and no max -> maxPercent 1 -> returns original
	q, err := GetMaxEphemeralStoreQuantity(in)
	assert.NoError(t, err)
	assert.Equal(t, hundredGi.Value(), q.Value())

	viper.Set("workload.max_ephemeral_store_percent", 0.5)
	q2, err := GetMaxEphemeralStoreQuantity(in)
	assert.NoError(t, err)
	assert.Less(t, q2.Value(), hundredGi.Value())
}

func TestCvtToResourceListBranches(t *testing.T) {
	viper.Reset()
	viper.Set("net.rdma_name", "net.x/rdma")

	// replica <= 0 -> nil,nil
	out, err := CvtToResourceList("1", "", "", "", "", "", 0)
	assert.NoError(t, err)
	assert.Nil(t, out)

	// full happy path with gpu + ephemeral + rdma
	out, err = CvtToResourceList("4", "8Gi", "2", "amd.com/gpu", "50Gi", "1", 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(4), qval(out, corev1.ResourceCPU))
	assert.Contains(t, out, corev1.ResourceName("amd.com/gpu"))
	assert.Contains(t, out, corev1.ResourceName("net.x/rdma"))

	// invalid cpu value
	_, err = CvtToResourceList("abc", "", "", "", "", "", 1)
	assert.Error(t, err)
	// zero cpu
	_, err = CvtToResourceList("0", "", "", "", "", "", 1)
	assert.Error(t, err)
	// invalid memory
	_, err = CvtToResourceList("", "xx", "", "", "", "", 1)
	assert.Error(t, err)
}
