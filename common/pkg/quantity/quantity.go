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

// AddResource combines multiple ResourceLists by adding corresponding resource quantities
// Only concerned resources are included in the result
// Parameters:
//
//	resources: Variable number of ResourceLists to add together
//
// Returns:
//
//	Combined ResourceList with summed quantities
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

// SubResource calculates the difference between two ResourceLists (rl1 - rl2)
// Returns a new ResourceList representing the subtraction result
// Parameters:
//
//	rl1: Minuend ResourceList
//	rl2: Subtrahend ResourceList
//
// Returns:
//
//	ResourceList representing rl1 - rl2
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

// IsSubResource checks if resource1 is a subset of resource2 (all resources in resource1 are less than or equal to corresponding resources in resource2)
// Parameters:
//
//	resource1: ResourceList to check if it's a subset
//	resource2: ResourceList to check against
//
// Returns:
//
//	bool: true if resource1 is a subset of resource2
//	string: name of the resource that violates the condition, empty if true
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

// Negative negates all resource quantities in the given ResourceList
// Parameters:
//
//	rl: ResourceList to negate
//
// Returns:
//
//	New ResourceList with negated quantities
func Negative(rl corev1.ResourceList) corev1.ResourceList {
	result := corev1.ResourceList{}
	for k, v := range rl {
		v2 := v.DeepCopy()
		v2.Neg()
		result[k] = v2
	}
	return result
}

// Copy creates a deep copy of the given ResourceList
// Parameters:
//
//	rl: ResourceList to copy
//
// Returns:
//
//	Deep copy of the ResourceList
func Copy(rl corev1.ResourceList) corev1.ResourceList {
	if len(rl) == 0 {
		return make(corev1.ResourceList)
	}
	return rl.DeepCopy()
}

// GetConcernedResources filters the ResourceList to include only concerned resources
// Filters out zero-value resources and non-concerned resource types
// Parameters:
//
//	res: ResourceList to filter
//
// Returns:
//
//	New ResourceList containing only concerned resources with non-zero values
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

// Equal compares two ResourceLists for equality after normalization
// Parameters:
//
//	rl1: First ResourceList to compare
//	rl2: Second ResourceList to compare
//
// Returns:
//
//	true if the normalized ResourceLists are equal, false otherwise
func Equal(rl1, rl2 corev1.ResourceList) bool {
	list1 := Normalize(rl1)
	list2 := Normalize(rl2)
	return equal(list1, list2)
}

// equal performs direct comparison of two ResourceLists without normalization
// Parameters:
//
//	rl1: First ResourceList to compare
//	rl2: Second ResourceList to compare
//
// Returns:
//
//	true if ResourceLists have same length and all corresponding resources are equal, false otherwise
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

// Normalize removes non-concerned resources from the ResourceList
// Parameters:
//
//	rl: ResourceList to normalize
//
// Returns:
//
//	New ResourceList containing only concerned resources
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

// IsConcernedResource checks if the given resource name is a concerned resource type
// Concerned resources include CPU, Memory, GPU types, Storage, EphemeralStorage, and RDMA
// Parameters:
//
//	name: ResourceName to check
//
// Returns:
//
//	true if the resource is concerned, false otherwise
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

// MultiResource multiplies all resource quantities in the ResourceList by the replica count
// Parameters:
//
//	inputs: ResourceList to multiply
//	replica: Multiplication factor
//
// Returns:
//
//	New ResourceList with multiplied quantities
func MultiResource(inputs corev1.ResourceList, replica int64) corev1.ResourceList {
	result := corev1.ResourceList{}
	for k, v := range inputs {
		result[k] = *resource.NewQuantity(replica*v.Value(), v.Format)
	}
	return result
}

// CvtToResourceList converts string representations of resources to a ResourceList
// Supports CPU, Memory, GPU, EphemeralStorage, and RDMA resources
// Parameters:
//
//	cpu, memory, gpu, gpuName, ephemeralStore, rdmaResource: String representations of resource quantities
//	replica: Replication factor to multiply resources by
//
// Returns:
//
//	ResourceList containing parsed resources, or error if parsing fail
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
		if gpuQuantity.Value() < 0 {
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
		if rdmaQuantity.Value() < 0 {
			return nil, fmt.Errorf("invalid rdma resource")
		}
		result[corev1.ResourceName(commonconfig.GetRdmaName())] = rdmaQuantity
	}
	return MultiResource(result, replica), nil
}

// Format formats a resource quantity for display based on resource type
// Memory and storage resources are formatted in GiB units
// Parameters:
//
//	key: Resource type as string
//	quantity: Resource quantity to format
//
// Returns:
//
//	Formatted string representation of the quantit
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

// GetAvailableResource calculates available resources after reserving configured percentages
// Reserves memory, CPU, and ephemeral storage based on configuration
// Parameters:
//
//	resources: ResourceList to calculate available resources from
//
// Returns:
//
//	ResourceList with reserved resources subtracted
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

// GetMaxEphemeralStoreQuantity calculates maximum ephemeral storage quantity based on configuration
// Considers reserve percentage and maximum percentage configurations
// Parameters:
//
//	resources: ResourceList containing ephemeral storage resource
//
// Returns:
//
//	Pointer to calculated maximum ephemeral storage quantity, or error if not found
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

// ToString converts a resource quantity to a human-readable string format
// Formats bytes as Mi or Gi units depending on magnitude
// Parameters:
//
//	q: Quantity to convert
//
// Returns:
//
//	String representation in Mi or Gi units, or empty string for very small values
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
