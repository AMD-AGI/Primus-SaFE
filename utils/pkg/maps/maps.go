/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package maps

// Difference returns a set of objects that are in m1 but not in m2
func Difference(m1, m2 map[string]string) map[string]string {
	result := make(map[string]string)
	for key, value := range m1 {
		if _, ok := m2[key]; !ok {
			result[key] = value
		}
	}
	return result
}

// Merge m1 into m2, preferring m1's values for duplicate keys
func Merge(m1, m2 map[string]string) map[string]string {
	result := make(map[string]string)
	for key, value := range m2 {
		result[key] = value
	}
	for key, value := range m1 {
		result[key] = value
	}
	return result
}

func EqualIgnoreOrder(m1, m2 map[string]string) bool {
	if len(m1) != len(m2) {
		return false
	}
	for key, value := range m1 {
		value2, ok := m2[key]
		if !ok {
			return false
		}
		if value != value2 {
			return false
		}
	}
	return true
}

func CompareWithKeys(m1, m2 map[string]string, keys []string) bool {
	for _, key := range keys {
		val1, ok1 := m1[key]
		val2, ok2 := m2[key]
		if ok1 != ok2 || (ok1 && val1 != val2) {
			return false
		}
	}
	return true
}

func Contain(m1, m2 map[string]string) bool {
	if len(m2) == 0 {
		return len(m1) == 0
	}
	for key, val := range m2 {
		val2, ok := m1[key]
		if !ok || val != val2 {
			return false
		}
	}
	return true
}

func Copy(m map[string]string) map[string]string {
	result := make(map[string]string)
	for key, val := range m {
		result[key] = val
	}
	return result
}

func RemoveValue(m map[string]string, input string) map[string]string {
	result := make(map[string]string)
	for key, val := range m {
		if val == input {
			continue
		}
		result[key] = val
	}
	return result
}
