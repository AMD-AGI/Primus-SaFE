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

// sftExperiment is an experiment of an SFT baseline on m1 and d1 with variants.
func sftExperiment(variants ...experimentVariant) experimentRequest {
	return experimentRequest{DisplayName: "lr-ab", Variants: variants, Baseline: recipe.SubmitRequest{
		Workspace: "ws1", TrainType: "sft", Method: "full", ModelID: "m1", DatasetID: "d1",
		Resources: recipe.Resources{Nodes: 1, GpusPerNode: 8}}}
}

// An experiment with a variant that fails its checks creates no run.
func TestAnExperimentWithAVariantThatFailsItsChecksCreatesNothing(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, cli := submitHandler(t, db, nil)
	code, body := postAs(h.SubmitExperiment, sftExperiment(
		experimentVariant{Name: "lr2e5", Changes: map[string]interface{}{"choices": map[string]interface{}{"lr": 2e-5}}},
		experimentVariant{Name: "retry", Changes: map[string]interface{}{"resources": map[string]interface{}{"maxRetry": 9}}}))
	require.Equal(t, http.StatusBadRequest, code, body)
	require.Contains(t, body, "variant retry")
	workloads := &v1.WorkloadList{}
	require.NoError(t, cli.List(context.Background(), workloads))
	require.Empty(t, workloads.Items, "no run starts before every variant passed its checks")
}

// An experiment whose workload the cluster refuses is rolled back with the cluster's answer.
func TestAnExperimentWhoseWorkloadIsRefusedIsRolledBack(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	refused := apierrors.NewForbidden(v1.SchemeGroupVersion.WithResource("workloads").GroupResource(), "x",
		errFromString("quota exceeded"))
	h, cli := submitHandler(t, db, func(w *v1.Workload) error {
		if strings.Contains(w.Labels[v1.DisplayNameLabel], "lr2e5") {
			return refused
		}
		return nil
	})
	db.EXPECT().CreatePostTrainingExperiment(gomock.Any(), gomock.Any()).Return(nil)
	db.EXPECT().CreatePostTrainingRun(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	db.EXPECT().UpdatePostTrainingRun(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	db.EXPECT().SetPostTrainingRunDeleted(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	db.EXPECT().DeletePostTrainingRunMetrics(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	db.EXPECT().DeletePostTrainingRunSamples(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
	db.EXPECT().SetPostTrainingExperimentDeleted(gomock.Any(), gomock.Any()).Return(nil)
	code, body := postAs(h.SubmitExperiment, sftExperiment(
		experimentVariant{Name: "lr2e5", Changes: map[string]interface{}{"choices": map[string]interface{}{"lr": 2e-5}}}))
	require.Equal(t, http.StatusForbidden, code, "the cluster's answer is kept: %s", body)
	require.Contains(t, body, "variant lr2e5")
	workloads := &v1.WorkloadList{}
	require.NoError(t, cli.List(context.Background(), workloads))
	require.Empty(t, workloads.Items, "the baseline that started is stopped with the experiment")
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

// Only the user who issued a scan may commit it.
func TestOnlyTheIssuerCommitsAScan(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, cli := submitHandler(t, db, nil)
	other := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u2", Labels: map[string]string{v1.UserIdLabel: "u2"}},
		Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.DefaultRole},
			Resources: map[string][]string{common.UserWorkspaces: {"ws1"}}}}
	require.NoError(t, cli.Create(context.Background(), other))
	db.EXPECT().GetPostTrainingImportToken(gomock.Any(), "pti-scan").Return(&dbclient.PostTrainingImportToken{
		TokenID: "pti-scan", Purpose: importPurposeScan, Workspace: "ws1", UserID: "u1"}, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "pti-scan"}}
	c.Set(common.UserId, "u2")
	h.CommitImportScan(c)
	require.Equal(t, http.StatusForbidden, w.Code, "another member may not import a scan staged for u1: %s",
		w.Body.String())
}

// A run name longer than its column is refused.
func TestANameLongerThanItsColumnIsRefused(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, _ := submitHandler(t, db, nil)
	req := sftExperiment().Baseline
	req.DisplayName = strings.Repeat("r", 129)
	code, body := postAs(h.SubmitRun, req)
	require.Equal(t, http.StatusBadRequest, code, "the run's name column holds 128 characters: %s", body)
}

// errFromString is an error with message.
type errFromString string

// Error returns the message.
func (e errFromString) Error() string { return string(e) }
