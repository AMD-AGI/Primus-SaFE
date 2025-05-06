/*
 * Copyright © AMD. 2025-2026. All rights reserved.
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
