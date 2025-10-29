package jobs

import (
	"context"
	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/device_info"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_allocation"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_consumers"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_pod"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_workload"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/node_info"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/storage_scan"
)

type Job interface {
	Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) error
	Schedule() string
}

var jobs = []Job{
	&gpu_allocation.GpuAllocationJob{},
	&gpu_consumers.GpuConsumersJob{},
	&node_info.NodeInfoJob{},
	&device_info.DeviceInfoJob{},
	&gpu_workload.GpuWorkloadJob{},
	&gpu_pod.GpuPodJob{},
	&storage_scan.StorageScanJob{},
}
