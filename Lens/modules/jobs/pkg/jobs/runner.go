package jobs

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/robfig/cron/v3"
)

func Start(ctx context.Context) error {
	c := cron.New()
	cm := clientsets.GetClusterManager()
	currentCluster := cm.GetCurrentClusterClients()
	for _, job := range jobs {
		c.AddFunc(job.Schedule(), func() {
			err := job.Run(ctx, currentCluster.K8SClientSet, currentCluster.StorageClientSet)
			if err != nil {
				log.Errorf("Job error %v", err)
			}
		})
	}
	c.Start()
	return nil
}
