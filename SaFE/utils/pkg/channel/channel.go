/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package channel

// IsChannelClosed returns true if the channel is closed.
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
