// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tracelens_cleanup

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TraceLensCleanupJob cleans up expired TraceLens sessions and their associated pods.
//
// Architecture notes:
// - TraceLens pods run in the management cluster (where this job runs)
// - Session metadata is stored in data cluster databases (queried via cluster parameter)
// - This job runs in the management cluster and:
//   1. Iterates over all known clusters to find expired sessions
//   2. Deletes pods from the management cluster's primus-lens namespace
//   3. Updates session status in the respective data cluster's database
type TraceLensCleanupJob struct{}

// NewTraceLensCleanupJob creates a new TraceLensCleanupJob instance
func NewTraceLensCleanupJob() *TraceLensCleanupJob {
	return &TraceLensCleanupJob{}
}

// Run executes the TraceLens session cleanup job
func (j *TraceLensCleanupJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	startTime := time.Now()
	stats := common.NewExecutionStats()

	cm := clientsets.GetClusterManager()
	mgmtClusterName := cm.GetCurrentClusterName()
	log.Infof("TraceLensCleanupJob: starting cleanup, management cluster: %s", mgmtClusterName)

	// Get all known cluster names to iterate over
	clusterNames := cm.GetClusterNames()
	if len(clusterNames) == 0 {
		// Fallback to current cluster only
		clusterNames = []string{mgmtClusterName}
	}

	totalExpired := 0
	podsDeleted := 0
	sessionsUpdated := 0

	for _, clusterName := range clusterNames {
		facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()

		// Get expired sessions for this cluster
		expiredSessions, err := facade.ListExpired(ctx)
		if err != nil {
			log.Warnf("TraceLensCleanupJob: failed to list expired sessions for cluster %s: %v", clusterName, err)
			stats.ErrorCount++
			continue
		}

		if len(expiredSessions) == 0 {
			continue
		}

		log.Infof("TraceLensCleanupJob: found %d expired sessions in cluster %s", len(expiredSessions), clusterName)
		totalExpired += len(expiredSessions)

		for _, session := range expiredSessions {
			// Delete pod from MANAGEMENT cluster (where pods actually run)
			if session.PodName != "" && clientSets != nil && clientSets.Clientsets != nil {
				err := clientSets.Clientsets.CoreV1().Pods(session.PodNamespace).Delete(
					ctx,
					session.PodName,
					metav1.DeleteOptions{},
				)
				if err != nil {
					if errors.IsNotFound(err) {
						log.Debugf("TraceLensCleanupJob: pod %s already deleted", session.PodName)
					} else {
						log.Warnf("TraceLensCleanupJob: failed to delete pod %s: %v", session.PodName, err)
						stats.ErrorCount++
					}
				} else {
					log.Infof("TraceLensCleanupJob: deleted expired pod %s", session.PodName)
					podsDeleted++
				}
			}

			// Mark session as expired in the DATA cluster's database
			err := facade.UpdateStatus(ctx, session.SessionID, tlconst.StatusExpired, "Session expired and cleaned up by cleanup job")
			if err != nil {
				log.Errorf("TraceLensCleanupJob: failed to update session %s status: %v", session.SessionID, err)
				stats.ErrorCount++
			} else {
				log.Infof("TraceLensCleanupJob: marked session %s as expired", session.SessionID)
				sessionsUpdated++
			}
		}
	}

	stats.RecordsProcessed = int64(totalExpired)
	stats.ItemsDeleted = int64(podsDeleted)
	stats.ItemsUpdated = int64(sessionsUpdated)
	stats.ProcessDuration = time.Since(startTime).Seconds()
	stats.AddMessage(fmt.Sprintf("Cleaned up %d expired sessions across %d clusters, deleted %d pods",
		sessionsUpdated, len(clusterNames), podsDeleted))

	log.Infof("TraceLensCleanupJob: completed cleanup - sessions updated: %d, pods deleted: %d, errors: %d",
		sessionsUpdated, podsDeleted, stats.ErrorCount)

	return stats, nil
}

// Schedule returns the cron schedule for this job
// Runs every 5 minutes to clean up expired sessions
func (j *TraceLensCleanupJob) Schedule() string {
	return "@every 5m"
}

