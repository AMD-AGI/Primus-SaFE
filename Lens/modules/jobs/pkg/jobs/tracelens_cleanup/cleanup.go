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

// TraceLensCleanupJob cleans up expired TraceLens sessions and their associated pods
type TraceLensCleanupJob struct{}

// NewTraceLensCleanupJob creates a new TraceLensCleanupJob instance
func NewTraceLensCleanupJob() *TraceLensCleanupJob {
	return &TraceLensCleanupJob{}
}

// Run executes the TraceLens session cleanup job
func (j *TraceLensCleanupJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	startTime := time.Now()
	stats := common.NewExecutionStats()

	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	log.Infof("TraceLensCleanupJob: starting cleanup for cluster: %s", clusterName)

	facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()

	// Get expired sessions
	expiredSessions, err := facade.ListExpired(ctx)
	if err != nil {
		log.Errorf("TraceLensCleanupJob: failed to list expired sessions: %v", err)
		stats.ErrorCount++
		stats.AddMessage(fmt.Sprintf("Failed to list expired sessions: %v", err))
		return stats, fmt.Errorf("failed to list expired sessions: %w", err)
	}

	if len(expiredSessions) == 0 {
		log.Debug("TraceLensCleanupJob: no expired sessions found")
		stats.AddMessage("No expired sessions found")
		stats.ProcessDuration = time.Since(startTime).Seconds()
		return stats, nil
	}

	log.Infof("TraceLensCleanupJob: found %d expired sessions to cleanup", len(expiredSessions))

	podsDeleted := 0
	sessionsUpdated := 0

	for _, session := range expiredSessions {
		// Delete pod if exists
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

		// Mark session as expired
		err := facade.UpdateStatus(ctx, session.SessionID, tlconst.StatusExpired, "Session expired and cleaned up by cleanup job")
		if err != nil {
			log.Errorf("TraceLensCleanupJob: failed to update session %s status: %v", session.SessionID, err)
			stats.ErrorCount++
		} else {
			log.Infof("TraceLensCleanupJob: marked session %s as expired", session.SessionID)
			sessionsUpdated++
		}
	}

	stats.RecordsProcessed = int64(len(expiredSessions))
	stats.ItemsDeleted = int64(podsDeleted)
	stats.ItemsUpdated = int64(sessionsUpdated)
	stats.ProcessDuration = time.Since(startTime).Seconds()
	stats.AddMessage(fmt.Sprintf("Cleaned up %d expired sessions, deleted %d pods", sessionsUpdated, podsDeleted))

	log.Infof("TraceLensCleanupJob: completed cleanup - sessions updated: %d, pods deleted: %d, errors: %d",
		sessionsUpdated, podsDeleted, stats.ErrorCount)

	return stats, nil
}

// Schedule returns the cron schedule for this job
// Runs every 5 minutes to clean up expired sessions
func (j *TraceLensCleanupJob) Schedule() string {
	return "@every 5m"
}

