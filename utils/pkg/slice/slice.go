/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package slice

import (
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

func ContainsStrings(slice1, slice2 []string) bool {
	if len(slice1) == 0 {
		return false
	}
	switch len(slice2) {
	case 0:
		return false
	case 1:
		return Contains(slice1, slice2[0])
	default:
	}

	slice1Set := sets.NewSet()
	for i := range slice1 {
		slice1Set.Insert(slice1[i])
	}
	for _, val := range slice2 {
		if !slice1Set.Has(val) {
			return false
		}
	}
	return true
}

func EqualIgnoreOrder(slice1, slice2 []string) bool {
	if len(slice1) != len(slice2) {
		return false
	}
	if len(slice1) == 0 {
		return true
	}
	record := make(map[string]int)
	for _, val := range slice1 {
		record[val]++
	}
	for _, val := range slice2 {
		if _, ok := record[val]; !ok {
			return false
		}
		record[val]--
		if record[val] == 0 {
			delete(record, val)
		}
	}

	return true
}

func RemoveString(slice []string, s string) ([]string, bool) {
	result := make([]string, 0, len(slice))
	hasRemove := false
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		} else {
			hasRemove = true
		}
	}
	return result, hasRemove
}

func RemoveStrings(slice1, slice2 []string) ([]string, bool) {
	switch len(slice2) {
	case 0:
		return slice1, false
	case 1:
		return RemoveString(slice1, slice2[0])
	default:
	}

	slice2Set := sets.NewSet()
	for i := range slice2 {
		slice2Set.Insert(slice2[i])
	}
	result := make([]string, 0, len(slice1))
	hasRemove := false
	for _, item := range slice1 {
		if slice2Set.Has(item) {
			hasRemove = true
			continue
		}
		result = append(result, item)
	}
	return result, hasRemove
}

// Appends strings from slice2 to slice1, skipping duplicates.
// Returns the resulting slice and a boolean indicating if any elements were newly added.
func AddAndDelDuplicates(slice1, slice2 []string) ([]string, bool) {
	result := make([]string, 0, len(slice1)+len(slice2))
	slice1Set := sets.NewSet()
	for i := range slice1 {
		result = append(result, slice1[i])
		slice1Set.Insert(slice1[i])
	}
	hasAdd := false
	for i := range slice2 {
		if slice1Set.Has(slice2[i]) {
			continue
		}
		hasAdd = true
		result = append(result, slice2[i])
	}
	return result, hasAdd
}

func Copy(slice []string, n int) []string {
	if n < 0 {
		return nil
	}
	l := len(slice)
	if l == 0 {
		return nil
	}
	if n > l {
		n = l
	}
	result := make([]string, 0, n)
	for i := 0; i < n; i++ {
		result = append(result, slice[i])
	}
	return result
}

// Difference returns a list of objects that are in slice1 but not in slice2.
func Difference(slice1, slice2 []string) []string {
	if len(slice1) == 0 {
		return nil
	}
	slice2Set := sets.NewSet()
	for _, str := range slice2 {
		slice2Set.Insert(str)
	}
	var result []string
	for _, str := range slice1 {
		if !slice2Set.Has(str) {
			result = append(result, str)
		}
	}
	return result
}

func Contains[T comparable](s []T, v T) bool {
	for _, elem := range s {
		if elem == v {
			return true
		}
	}
	return false
}
