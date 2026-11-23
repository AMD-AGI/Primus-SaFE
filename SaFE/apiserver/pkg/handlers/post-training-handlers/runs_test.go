/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// call runs handle as user on a request to target with the run id param, and body as JSON when it is not nil; it
// returns the status and body of the answer.
func call(handle gin.HandlerFunc, method, target, user, runID string, body interface{}) (int, string) {
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, reader)
	c.Request.Header.Set("Content-Type", "application/json")
	if runID != "" {
		c.Params = gin.Params{{Key: "id", Value: runID}}
	}
	c.Set(common.UserId, user)
	c.Set(common.UserName, user)
	handle(c)
	return w.Code, w.Body.String()
}

// runOf returns run pt-1 of u1 in ws1, whose training workload is pt-1.
func runOf() *dbclient.PostTrainingRun {
	return &dbclient.PostTrainingRun{RunID: "pt-1", WorkloadID: "pt-1", Workspace: "ws1", UserID: "u1",
		TrainType: "sft", Status: string(v1.WorkloadRunning), Recipe: `{"runId":"pt-1"}`}
}

// withRun makes the database hold run pt-1 of u1 and the cluster its running training workload.
func withRun(t *testing.T, db *mockclient.MockInterface, cli ctrlclient.Client) {
	t.Helper()
	db.EXPECT().GetPostTrainingRun(gomock.Any(), "pt-1").DoAndReturn(
		func(context.Context, string) (*dbclient.PostTrainingRun, error) { return runOf(), nil }).AnyTimes()
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-1", Labels: map[string]string{
		v1.PostTrainingRunIdLabel: "pt-1", v1.UserIdLabel: "u1"}}, Spec: v1.WorkloadSpec{Workspace: "ws1"}}
	require.NoError(t, cli.Create(context.Background(), w))
	w.Status.Phase = v1.WorkloadRunning
	require.NoError(t, cli.Status().Update(context.Background(), w))
}

// workloadExists tells whether the cluster still holds workload name.
func workloadExists(cli ctrlclient.Client, name string) bool {
	return cli.Get(context.Background(), ctrlclient.ObjectKey{Name: name}, &v1.Workload{}) == nil
}

// Another member may neither read, stop nor delete a run; its owner and an admin may.
func TestOnlyTheOwnerAndAnAdminActOnARun(t *testing.T) {
	for _, user := range []string{"u2", "outsider"} {
		db := mockclient.NewMockInterface(gomock.NewController(t))
		h, cli := submitHandler(t, db, nil)
		withRun(t, db, cli)
		code, body := call(h.GetRun, http.MethodGet, "/", user, "pt-1", nil)
		require.Equal(t, http.StatusForbidden, code, "%s reads the run: %s", user, body)
		code, body = call(h.StopRun, http.MethodPost, "/", user, "pt-1", nil)
		require.Equal(t, http.StatusForbidden, code, "%s stops the run: %s", user, body)
		code, body = call(h.DeleteRun, http.MethodDelete, "/", user, "pt-1", nil)
		require.Equal(t, http.StatusForbidden, code, "%s deletes the run: %s", user, body)
		code, body = call(h.ExportMetrics, http.MethodGet, "/", user, "pt-1", nil)
		require.Equal(t, http.StatusForbidden, code, "%s exports the metrics: %s", user, body)
		require.True(t, workloadExists(cli, "pt-1"), "a refused stop leaves the workload")
	}

	for _, user := range []string{"u1", "admin"} {
		db := mockclient.NewMockInterface(gomock.NewController(t))
		h, cli := submitHandler(t, db, nil)
		withRun(t, db, cli)
		code, body := call(h.GetRun, http.MethodGet, "/", user, "pt-1", nil)
		require.Equal(t, http.StatusOK, code, "%s reads the run: %s", user, body)
		require.Contains(t, body, `"recipe":{"runId":"pt-1"}`, "the recipe is returned as JSON")
		require.Contains(t, body, `"imageListed":false`)

		code, body = call(h.StopRun, http.MethodPost, "/", user, "pt-1", nil)
		require.Equal(t, http.StatusOK, code, "%s stops the run: %s", user, body)
		require.False(t, workloadExists(cli, "pt-1"), "the training workload is stopped")

		db.EXPECT().SetPostTrainingRunDeleted(gomock.Any(), "pt-1").Return(nil)
		db.EXPECT().DeletePostTrainingRunMetrics(gomock.Any(), "pt-1").Return(nil)
		code, body = call(h.DeleteRun, http.MethodDelete, "/", user, "pt-1", nil)
		require.Equal(t, http.StatusOK, code, "%s deletes the run: %s", user, body)
	}
}

// A missing run is a 404, not a refusal.
func TestAMissingRunIsNotFound(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, _ := submitHandler(t, db, nil)
	db.EXPECT().GetPostTrainingRun(gomock.Any(), "gone").Return(nil, gorm.ErrRecordNotFound)
	code, body := call(h.GetRun, http.MethodGet, "/", "u1", "gone", nil)
	require.Equal(t, http.StatusNotFound, code, body)
}

// A user outside the workspace cannot list its runs.
func TestListRunsRefusesANonMember(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, _ := submitHandler(t, db, nil)
	code, body := call(h.ListRuns, http.MethodGet, "/?workspace=ws1", "outsider", "", nil)
	require.Equal(t, http.StatusForbidden, code, body)
	code, body = call(h.ListRuns, http.MethodGet, "/", "u1", "", nil)
	require.Equal(t, http.StatusBadRequest, code, "the workspace is required: %s", body)
}

// The compiler learns only of the secrets the user may read.
func TestReadSecretsSkipsWhatTheUserCannotRead(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, cli := submitHandler(t, db, nil)
	for owner, name := range map[string]string{"u1": "wb-mine", "u2": "wb-theirs"} {
		require.NoError(t, cli.Create(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: common.PrimusSafeNamespace, Labels: map[string]string{v1.UserIdLabel: owner,
				v1.SecretTypeLabel: string(v1.SecretGeneral)}},
			Data: map[string][]byte{"WANDB_API_KEY": []byte("k"), "WANDB_BASE_URL": []byte("u")}}))
	}
	read := func(wandb, reward string) map[string]*recipe.SecretInfo {
		req := sftRequest()
		req.Tracking.Wandb = recipe.Wandb{Enabled: true, SecretID: wandb, Project: "p"}
		if reward != "" {
			req.TrainType = "rl"
			req.Choices.RewardType = "remote"
			req.Choices.RewardRemote = &recipe.RewardRemote{URL: "https://rm.example.com", SecretID: reward}
		}
		secrets, err := h.readSecrets(testContextAs("u1"), &req)
		require.NoError(t, err)
		return secrets
	}
	got := read("wb-mine", "wb-theirs")
	require.Equal(t, map[string]*recipe.SecretInfo{"wb-mine": {Type: string(v1.SecretGeneral),
		Keys: []string{"WANDB_API_KEY", "WANDB_BASE_URL"}}}, got, "another member's secret is left out")
	require.Empty(t, read("missing", ""), "a missing secret is left out")
}

// testContextAs returns a gin context of user.
func testContextAs(user string) *gin.Context {
	c := testContext()
	c.Set(common.UserId, user)
	c.Set(common.UserName, user)
	return c
}

// The model list tells which ready models of the workspace a framework supports.
func TestListModels(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, _ := submitHandler(t, db, nil)
	code, body := call(h.ListModels, http.MethodGet, "/?workspace=ws1&trainType=sft&method=full", "u1", "", nil)
	require.Equal(t, http.StatusOK, code, body)
	var resp modelsResponse
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.Equal(t, []modelSupport{{ModelID: "m1", DisplayName: "m1", Spec: "qwen3-4B", Supported: true}},
		resp.Items)
	require.Empty(t, resp.UnavailableReason)

	code, body = call(h.ListModels, http.MethodGet, "/?workspace=ws1", "outsider", "", nil)
	require.Equal(t, http.StatusForbidden, code, body)
}

// The wizard configuration of a model lists its frameworks, verdict and options.
func TestGetConfig(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, _ := submitHandler(t, db, nil)
	code, body := call(h.GetConfig, http.MethodGet, "/?workspace=ws1&modelId=m1&trainType=sft&method=full", "u1",
		"", nil)
	require.Equal(t, http.StatusOK, code, body)
	var resp configResponse
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.Equal(t, []frameworkItem{{Name: "slime", Default: true}}, resp.Frameworks)
	require.Equal(t, "slime", resp.Framework)
	require.Equal(t, "qwen3-4B", resp.Spec)
	require.True(t, resp.Supported, resp.Reason)
	require.NotNil(t, resp.Options)

	code, body = call(h.GetConfig, http.MethodGet, "/?workspace=ws1&modelId=m1", "outsider", "", nil)
	require.Equal(t, http.StatusForbidden, code, body)
	code, body = call(h.GetConfig, http.MethodGet, "/?workspace=ws1&modelId=nope", "u1", "", nil)
	require.Equal(t, http.StatusNotFound, code, body)
}

// The export streams one metric at a time, each capped at its newest points, in name order.
func TestExportMetricsStreamsOneMetricAtATime(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, cli := submitHandler(t, db, nil)
	withRun(t, db, cli)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	db.EXPECT().ListPostTrainingRunMetricNames(gomock.Any(), "pt-1").
		Return([]string{"train/loss", "eval/val/loss", "train/grad_norm"}, nil).Times(2)
	for name, value := range map[string]float64{"train/loss": 0.5, "eval/val/loss": 0.7, "train/grad_norm": 2} {
		db.EXPECT().ListLatestPostTrainingRunMetrics(gomock.Any(), "pt-1", []string{name}, maxMetricRows).Return(
			[]*dbclient.PostTrainingRunMetric{{Name: name, Step: 1, Value: value, WallTime: at}}, nil).AnyTimes()
	}
	code, body := call(h.ExportMetrics, http.MethodGet, "/", "u1", "pt-1", nil)
	require.Equal(t, http.StatusOK, code, body)
	require.Equal(t, "name,step,value,time\n"+
		"eval/val/loss,1,0.7,2026-01-02T03:04:05Z\n"+
		"train/grad_norm,1,2,2026-01-02T03:04:05Z\n"+
		"train/loss,1,0.5,2026-01-02T03:04:05Z\n", body)

	code, body = call(h.ExportMetrics, http.MethodGet, "/?names=train/*", "u1", "pt-1", nil)
	require.Equal(t, http.StatusOK, code, body)
	require.NotContains(t, body, "eval/val/loss", "only the selected metrics are exported")
	require.Equal(t, 3, strings.Count(body, "\n"))
}

// A curve request of named metrics reads only those.
func TestACurveRequestReadsTheNamedMetrics(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h, cli := submitHandler(t, db, nil)
	withRun(t, db, cli)
	db.EXPECT().ListPostTrainingRunMetricNames(gomock.Any(), "pt-1").Return([]string{"train/loss", "eval/x"}, nil)
	db.EXPECT().ListLatestPostTrainingRunMetrics(gomock.Any(), "pt-1", []string{"train/loss"}, maxMetricRows).
		Return([]*dbclient.PostTrainingRunMetric{{Name: "train/loss", Step: 1, Value: 0.5}}, nil)
	code, body := call(h.GetMetrics, http.MethodGet, "/?names=train/*", "u1", "pt-1", nil)
	require.Equal(t, http.StatusOK, code, body)
	require.Contains(t, body, `"train/loss"`)

	db.EXPECT().ListPostTrainingRunMetricNames(gomock.Any(), "pt-1").Return([]string{"train/loss"}, nil)
	code, body = call(h.GetMetrics, http.MethodGet, "/?names=rollout/*", "u1", "pt-1", nil)
	require.Equal(t, http.StatusOK, code, body)
	require.Contains(t, body, `"series":[]`, "no metric matches")
}
