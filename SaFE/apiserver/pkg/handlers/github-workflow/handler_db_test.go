/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package githubworkflow

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// withMockDB installs a sqlmock-backed *sql.DB as the package getDB source and
// returns the mock plus a cleanup func that restores the original resolver.
func withMockDB(t *testing.T) (sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	orig := getDB
	getDB = func() *sql.DB { return db }
	return mock, func() {
		getDB = orig
		db.Close()
	}
}

func ctxWith(method, target, body string, params gin.Params, query string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	r := httptest.NewRequest(method, target+"?"+query, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	c.Request = r
	c.Params = params
	return c, w
}

// TestHandleListConfigsSuccess verifies a config row is rendered as JSON.
func TestHandleListConfigsSuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "name", "github_owner", "github_repo", "workflow_patterns", "branch_patterns",
		"file_patterns", "display_settings", "enabled", "created_by", "created_at", "updated_at",
	}).AddRow(1, "cfg", "o", "r", "{a}", "{b}", "{c}", "{}", true, "user", now, now)
	mock.ExpectQuery("SELECT id, name, github_owner").WillReturnRows(rows)

	c, w := ctxWith(http.MethodGet, "/configs", "", nil, "")
	handleListConfigs(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleListConfigsError verifies a query error yields a 500.
func TestHandleListConfigsError(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()
	mock.ExpectQuery("SELECT id, name, github_owner").WillReturnError(sql.ErrConnDone)

	c, w := ctxWith(http.MethodGet, "/configs", "", nil, "")
	handleListConfigs(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestHandleListConfigsNilDBReturnsError(t *testing.T) {
	orig := getDB
	getDB = func() *sql.DB { return nil }
	defer func() { getDB = orig }()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleListConfigs panicked with nil DB: %v", r)
		}
	}()

	c, w := ctxWith(http.MethodGet, "/configs", "", nil, "")
	handleListConfigs(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestHandleCreateConfigSuccess verifies a valid request inserts and returns 201.
func TestHandleCreateConfigSuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()
	mock.ExpectExec("INSERT INTO github_collection_configs").
		WillReturnResult(sqlmock.NewResult(1, 1))

	body := `{"name":"n","github_owner":"o","github_repo":"r","file_patterns":["*.json"]}`
	c, w := ctxWith(http.MethodPost, "/configs", body, nil, "")
	handleCreateConfig(c)
	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleCreateConfigDBError verifies an insert error yields a 500.
func TestHandleCreateConfigDBError(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()
	mock.ExpectExec("INSERT INTO github_collection_configs").WillReturnError(sql.ErrConnDone)

	body := `{"name":"n","github_owner":"o","github_repo":"r","file_patterns":["*.json"]}`
	c, w := ctxWith(http.MethodPost, "/configs", body, nil, "")
	handleCreateConfig(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestHandleDeleteConfig verifies the delete path returns 200.
func TestHandleDeleteConfig(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()
	mock.ExpectExec("DELETE FROM github_collection_configs").
		WillReturnResult(sqlmock.NewResult(0, 1))

	c, w := ctxWith(http.MethodDelete, "/configs/5", "", gin.Params{{Key: "id", Value: "5"}}, "")
	handleDeleteConfig(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestHandleListRunsSuccess verifies runs are listed with query filters applied.
func TestHandleListRunsSuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "workload_id", "cluster", "github_run_id", "github_job_id", "workflow_name",
		"github_owner", "github_repo", "head_branch", "head_sha", "status", "conclusion",
		"sync_status", "started_at", "completed_at", "created_at",
	}).AddRow(1, "wl", "c", int64(10), int64(20), "wf", "o", "r", "main", "sha", "completed", "success", "synced", now, now, now)
	mock.ExpectQuery("FROM github_workflow_runs WHERE").WillReturnRows(rows)

	c, w := ctxWith(http.MethodGet, "/runs", "", nil, "github_owner=o&github_repo=r&status=completed&workflow=wf&limit=10")
	handleListRuns(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleListRunsError verifies a query error yields a 500.
func TestHandleListRunsError(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()
	mock.ExpectQuery("FROM github_workflow_runs WHERE").WillReturnError(sql.ErrConnDone)

	c, w := ctxWith(http.MethodGet, "/runs", "", nil, "")
	handleListRuns(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestHandleGetRunNotFound verifies a missing run yields a 404.
func TestHandleGetRunNotFound(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()
	mock.ExpectQuery("FROM github_workflow_runs WHERE id =").WillReturnError(sql.ErrNoRows)

	c, w := ctxWith(http.MethodGet, "/runs/1", "", gin.Params{{Key: "id", Value: "1"}}, "")
	handleGetRun(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// TestHandleGetRunSuccess verifies a found run is rendered with details.
func TestHandleGetRunSuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	now := time.Now()
	mock.ExpectQuery("FROM github_workflow_runs WHERE id =").
		WillReturnRows(sqlmock.NewRows([]string{
			"workload_id", "cluster", "github_run_id", "github_job_id", "workflow_name",
			"github_owner", "github_repo", "head_branch", "head_sha", "status", "conclusion",
			"sync_status", "started_at", "completed_at", "created_at",
		}).AddRow("wl", "c", int64(1), int64(2), "wf", "o", "r", "main", "sha",
			"completed", "success", "synced", now, now, now))
	mock.ExpectQuery("FROM github_workflow_run_details WHERE run_id =").
		WillReturnRows(sqlmock.NewRows([]string{"raw_data"}).AddRow([]byte(`{"k":"v"}`)))

	c, w := ctxWith(http.MethodGet, "/runs/1", "", gin.Params{{Key: "id", Value: "1"}}, "")
	handleGetRun(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleGetCommitNotFound verifies a missing commit yields a 404.
func TestHandleGetCommitNotFound(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()
	mock.ExpectQuery("FROM github_workflow_commits WHERE sha =").WillReturnError(sql.ErrNoRows)

	c, w := ctxWith(http.MethodGet, "/commits/abc", "", gin.Params{{Key: "sha", Value: "abc"}}, "")
	handleGetCommit(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// TestHandleGetCommitSuccess verifies a found commit is rendered.
func TestHandleGetCommitSuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	mock.ExpectQuery("FROM github_workflow_commits WHERE sha =").
		WillReturnRows(sqlmock.NewRows([]string{
			"github_owner", "github_repo", "message", "author_name", "author_email",
			"authored_at", "additions", "deletions", "files_changed",
		}).AddRow("o", "r", "msg", "auth", "a@b.com", time.Now(), 1, 2, 3))

	c, w := ctxWith(http.MethodGet, "/commits/abc", "", gin.Params{{Key: "sha", Value: "abc"}}, "")
	handleGetCommit(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleStats verifies aggregate counts are returned.
func TestHandleStats(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	count := func(n int) *sqlmock.Rows { return sqlmock.NewRows([]string{"c"}).AddRow(n) }
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM github_workflow_runs$").WillReturnRows(count(10))
	mock.ExpectQuery("status='running'").WillReturnRows(count(2))
	mock.ExpectQuery("status='completed'").WillReturnRows(count(7))
	mock.ExpectQuery("conclusion='failure'").WillReturnRows(count(1))

	c, w := ctxWith(http.MethodGet, "/stats", "", nil, "")
	handleStats(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleGetRunJobsSuccess verifies jobs and their steps are rendered.
func TestHandleGetRunJobsSuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	now := time.Now()
	mock.ExpectQuery("FROM github_workflow_jobs j WHERE j.run_id =").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "github_job_id", "name", "status", "conclusion",
			"started_at", "completed_at", "runner_name",
		}).AddRow(1, int64(5), "build", "completed", "success", now, now, "runner-1"))
	mock.ExpectQuery("FROM github_workflow_steps WHERE job_id =").
		WillReturnRows(sqlmock.NewRows([]string{
			"step_number", "name", "status", "conclusion", "duration_seconds",
		}).AddRow(1, "checkout", "completed", "success", 12))

	c, w := ctxWith(http.MethodGet, "/runs/1/jobs", "", gin.Params{{Key: "id", Value: "1"}}, "")
	handleGetRunJobs(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleGetRunMetricsSuccess verifies metrics rows are rendered.
func TestHandleGetRunMetricsSuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	now := time.Now()
	mock.ExpectQuery("FROM github_workflow_metrics WHERE run_id =").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "config_id", "source_file", "row_data", "timestamp", "dimensions", "metrics", "created_at",
		}).AddRow(1, int64(2), "f.json", []byte(`{"a":1}`), now, []byte(`{"d":"x"}`), []byte(`{"m":2}`), now))

	c, w := ctxWith(http.MethodGet, "/runs/1/metrics", "", gin.Params{{Key: "id", Value: "1"}}, "")
	handleGetRunMetrics(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleGetConfigMetricsSuccess verifies config metrics with pagination are rendered.
func TestHandleGetConfigMetricsSuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	now := time.Now()
	mock.ExpectQuery("count\\(\\*\\) FROM github_workflow_metrics WHERE config_id =").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))
	mock.ExpectQuery("FROM github_workflow_metrics").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_file", "row_data", "created_at",
		}).AddRow(1, "f.json", []byte(`{"a":1}`), now))

	c, w := ctxWith(http.MethodGet, "/configs/2/metrics", "", gin.Params{{Key: "id", Value: "2"}}, "limit=10&offset=0")
	handleGetConfigMetrics(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleGetFieldsSuccess verifies field stats and display settings are rendered.
func TestHandleGetFieldsSuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	mock.ExpectQuery("jsonb_each_text").
		WillReturnRows(sqlmock.NewRows([]string{"key", "total", "numeric_count", "distinct_count"}).
			AddRow("latency", 100, 90, 50).
			AddRow("name", 100, 0, 3))
	mock.ExpectQuery("FROM github_collection_configs WHERE id =").
		WillReturnRows(sqlmock.NewRows([]string{"display_settings"}).AddRow("{}"))

	c, w := ctxWith(http.MethodGet, "/configs/2/fields", "", gin.Params{{Key: "id", Value: "2"}}, "")
	handleGetFields(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleUpdateConfigSuccess verifies a full update returns 200.
func TestHandleUpdateConfigSuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()
	mock.ExpectExec("UPDATE github_collection_configs SET").
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"name":"n","file_patterns":["*.json"],"workflow_patterns":["ci"],"branch_patterns":["main"],"display_settings":{"x":1},"enabled":true}`
	c, w := ctxWith(http.MethodPut, "/configs/2", body, gin.Params{{Key: "id", Value: "2"}}, "")
	handleUpdateConfig(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleListRepositoriesSuccess verifies repository aggregation is rendered.
func TestHandleListRepositoriesSuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	now := time.Now()
	mock.ExpectQuery("FROM github_workflow_runs r").
		WillReturnRows(sqlmock.NewRows([]string{
			"github_owner", "github_repo", "total_runs", "running_runs", "completed_runs", "failed_runs", "latest_run_at",
		}).AddRow("o", "r", 10, 1, 8, 1, now))
	mock.ExpectQuery("SELECT id FROM github_collection_configs WHERE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2))

	c, w := ctxWith(http.MethodGet, "/repositories", "", nil, "search=o")
	handleListRepositories(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleGetRepositorySuccess verifies repository detail aggregation is rendered.
func TestHandleGetRepositorySuccess(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	mock.ExpectQuery("FROM github_workflow_runs WHERE github_owner =").
		WillReturnRows(sqlmock.NewRows([]string{"total", "running", "completed", "failed"}).AddRow(5, 1, 3, 1))
	mock.ExpectQuery("SELECT DISTINCT workflow_name FROM github_workflow_runs").
		WillReturnRows(sqlmock.NewRows([]string{"workflow_name"}).AddRow("ci").AddRow("release"))
	mock.ExpectQuery("FROM github_collection_configs c").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "display_settings", "metrics_count"}).
			AddRow(1, "cfg", "{}", 7))

	c, w := ctxWith(http.MethodGet, "/repositories/o/r", "",
		gin.Params{{Key: "owner", Value: "o"}, {Key: "repo", Value: "r"}}, "")
	handleGetRepository(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}
