/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

// exportRun is a finished SFT run that saved every 10 steps up to step 30 and keeps the last checkpoint and the best.
func exportRun(status string) *dbclient.PostTrainingRun {
	return &dbclient.PostTrainingRun{RunID: "pt-1", DisplayName: "qwen-sft", Status: status, TrainType: "sft",
		Method: "full", Workspace: "ws", Priority: 1,
		Recipe: `{"output":{"base":"/w/pt-1"},"choices":{"saveInterval":10},"checkpoint":{"keepLast":1,"keepBest":true},` +
			`"layout":{"nodes":2,"gpusPerNode":8},"provenance":{"image":"img:1"}}`}
}

// exportHandler returns a handler with the run's metrics, a workspace with a flavor and the given objects.
func exportHandler(t *testing.T, objs ...runtime.Object) *Handler {
	h := cleanupHandler(t)
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	nf := flavor()
	nf.Name = "mi355x"
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}, Spec: v1.WorkspaceSpec{NodeFlavor: "mi355x"}}
	h.client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(nf, ws).WithRuntimeObjects(objs...).Build()
	return h
}

func TestExportWorkload(t *testing.T) {
	reg, err := registry.Parse([]byte(testRegistry))
	require.NoError(t, err)
	h := exportHandler(t)
	c := testContext()

	w, err := h.exportWorkload(c, reg, exportRun("Stopped"), &exportRequest{Step: 20})
	require.NoError(t, err)
	require.Equal(t, "20", w.Labels[v1.PostTrainingExportStepLabel])
	require.Equal(t, v1.PostTrainingRoleExport, w.Labels[v1.PostTrainingRoleLabel])
	require.Equal(t, []string{"img:1"}, w.Spec.Images, "the run's image reads its checkpoint format")
	require.Len(t, w.Spec.Resources, 1)
	require.Equal(t, 1, w.Spec.Resources[0].Replica)
	require.Equal(t, "1", w.Spec.Env["RAY_EXPERIMENTAL_NOSET_HIP_VISIBLE_DEVICES"], "the image's environment")
	data, err := base64.StdEncoding.DecodeString(w.Spec.Env[recipeEnv])
	require.NoError(t, err)
	var snapshot struct {
		Output     struct{ Base string } `json:"output"`
		ExportOnly exportOnly            `json:"exportOnly"`
	}
	require.NoError(t, json.Unmarshal(data, &snapshot))
	require.Equal(t, "/w/pt-1", snapshot.Output.Base, "the rest of the snapshot stays")
	require.Equal(t, exportOnly{Step: 20, Dir: "/w/pt-1/exports/step-20", DisplayName: "qwen-sft-step20"},
		snapshot.ExportOnly)

	for req, code := range map[exportRequest]int{
		{Step: 25}:                    http.StatusBadRequest,
		{Step: 10}:                    http.StatusBadRequest,
		{Step: 20, PeftAdapter: true}: http.StatusBadRequest,
	} {
		_, err = h.exportWorkload(c, reg, exportRun("Succeeded"), &req)
		require.Equal(t, code, statusCode(err), "%+v", req)
	}
	_, err = h.exportWorkload(c, reg, exportRun("Running"), &exportRequest{Step: 20})
	require.Equal(t, http.StatusConflict, statusCode(err))
}

// exportJob is an export job of step 20 of pt-1 in phase, created at minute.
func exportJob(name string, phase v1.WorkloadPhase, minute int) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{
		v1.PostTrainingRunIdLabel: "pt-1", v1.PostTrainingRoleLabel: v1.PostTrainingRoleExport,
		v1.PostTrainingExportStepLabel: "20"},
		CreationTimestamp: metav1.NewTime(time.Date(2026, 11, 4, 10, minute, 0, 0, time.UTC))}}
	w.Status.Phase = phase
	if phase == v1.WorkloadFailed || phase == v1.WorkloadSucceeded {
		w.Status.EndTime = &metav1.Time{Time: time.Now()}
	}
	w.Status.Message = string(phase)
	return w
}

func TestExportConflictsAndList(t *testing.T) {
	failed := exportJob("pt-1-x20-a", v1.WorkloadFailed, 1)
	h := exportHandler(t, failed)
	require.NoError(t, h.checkExportConflict(testContext(), "pt-1", 20), "a failed export can be retried")

	running := exportJob("pt-1-x20-b", v1.WorkloadRunning, 2)
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m-20", Labels: map[string]string{
		v1.PostTrainingRunIdLabel: "pt-1", v1.PostTrainingExportStepLabel: "20"}},
		Spec: v1.ModelSpec{DisplayName: "qwen-sft-step20"}}
	output := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m-out", Labels: map[string]string{
		v1.PostTrainingRunIdLabel: "pt-1"}}}
	h = exportHandler(t, failed, running, model, output)
	require.Equal(t, http.StatusConflict, statusCode(h.checkExportConflict(testContext(), "pt-1", 20)))

	items, err := h.listExports(context.Background(), "pt-1")
	require.NoError(t, err)
	require.Len(t, items, 1, "the output model of the last step is not an export")
	require.Equal(t, "pt-1-x20-b", items[0].WorkloadID, "the latest job of a step is shown")
	require.Equal(t, string(v1.WorkloadRunning), items[0].Status)
	require.Equal(t, "m-20", items[0].ModelID)
	require.Equal(t, "qwen-sft-step20", items[0].DisplayName)
}
