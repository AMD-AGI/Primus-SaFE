/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package channel

// IsChannelClosed checks if an unbuffered channel is closed.
// This function should only be used with unbuffered channels, not buffered ones.
//
// For unbuffered channels:
//   - If the channel is nil, it's considered closed and returns true
//   - If the channel is closed, it returns true
//   - If the channel is open and has no data, it returns false
//
// Note: This function does not block. It uses a non-blocking select statement
// to check the channel status.
//
// Parameters:
//   - ch: The unbuffered channel to check for closure status
//
// Returns:
//   - bool: true if the channel is closed or nil, false if the channel is open
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
