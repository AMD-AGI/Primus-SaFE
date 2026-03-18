/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"k8s.io/klog/v2"
)

// trackGithubWorkflow extracts GitHub metadata from EphemeralRunner events
// and records workflow runs in SaFE DB for CI/CD observability.
func (r *SyncerReconciler) trackGithubWorkflow(ctx context.Context,
	message *resourceMessage, adminWorkload *v1.Workload, clientSets *ClusterClientSets) {

	obj, err := jobutils.GetObject(ctx, clientSets.ClientFactory(), message.name, message.namespace, message.gvk)
	if err != nil {
		if message.action != ResourceDel {
			klog.V(2).Infof("[github-hook] cannot get EphemeralRunner %s/%s: %v", message.namespace, message.name, err)
		}
		return
	}

	isCompleted := message.action == ResourceDel || adminWorkload.IsEnd()
	cluster := v1.GetClusterId(adminWorkload)

	r.workflowTracker.OnEphemeralRunnerEvent(ctx, obj, adminWorkload.Name, cluster, isCompleted)
}
