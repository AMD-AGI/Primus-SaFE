package sliceUtil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPaginateSlice(t *testing.T) {
	t.Run("正常分页", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 3)
		
		assert.Equal(t, []int{1, 2, 3}, paged)
		assert.Equal(t, 4, totalPages)
		assert.Equal(t, 10, total)
		assert.True(t, hasNext)
	})

	t.Run("第二页", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		paged, totalPages, total, hasNext := PaginateSlice(data, 2, 3)
		
		assert.Equal(t, []int{4, 5, 6}, paged)
		assert.Equal(t, 4, totalPages)
		assert.Equal(t, 10, total)
		assert.True(t, hasNext)
	})

	t.Run("最后一页", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		paged, totalPages, total, hasNext := PaginateSlice(data, 4, 3)
		
		assert.Equal(t, []int{10}, paged)
		assert.Equal(t, 4, totalPages)
		assert.Equal(t, 10, total)
		assert.False(t, hasNext)
	})

	t.Run("页码超出范围", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		paged, totalPages, total, hasNext := PaginateSlice(data, 10, 3)
		
		assert.Equal(t, []int{}, paged)
		assert.Equal(t, 2, totalPages)
		assert.Equal(t, 5, total)
		assert.False(t, hasNext)
	})

	t.Run("空数据", func(t *testing.T) {
		data := []int{}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 10)
		
		assert.Equal(t, []int{}, paged)
		assert.Equal(t, 0, totalPages)
		assert.Equal(t, 0, total)
		assert.False(t, hasNext)
	})

	t.Run("页码为0时默认为1", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		paged, totalPages, total, hasNext := PaginateSlice(data, 0, 2)
		
		assert.Equal(t, []int{1, 2}, paged)
		assert.Equal(t, 3, totalPages)
		assert.Equal(t, 5, total)
		assert.True(t, hasNext)
	})

	t.Run("页大小为0时默认为10", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 0)
		
		assert.Equal(t, []int{1, 2, 3, 4, 5}, paged)
		assert.Equal(t, 1, totalPages)
		assert.Equal(t, 5, total)
		assert.False(t, hasNext)
	})

	t.Run("页大小为负数时默认为10", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, -5)
		
		assert.Equal(t, []int{1, 2, 3, 4, 5}, paged)
		assert.Equal(t, 1, totalPages)
		assert.Equal(t, 5, total)
		assert.False(t, hasNext)
	})

	t.Run("字符串切片分页", func(t *testing.T) {
		data := []string{"a", "b", "c", "d", "e"}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 2)
		
		assert.Equal(t, []string{"a", "b"}, paged)
		assert.Equal(t, 3, totalPages)
		assert.Equal(t, 5, total)
		assert.True(t, hasNext)
	})

	t.Run("结构体切片分页", func(t *testing.T) {
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

	t.Run("单页数据", func(t *testing.T) {
		data := []int{1, 2, 3}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 10)
		
		assert.Equal(t, []int{1, 2, 3}, paged)
		assert.Equal(t, 1, totalPages)
		assert.Equal(t, 3, total)
		assert.False(t, hasNext)
	})

	t.Run("页大小等于数据长度", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		paged, totalPages, total, hasNext := PaginateSlice(data, 1, 5)
		
		assert.Equal(t, []int{1, 2, 3, 4, 5}, paged)
		assert.Equal(t, 1, totalPages)
		assert.Equal(t, 5, total)
		assert.False(t, hasNext)
	})
}

