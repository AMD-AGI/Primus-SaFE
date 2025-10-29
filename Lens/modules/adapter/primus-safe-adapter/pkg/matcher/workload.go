package matcher

import (
	"context"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	primusSafeConstant "github.com/AMD-AGI/primus-lens/primus-safe-adapter/pkg/constant"
)

var DefaultWorkloadMatcher = &WorkloadMatcher{}

func InitWorkloadMatcher(ctx context.Context) {
	DefaultWorkloadMatcher.Start(ctx)
}

type WorkloadMatcher struct {
}

func (w *WorkloadMatcher) Start(ctx context.Context) {
	go w.run(ctx)
}

func (w *WorkloadMatcher) run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			err := w.doScan(ctx)
			if err != nil {
				log.Errorf("failed to scan workloads: %v", err)
			}
		case <-ctx.Done():
			log.Info("WorkloadMatcher stopped")
			return
		}
	}
}

func (w *WorkloadMatcher) scanForSingleWorkload(ctx context.Context, dbWorkload *model.GpuWorkload) error {
	children, err := database.ListChildrenWorkloadByParentUid(ctx, dbWorkload.UID)
	if err != nil {
		return err
	}
	if countInter, ok := dbWorkload.Labels[primusSafeConstant.WorkloadDispatchCountLabel]; ok {
		count, converted := countInter.(int)
		if !converted {
			log.Warnf("workload %s/%s has invalid dispatch count label", dbWorkload.Namespace, dbWorkload.Name)
			return nil
		}
		if len(children) == count {
			return nil
		}
	}
	referencedWorkload, err := database.ListWorkloadByLabelValue(ctx, primusSafeConstant.WorkloadIdLabel, dbWorkload.Name)
	if err != nil {
		return err
	}
	if len(referencedWorkload) == 0 {
		return nil
	}
	for _, workload := range referencedWorkload {
		if workload.ParentUID == "" {
			dbWorkload.ParentUID = workload.UID
			err = database.UpdateGpuWorkload(ctx, dbWorkload)
			if err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (w *WorkloadMatcher) doScan(ctx context.Context) error {
	workloads, err := database.ListWorkloadNotEndByKind(ctx, "Workload")
	if err != nil {
		return err
	}
	for i := range workloads {
		err := w.scanForSingleWorkload(ctx, workloads[i])
		if err != nil {
			log.Errorf("failed to scan workload %s/%s: %v", workloads[i].Namespace, workloads[i].Name, err)
			continue
		}
	}
	return nil
}
