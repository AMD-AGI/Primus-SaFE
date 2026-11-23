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

// submitHandler returns a handler with Post Training on whose users u1 and u2 are members of workspace ws1, admin is
// a system admin and outsider is in no workspace. ws1 has two MI355X nodes, a volume at /wekafs, the Ready Qwen3-4B
// model m1 and the SFT dataset d1; refuse, when set, decides which workload creations the cluster refuses.
func submitHandler(t *testing.T, db *mockclient.MockInterface,
	refuse func(*v1.Workload) error) (*Handler, ctrlclient.Client) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	user := func(name string, role v1.UserRole, workspaces ...string) *v1.User {
		return &v1.User{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}},
			Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{role},
				Resources: map[string][]string{common.UserWorkspaces: workspaces}}}
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: common.PrimusSafeNamespace,
		Name: registry.ConfigMapName}, Data: map[string]string{registry.DataKey: testRegistry}}
	nf := flavor()
	nf.Name = "mi355x"
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{NodeFlavor: nf.Name,
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	ws.Status.AvailableReplica = 2
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	model.Spec.Source.AccessMode = v1.AccessModeLocal
	model.Status.Phase = v1.ModelPhaseReady
	model.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws1", Path: "/wekafs/models/m1",
		Status: v1.LocalPathStatusReady}}
	model.Status.ModelConfig = &v1.ModelConfig{Architectures: []string{"Qwen3ForCausalLM"}, HiddenSize: 2560}
	objs := append(authtest.ChartRoles(t), user("u1", v1.DefaultRole, "ws1"), user("u2", v1.DefaultRole, "ws1"),
		user("admin", v1.SystemAdminRole), user("outsider", v1.DefaultRole), cm, nf, ws, model)
	builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(&v1.Workload{})
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

// A dataset lookup that failed is not a dataset that is missing: the request fails as an error of the server.
func TestAFailedDatasetLookupIsNotAMissingDataset(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, _ := submitHandler(t, db, nil)
	db.EXPECT().GetDataset(gomock.Any(), "flaky").Return(nil, errFromString("connection reset")).AnyTimes()
	db.EXPECT().GetDataset(gomock.Any(), "gone").
		Return(nil, commonerrors.NewNotFoundWithMessage("dataset gone not found")).AnyTimes()
	req := sftRequest()
	for id, want := range map[string]int{"flaky": http.StatusInternalServerError, "gone": http.StatusNotFound} {
		req.DatasetID = id
		code, body := postAs(h.PreviewRecipe, req)
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

// submittable is an SFT request on m1 and d1 that compiles in ws1.
func submittable() recipe.SubmitRequest {
	req := sftRequest()
	req.DisplayName = "Qwen3 SFT"
	return req
}

// A valid request records its run and creates its training workload in the run's directory on the workspace volume.
func TestSubmitRecordsTheRunAndCreatesItsWorkload(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, cli := submitHandler(t, db, nil)
	var row *dbclient.PostTrainingRun
	db.EXPECT().CreatePostTrainingRun(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, r *dbclient.PostTrainingRun) error {
			row = r
			return nil
		})
	req := submittable()
	req.Ext = map[string]interface{}{"note": "x"}
	db.EXPECT().ListPostTrainingRecipeRecords(gomock.Any(), "ws1", gomock.Len(1)).Return(
		map[string]dbclient.RecipeRecord{}, nil)
	code, body := postAs(h.PreviewRecipe, req)
	require.Equal(t, http.StatusOK, code, body)
	require.Contains(t, body, `"frameworkArgs":["--actor-num-nodes","1"`, "the preview shows the compiled recipe")

	code, body = postAs(h.SubmitRun, req)
	require.Equal(t, http.StatusOK, code, body)
	var resp submitResponse
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.Equal(t, resp.RunID, row.RunID)
	require.Equal(t, "u1", row.UserID)
	require.Equal(t, "m1", row.BaseModelID)
	require.Equal(t, string(v1.WorkloadPending), row.Status)
	require.Equal(t, `{"note":"x"}`, row.Ext)
	require.Equal(t, "/wekafs/custom/models/"+row.RunID, row.OutputPath, "the run writes to the workspace volume")
	w := &v1.Workload{}
	require.NoError(t, cli.Get(context.Background(), ctrlclient.ObjectKey{Name: resp.WorkloadID}, w))
	require.Empty(t, v1.GetAnnotation(w, v1.WorkloadRuntimeExtendedByAnnotation))
}

// Parallel sizes as large as a request may send are refused with diagnostics on preview and submission alike, and
// no run is recorded.
func TestHugeParallelSizesAreDiagnosed(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, _ := submitHandler(t, db, nil)
	db.EXPECT().ListPostTrainingRecipeRecords(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	req := submittable()
	req.Choices.TP, req.Choices.PP = 4294967296, 4294967296
	const want = "TP × PP × CP and EP × PP must each be between 1 and the training GPUs (8)"
	code, body := postAs(h.PreviewRecipe, req)
	require.Equal(t, http.StatusOK, code, body)
	require.Contains(t, body, want)
	code, body = postAs(h.SubmitRun, req)
	require.Equal(t, http.StatusBadRequest, code, body)
	require.Contains(t, body, want)
}

// A run cannot take a model that is being deleted as its base: the model controller removes its weights.
func TestARunCannotTrainAModelBeingDeleted(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, cli := submitHandler(t, db, nil)
	ctx := context.Background()
	m := &v1.Model{}
	require.NoError(t, cli.Get(ctx, ctrlclient.ObjectKey{Name: "m1"}, m))
	m.Finalizers = []string{"amd.com/model"}
	require.NoError(t, cli.Update(ctx, m))
	require.NoError(t, cli.Delete(ctx, m))
	code, body := postAs(h.SubmitRun, submittable())
	require.Equal(t, http.StatusBadRequest, code, body)
	require.Contains(t, body, "model m1 is being deleted")
}

// A workspace without a volume has nowhere to write the run.
func TestAWorkspaceWithoutAVolumeIsRefused(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, cli := submitHandler(t, db, nil)
	ctx := context.Background()
	ws := &v1.Workspace{}
	require.NoError(t, cli.Get(ctx, ctrlclient.ObjectKey{Name: "ws1"}, ws))
	ws.Spec.Volumes = nil
	require.NoError(t, cli.Update(ctx, ws))
	for _, handle := range []gin.HandlerFunc{h.PreviewRecipe, h.SubmitRun} {
		code, body := postAs(handle, submittable())
		require.Equal(t, http.StatusBadRequest, code, body)
		require.Contains(t, body, "workspace ws1 has no volume")
	}
}

// The body and its ext are bounded before anything is recorded.
func TestTheRequestBodyIsBounded(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, _ := submitHandler(t, db, nil)
	req := submittable()
	req.Ext = map[string]interface{}{"note": strings.Repeat("x", maxRequestBytes)}
	for _, handle := range []gin.HandlerFunc{h.PreviewRecipe, h.SubmitRun} {
		code, body := postAs(handle, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, code, body)
	}
	req.Ext = map[string]interface{}{"note": strings.Repeat("x", maxExtBytes)}
	code, body := postAs(h.SubmitRun, req)
	require.Equal(t, http.StatusBadRequest, code, body)
	require.Contains(t, body, "ext must be JSON of at most 16384 bytes")
}

// A timeout above the QoS runtime ceiling needs the long-runtime grant, as for POST /workloads: without it the
// submission is refused before the run is recorded, and with it the workload carries who extended it.
func TestATimeoutAboveTheRuntimeCeilingNeedsTheGrant(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, cli := submitHandler(t, db, nil)
	require.NoError(t, cli.Create(context.Background(), &v1.Qos{ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: v1.QosSpec{MaxRuntime: map[string]int{string(v1.RayScope): 3600}}}))
	req := submittable()
	req.Resources.TimeoutSeconds = 7200
	code, body := postAs(h.SubmitRun, req)
	require.Equal(t, http.StatusForbidden, code, body)
	require.Contains(t, body, "workload/long-runtime")

	var extended []metav1.Condition
	extendedBy := func(user string, timeout int) string {
		var runID string
		db.EXPECT().CreatePostTrainingRun(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, r *dbclient.PostTrainingRun) error {
				runID = r.RunID
				return nil
			})
		req.Resources.TimeoutSeconds = timeout
		code, body := call(h.SubmitRun, http.MethodPost, "/", user, "", req)
		require.Equal(t, http.StatusOK, code, body)
		w := &v1.Workload{}
		require.NoError(t, cli.Get(context.Background(), ctrlclient.ObjectKey{Name: runID}, w))
		require.Equal(t, timeout, *w.Spec.Timeout)
		extended = nil
		for _, cond := range w.Status.Conditions {
			if cond.Type == string(v1.RuntimeExtended) {
				extended = append(extended, cond)
			}
		}
		return v1.GetAnnotation(w, v1.WorkloadRuntimeExtendedByAnnotation)
	}
	require.Equal(t, "admin", extendedBy("admin", 7200), "the webhook keeps an authorized timeout")
	require.Len(t, extended, 1, "the timeline says who set the timeout above the ceiling, as for any workload")
	require.Contains(t, extended[0].Message, "runtime set to 2h by")
	require.Contains(t, extended[0].Message, "above the qos limit 1h")
	require.Empty(t, extendedBy("u1", 1800), "a timeout within the ceiling needs no grant")
	require.Empty(t, extended)
}

// A timeline entry written right after the create, while controllers write the status, is written again after
// each conflict, with a pause between tries, rather than dropped after a few back-to-back ones.
func TestAppendWorkloadConditionOutlastsConflicts(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-x"}}
	conflicts := 0
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(w).WithStatusSubresource(w).
		WithInterceptorFuncs(interceptor.Funcs{SubResourcePatch: func(ctx context.Context, c ctrlclient.Client,
			sub string, obj ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.SubResourcePatchOption) error {
			if conflicts < 3 {
				conflicts++
				return apierrors.NewConflict(v1.SchemeGroupVersion.WithResource("workloads").GroupResource(), obj.GetName(),
					nil)
			}
			return c.SubResource(sub).Patch(ctx, obj, patch, opts...)
		}}).Build()
	h := &Handler{client: cli}
	stored := &v1.Workload{}
	require.NoError(t, cli.Get(context.Background(), ctrlclient.ObjectKeyFromObject(w), stored))
	cond := &metav1.Condition{Type: string(v1.RuntimeExtended), Status: metav1.ConditionTrue, Reason: "Extended",
		LastTransitionTime: metav1.Now()}
	require.NoError(t, h.appendWorkloadCondition(context.Background(), stored, cond))
	require.Equal(t, 3, conflicts)
	require.NoError(t, cli.Get(context.Background(), ctrlclient.ObjectKeyFromObject(w), stored))
	require.Len(t, stored.Status.Conditions, 1)
}
