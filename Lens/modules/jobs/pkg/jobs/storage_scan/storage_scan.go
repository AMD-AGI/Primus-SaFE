package storage_scan

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/jobs/pkg/common"
)

type StorageScanJob struct {
}

func (s *StorageScanJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	stats := common.NewExecutionStats()
	
	scanner := &Scanner{Targets: []ClusterTarget{
		{
			Name:       "K8S",
			ClientSets: clientSets,
			Extra:      nil,
		},
	}}
	
	scanStart := time.Now()
	result, err := scanner.Run(ctx)
	stats.QueryDuration = time.Since(scanStart).Seconds()
	if err != nil {
		return stats, err
	}
	
	for _, report := range result {
		for _, item := range report.BackendItems {
			dbItem := &dbModel.Storage{
				Name: item.BackendName,
				Kind: string(item.BackendKind),
				Config: map[string]interface{}{
					"meta_secret": item.MetaSecret,
				},
				Source:    "scan",
				Status:    string(item.Health),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			existDbItem, err := database.GetFacade().GetStorage().GetStorageByKindAndName(ctx, dbItem.Kind, dbItem.Name)
			if err != nil {
				stats.ErrorCount++
				log.Errorf("Fail to get storage %s/%s: %v", dbItem.Kind, dbItem.Name, err)
				continue
			}
			if existDbItem != nil {
				dbItem.ID = existDbItem.ID
				dbItem.CreatedAt = existDbItem.CreatedAt
				err = database.GetFacade().GetStorage().UpdateStorage(ctx, dbItem)
				if err != nil {
					stats.ErrorCount++
					log.Errorf("Fail to update storage %s/%s: %v", dbItem.Kind, dbItem.Name, err)
					continue
				}
				stats.ItemsUpdated++
				log.Infof("Storage %s/%s updated", dbItem.Kind, dbItem.Name)
			} else {
				err = database.GetFacade().GetStorage().CreateStorage(ctx, dbItem)
				if err != nil {
					stats.ErrorCount++
					log.Errorf("Fail to create storage %s/%s: %v", dbItem.Kind, dbItem.Name, err)
					continue
				}
				stats.ItemsCreated++
				log.Infof("Storage %s/%s created", dbItem.Kind, dbItem.Name)
			}
			stats.RecordsProcessed++
		}
	}
	
	stats.AddMessage("Storage scan completed successfully")
	return stats, nil
}

func (s *StorageScanJob) Schedule() string {
	return "@every 1m"
}
