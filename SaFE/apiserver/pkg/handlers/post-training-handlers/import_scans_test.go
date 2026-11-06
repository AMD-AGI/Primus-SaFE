/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

func TestScanPath(t *testing.T) {
	all := []string{"/shared_nfs"}
	for in, want := range map[string]string{
		"/shared_nfs/zhanglei/runs/j2r5-229357":    "/shared_nfs/zhanglei/runs/j2r5-229357",
		" /shared_nfs/zhanglei/runs/j2r5-229357/ ": "/shared_nfs/zhanglei/runs/j2r5-229357",
		"/shared_nfs/runs/j2 $(id)":                "/shared_nfs/runs/j2 $(id)",
	} {
		got, _, err := scanPath(in, "/shared_nfs", all)
		require.NoError(t, err, in)
		require.Equal(t, want, got)
	}
	for _, in := range []string{"", "runs/j2", "/", "/shared_nfs", "/shared_nfsx/j2", "/other/j2",
		"/shared_nfs/a/../../etc", "/shared_nfs/./a", "/shared_nfs//a", "/shared_nfs/a\nb"} {
		_, _, err := scanPath(in, "/shared_nfs", all)
		require.Error(t, err, in)
	}

	mine := []string{"/shared_nfs/zhanglei", "/shared_nfs/shared/runs/"}
	got, prefix, err := scanPath("/shared_nfs/zhanglei/runs/j2", "/shared_nfs", mine)
	require.NoError(t, err)
	require.Equal(t, "/shared_nfs/zhanglei/runs/j2", got)
	require.Equal(t, "/shared_nfs/zhanglei", prefix, "the scan job resolves symlinks against the prefix")
	_, prefix, err = scanPath("/shared_nfs/shared/runs/j2", "/shared_nfs", mine)
	require.NoError(t, err)
	require.Equal(t, "/shared_nfs/shared/runs", prefix)
	_, _, err = scanPath("/shared_nfs/zhanglei", "/shared_nfs", mine)
	require.NoError(t, err, "the prefix itself may be a run directory")
	for _, in := range []string{"/shared_nfs/alice/runs/j2", "/shared_nfs/zhangleix/runs/j2"} {
		_, _, err = scanPath(in, "/shared_nfs", mine)
		require.ErrorContains(t, err, "must be under /shared_nfs/zhanglei", in)
	}
	_, _, err = scanPath("/shared_nfs/zhanglei/runs/j2", "/shared_nfs", nil)
	require.ErrorContains(t, err, "allowedPrefixes", "without prefixes nothing can be scanned")
}

func TestScanWorkload(t *testing.T) {
	m := &registry.ImportMount{Cluster: "c1", Workspace: "spur-import", Root: "/shared_nfs"}
	tok := &dbclient.PostTrainingImportToken{TokenID: "pti-0123456789abcdef", Workspace: "ws", UserID: "u1",
		UserName: "alice"}
	w := scanWorkload(m, tok, "sst_a.b", "/shared_nfs/runs/j2 $(id)", "/shared_nfs/runs")
	require.Equal(t, "pti-0123456789abcdef", w.Name, "the scan's id finds its job")
	require.Equal(t, "spur-import", w.Spec.Workspace, "the scan runs where Spur's storage is mounted")
	require.Equal(t, common.JobKind, w.Spec.Kind)
	require.Equal(t, v1.PostTrainingRoleImportScan, w.Labels[v1.PostTrainingRoleLabel])
	require.Empty(t, w.Labels[v1.PostTrainingRunIdLabel], "the Post Training controller leaves it alone")
	require.Empty(t, w.Labels[v1.DisplayNameLabel], "a directory name that is no label value is left out")
	require.Equal(t, "u1", w.Labels[v1.UserIdLabel])
	require.Equal(t, importScanTimeout, *w.Spec.Timeout)
	require.Equal(t, map[string]string{"SAFE_IMPORT_TOKEN": "sst_a.b", "PT_IMPORT_DIR": "/shared_nfs/runs/j2 $(id)",
		"PT_IMPORT_ROOT": "/shared_nfs/runs", "PYTHONUNBUFFERED": "1"}, w.Spec.Env,
		"the command checks the resolved directory against the user's prefix, not the whole mount")
	script, err := base64.StdEncoding.DecodeString(w.Spec.EntryPoints[0])
	require.NoError(t, err)
	require.NotContains(t, string(script), "j2", "the directory reaches the command through the environment")
	require.Contains(t, string(script), `--stage --root "$PT_IMPORT_ROOT" "$PT_IMPORT_DIR"`)
	require.Equal(t, "import-scan-j2r5-229357",
		scanWorkload(m, tok, "sst_a.b", "/shared_nfs/runs/j2r5-229357", "/shared_nfs/runs").Labels[v1.DisplayNameLabel])
}

func TestImportScanStates(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	running := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pti-run"}}
	failed := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pti-fail"}, Status: v1.WorkloadStatus{
		Phase: v1.WorkloadFailed, Message: "BackoffLimitExceeded", Pods: []v1.WorkloadPod{{Containers: []v1.Container{
			{Name: "main", Message: "The directory must be under /shared_nfs."}}}}}}
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h := NewHandler(fake.NewClientBuilder().WithScheme(scheme).WithObjects(running, failed).Build(), db)
	old, future := pq.NullTime{Time: time.Now().Add(-5 * time.Minute), Valid: true}, time.Now().Add(20*time.Minute)
	state := func(tok *dbclient.PostTrainingImportToken) *importScanStatus {
		s, err := h.scanStatus(context.Background(), tok)
		require.NoError(t, err)
		return s
	}
	job := func(id string) *dbclient.PostTrainingImportToken {
		return &dbclient.PostTrainingImportToken{TokenID: id, Workspace: "ws", ExpiresAt: future, CreatedAt: old}
	}

	require.Equal(t, scanRunning, state(job("pti-run")).State)
	s := state(job("pti-fail"))
	require.Equal(t, scanFailed, s.State)
	require.Equal(t, "The directory must be under /shared_nfs.", s.Message, "the command's own reason")
	refused := job("pti-run")
	refused.LastError = "unknown parserVersion 9"
	require.Equal(t, "unknown parserVersion 9", state(refused).Message)
	fresh := job("pti-new")
	fresh.CreatedAt = pq.NullTime{Time: time.Now(), Valid: true}
	require.Equal(t, scanRunning, state(fresh).State, "a job just created may not be listed yet")
	require.Equal(t, scanFailed, state(job("pti-gone")).State)
	late := job("pti-run")
	late.ExpiresAt = time.Now().Add(-time.Second)
	require.Equal(t, "The scan did not finish within 30 minutes.", state(late).Message)

	used := job("pti-done")
	used.UsedAt, used.ImportID = sql.NullTime{Time: time.Now(), Valid: true}, "imp-1"
	staged := &dbclient.PostTrainingImport{ImportID: "imp-1", SourcePath: "/shared_nfs/runs/j2r5-229357",
		State: dbclient.PostTrainingImportStaged, Summary: string(importSummaryJSON(t, nil)), CreatedAt: old}
	db.EXPECT().GetPostTrainingImport(gomock.Any(), "imp-1").Return(staged, nil)
	db.EXPECT().GetPostTrainingImportByPathKey(gomock.Any(), importPathKey("ws", staged.SourcePath)).Return(
		&dbclient.PostTrainingImport{RunID: "spur-j2r5-229357"}, nil)
	s = state(used)
	require.Equal(t, scanReady, s.State)
	require.Equal(t, "spur-j2r5-229357", s.ExistingRunID, "the dialog says the directory is already imported")
	require.Equal(t, 2, s.Preview.Nodes)
	require.Equal(t, 3, s.Preview.Series, "the dialog warns when a directory has no curves")
	require.WithinDuration(t, old.Time.Add(importStagedTTL), *s.ExpiresAt, time.Second)

	expired := *staged
	expired.CreatedAt = pq.NullTime{Time: time.Now().Add(-2 * time.Hour), Valid: true}
	db.EXPECT().GetPostTrainingImport(gomock.Any(), "imp-1").Return(&expired, nil)
	s = state(used)
	require.Equal(t, scanFailed, s.State)
	require.Nil(t, s.Preview)

	committed := *staged
	committed.State, committed.RunID = dbclient.PostTrainingImportCommitted, "spur-j2r5-229357"
	db.EXPECT().GetPostTrainingImport(gomock.Any(), "imp-1").Return(&committed, nil)
	s = state(used)
	require.Equal(t, scanImported, s.State)
	require.Equal(t, "spur-j2r5-229357", s.RunID)
}

func TestCommitImportScan(t *testing.T) {
	reg, err := registry.Parse([]byte(importRegistry))
	require.NoError(t, err)
	h, db := importHandler(t)
	commit := func(tok *dbclient.PostTrainingImportToken) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/post-training/imports/scans/pti-1/commit", nil)
		h.commitScan(c, reg, tok)
		return w
	}
	tok := &dbclient.PostTrainingImportToken{TokenID: "pti-1", Workspace: "ws", Purpose: importPurposeScan,
		UserID: "u1", UserName: "alice"}
	require.Equal(t, http.StatusConflict, commit(tok).Code, "nothing staged yet")

	tok.UsedAt, tok.ImportID = sql.NullTime{Time: time.Now(), Valid: true}, "imp-1"
	staged := &dbclient.PostTrainingImport{ImportID: "imp-1", SourcePath: "/shared_nfs/runs/j2r5-229357",
		State: dbclient.PostTrainingImportStaged, Summary: string(importSummaryJSON(t, nil)),
		CreatedAt: pq.NullTime{Time: time.Now().Add(-time.Minute), Valid: true}}
	db.EXPECT().GetPostTrainingImport(gomock.Any(), "imp-1").Return(staged, nil)
	db.EXPECT().GetPostTrainingRun(gomock.Any(), "spur-j2r5-229357").Return(nil, gorm.ErrRecordNotFound)
	var write dbclient.PostTrainingImportWrite
	db.EXPECT().SavePostTrainingImport(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, w dbclient.PostTrainingImportWrite) error { write = w; return nil })
	w := commit(tok)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"runId":"spur-j2r5-229357"`)
	require.Equal(t, "imp-1", write.StagedID)
	require.Empty(t, write.TokenID, "the scan spent its token when it staged")
	require.Equal(t, importViaScan, write.Import.Via)
	require.True(t, write.Import.PathKey.Valid)
	require.Len(t, write.Points, 3)
	require.Equal(t, "alice", write.Run.UserName, "the run belongs to who scanned it, in the scan's workspace")
	require.Equal(t, "ws", write.Run.Workspace)

	db.EXPECT().GetPostTrainingImport(gomock.Any(), "imp-1").Return(staged, nil)
	db.EXPECT().GetPostTrainingRun(gomock.Any(), gomock.Any()).Return(nil, gorm.ErrRecordNotFound)
	db.EXPECT().SavePostTrainingImport(gomock.Any(), gomock.Any()).Return(dbclient.ErrPostTrainingImportExists)
	db.EXPECT().GetPostTrainingImportByPathKey(gomock.Any(), gomock.Any()).Return(
		&dbclient.PostTrainingImport{RunID: "spur-j2r5-229357"}, nil)
	w = commit(tok)
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), `"runId":"spur-j2r5-229357"`)

	committed := *staged
	committed.State, committed.RunID = dbclient.PostTrainingImportCommitted, "spur-j2r5-229357"
	db.EXPECT().GetPostTrainingImport(gomock.Any(), "imp-1").Return(&committed, nil)
	w = commit(tok)
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), "already imported as spur-j2r5-229357")

	expired := *staged
	expired.CreatedAt = pq.NullTime{Time: time.Now().Add(-2 * time.Hour), Valid: true}
	db.EXPECT().GetPostTrainingImport(gomock.Any(), "imp-1").Return(&expired, nil)
	require.Equal(t, http.StatusBadRequest, commit(tok).Code)
}

func TestScanUploadsAreStaged(t *testing.T) {
	h, db := importHandler(t)
	scan := cliToken()
	scan.Purpose = importPurposeScan
	db.EXPECT().GetPostTrainingImportTokenByHash(gomock.Any(), gomock.Any()).Return(scan, nil).Times(2)
	require.Equal(t, http.StatusBadRequest, upload(h, "sst_a.b", importSummaryJSON(t, nil)).Code,
		"a scan's token only stages")

	var write dbclient.PostTrainingImportWrite
	db.EXPECT().SavePostTrainingImport(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, w dbclient.PostTrainingImportWrite) error { write = w; return nil })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/post-training/imports?stage=true",
		bytes.NewReader(importSummaryJSON(t, nil)))
	c.Request.Header.Set("Authorization", "Bearer sst_a.b")
	h.UploadImport(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Nil(t, write.Run, "nothing is imported before the dialog's commit")
	require.Equal(t, "pti-1", write.TokenID)
	require.Equal(t, dbclient.PostTrainingImportStaged, write.Import.State)
	require.False(t, write.Import.PathKey.Valid, "a staged scan does not hold the directory")
	require.Contains(t, write.Import.Summary, `"metrics"`, "the commit writes the points from the staged summary")
}
