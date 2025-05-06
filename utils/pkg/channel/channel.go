/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package channel

// This behavior only applies to unbuffered channels, not to buffered ones.
func IsChannelClosed(ch chan struct{}) bool {
	if ch == nil {
		return true
	}
	select {
	case _, received := <-ch:
		if !received {
			return true
		}
	default:
	}
	return false
}
