/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

// submitHandler returns a handler with Post Training on whose user u1 is a member of workspace ws1, which has
// two MI355X nodes, the Ready Qwen3-4B model m1 and the SFT dataset d1; refuse, when set, decides which workload
// creations the cluster refuses.
func submitHandler(t *testing.T, db *mockclient.MockInterface,
	refuse func(*v1.Workload) error) (*Handler, ctrlclient.Client) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	member := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1", Labels: map[string]string{v1.UserIdLabel: "u1"}},
		Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.DefaultRole},
			Resources: map[string][]string{common.UserWorkspaces: {"ws1"}}}}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: common.PrimusSafeNamespace,
		Name: registry.ConfigMapName}, Data: map[string]string{registry.DataKey: testRegistry}}
	nf := flavor()
	nf.Name = "mi355x"
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{NodeFlavor: nf.Name}}
	ws.Status.AvailableReplica = 2
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	model.Spec.Source.AccessMode = v1.AccessModeLocal
	model.Status.Phase = v1.ModelPhaseReady
	model.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws1", Path: "/wekafs/models/m1",
		Status: v1.LocalPathStatusReady}}
	model.Status.ModelConfig = &v1.ModelConfig{Architectures: []string{"Qwen3ForCausalLM"}, HiddenSize: 2560}
	objs := append(authtest.ChartRoles(t), member, cm, nf, ws, model)
	builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...)
	if refuse != nil {
		builder = builder.WithInterceptorFuncs(interceptor.Funcs{Create: func(ctx context.Context,
			c ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
			if w, ok := obj.(*v1.Workload); ok {
				if err := refuse(w); err != nil {
					return err
				}
			}
			return c.Create(ctx, obj, opts...)
		}})
	}
	cli := builder.Build()
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	db.EXPECT().GetDataset(gomock.Any(), "d1").Return(&dbclient.Dataset{DatasetId: "d1", DatasetType: "sft",
		LocalPaths: `[{"workspace":"ws1","path":"/wekafs/datasets/d1","status":"Ready"}]`}, nil).AnyTimes()
	db.EXPECT().GetPostTrainingConvertCache(gomock.Any(), "ws1", gomock.Any()).Return(nil, gorm.ErrRecordNotFound).AnyTimes()
	h := NewHandler(cli, db)
	h.access = &authority.AccessController{Client: cli}
	return h, cli
}

// postAs calls handle as user u1 with body and returns the status and body of the answer.
func postAs(handle gin.HandlerFunc, body interface{}) (int, string) {
	data, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, "u1")
	c.Set(common.UserName, "alice")
	handle(c)
	return w.Code, w.Body.String()
}

// sftRequest is an SFT request on m1 and d1.
func sftRequest() recipe.SubmitRequest {
	return recipe.SubmitRequest{Workspace: "ws1", TrainType: "sft", Method: "full", ModelID: "m1", DatasetID: "d1",
		Resources: recipe.Resources{Nodes: 1, GpusPerNode: 8}}
}

// A run whose automatic evaluation names a benchmark that does not exist, or a dataset of another type, is refused
// with the reason: it would train, start an inference service and only then find nothing to evaluate on.
func TestARunWithABenchmarkThatDoesNotExistIsRefused(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, cli := submitHandler(t, db, nil)
	db.EXPECT().GetDataset(gomock.Any(), "gone").
		Return(nil, commonerrors.NewNotFoundWithMessage("dataset gone not found")).AnyTimes()
	req := recipe.SubmitRequest{Workspace: "ws1", DisplayName: "sft-eval", TrainType: "sft", Method: "full",
		ModelID: "m1", DatasetID: "d1", Resources: recipe.Resources{Nodes: 1, GpusPerNode: 8},
		Evaluation: recipe.EvaluationRequest{Enabled: true, Benchmarks: []string{"gone", "d1"}}}
	for name, handle := range map[string]gin.HandlerFunc{"submit": h.SubmitRun, "preview": h.PreviewRecipe} {
		_, body := postAs(handle, req)
		require.Contains(t, body, "Benchmark dataset gone was not found", name)
		require.Contains(t, body, "Dataset d1 is not an evaluation dataset", name)
		require.Contains(t, body, `"field":"evaluation.benchmarks"`, name)
	}
	code, _ := postAs(h.SubmitRun, req)
	require.Equal(t, http.StatusBadRequest, code)
	workloads := &v1.WorkloadList{}
	require.NoError(t, cli.List(context.Background(), workloads))
	require.Empty(t, workloads.Items)
}

// A benchmark that is an evaluation dataset passes the preview with the type the lookup read.
func TestARunWithAnEvaluationBenchmarkPassesThePreview(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, _ := submitHandler(t, db, nil)
	db.EXPECT().GetDataset(gomock.Any(), "gsm8k").Return(&dbclient.Dataset{DatasetId: "gsm8k",
		DatasetType: recipe.BenchmarkDatasetType}, nil).AnyTimes()
	db.EXPECT().ListPostTrainingRecipeRecords(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	code, body := postAs(h.PreviewRecipe, recipe.SubmitRequest{Workspace: "ws1", DisplayName: "sft-eval",
		TrainType: "sft", Method: "full", ModelID: "m1", DatasetID: "d1",
		Resources:  recipe.Resources{Nodes: 1, GpusPerNode: 8},
		Evaluation: recipe.EvaluationRequest{Enabled: true, Benchmarks: []string{"gsm8k"}}})
	require.Equal(t, http.StatusOK, code, body)
	require.NotContains(t, body, "evaluation.benchmarks")
	require.Contains(t, body, `"benchmarks":["gsm8k"]`)
}

// A benchmark lookup that failed is not a benchmark that is missing: the request fails as an error of the server
// and says nothing against the benchmarks chosen.
func TestAFailedBenchmarkLookupIsNotAMissingBenchmark(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, _ := submitHandler(t, db, nil)
	db.EXPECT().GetDataset(gomock.Any(), "flaky").Return(nil, errFromString("connection reset")).AnyTimes()
	req := recipe.SubmitRequest{Workspace: "ws1", DisplayName: "sft-eval", TrainType: "sft", Method: "full",
		ModelID: "m1", DatasetID: "d1", Resources: recipe.Resources{Nodes: 1, GpusPerNode: 8},
		Evaluation: recipe.EvaluationRequest{Enabled: true, Benchmarks: []string{"flaky"}}}
	for name, handle := range map[string]gin.HandlerFunc{"submit": h.SubmitRun, "preview": h.PreviewRecipe} {
		code, body := postAs(handle, req)
		require.Equal(t, http.StatusInternalServerError, code, name)
		require.Contains(t, body, "failed to read dataset flaky", name)
		require.NotContains(t, body, "was not found", name)
	}
	req.Evaluation.Enabled = false
	db.EXPECT().ListPostTrainingRecipeRecords(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	code, body := postAs(h.PreviewRecipe, req)
	require.Equal(t, http.StatusOK, code, "the benchmarks of an evaluation that is off are not looked up: %s", body)

	// The dataset to train on is looked up the same way.
	db.EXPECT().GetDataset(gomock.Any(), "gone").
		Return(nil, commonerrors.NewNotFoundWithMessage("dataset gone not found")).AnyTimes()
	for id, want := range map[string]int{"flaky": http.StatusInternalServerError, "gone": http.StatusNotFound} {
		req.DatasetID = id
		code, body = postAs(h.PreviewRecipe, req)
		require.Equal(t, want, code, "%s: %s", id, body)
	}
}

// A run's model is deleted with the run only for a user who may delete the model.
func TestDeletingARunDeletesItsModelOnlyForWhoMayDeleteTheModel(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, cli := submitHandler(t, db, nil)
	ctx := context.Background()
	for _, owner := range []string{"u2", "u1"} {
		model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m-out-" + owner,
			Labels: map[string]string{v1.UserIdLabel: owner}}, Spec: v1.ModelSpec{Workspace: "ws1"}}
		require.NoError(t, cli.Create(ctx, model))
		run := &dbclient.PostTrainingRun{RunID: "pt-" + owner, WorkloadID: "pt-" + owner, Workspace: "ws1",
			OutputModelID: model.Name, Status: string(v1.WorkloadSucceeded)}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		c.Set(common.UserId, "u1")
		if owner == "u2" {
			err := h.deleteRun(c, run, true)
			require.Error(t, err, "a member may not delete another member's model through a run")
			require.Equal(t, http.StatusForbidden, statusCode(err))
			require.NoError(t, cli.Get(ctx, ctrlclient.ObjectKeyFromObject(model), &v1.Model{}))
			continue
		}
		db.EXPECT().SetPostTrainingRunDeleted(gomock.Any(), run.RunID).Return(nil)
		db.EXPECT().DeletePostTrainingRunMetrics(gomock.Any(), run.RunID).Return(nil)
		db.EXPECT().DeletePostTrainingRunSamples(gomock.Any(), run.RunID, gomock.Any()).Return(nil)
		require.NoError(t, h.deleteRun(c, run, true))
		require.Error(t, cli.Get(ctx, ctrlclient.ObjectKeyFromObject(model), &v1.Model{}), "the owner deletes it")
	}
}

// A run name longer than its column is refused.
func TestANameLongerThanItsColumnIsRefused(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, _ := submitHandler(t, db, nil)
	req := sftRequest()
	req.DisplayName = strings.Repeat("r", 129)
	code, body := postAs(h.SubmitRun, req)
	require.Equal(t, http.StatusBadRequest, code, "the run's name column holds 128 characters: %s", body)
}

// A run whose workload the cluster refuses is marked failed although its caller left meanwhile.
func TestARefusedRunIsMarkedFailedAlthoughItsRequestEnded(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	c := testContext()
	ctx, leave := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	h, _ := submitHandler(t, db, func(*v1.Workload) error {
		leave()
		return apierrors.NewForbidden(v1.SchemeGroupVersion.WithResource("workloads").GroupResource(), "pt-1",
			errFromString("quota exceeded"))
	})
	db.EXPECT().CreatePostTrainingRun(gomock.Any(), gomock.Any()).Return(nil)
	db.EXPECT().UpdatePostTrainingRun(gomock.Any(), "pt-1", gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ string, fields map[string]interface{}) error {
			require.NoError(t, ctx.Err(), "the mark outlives the request")
			require.Equal(t, string(v1.WorkloadFailed), fields["status"])
			return nil
		})
	_, err := h.createRun(c, &preparedRun{row: &dbclient.PostTrainingRun{RunID: "pt-1"},
		workload: &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-1"}}})
	require.ErrorContains(t, err, "quota exceeded")
}

// A run stays pending when the cluster's answer to its workload is lost: the workload may run, and a failed run is
// not followed.
func TestARunWhoseWorkloadMayExistIsNotMarkedFailed(t *testing.T) {
	workloads := v1.SchemeGroupVersion.WithResource("workloads").GroupResource()
	lost := map[string]error{
		"the caller left":       context.Canceled,
		"the request timed out": apierrors.NewTimeoutError("etcd is slow", 1),
		"the connection broke":  errFromString("connection reset by peer"),
		"a workload of the name exists, which an earlier try of the request stored": apierrors.NewAlreadyExists(
			workloads, "pt-1"),
		"the server failed for a reason it does not name": &apierrors.StatusError{ErrStatus: metav1.Status{
			Status: metav1.StatusFailure, Code: http.StatusInternalServerError, Reason: metav1.StatusReasonUnknown,
			Message: "etcdserver: request timed out"}},
		"the server is unavailable": apierrors.NewServiceUnavailable("the server is shutting down"),
		"the answer is not the API server's": apierrors.NewGenericServerResponse(http.StatusInternalServerError,
			"POST", workloads, "pt-1", "<html>bad gateway</html>", 0, true),
	}
	for name, answer := range lost {
		db := mockclient.NewMockInterface(gomock.NewController(t))
		var cli ctrlclient.Client
		var h *Handler
		stored := false
		h, cli = submitHandler(t, db, func(w *v1.Workload) error {
			if stored {
				return nil
			}
			// The cluster stores the workload, and its answer does not arrive.
			stored = true
			require.NoError(t, cli.Create(context.Background(), w.DeepCopy()))
			return answer
		})
		// The run is not updated: the controller follows the workload, or fails the run when none shows.
		db.EXPECT().CreatePostTrainingRun(gomock.Any(), gomock.Any()).Return(nil)
		_, err := h.createRun(testContext(), &preparedRun{row: &dbclient.PostTrainingRun{RunID: "pt-1"},
			workload: &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-1"}}})
		require.ErrorIs(t, err, answer, name)
		require.NoError(t, cli.Get(context.Background(), ctrlclient.ObjectKey{Name: "pt-1"}, &v1.Workload{}), name)
	}
}

// A run is marked failed when the cluster refused its workload, as when an admission webhook cannot be called:
// the workload is then not stored.
func TestARunTheClusterRefusedIsMarkedFailed(t *testing.T) {
	refusals := map[string]error{
		"the request is invalid":     apierrors.NewBadRequest("the priority is out of range"),
		"a webhook cannot be called": apierrors.NewInternalError(errFromString("failed calling webhook")),
		// The API server passes a webhook's denial on with the webhook's status, which may name no reason.
		"a webhook denies without a reason": &apierrors.StatusError{ErrStatus: metav1.Status{
			Status: metav1.StatusFailure, Code: http.StatusForbidden,
			Message: `admission webhook "quota.example.com" denied the request: over quota`}},
	}
	for name, answer := range refusals {
		db := mockclient.NewMockInterface(gomock.NewController(t))
		h, _ := submitHandler(t, db, func(*v1.Workload) error { return answer })
		db.EXPECT().CreatePostTrainingRun(gomock.Any(), gomock.Any()).Return(nil)
		db.EXPECT().UpdatePostTrainingRun(gomock.Any(), "pt-1", gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, fields map[string]interface{}) error {
				require.Equal(t, string(v1.WorkloadFailed), fields["status"], name)
				return nil
			})
		_, err := h.createRun(testContext(), &preparedRun{row: &dbclient.PostTrainingRun{RunID: "pt-1"},
			workload: &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-1"}}})
		require.ErrorIs(t, err, answer, name)
	}
}

// errFromString is an error with message.
type errFromString string

// Error returns the message.
func (e errFromString) Error() string { return string(e) }
