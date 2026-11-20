/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
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
	h.reader = h.client
	return h
}

// An export workload converts the step's checkpoint with the run's image under a DNS display name.
func TestExportWorkload(t *testing.T) {
	reg, err := registry.Parse([]byte(testRegistry))
	require.NoError(t, err)
	h := exportHandler(t)
	c := testContext()

	w, err := h.exportWorkload(c, reg, exportRun("Stopped"), &exportRequest{Step: 20})
	require.NoError(t, err)
	requireDisplayName(t, w, commonutils.MaxRayJobNameLen)
	require.Equal(t, "export-20-qwen-sft", w.Labels[v1.DisplayNameLabel])
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
	require.Equal(t, exportOnly{Step: 20, Iteration: 19, Dir: "/w/pt-1/exports/step-20", DisplayName: "qwen-sft-step20"},
		snapshot.ExportOnly, "the launcher reads slime's iter_0000019 for step 20")

	for req, code := range map[exportRequest]int{
		{Step: 25}:                    http.StatusBadRequest,
		{Step: 10}:                    http.StatusBadRequest,
		{Step: 20, PeftAdapter: true}: http.StatusBadRequest,
	} {
		_, err = h.exportWorkload(c, reg, exportRun("Succeeded"), &req)
		require.Equal(t, code, statusCode(err), "%+v", req)
	}
	cleaning := exportRun("Succeeded")
	cleaning.CleanupRequest = `{"keep":"none","paths":["/w/pt-1/checkpoints"]}`
	_, err = h.exportWorkload(c, reg, cleaning, &exportRequest{Step: 20})
	require.Equal(t, http.StatusConflict, statusCode(err), "the checkpoint may be gone by the time the export reads it")
	_, err = h.exportWorkload(c, reg, exportRun("Running"), &exportRequest{Step: 20})
	require.Equal(t, http.StatusConflict, statusCode(err))

	experts := exportRun("Succeeded")
	experts.Method = registry.MethodLora
	experts.Recipe = strings.Replace(experts.Recipe, `"output"`, `"lora":{"rank":16,"experts":true},"output"`, 1)
	_, err = h.exportWorkload(c, reg, experts, &exportRequest{Step: 20, PeftAdapter: true})
	require.Equal(t, http.StatusBadRequest, statusCode(err))
	require.Contains(t, err.Error(), "A PEFT adapter cannot hold the experts' LoRA")
	_, err = h.exportWorkload(c, reg, experts, &exportRequest{Step: 20})
	require.NoError(t, err, "the merged model is exported as usual")
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

// A running export of a step refuses another, a failed one can be retried, and the latest is listed.
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

// An export job made while a cleanup of its run was asked for is removed again.
func TestAnExportGivesWayToACleanupAskedForMeanwhile(t *testing.T) {
	h := exportHandler(t, exportJob("pt-1-x20-b", "Running", 2))
	db := h.db.(*mockclient.MockInterface)
	ctx := context.Background()
	job := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-1-x20-b"}}
	db.EXPECT().GetPostTrainingRun(gomock.Any(), "pt-1").Return(&dbclient.PostTrainingRun{RunID: "pt-1"}, nil)
	require.NoError(t, h.dropExportIfCleaning(ctx, "pt-1", job))
	require.NoError(t, h.client.Get(ctx, ctrlclient.ObjectKeyFromObject(job), &v1.Workload{}), "no cleanup: the job stays")

	db.EXPECT().GetPostTrainingRun(gomock.Any(), "pt-1").Return(
		&dbclient.PostTrainingRun{RunID: "pt-1", CleanupRequest: `{"keep":"none"}`}, nil)
	require.Equal(t, http.StatusConflict, statusCode(h.dropExportIfCleaning(ctx, "pt-1", job)))
	require.Error(t, h.client.Get(ctx, ctrlclient.ObjectKeyFromObject(job), &v1.Workload{}),
		"the cleanup would delete the checkpoint the job reads")

	// A run that cannot be read may have a cleanup queued, and a request that ended still has its job checked.
	h = exportHandler(t, exportJob("pt-1-x20-b", "Running", 2))
	db = h.db.(*mockclient.MockInterface)
	db.EXPECT().GetPostTrainingRun(gomock.Any(), "pt-1").DoAndReturn(
		func(ctx context.Context, _ string) (*dbclient.PostTrainingRun, error) {
			require.NoError(t, ctx.Err(), "the check outlives the request")
			return nil, errors.New("connection reset")
		})
	ended, cancel := context.WithCancel(ctx)
	cancel()
	require.Equal(t, http.StatusInternalServerError, statusCode(h.dropExportIfCleaning(ended, "pt-1", job)))
	require.Error(t, h.client.Get(ctx, ctrlclient.ObjectKeyFromObject(job), &v1.Workload{}),
		"a job that cannot be told safe is removed")

	// A job that cannot be removed is answered as an error, and its removal has time of its own.
	h = exportHandler(t, exportJob("pt-1-x20-b", "Running", 2))
	var lookBy, undoBy time.Time
	h.client = intercepted(h.client, interceptor.Funcs{Delete: func(ctx context.Context, _ ctrlclient.WithWatch,
		_ ctrlclient.Object, _ ...ctrlclient.DeleteOption) error {
		undoBy, _ = ctx.Deadline()
		return errors.New("the API server is unavailable")
	}})
	h.db.(*mockclient.MockInterface).EXPECT().GetPostTrainingRun(gomock.Any(), "pt-1").DoAndReturn(
		func(ctx context.Context, _ string) (*dbclient.PostTrainingRun, error) {
			lookBy, _ = ctx.Deadline()
			return &dbclient.PostTrainingRun{RunID: "pt-1", CleanupRequest: `{"keep":"none"}`}, nil
		})
	require.Equal(t, http.StatusInternalServerError, statusCode(h.dropExportIfCleaning(ctx, "pt-1", job)),
		"the job may still read the checkpoint")
	require.False(t, lookBy.IsZero())
	require.True(t, undoBy.After(lookBy.Add(time.Second)), "a look that used up its time leaves the removal its own")
}

// intercepted returns the fake client cli with funcs in front of it.
func intercepted(cli ctrlclient.Client, funcs interceptor.Funcs) ctrlclient.Client {
	return interceptor.NewClient(cli.(ctrlclient.WithWatch), funcs)
}

// runCall runs handle as user u1 on run pt-1 with body, in a request of ctx, and returns the status and body.
func runCall(ctx context.Context, handle gin.HandlerFunc, body string) (int, string) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "pt-1"}}
	c.Set(common.UserId, "u1")
	c.Set(common.UserName, "alice")
	handle(c)
	return w.Code, w.Body.String()
}

// exportEndpoint returns a handler that user u1 may export the returned run with, and its database: the run's
// first read finds no cleanup, the read after the job was made finds one.
func exportEndpoint(t *testing.T) (*Handler, *mockclient.MockInterface) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h := memberHandler(t, db)
	ctx := context.Background()
	nf := flavor()
	nf.Name = "mi355x"
	require.NoError(t, h.client.Create(ctx, nf))
	ws := &v1.Workspace{}
	require.NoError(t, h.client.Get(ctx, ctrlclient.ObjectKey{Name: "ws1"}, ws))
	ws.Spec.NodeFlavor = "mi355x"
	require.NoError(t, h.client.Update(ctx, ws))
	rows, err := cleanupHandler(t).db.ListPostTrainingRunMetrics(ctx, "pt-1", nil)
	require.NoError(t, err)
	db.EXPECT().ListPostTrainingRunMetrics(gomock.Any(), "pt-1", gomock.Any()).Return(rows, nil).AnyTimes()
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	run := exportRun("Stopped")
	run.Workspace, run.UserID, run.WorkloadID = "ws1", "u1", "pt-1"
	read := db.EXPECT().GetPostTrainingRun(gomock.Any(), "pt-1").Return(run, nil)
	queued := *run
	queued.CleanupRequest = `{"keep":"none"}`
	db.EXPECT().GetPostTrainingRun(gomock.Any(), "pt-1").Return(&queued, nil).After(read)
	return h, db
}

// The export endpoint removes the job it made when a cleanup was asked for while it was made.
func TestTheExportEndpointRemovesItsJobForACleanupAskedMeanwhile(t *testing.T) {
	h, _ := exportEndpoint(t)
	code, body := runCall(context.Background(), h.ExportCheckpoint, `{"step":20}`)
	require.Equal(t, http.StatusConflict, code, body)
	list := &v1.WorkloadList{}
	require.NoError(t, h.client.List(context.Background(), list))
	require.Empty(t, list.Items, "the cleanup would delete the checkpoint the job reads")
}

// The export endpoint looks for a cleanup although its caller left while the job was made: the cluster stored the
// job, and a call that ends with its caller would answer the cancellation and skip the look.
func TestTheExportEndpointChecksItsJobAlthoughItsCallerLeft(t *testing.T) {
	h, _ := exportEndpoint(t)
	ctx, leave := context.WithCancel(context.Background())
	h.client = intercepted(h.client, interceptor.Funcs{Create: func(ctx context.Context, c ctrlclient.WithWatch,
		obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
		leave()
		require.NoError(t, c.Create(context.Background(), obj, opts...))
		return ctx.Err()
	}})
	code, body := runCall(ctx, h.ExportCheckpoint, `{"step":20}`)
	require.Equal(t, http.StatusConflict, code, body)
	list := &v1.WorkloadList{}
	require.NoError(t, h.client.List(context.Background(), list))
	require.Empty(t, list.Items, "the cleanup would delete the checkpoint the job reads")
}
