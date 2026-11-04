/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
)

func TestCheckpointCleanup(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	run := testRunRow("Succeeded", "done")
	run.OutputPath = testRunDir
	run.CleanupRequest = `{"keep":"best","paths":["` + testRunDir + `/checkpoints/iter_0000010"],"keptStep":20}`
	r, _, updates := newTestReconciler(t, run, ws)
	ctx := context.Background()

	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	cleanup := &v1.OpsJob{}
	name := resource.DirectoryCleanupJobName("ws1", testRunDir+"/checkpoints/iter_0000010")
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: name}, cleanup))
	require.Equal(t, testRun, cleanup.Labels[v1.PostTrainingRunIdLabel])

	cleanup.Status.Phase = v1.OpsJobSucceeded
	require.NoError(t, r.Update(ctx, cleanup))
	reconcileRun(t, r)
	disk := &v1.OpsJob{}
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: diskJobName(testRun)}, disk), "the run directory is counted again")
	require.Equal(t, testRunDir, disk.GetParameter(v1.ParameterDestPath).Value)

	disk.Status.Phase = v1.OpsJobSucceeded
	disk.Status.Outputs = []v1.Parameter{{Name: "result", Value: `["[SUCCESS] VERIFY={\"output\":\"x\",\"disk\":{\"checkpoints\":7,\"total\":7}}"]`}}
	require.NoError(t, r.Update(ctx, disk))
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "", last["cleanup_request"])
	require.Equal(t, `{"checkpoints":7,"total":7}`, last["disk_usage"])
	require.Contains(t, last["stage_history"], `"event":"checkpoints-cleaned"`)
	require.Contains(t, last["stage_history"], `"keptStep":20`)
	jobs := &v1.OpsJobList{}
	require.NoError(t, r.List(ctx, jobs))
	require.Empty(t, jobs.Items, "finished cleanup and count jobs are removed")
}
