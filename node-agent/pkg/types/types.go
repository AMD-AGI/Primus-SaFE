/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package types

import (
	"k8s.io/client-go/util/workqueue"
)

const (
	NvidiaGpuChip = "nvidia"
	AmdGpuChip    = "amd"

	StatusOk      = 0
	StatusError   = 1
	StatusUnknown = 2
	StatusDisable = 127
)

type MonitorQueue workqueue.TypedRateLimitingInterface[*MonitorMessage]

type MonitorMessage struct {
	Id         string
	StatusCode int
	Value      string
}
