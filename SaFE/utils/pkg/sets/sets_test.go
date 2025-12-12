/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package sets

import (
	"reflect"
	"sort"
	"testing"

	"gotest.tools/assert"
)

func TestBasic(t *testing.T) {
	s1 := NewSet()
	s1.Insert("a1", "a2")
	assert.Equal(t, s1.Len(), 2)
	assert.Equal(t, s1.Has("a1"), true)
	assert.Equal(t, s1.Has("a3"), false)

	s1.Insert("a3")
	assert.Equal(t, s1.Has("a3"), true)

	s1.Delete("a1")
	assert.Equal(t, s1.Has("a1"), false)
	assert.Equal(t, s1.Len(), 2)

	keys := s1.UnsortedList()
	sort.Strings(keys)
	assert.Equal(t, reflect.DeepEqual(keys, []string{"a2", "a3"}), true)

	s2 := s1.Clone()
	assert.Equal(t, s1.Equal(s2), true)

	s1.Clear()
	assert.Equal(t, s1.Len(), 0)
	assert.Equal(t, s2.Len(), 2)
}

func TestDifference(t *testing.T) {
	s1 := NewSet()
	s1.Insert("a1", "a2", "a3")
	s2 := NewSetByKeys("a1", "a2", "a4", "a5")

	resp := s1.Difference(s2)
	assert.Equal(t, resp.Equal(NewSetByKeys("a3")), true)

	resp = s2.Difference(s1)
	assert.Equal(t, resp.Equal(NewSetByKeys("a4", "a5")), true)
}

func TestNewEmptyValues(t *testing.T) {
	var nullList []string
	nullList = nil
	s := NewSetByKeys(nullList...)
	assert.Equal(t, s.Len(), 0)
}

func TestUnion(t *testing.T) {
	s1 := NewSetByKeys("a1", "a2")
	s2 := NewSetByKeys("a1", "a3")

	resp := s1.Union(s2)
	assert.Equal(t, resp.Equal(NewSetByKeys("a1", "a2", "a3")), true)

	resp = s2.Union(s1)
	assert.Equal(t, resp.Equal(NewSetByKeys("a1", "a2", "a3")), true)
}

func TestUIntersection(t *testing.T) {
	s1 := NewSetByKeys("a1", "a2")
	s2 := NewSetByKeys("a2", "a3")

	resp := s1.Intersection(s2)
	assert.Equal(t, resp.Equal(NewSetByKeys("a2")), true)
}
