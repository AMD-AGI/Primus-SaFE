package jobs

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/robfig/cron/v3"
)

func Start(ctx context.Context) error {
	c := cron.New()
	for _, job := range jobs {
		c.AddFunc(job.Schedule(), func() {
			err := job.Run(ctx, clientsets.GetCurrentClusterK8SClientSet(), clientsets.GetCurrentClusterStorageClientSet())
			if err != nil {
				log.Errorf("Job error %v", err)
			}
		})
	}
	c.Start()
	return nil
}
