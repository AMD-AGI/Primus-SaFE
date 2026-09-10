/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package rfwdmux

// Markers the pod-side mux writes on its stderr, which is the only channel it has
// to the apiserver besides the multiplexed session on stdout. They are defined
// here so both ends of the exec agree on them by construction.
const (
	// ReadyMarker says the listen socket is bound. Nothing before it means the
	// forward is usable.
	ReadyMarker = "SAFE-RFWD-READY"
	// ErrMarker prefixes a failure the pod side detected, with the reason after it.
	ErrMarker = "SAFE-RFWD-ERR"
	// StatMarker prefixes the periodic counters: how many connections the forward
	// is carrying and how many it refused.
	StatMarker = "SAFE-RFWD-STAT"
)
