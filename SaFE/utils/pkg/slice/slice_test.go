/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package slice

import (
	"reflect"
	"testing"

	"gotest.tools/assert"
)

func TestContainsStrings(t *testing.T) {
	slice1 := []string{"1", "2"}
	assert.Equal(t, ContainsStrings(slice1, []string{"1"}), true)
	assert.Equal(t, ContainsStrings(slice1, []string{"1", "2"}), true)
	assert.Equal(t, ContainsStrings(slice1, []string{"3"}), false)
	assert.Equal(t, ContainsStrings(slice1, []string{""}), false)
	assert.Equal(t, ContainsStrings(slice1, []string{"1", ""}), false)
}

func TestDifference(t *testing.T) {
	// Difference returns elements in slice1 that are NOT in slice2
	assert.Equal(t, reflect.DeepEqual(Difference([]string{"1", "2"}, []string{"1"}), []string{"2"}), true)
	assert.Equal(t, reflect.DeepEqual(Difference([]string{"1", "2"}, []string{}), []string{"1", "2"}), true)
	// Fixed: Difference(["1", "2"], ["3"]) should return ["1", "2"], not ["1", "2", "3"]
	assert.Equal(t, reflect.DeepEqual(Difference([]string{"1", "2"}, []string{"3"}), []string{"1", "2"}), true)
	assert.Equal(t, len(Difference([]string{"1", "2"}, []string{"1", "2"})), 0)
	// Fixed: Difference(["1", "2"], ["1", "2", "3"]) should return [] (empty), not ["3"]
	assert.Equal(t, len(Difference([]string{"1", "2"}, []string{"1", "2", "3"})), 0)
	// Fixed: Difference(["1", "2"], ["1", "2", "3", "3"]) should return [] (empty)
	assert.Equal(t, len(Difference([]string{"1", "2"}, []string{"1", "2", "3", "3"})), 0)
	// Fixed: Difference(["1", "2", "4", "4"], ["1", "2", "3", "3"]) should return ["4", "4"]
	assert.Equal(t, reflect.DeepEqual(Difference([]string{"1", "2", "4", "4"}, []string{"1", "2", "3", "3"}),
		[]string{"4", "4"}), true)
}

func TestRemoveStrings(t *testing.T) {
	slice1 := []string{"1", "2"}
	slice2 := []string{"1", "2"}
	resp, ok := RemoveStrings(slice1, slice2)
	assert.Equal(t, ok, true)
	assert.Equal(t, len(resp) == 0, true)

	slice2 = []string{"1"}
	resp, ok = RemoveStrings(slice1, slice2)
	assert.Equal(t, ok, true)
	assert.Equal(t, reflect.DeepEqual(resp, []string{"2"}), true)

	slice2 = []string{"1", "3"}
	resp, ok = RemoveStrings(slice1, slice2)
	assert.Equal(t, ok, true)
	assert.Equal(t, reflect.DeepEqual(resp, []string{"2"}), true)

	slice2 = []string{}
	resp, ok = RemoveStrings(slice1, slice2)
	assert.Equal(t, ok, false)
	assert.Equal(t, reflect.DeepEqual(resp, []string{"1", "2"}), true)

	slice2 = []string{"3", "4"}
	resp, ok = RemoveStrings(slice1, slice2)
	assert.Equal(t, ok, false)
	assert.Equal(t, reflect.DeepEqual(resp, []string{"1", "2"}), true)
}

func TestAddStrings(t *testing.T) {
	slice1 := []string{"1", "2"}
	slice2 := []string{"1", "2"}
	resp, ok := AddAndDelDuplicates(slice1, slice2)
	assert.Equal(t, reflect.DeepEqual(resp, []string{"1", "2"}), true)
	assert.Equal(t, ok, false)

	slice2 = []string{"3"}
	resp, ok = AddAndDelDuplicates(slice1, slice2)
	assert.Equal(t, reflect.DeepEqual(resp, []string{"1", "2", "3"}), true)
	assert.Equal(t, ok, true)

	slice2 = []string{"1", "3"}
	resp, ok = AddAndDelDuplicates(slice1, slice2)
	assert.Equal(t, reflect.DeepEqual(resp, []string{"1", "2", "3"}), true)
	assert.Equal(t, ok, true)

	slice2 = []string{}
	resp, ok = AddAndDelDuplicates(slice1, slice2)
	assert.Equal(t, reflect.DeepEqual(resp, []string{"1", "2"}), true)
	assert.Equal(t, ok, false)

	slice2 = []string{"3", "4"}
	resp, ok = AddAndDelDuplicates(slice1, slice2)
	assert.Equal(t, reflect.DeepEqual(resp, []string{"1", "2", "3", "4"}), true)
	assert.Equal(t, ok, true)
}
