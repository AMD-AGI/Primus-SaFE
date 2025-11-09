package matcher

import (
	"context"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	primusSafeConstant "github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/constant"
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
	// Get cluster ID from workload labels
	clusterID := ""
	if clusterIDInter, ok := dbWorkload.Labels[primusSafeConstant.ClusterIdLabel]; ok {
		clusterID, _ = clusterIDInter.(string)
	}

	// Get the appropriate facade based on cluster ID
	var facade database.FacadeInterface
	if clusterID != "" {
		facade = database.GetFacadeForCluster(clusterID)
	} else {
		facade = database.GetFacade()
	}

	children, err := facade.GetWorkload().ListChildrenWorkloadByParentUid(ctx, dbWorkload.UID)
	if err != nil {
		return err
	}
	if countInter, ok := dbWorkload.Labels[primusSafeConstant.WorkloadDispatchCountLabel]; ok {
		var count = 0
		var err error
		if countStr, ok := countInter.(string); ok {
			count, err = strconv.Atoi(countStr)
			if err != nil {
				log.Warnf("workload %s/%s has invalid dispatch count label. Label value: %v type: %T. Error: %v", dbWorkload.Namespace, dbWorkload.Name, countInter, countInter, err)
				return nil
			}
		} else if countInt, ok := countInter.(int); ok {
			count = countInt
		} else if countFloat, ok := countInter.(float64); ok {
			count = int(countFloat)
		} else {
			log.Warnf("workload %s/%s has invalid dispatch count label. Label value: %v type: %T", dbWorkload.Namespace, dbWorkload.Name, countInter, countInter)
			return nil
		}
		if len(children) == int(count) {
			return nil
		}
	}
	referencedWorkload, err := facade.GetWorkload().ListWorkloadByLabelValue(ctx, primusSafeConstant.WorkloadIdLabel, dbWorkload.Name)
	if err != nil {
		return err
	}
	if len(referencedWorkload) == 0 {
		return nil
	}
	// 将当前 Workload（父）的 UID 设置为子 workload 的 parent_uid
	for _, childWorkload := range referencedWorkload {
		if childWorkload.UID == dbWorkload.UID {
			if childWorkload.ParentUID == childWorkload.UID {
				childWorkload.ParentUID = ""
				err = facade.GetWorkload().UpdateGpuWorkload(ctx, childWorkload)
				if err != nil {
					log.Errorf("failed to update child workload %s/%s parent_uid: %v",
						childWorkload.Namespace, childWorkload.Name, err)
					continue
				}
			}
			continue
		}
		if childWorkload.ParentUID == "" {
			childWorkload.ParentUID = dbWorkload.UID
			err = facade.GetWorkload().UpdateGpuWorkload(ctx, childWorkload)
			if err != nil {
				log.Errorf("failed to update child workload %s/%s parent_uid: %v",
					childWorkload.Namespace, childWorkload.Name, err)
				continue
			}
		}
	}

	// 复制子workload的pod引用到父workload
	err = w.copyChildPodReferencesToParent(ctx, facade, dbWorkload, referencedWorkload)
	if err != nil {
		log.Errorf("failed to copy child pod references to parent workload %s/%s: %v",
			dbWorkload.Namespace, dbWorkload.Name, err)
		return err
	}

	return nil
}

func (w *WorkloadMatcher) copyChildPodReferencesToParent(ctx context.Context, facade database.FacadeInterface, parentWorkload *model.GpuWorkload, childWorkloads []*model.GpuWorkload) error {
	// 收集所有子workload的UID（排除父workload自己）
	childUIDs := make([]string, 0, len(childWorkloads))
	for _, child := range childWorkloads {
		if child.UID != parentWorkload.UID {
			childUIDs = append(childUIDs, child.UID)
		}
	}

	if len(childUIDs) == 0 {
		return nil
	}

	// 获取父workload已有的pod引用
	existingParentRefs, err := facade.GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, parentWorkload.UID)
	if err != nil {
		return err
	}

	// 创建已存在的pod UID集合，用于快速查找
	existingPodUIDs := make(map[string]bool)
	for _, ref := range existingParentRefs {
		existingPodUIDs[ref.PodUID] = true
	}

	// 收集所有子workload的pod引用
	allChildPodUIDs := make(map[string]bool)
	for _, childUID := range childUIDs {
		childPodRefs, err := facade.GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, childUID)
		if err != nil {
			log.Warnf("failed to get pod references for child workload %s: %v", childUID, err)
			continue
		}
		for _, ref := range childPodRefs {
			allChildPodUIDs[ref.PodUID] = true
		}
	}

	// 为父workload创建尚不存在的pod引用
	createdCount := 0
	for podUID := range allChildPodUIDs {
		if !existingPodUIDs[podUID] {
			err := facade.GetWorkload().CreateWorkloadPodReference(ctx, parentWorkload.UID, podUID)
			if err != nil {
				log.Warnf("failed to create pod reference for parent workload %s/%s, pod %s: %v",
					parentWorkload.Namespace, parentWorkload.Name, podUID, err)
				continue
			}
			createdCount++
		}
	}

	if createdCount > 0 {
		log.Infof("copied %d pod references from child workloads to parent workload %s/%s",
			createdCount, parentWorkload.Namespace, parentWorkload.Name)
	}

	return nil
}

func (w *WorkloadMatcher) doScan(ctx context.Context) error {
	// Get all cluster names from ClusterManager
	clusterManager := clientsets.GetClusterManager()
	clusterNames := clusterManager.GetClusterNames()

	// If no clusters found, scan the default database
	if len(clusterNames) == 0 {
		return w.scanCluster(ctx, "")
	}

	// Scan each cluster
	for _, clusterName := range clusterNames {
		if err := w.scanCluster(ctx, clusterName); err != nil {
			log.Errorf("failed to scan cluster %s: %v", clusterName, err)
			// Continue to next cluster even if one fails
			continue
		}
	}

	return nil
}

func (w *WorkloadMatcher) scanCluster(ctx context.Context, clusterName string) error {
	// Get the appropriate facade based on cluster name
	var facade database.FacadeInterface
	if clusterName != "" {
		facade = database.GetFacadeForCluster(clusterName)
	} else {
		facade = database.GetFacade()
	}

	workloads, err := facade.GetWorkload().ListWorkloadNotEndByKind(ctx, "Workload")
	if err != nil {
		return err
	}

	log.Infof("scanning %d workloads in cluster %s", len(workloads), clusterName)

	for i := range workloads {
		err := w.scanForSingleWorkload(ctx, workloads[i])
		if err != nil {
			log.Errorf("failed to scan workload %s/%s in cluster %s: %v",
				workloads[i].Namespace, workloads[i].Name, clusterName, err)
			continue
		}
	}
	return nil
}
