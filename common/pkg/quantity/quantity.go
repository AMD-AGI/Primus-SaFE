/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package quantity

import (
	"fmt"
	"math"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/floatutil"
)

func AddResource(resources ...corev1.ResourceList) corev1.ResourceList {
	result := corev1.ResourceList{}
	for _, res := range resources {
		for k, v := range res {
			if !IsConcernedResource(k) {
				continue
			}
			v2 := v.DeepCopy()
			if s, ok := result[k]; ok {
				v2.Add(s)
			}
			result[k] = v2
		}
	}
	return result
}

// return rl1 - rl2
func SubResource(rl1, rl2 corev1.ResourceList) corev1.ResourceList {
	list1 := Normalize(rl1)
	list2 := Normalize(rl2)
	if equal(list1, list2) {
		return nil
	}
	if len(list2) == 0 {
		return list1
	}

	result := corev1.ResourceList{}
	for resourceType, quantity1 := range list1 {
		quantity2, exists := list2[resourceType]
		if exists {
			subtractedQuantity := quantity1.DeepCopy()
			subtractedQuantity.Sub(quantity2)
			if !subtractedQuantity.IsZero() {
				result[resourceType] = subtractedQuantity
			}
		} else {
			result[resourceType] = quantity1.DeepCopy()
		}
	}

	for resourceType, quantity2 := range list2 {
		if _, exists := list1[resourceType]; !exists {
			v := quantity2.DeepCopy()
			v.Neg()
			result[resourceType] = v
		}
	}
	return result
}

// return whether resource1 is the sub resource of resource2 (including equal)
func IsSubResource(resource1, resource2 corev1.ResourceList) (bool, string) {
	for key, val1 := range resource1 {
		val2, ok := resource2[key]
		if !ok {
			return false, string(key)
		}
		if val1.Cmp(val2) > 0 {
			return false, string(key)
		}
	}
	return true, ""
}

func Negative(rl corev1.ResourceList) corev1.ResourceList {
	result := corev1.ResourceList{}
	for k, v := range rl {
		v2 := v.DeepCopy()
		v2.Neg()
		result[k] = v2
	}
	return result
}

// Returns true as long as one value of a resource is less than 0
func IsNegative(rl corev1.ResourceList) bool {
	for _, val := range rl {
		if val.Value() < 0 {
			return true
		}
	}
	return false
}

func Copy(rl corev1.ResourceList) corev1.ResourceList {
	if len(rl) == 0 {
		return make(corev1.ResourceList)
	}
	return rl.DeepCopy()
}

func GetConcernedResources(res corev1.ResourceList) corev1.ResourceList {
	result := make(corev1.ResourceList)
	for key, val := range res {
		if !IsConcernedResource(key) {
			continue
		}
		if !val.IsZero() {
			result[key] = val
		}
	}
	return result
}

func Equal(rl1, rl2 corev1.ResourceList) bool {
	list1 := Normalize(rl1)
	list2 := Normalize(rl2)
	return equal(list1, list2)
}

func equal(rl1, rl2 corev1.ResourceList) bool {
	if len(rl1) != len(rl2) {
		return false
	}
	for k, v := range rl1 {
		if !v.Equal(rl2[k]) {
			return false
		}
	}
	return true
}

func Normalize(rl corev1.ResourceList) corev1.ResourceList {
	if rl == nil {
		return nil
	}
	result := corev1.ResourceList{}
	for k, v := range rl {
		if !IsConcernedResource(k) {
			continue
		}
		result[k] = v
	}
	return result
}

// Check if it is the concerned resource
func IsConcernedResource(name corev1.ResourceName) bool {
	if name == common.NvidiaGpu || name == common.AmdGpu {
		return true
	}
	if name == corev1.ResourceCPU || name == corev1.ResourceMemory {
		return true
	}
	if name == corev1.ResourceStorage || name == corev1.ResourceEphemeralStorage {
		return true
	}
	if string(name) == commonconfig.GetRdmaName() {
		return true
	}
	return false
}

func MultiResource(inputs corev1.ResourceList, replica int64) corev1.ResourceList {
	result := corev1.ResourceList{}
	for k, v := range inputs {
		result[k] = *resource.NewQuantity(replica*v.Value(), v.Format)
	}
	return result
}

func CvtToResourceList(cpu, memory, gpu, gpuName, ephemeralStore, rdmaResource string, replica int64) (corev1.ResourceList, error) {
	if replica <= 0 {
		return nil, nil
	}
	result := make(corev1.ResourceList)
	if cpu != "" {
		cpuQuantity, err := resource.ParseQuantity(cpu)
		if err != nil {
			return nil, fmt.Errorf("%s, value: %s", err.Error(), cpu)
		}
		if cpuQuantity.Value() <= 0 {
			return nil, fmt.Errorf("invalid cpu")
		}
		result[corev1.ResourceCPU] = cpuQuantity
	}

	if memory != "" {
		memQuantity, err := resource.ParseQuantity(memory)
		if err != nil {
			return nil, fmt.Errorf("%s, value: %s", err.Error(), memory)
		}
		if memQuantity.Value() <= 0 {
			return nil, fmt.Errorf("invalid memory")
		}
		result[corev1.ResourceMemory] = memQuantity
	}

	if gpu != "" && gpuName != "" {
		gpuQuantity, err := resource.ParseQuantity(gpu)
		if err != nil {
			return nil, fmt.Errorf("%s, value: %s", err.Error(), gpu)
		}
		if gpuQuantity.Value() <= 0 {
			return nil, fmt.Errorf("invalid gpu")
		}
		result[corev1.ResourceName(gpuName)] = gpuQuantity
	}

	if ephemeralStore != "" {
		ephemeralStoreQuantity, err := resource.ParseQuantity(ephemeralStore)
		if err != nil {
			return nil, fmt.Errorf("%s, value: %s", err.Error(), ephemeralStore)
		}
		if ephemeralStoreQuantity.Value() <= 0 {
			return nil, fmt.Errorf("invalid ephemeral store")
		}
		result[corev1.ResourceEphemeralStorage] = ephemeralStoreQuantity
	}

	if rdmaResource != "" && commonconfig.GetRdmaName() != "" {
		rdmaQuantity, err := resource.ParseQuantity(rdmaResource)
		if err != nil {
			return nil, fmt.Errorf("%s, value: %s", err.Error(), rdmaResource)
		}
		if rdmaQuantity.Value() <= 0 {
			return nil, fmt.Errorf("invalid rdma resource")
		}
		result[corev1.ResourceName(commonconfig.GetRdmaName())] = rdmaQuantity
	}
	return MultiResource(result, replica), nil
}

func Format(key string, quantity resource.Quantity) string {
	quantityStr := ""
	if key == string(corev1.ResourceMemory) || key == string(corev1.ResourceEphemeralStorage) {
		n := quantity.Value() / (1024 * 1024 * 1024)
		quantityStr = fmt.Sprintf("%d Gi", n)
	} else {
		quantityStr = quantity.String()
	}
	return quantityStr
}

func GetAvailableResource(resources corev1.ResourceList) corev1.ResourceList {
	if len(resources) == 0 {
		return resources
	}
	if floatutil.FloatEqual(commonconfig.GetMemoryReservePercent(), 0) &&
		floatutil.FloatEqual(commonconfig.GetCpuReservePercent(), 0) &&
		floatutil.FloatEqual(commonconfig.GetEphemeralStoreReservePercent(), 0) {
		return resources
	}
	result := resources.DeepCopy()
	if !floatutil.FloatEqual(commonconfig.GetMemoryReservePercent(), 0) {
		memQuantity, ok := result[corev1.ResourceMemory]
		if ok {
			reserveQuantity := int64(math.Ceil(float64(memQuantity.Value()) * commonconfig.GetMemoryReservePercent()))
			result[corev1.ResourceMemory] = *resource.NewQuantity(memQuantity.Value()-reserveQuantity, resource.BinarySI)
		}
	}
	if !floatutil.FloatEqual(commonconfig.GetCpuReservePercent(), 0) {
		cpuQuantity, ok := result[corev1.ResourceCPU]
		if ok {
			reserveQuantity := int64(math.Ceil(float64(cpuQuantity.Value()) * commonconfig.GetCpuReservePercent()))
			result[corev1.ResourceCPU] = *resource.NewQuantity(cpuQuantity.Value()-reserveQuantity, resource.DecimalSI)
		}
	}
	if !floatutil.FloatEqual(commonconfig.GetEphemeralStoreReservePercent(), 0) {
		storeQuantity, ok := result[corev1.ResourceEphemeralStorage]
		if ok {
			reserveQuantity := int64(math.Ceil(float64(storeQuantity.Value()) *
				commonconfig.GetEphemeralStoreReservePercent()))
			result[corev1.ResourceEphemeralStorage] = *resource.NewQuantity(
				storeQuantity.Value()-reserveQuantity, resource.BinarySI)
		}
	}
	return result
}

func GetMaxEphemeralStoreQuantity(resources corev1.ResourceList) (*resource.Quantity, error) {
	storeQuantity, ok := resources[corev1.ResourceEphemeralStorage]
	if !ok {
		return nil, fmt.Errorf("the ephemeralStore is not found")
	}
	var maxPercent float64 = 0
	maxPercent = 1 - commonconfig.GetEphemeralStoreReservePercent()
	if !floatutil.FloatEqual(commonconfig.GetMaxEphemeralStorePercent(), 0) {
		if maxPercent > commonconfig.GetMaxEphemeralStorePercent() {
			maxPercent = commonconfig.GetMaxEphemeralStorePercent()
		}
	}
	if floatutil.FloatEqual(maxPercent, 1) {
		return &storeQuantity, nil
	}
	newQuantity := float64(storeQuantity.Value()) * maxPercent
	return resource.NewQuantity(int64(newQuantity), resource.BinarySI), nil
}

func ToString(q resource.Quantity) string {
	bytes := q.AsApproximateFloat64()
	gibibytes := bytes / (1024 * 1024 * 1024)
	if gibibytes < 1 {
		mebibytes := bytes / (1024 * 1024)
		if mebibytes < 1 {
			return ""
		}
		return fmt.Sprintf("%dMi", int64(mebibytes))
	}
	return fmt.Sprintf("%dGi", int64(gibibytes))
}
