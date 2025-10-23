/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// CronJobManager manages all cron jobs for workload scheduling
// It maintains a map of workload IDs to their associated cron jobs
type CronJobManager struct {
	sync.RWMutex
	// store all cronjobs, the key is workload id
	allCronJobs map[string][]CronJob
	client.Client
}

// CronJob represents a single cron job that can activate a workload
// It contains the cron scheduler, workload ID, and scheduling configuration
type CronJob struct {
	client.Client
	job        *cron.Cron
	workloadId string
	scheduler  v1.CronSchedule
}

// newCronJobManager creates and initializes a new CronJobManager
// It takes a controller manager and returns a configured CronJobManager instance
func newCronJobManager(mgr manager.Manager) *CronJobManager {
	return &CronJobManager{
		Client:      mgr.GetClient(),
		allCronJobs: make(map[string][]CronJob),
	}
}

// addOrReplace creates and starts cron jobs for a workload based on its specification.
// If cron jobs already exist for the workload, they are removed first to implement update logic.
func (m *CronJobManager) addOrReplace(workload *v1.Workload) {
	m.Lock()
	defer m.Unlock()
	// Remove existing cron jobs for this workload (update logic)
	m.removeInternal(workload.Name)
	if len(workload.Spec.CronSchedules) == 0 {
		return
	}

	cronJobs := make([]CronJob, 0, len(workload.Spec.CronSchedules))
	// Create cron jobs for each schedule in the workload specification
	for i, cs := range workload.Spec.CronSchedules {
		// Parse the cron schedule string into a cron.Schedule
		schedule, err := timeutil.ParseCronString(cs.Schedule)
		if err != nil {
			klog.ErrorS(err, "failed to parse cron schedule",
				"workload", workload.Name, "schedule", cs.Schedule)
			continue
		}
		job := cron.New(cron.WithChain(
			cron.SkipIfStillRunning(cron.DiscardLogger),
		))
		cj := CronJob{
			Client:     m.Client,
			job:        job,
			workloadId: workload.Name,
			scheduler:  workload.Spec.CronSchedules[i],
		}
		// Schedule the execute function to run according to the parsed schedule
		job.Schedule(schedule, cron.FuncJob(cj.execute))
		// Start the cron scheduler
		job.Start()
		cronJobs = append(cronJobs, cj)
		klog.Infof("add cronjob for workload: %s, schedule: %s", workload.Name, cs.Schedule)
	}
	// Store the cron jobs in the manager's map
	m.allCronJobs[workload.Name] = cronJobs
}

// remove stops and removes all cron jobs associated with a workload ID
// It locks the manager, stops all cron jobs, and removes them from the internal map
func (m *CronJobManager) remove(workloadId string) {
	m.Lock()
	defer m.Unlock()
	m.removeInternal(workloadId)
}

// removeInternal stops and removes all cron jobs associated with a workload ID
// This function is not thread-safe and should only be called when the lock is already held
func (m *CronJobManager) removeInternal(workloadId string) {
	cronJobs, ok := m.allCronJobs[workloadId]
	if !ok {
		return
	}
	for i := range cronJobs {
		cronJobs[i].job.Stop()
	}
	delete(m.allCronJobs, workloadId)
}

// execute is the function that gets called when a cron job is triggered
// It activates a suspended workload by setting IsSuspended to false
func (cj *CronJob) execute() {
	const maxRetry = 10
	waitTime := time.Millisecond * 200
	maxWaitTime := waitTime * maxRetry

	err := backoff.Retry(func() error {
		workload := &v1.Workload{}
		err := cj.Get(context.Background(), client.ObjectKey{Name: cj.workloadId}, workload)
		if err != nil {
			return client.IgnoreNotFound(err)
		}
		if !workload.IsSuspended() || v1.IsWorkloadScheduled(workload) {
			return nil
		}
		// Activate the workload by setting IsSuspended to false
		workload.Spec.IsSuspended = false
		if err = cj.Update(context.Background(), workload); err != nil {
			return err
		}
		klog.Infof("activate workload %s by cronjob", workload.Name)
		return nil
	}, maxWaitTime, waitTime)
	if err != nil {
		klog.ErrorS(err, "failed to cron-schedule", "workload", cj.workloadId)
	}
}
