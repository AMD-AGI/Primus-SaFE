// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package sliceUtil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPaginateSlice(t *testing.T) {
	t.Run("normal pagination", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 3)
		
		assert.Equal(t, []int{1, 2, 3}, paged)
		assert.Equal(t, 4, totalPages)
		assert.Equal(t, 10, total)
		assert.True(t, hasNext)
	})

	t.Run("second page", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		paged, totalPages, total, hasNext := PaginateSlice(data, 2, 3)
		
		assert.Equal(t, []int{4, 5, 6}, paged)
		assert.Equal(t, 4, totalPages)
		assert.Equal(t, 10, total)
		assert.True(t, hasNext)
	})

	t.Run("last page", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		paged, totalPages, total, hasNext := PaginateSlice(data, 4, 3)
		
		assert.Equal(t, []int{10}, paged)
		assert.Equal(t, 4, totalPages)
		assert.Equal(t, 10, total)
		assert.False(t, hasNext)
	})

	t.Run("page number out of range", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		paged, totalPages, total, hasNext := PaginateSlice(data, 10, 3)
		
		assert.Equal(t, []int{}, paged)
		assert.Equal(t, 2, totalPages)
		assert.Equal(t, 5, total)
		assert.False(t, hasNext)
	})

	t.Run("empty data", func(t *testing.T) {
		data := []int{}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 10)
		
		assert.Equal(t, []int{}, paged)
		assert.Equal(t, 0, totalPages)
		assert.Equal(t, 0, total)
		assert.False(t, hasNext)
	})

	t.Run("page defaults to 1 when 0", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		paged, totalPages, total, hasNext := PaginateSlice(data, 0, 2)
		
		assert.Equal(t, []int{1, 2}, paged)
		assert.Equal(t, 3, totalPages)
		assert.Equal(t, 5, total)
		assert.True(t, hasNext)
	})

	t.Run("page size defaults to 10 when 0", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 0)
		
		assert.Equal(t, []int{1, 2, 3, 4, 5}, paged)
		assert.Equal(t, 1, totalPages)
		assert.Equal(t, 5, total)
		assert.False(t, hasNext)
	})

	t.Run("page size defaults to 10 when negative", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, -5)
		
		assert.Equal(t, []int{1, 2, 3, 4, 5}, paged)
		assert.Equal(t, 1, totalPages)
		assert.Equal(t, 5, total)
		assert.False(t, hasNext)
	})

	t.Run("string slice pagination", func(t *testing.T) {
		data := []string{"a", "b", "c", "d", "e"}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 2)
		
		assert.Equal(t, []string{"a", "b"}, paged)
		assert.Equal(t, 3, totalPages)
		assert.Equal(t, 5, total)
		assert.True(t, hasNext)
	})

	t.Run("struct slice pagination", func(t *testing.T) {
		type User struct {
			Name string
			Age  int
		}
		data := []User{
			{Name: "Alice", Age: 25},
			{Name: "Bob", Age: 30},
			{Name: "Charlie", Age: 35},
		}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 2)
		
		assert.Equal(t, []User{
			{Name: "Alice", Age: 25},
			{Name: "Bob", Age: 30},
		}, paged)
		assert.Equal(t, 2, totalPages)
		assert.Equal(t, 3, total)
		assert.True(t, hasNext)
	})

	t.Run("single page data", func(t *testing.T) {
		data := []int{1, 2, 3}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 10)
		
		assert.Equal(t, []int{1, 2, 3}, paged)
		assert.Equal(t, 1, totalPages)
		assert.Equal(t, 3, total)
		assert.False(t, hasNext)
	})

	t.Run("page size equals data length", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 5)
		
		assert.Equal(t, []int{1, 2, 3, 4, 5}, paged)
		assert.Equal(t, 1, totalPages)
		assert.Equal(t, 5, total)
		assert.False(t, hasNext)
	})
}

