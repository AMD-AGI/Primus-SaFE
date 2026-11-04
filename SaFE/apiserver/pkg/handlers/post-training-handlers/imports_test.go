/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

// importRegistry is the test registry with slime importing slime-rocm directories.
var importRegistry = strings.Replace(strings.Replace(testRegistry, "adapter: frameworks/slime",
	"adapter: frameworks/slime\n    importFormats: [slime-rocm]", 1),
	`metrics: {include: ["train/*"], maxPointsPerRun: 1000}`,
	`metrics: {include: ["train/*", "eval/*/*"], summary: {sft: train/loss}, maxPointsPerRun: 1000}`, 1)

// importSummaryJSON is a slime-rocm-run/1 summary of a failed SFT run, changed by edit.
func importSummaryJSON(t *testing.T, edit func(map[string]interface{})) []byte {
	s := map[string]interface{}{
		"schema": "slime-rocm-run/1", "parserVersion": "1", "format": "slime-rocm",
		"path": "/shared_nfs/runs/j2r5-229357", "name": "j2r5-229357", "slurmJobId": "229357", "verdict": "FAIL",
		"status": "Failed", "failedStage": "train", "message": "FAIL: VRAM still rising over steps 10-20",
		"trainType": "sft", "method": "full", "startTime": "2026-10-21T10:13:43Z", "endTime": "2026-10-21T11:54:03Z",
		"nodes": []interface{}{map[string]interface{}{"host": "h1", "head": true}, map[string]interface{}{"host": "h2"}},
		"image": "sglang:v0.5", "gpusPerNode": 8, "baseModel": "GLM-5.3-BF16", "dataset": "/d/train.jsonl",
		"stages":  []interface{}{map[string]interface{}{"stage": "train", "label": "Train", "result": "fail"}},
		"gates":   []interface{}{map[string]interface{}{"gate": "b2_sft", "node": "h1", "result": "fail(1)"}},
		"markers": []interface{}{}, "arguments": map[string]interface{}{"kept": map[string]interface{}{"lr": 1e-6},
			"total": 1031},
		"resources":   map[string]interface{}{"h1": map[string]interface{}{"samples": [][]float64{{1, 2, 3}}}},
		"checkpoints": map[string]interface{}{"iterations": []interface{}{}},
		"metrics": map[string]interface{}{
			"train/loss": map[string]interface{}{"axis": "train", "count": 40, "downsampled": true,
				"points": [][]interface{}{{0, 2.0, 1792578000}, {1, 1.5, nil}}},
			"eval/val/loss": map[string]interface{}{"axis": "train", "count": 1, "points": [][]interface{}{{2, 1.2, nil}}},
			"perf/step_time": map[string]interface{}{"axis": "train", "count": 1,
				"points": [][]interface{}{{0, 9.0, nil}}},
		},
		"warnings": []string{"Gate c9 is not a known gate."},
	}
	if edit != nil {
		edit(s)
	}
	data, err := json.Marshal(s)
	require.NoError(t, err)
	return data
}

// importHandler returns a handler whose registry imports slime-rocm directories, with a mock database.
func importHandler(t *testing.T) (*Handler, *mockclient.MockInterface) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: common.PrimusSafeNamespace,
		Name: registry.ConfigMapName}, Data: map[string]string{registry.DataKey: importRegistry}}
	db := mockclient.NewMockInterface(gomock.NewController(t))
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	return NewHandler(fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build(), db), db
}

// upload posts a summary with a token to UploadImport and returns the response.
func upload(h *Handler, token string, summary []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	gz := gzip.NewWriter(&body)
	_, _ = gz.Write(summary)
	_ = gz.Close()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/post-training/imports", &body)
	c.Request.Header.Set("Authorization", "Bearer "+token)
	c.Request.Header.Set("Content-Encoding", "gzip")
	h.UploadImport(c)
	return w
}

// cliToken is an unused command upload token of alice in workspace ws.
func cliToken() *dbclient.PostTrainingImportToken {
	return &dbclient.PostTrainingImportToken{TokenID: "pti-1", Workspace: "ws", Purpose: importPurposeCLI,
		UserID: "u1", UserName: "alice", ExpiresAt: time.Now().Add(10 * time.Minute)}
}

func TestUploadImportWritesTheRun(t *testing.T) {
	h, db := importHandler(t)
	token := "sst_aHR0cHM6Ly9zYWZl.secret"
	db.EXPECT().GetPostTrainingImportTokenByHash(gomock.Any(), tokenHash(token)).Return(cliToken(), nil)
	db.EXPECT().GetPostTrainingRun(gomock.Any(), "spur-j2r5-229357").Return(nil, gorm.ErrRecordNotFound)
	var write dbclient.PostTrainingImportWrite
	db.EXPECT().SavePostTrainingImport(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, w dbclient.PostTrainingImportWrite) error { write = w; return nil })
	w := upload(h, token, importSummaryJSON(t, nil))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"runId":"spur-j2r5-229357"`)

	run := write.Run
	require.Equal(t, dbclient.PostTrainingSourceSpur, run.Source)
	require.Equal(t, "Failed", run.Status)
	require.Equal(t, "done", run.Stage)
	require.Equal(t, "{}", run.Recipe)
	require.Empty(t, run.RecipeHash, "an imported run never counts as a platform success")
	require.Equal(t, "alice", run.UserName)
	require.Equal(t, 2, run.NodeCount)
	require.Equal(t, "/shared_nfs/runs/j2r5-229357", run.OutputPath)
	require.True(t, run.StartTime.Valid)
	require.Equal(t, "pti-1", write.TokenID)
	require.Len(t, write.Points, 3, "perf/step_time is not in the registry's include")
	var sum importMetricSummary
	require.NoError(t, json.Unmarshal([]byte(run.MetricSummary), &sum))
	require.Equal(t, "train/loss", sum.Main.Name)
	require.Equal(t, 1.5, sum.Main.Value)
	require.Equal(t, 1.2, sum.BestValLoss.Value)
	require.Equal(t, map[string]int{"train/loss": 40}, sum.Downsampled, "the curves say how many points the log had")

	imp := write.Import
	require.True(t, imp.PathKey.Valid)
	require.Equal(t, importPathKey("ws", "/shared_nfs/runs/j2r5-229357"), imp.PathKey.String)
	require.NotContains(t, imp.Summary, `"metrics"`, "the points live in the metric table")
	require.Contains(t, imp.Summary, `"gates"`)
	require.Contains(t, imp.Resources, `"h1"`)
	require.Equal(t, dbclient.PostTrainingImportImported, imp.State)
}

func TestUploadImportRefusals(t *testing.T) {
	h, db := importHandler(t)
	db.EXPECT().GetPostTrainingImportTokenByHash(gomock.Any(), gomock.Any()).Return(nil, gorm.ErrRecordNotFound)
	require.Equal(t, http.StatusUnauthorized, upload(h, "sst_unknown.x", importSummaryJSON(t, nil)).Code)

	h, db = importHandler(t)
	db.EXPECT().GetPostTrainingImportTokenByHash(gomock.Any(), gomock.Any()).Return(cliToken(), nil).Times(2)
	db.EXPECT().SetPostTrainingImportTokenError(gomock.Any(), "pti-1", gomock.Any()).Times(2)
	w := upload(h, "sst_a.b", importSummaryJSON(t, func(s map[string]interface{}) { s["schema"] = "other/9" }))
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.Contains(t, w.Body.String(), `"field":"schema"`)

	db.EXPECT().GetPostTrainingRun(gomock.Any(), gomock.Any()).Return(nil, gorm.ErrRecordNotFound)
	db.EXPECT().SavePostTrainingImport(gomock.Any(), gomock.Any()).Return(dbclient.ErrPostTrainingImportExists)
	db.EXPECT().GetPostTrainingImportByPathKey(gomock.Any(), gomock.Any()).Return(
		&dbclient.PostTrainingImport{RunID: "spur-j2r5-229357"}, nil)
	w = upload(h, "sst_a.b", importSummaryJSON(t, nil))
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), `"runId":"spur-j2r5-229357"`)
}

func TestImportSummaryChecks(t *testing.T) {
	reg, err := registry.Parse([]byte(importRegistry))
	require.NoError(t, err)
	for field, edit := range map[string]func(map[string]interface{}){
		"path":   func(s map[string]interface{}) { s["path"] = "/runs/../etc" },
		"status": func(s map[string]interface{}) { s["status"] = "Running" },
		"format": func(s map[string]interface{}) { s["format"] = "verl" },
		"metrics.train/loss": func(s map[string]interface{}) {
			s["metrics"] = map[string]interface{}{"train/loss": map[string]interface{}{"axis": "train", "count": 1,
				"points": [][]interface{}{{0.5, 1.0}}}}
		},
	} {
		_, err := parseImportSummary(importSummaryJSON(t, edit), reg)
		var invalid *importInvalid
		require.ErrorAs(t, err, &invalid, field)
		require.Equal(t, field, invalid.Field)
	}
	require.Equal(t, "spur-j2r5-229357", importRunID("J2r5_229357", ""))
	require.Equal(t, "spur-j2r5-229357-ab12", importRunID("j2r5 229357", "ab12"))
}
