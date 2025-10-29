package sliceUtil

import "math"

func PaginateSlice[T any](data []T, page int, pageSize int) ([]T, int, int, bool) {
	total := len(data)
	if pageSize <= 0 {
		pageSize = 10
	}
	if page <= 0 {
		page = 1
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	start := (page - 1) * pageSize
	end := start + pageSize
	if start >= total {
		return []T{}, totalPages, total, false
	}
	if end > total {
		end = total
	}

	paged := data[start:end]
	hasNext := page < totalPages

	return paged, totalPages, total, hasNext
}
