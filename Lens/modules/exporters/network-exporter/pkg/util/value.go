package util

func CalculateMaxValue(bits int) uint64 {
	if bits < 1 || bits > 64 {
		// 位数无效，返回错误值
		return 0
	}

	// 使用位运算计算最大值
	maxValue := uint64(1)<<uint(bits-1) - 1
	return maxValue
}
