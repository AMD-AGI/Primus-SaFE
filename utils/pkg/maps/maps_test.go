/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package maps

import (
	"reflect"
	"testing"

	"gotest.tools/assert"
)

func TestDifference(t *testing.T) {
	m1 := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	m2 := map[string]string{
		"key1": "value11",
		"key3": "value3",
	}
	result := Difference(m1, m2)
	assert.DeepEqual(t, result, map[string]string{
		"key2": "value2",
	})
}

func TestMerge(t *testing.T) {
	m1 := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	m2 := map[string]string{
		"key1": "value11",
		"key3": "value3",
	}
	result := Merge(m1, m2)
	assert.DeepEqual(t, EqualIgnoreOrder(result, map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}), true)
}

func TestCompareWithKeys(t *testing.T) {
	m1 := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	m2 := map[string]string{
		"key1": "value11",
		"key2": "value2",
		"key3": "value3",
	}
	result := CompareWithKeys(m1, m2, []string{"key1"})
	assert.DeepEqual(t, result, false)

	result = CompareWithKeys(m1, m2, []string{"key2"})
	assert.DeepEqual(t, result, true)

	result = CompareWithKeys(m1, m2, []string{"key3"})
	assert.DeepEqual(t, result, false)
}

func TestContain(t *testing.T) {
	m1 := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	m2 := map[string]string{
		"key1": "value11",
		"key2": "value2",
	}
	assert.DeepEqual(t, Contain(m1, m2), false)

	m2 = map[string]string{
		"key2": "value2",
	}
	assert.DeepEqual(t, Contain(m1, m2), true)

	m2 = map[string]string{
		"key2": "value2",
		"key3": "value3",
	}
	assert.DeepEqual(t, Contain(m1, m2), false)
}

func TestCopy(t *testing.T) {
	m1 := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	m2 := Copy(m1)
	assert.DeepEqual(t, reflect.DeepEqual(m1, m2), true)
}
