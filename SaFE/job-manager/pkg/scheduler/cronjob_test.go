/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"testing"
	"time"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func futureSchedule() string {
	return time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
}

func TestCronJobManagerAddRemove(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	m := &CronJobManager{Client: cl, allCronJobs: make(map[string][]CronJob)}

	// Workload with no cron jobs -> nothing stored.
	m.addOrReplace(&v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "none"}})
	assert.Equal(t, len(m.allCronJobs), 0)

	// Workload with a valid cron job -> stored.
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.CronJobs = []v1.CronJob{{Schedule: futureSchedule(), Action: v1.CronStart}}
	m.addOrReplace(w)
	assert.Equal(t, len(m.allCronJobs["w"]), 1)

	// addOrReplace again replaces the existing jobs.
	m.addOrReplace(w)
	assert.Equal(t, len(m.allCronJobs["w"]), 1)

	// remove cleans up.
	m.remove("w")
	_, ok := m.allCronJobs["w"]
	assert.Equal(t, ok, false)

	// remove a non-existent id is a no-op.
	m.remove("ghost")
}

func TestCronJobManagerAddInvalidSchedule(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	m := &CronJobManager{Client: cl, allCronJobs: make(map[string][]CronJob)}

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "bad"}}
	w.Spec.CronJobs = []v1.CronJob{{Schedule: "not-a-time", Action: v1.CronStart}}
	m.addOrReplace(w)
	// Invalid schedule is skipped -> nothing stored.
	_, ok := m.allCronJobs["bad"]
	assert.Equal(t, ok, false)
}

func TestCronJobExecuteUnknownAction(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	cj := &CronJob{Client: cl, workloadId: "w", config: v1.CronJob{Action: v1.CronAction("noop")}}
	// Unknown action -> no-op, no panic.
	cj.execute()
}

func TestCronJobExecuteStartNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	cj := &CronJob{Client: cl, workloadId: "missing", config: v1.CronJob{Action: v1.CronStart}}
	// Workload not found -> IgnoreNotFound -> no error path.
	cj.execute()
}

func TestCronJobDoStartActivates(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(w).Build()
	cj := &CronJob{Client: cl, workloadId: "w", config: v1.CronJob{Action: v1.CronStart}}
	err := cj.doStart()
	assert.NilError(t, err)
}
