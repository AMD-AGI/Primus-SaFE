/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"database/sql"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	githubpkg "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/github"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"k8s.io/klog/v2"
)

// initWorkflowTracker creates the WorkflowTracker and starts the SyncJob.
// Returns nil if DB is not enabled (tracker is optional).
func initWorkflowTracker(ctx context.Context) *githubpkg.WorkflowTracker {
	if !commonconfig.IsDBEnable() {
		klog.Info("[github] DB not enabled, workflow tracker disabled")
		return nil
	}

	gormDB, err := dbclient.NewClient().GetGormDB()
	if err != nil {
		klog.Warningf("[github] failed to get DB, workflow tracker disabled: %v", err)
		return nil
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		klog.Warningf("[github] failed to get sql.DB, workflow tracker disabled: %v", err)
		return nil
	}

	store := githubpkg.NewStore(sqlDB)
	tracker := githubpkg.NewWorkflowTracker(store)

	startSyncJob(ctx, store, sqlDB)

	klog.Info("[github] workflow tracker initialized")
	return tracker
}

func startSyncJob(ctx context.Context, store *githubpkg.Store, db *sql.DB) {
	syncJob := githubpkg.NewSyncJob(store, 20, 30*time.Second)

	go func() {
		klog.Info("[github] sync job starting")
		syncJob.Start(ctx)
	}()
}

// trackGithubWorkflow extracts GitHub metadata from EphemeralRunner events
// and records workflow runs in SaFE DB for CI/CD observability.
// This function is panic-safe — errors are logged but never propagated to the caller,
// ensuring the normal scheduling logic is not affected.
func (r *SyncerReconciler) trackGithubWorkflow(ctx context.Context,
	message *resourceMessage, adminWorkload *v1.Workload, clientSets *ClusterClientSets) {

	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("[github-hook] panic recovered (scheduling unaffected): %v", r)
		}
	}()

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
