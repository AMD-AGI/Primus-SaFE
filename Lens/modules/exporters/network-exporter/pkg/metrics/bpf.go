// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metrics

import "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/metrics"

var (
	BpfEventChanDrop = metrics.NewCounterVec("bpf_event_chan_drop", "bpf event channel drop", []string{"type"})
	BpfEventRecv     = metrics.NewCounterVec("bpf_event_recv", "bpf event received", []string{"type"})
)
