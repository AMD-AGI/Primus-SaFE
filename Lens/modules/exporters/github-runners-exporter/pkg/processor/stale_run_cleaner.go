// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package processor

import (
	"context"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

const (
	// DefaultStaleCheckInterval is the interval between stale run checks
	DefaultStaleCheckInterval = 30 * time.Second

	// DefaultStaleThreshold is how long a run must be unchanged before
	// it is considered potentially stale and checked against K8s
	DefaultStaleThreshold = 10 * time.Minute

	// staleCheckBatchSize is the max number of records to check per cycle
	staleCheckBatchSize = 50
)

// StaleRunCleaner periodically checks for workflow runs that are marked as
// workload_running or workload_pending in the database but whose corresponding
// EphemeralRunner no longer exists in K8s. This handles edge cases where
// the reconciler misses deletion events (e.g., during exporter restarts).
type StaleRunCleaner struct {
	checkInterval  time.Duration
	staleThreshold time.Duration
	dynamicClient  dynamic.Interface

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// StaleRunCleanerConfig holds configuration for the StaleRunCleaner
type StaleRunCleanerConfig struct {
	CheckInterval  time.Duration
	StaleThreshold time.Duration
}

// NewStaleRunCleaner creates a new StaleRunCleaner
func NewStaleRunCleaner(cfg *StaleRunCleanerConfig) *StaleRunCleaner {
	if cfg == nil {
		cfg = &StaleRunCleanerConfig{}
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = DefaultStaleCheckInterval
	}
	if cfg.StaleThreshold <= 0 {
		cfg.StaleThreshold = DefaultStaleThreshold
	}

	// Get dynamic client
	var dynClient dynamic.Interface
	clusterManager := clientsets.GetClusterManager()
	if clusterManager != nil {
		if current := clusterManager.GetCurrentClusterClients(); current != nil && current.K8SClientSet != nil {
			dynClient = current.K8SClientSet.Dynamic
		}
	}

	return &StaleRunCleaner{
		checkInterval:  cfg.CheckInterval,
		staleThreshold: cfg.StaleThreshold,
		dynamicClient:  dynClient,
	}
}

// Start begins the background cleanup loop
func (c *StaleRunCleaner) Start(ctx context.Context) error {
	if c.dynamicClient == nil {
		log.Warn("StaleRunCleaner: dynamic client not available, stale run cleanup disabled")
		return nil
	}

	ctx, c.cancel = context.WithCancel(ctx)

	c.wg.Add(1)
	go c.cleanupLoop(ctx)

	log.Infof("StaleRunCleaner started (check_interval: %v, stale_threshold: %v)", c.checkInterval, c.staleThreshold)
	return nil
}

// Stop gracefully stops the cleaner
func (c *StaleRunCleaner) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	log.Info("StaleRunCleaner stopped")
	return nil
}

// cleanupLoop periodically checks for stale runs
func (c *StaleRunCleaner) cleanupLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkAndCleanStaleRuns(ctx)
		}
	}
}

// checkAndCleanStaleRuns finds runs that haven't been updated recently
// and verifies if their EphemeralRunner still exists in K8s
func (c *StaleRunCleaner) checkAndCleanStaleRuns(ctx context.Context) {
	runFacade := database.GetFacade().GetGithubWorkflowRun()

	cutoff := time.Now().Add(-c.staleThreshold)
	staleRuns, err := runFacade.ListStaleRunning(ctx, cutoff, staleCheckBatchSize)
	if err != nil {
		log.Warnf("StaleRunCleaner: failed to query stale records: %v", err)
		return
	}

	if len(staleRuns) == 0 {
		return
	}

	log.Debugf("StaleRunCleaner: checking %d potentially stale runs", len(staleRuns))

	cleaned := 0
	for _, run := range staleRuns {
		ns := run.RunnerSetNamespace
		name := run.WorkloadName
		if ns == "" || name == "" {
			continue
		}

		// Check if EphemeralRunner still exists in K8s
		_, err := c.dynamicClient.Resource(types.EphemeralRunnerGVR).
			Namespace(ns).
			Get(ctx, name, metav1.GetOptions{})

		if err == nil {
			// EphemeralRunner still exists - not stale
			continue
		}

		if !apierrors.IsNotFound(err) {
			// Genuine API error - skip this record
			log.Debugf("StaleRunCleaner: error checking runner %s/%s: %v", ns, name, err)
			continue
		}

		// EphemeralRunner is gone from K8s - mark as completed
		if updateErr := runFacade.UpdateFields(ctx, run.ID, map[string]interface{}{
			"status": database.WorkflowRunStatusCompleted,
		}); updateErr != nil {
			log.Warnf("StaleRunCleaner: failed to mark run %d (%s) as completed: %v", run.ID, name, updateErr)
		} else {
			cleaned++
			log.Infof("StaleRunCleaner: marked stale run %d (%s/%s) as completed - EphemeralRunner no longer exists",
				run.ID, ns, name)
		}
	}

	if cleaned > 0 {
		log.Infof("StaleRunCleaner: cleaned %d stale runs in this cycle", cleaned)
	}
}
