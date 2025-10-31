package util

func CalculateMaxValue(bits int) uint64 {
	if bits < 1 || bits > 64 {
		// Invalid bit count, return error value
		return 0
	}

	// Calculate max value using bit operations
	maxValue := uint64(1)<<uint(bits-1) - 1
	return maxValue
}
