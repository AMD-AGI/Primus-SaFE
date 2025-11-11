package metrics

import "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/metrics"

var (
	BpfEventChanDrop = metrics.NewCounterVec("bpf_event_chan_drop", "bpf event channel drop", []string{"type"})
	BpfEventRecv     = metrics.NewCounterVec("bpf_event_recv", "bpf event received", []string{"type"})
)
