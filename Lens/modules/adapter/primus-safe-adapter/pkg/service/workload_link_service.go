// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	primusSafeConstant "github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/constant"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// WorkloadLinkService periodically scans active Workload CRDs and links
// child workloads that were not yet present in the database during the
// initial event-driven reconciliation.
type WorkloadLinkService struct {
	k8sClient client.Client
}

func NewWorkloadLinkService(k8sClient client.Client) *WorkloadLinkService {
	return &WorkloadLinkService{
		k8sClient: k8sClient,
	}
}

func (s *WorkloadLinkService) Name() string {
	return "workload-children-linker"
}

func (s *WorkloadLinkService) Run(ctx context.Context) error {
	workloadList := &primusSafeV1.WorkloadList{}
	if err := s.k8sClient.List(ctx, workloadList); err != nil {
		log.Errorf("workload-children-linker: failed to list Workload CRDs: %v", err)
		return err
	}

	linked := 0
	copied := 0
	for i := range workloadList.Items {
		wl := &workloadList.Items[i]

		if wl.DeletionTimestamp != nil {
			continue
		}
		if wl.Status.Phase != primusSafeV1.WorkloadRunning &&
			wl.Status.Phase != primusSafeV1.WorkloadPending {
			continue
		}

		l, c, err := s.linkOne(ctx, wl)
		if err != nil {
			log.Warnf("workload-children-linker: %s/%s: %v", wl.Namespace, wl.Name, err)
			continue
		}
		linked += l
		copied += c
	}

	if linked > 0 || copied > 0 {
		log.Infof("workload-children-linker: linked %d children, copied %d pod refs", linked, copied)
	}
	return nil
}

// linkOne links child workloads and copies their pod references for a single
// Workload CRD.  Returns (children linked, pod refs copied, error).
func (s *WorkloadLinkService) linkOne(ctx context.Context, workload *primusSafeV1.Workload) (int, int, error) {
	clusterID := primusSafeV1.GetClusterId(workload)

	var facade database.FacadeInterface
	if clusterID != "" {
		cm := clientsets.GetClusterManager()
		if cm != nil {
			if _, err := cm.GetClientSetByClusterName(clusterID); err != nil {
				return 0, 0, nil
			}
		}
		facade = database.GetFacadeForCluster(clusterID)
	} else {
		facade = database.GetFacade()
	}

	parentUID := string(workload.UID)

	children, err := facade.GetWorkload().ListWorkloadByLabelValue(
		ctx, primusSafeConstant.WorkloadIdLabel, workload.Name)
	if err != nil {
		return 0, 0, err
	}
	if len(children) == 0 {
		return 0, 0, nil
	}

	linkedCount := 0
	for _, child := range children {
		if child.UID == parentUID {
			continue
		}
		if child.ParentUID == "" {
			child.ParentUID = parentUID
			if err := facade.GetWorkload().UpdateGpuWorkload(ctx, child); err != nil {
				log.Warnf("workload-children-linker: failed to set parent_uid for %s: %v", child.UID, err)
				continue
			}
			log.Debugf("workload-children-linker: linked child %s (uid=%s) to parent %s", child.Name, child.UID, workload.Name)
			linkedCount++
		}
	}

	existingRefs, err := facade.GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, parentUID)
	if err != nil {
		return linkedCount, 0, err
	}
	existingPods := make(map[string]struct{}, len(existingRefs))
	for _, ref := range existingRefs {
		existingPods[ref.PodUID] = struct{}{}
	}

	copiedCount := 0
	for _, child := range children {
		if child.UID == parentUID {
			continue
		}
		childRefs, err := facade.GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, child.UID)
		if err != nil {
			log.Warnf("workload-children-linker: failed to get pod refs for child %s: %v", child.UID, err)
			continue
		}
		for _, ref := range childRefs {
			if _, exists := existingPods[ref.PodUID]; exists {
				continue
			}
			if err := facade.GetWorkload().CreateWorkloadPodReference(ctx, parentUID, ref.PodUID); err != nil {
				log.Warnf("workload-children-linker: failed to copy pod ref %s to parent %s: %v", ref.PodUID, workload.Name, err)
				continue
			}
			existingPods[ref.PodUID] = struct{}{}
			copiedCount++
		}
	}

	return linkedCount, copiedCount, nil
}
