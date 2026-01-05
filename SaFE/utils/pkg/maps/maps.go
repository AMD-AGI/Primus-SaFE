/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package maps

// Difference returns a map containing key-value pairs that exist in m1 but not in m2.
func Difference(m1, m2 map[string]string) map[string]string {
	result := make(map[string]string)
	for key, value := range m1 {
		if _, ok := m2[key]; !ok {
			result[key] = value
		}
	}
	return result
}

// Append combines two maps into one, with an option to control whether existing keys in m1 should be overwritten by those from m2.
// Returns the modified m1 after appending m2 to it based on the overwrite rule.
func Append(m1, m2 map[string]string, overwrite bool) map[string]string {
	for key, value := range m2 {
		_, ok := m1[key]
		if ok && !overwrite {
			continue
		}
		m1[key] = value
	}
	return m1
}

// EqualIgnoreOrder checks if two maps have the same key-value pairs regardless of order.
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

// CompareWithKeys compares specific keys between two maps.
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

// Contain checks if m1 contains all key-value pairs from m2.
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

// Copy creates a shallow copy of the given map.
func Copy(m map[string]string, ignoreKeys ...string) map[string]string {
	result := make(map[string]string)
	for key, val := range m {
		isIgnored := false
		for _, ignoreKey := range ignoreKeys {
			if key == ignoreKey {
				isIgnored = true
				break
			}
		}
		if !isIgnored {
			result[key] = val
		}
	}
	return result
}

// RemoveValue removes all entries from the map where the value matches the input string.
func RemoveValue(m map[string]string, value string) map[string]string {
	result := make(map[string]string)
	for key, val := range m {
		if val == value {
			continue
		}
		result[key] = val
	}
	return result
}
