/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

const (
	PrimusSafePrefix = "primus-safe."

	// node
	NodePrefix = PrimusSafePrefix + "node."
	// The expected GPU count for the node, it should be annotated as a label
	NodeGpuCountLabel = NodePrefix + "gpu.count"
	// The node's last startup time
	NodeStartupTimeLabel = NodePrefix + "startup.time"
)
