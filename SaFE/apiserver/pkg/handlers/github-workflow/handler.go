/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package githubworkflow

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func RegisterRoutes(router *gin.RouterGroup) {
	gh := router.Group("/github-workflow")
	gh.GET("/collection-configs", handleListConfigs)
	gh.POST("/collection-configs", handleCreateConfig)
	gh.DELETE("/collection-configs/:id", handleDeleteConfig)
	gh.GET("/runs", handleListRuns)
	gh.GET("/runs/:id", handleGetRun)
	gh.GET("/runs/:id/jobs", handleGetRunJobs)
	gh.GET("/runs/:id/metrics", handleGetRunMetrics)
	gh.GET("/commits/:sha", handleGetCommit)
	gh.GET("/stats", handleStats)
	gh.GET("/collection-configs/:id/fields", handleGetFields)
}

func getDB() *sql.DB {
	gormDB, err := dbclient.NewClient().GetGormDB()
	if err != nil {
		return nil
	}
	sqlDB, _ := gormDB.DB()
	return sqlDB
}

func handleListConfigs(c *gin.Context) {
	db := getDB()
	rows, err := db.QueryContext(c.Request.Context(), `
		SELECT id, name, github_owner, github_repo, workflow_patterns, branch_patterns,
		       file_patterns, COALESCE(display_settings::text, '{}'), enabled, COALESCE(created_by, ''), created_at, updated_at
		FROM github_collection_configs ORDER BY name`)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var configs []map[string]interface{}
	for rows.Next() {
		var id int
		var name, owner, repo, wp, bp, fp, ds, createdBy string
		var enabled bool
		var createdAt, updatedAt time.Time
		rows.Scan(&id, &name, &owner, &repo, &wp, &bp, &fp, &ds, &enabled, &createdBy, &createdAt, &updatedAt)
		var displaySettings interface{}
		json.Unmarshal([]byte(ds), &displaySettings)
		configs = append(configs, map[string]interface{}{
			"id": id, "name": name, "github_owner": owner, "github_repo": repo,
			"workflow_patterns": pgArr(wp), "branch_patterns": pgArr(bp), "file_patterns": pgArr(fp),
			"display_settings": displaySettings, "enabled": enabled, "created_by": createdBy,
			"created_at": createdAt.Format(time.RFC3339), "updated_at": updatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(200, gin.H{"configs": configs, "count": len(configs)})
}

func handleCreateConfig(c *gin.Context) {
	var req struct {
		Name             string                 `json:"name" binding:"required"`
		GithubOwner      string                 `json:"github_owner" binding:"required"`
		GithubRepo       string                 `json:"github_repo" binding:"required"`
		WorkflowPatterns []string               `json:"workflow_patterns"`
		BranchPatterns   []string               `json:"branch_patterns"`
		FilePatterns     []string               `json:"file_patterns" binding:"required"`
		DisplaySettings  map[string]interface{} `json:"display_settings"`
		Enabled          *bool                  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	displayJSON, _ := json.Marshal(req.DisplaySettings)
	if req.DisplaySettings == nil {
		displayJSON = []byte("{}")
	}

	db := getDB()
	_, err := db.ExecContext(c.Request.Context(), `
		INSERT INTO github_collection_configs
			(name, github_owner, github_repo, workflow_patterns, branch_patterns, file_patterns, display_settings, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		req.Name, req.GithubOwner, req.GithubRepo,
		sliceToArr(req.WorkflowPatterns), sliceToArr(req.BranchPatterns), sliceToArr(req.FilePatterns),
		displayJSON, enabled)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"created": req.Name})
}

func handleDeleteConfig(c *gin.Context) {
	id := c.Param("id")
	db := getDB()
	db.ExecContext(c.Request.Context(), `DELETE FROM github_collection_configs WHERE id = $1`, id)
	c.JSON(200, gin.H{"deleted": id})
}

func handleListRuns(c *gin.Context) {
	db := getDB()
	owner := c.Query("github_owner")
	repo := c.Query("github_repo")
	status := c.Query("status")
	workflow := c.Query("workflow")
	limit := 50
	if l := c.Query("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}

	query := `SELECT id, workload_id, cluster, github_run_id, github_job_id, workflow_name,
	                 github_owner, github_repo, head_branch, head_sha, status, conclusion,
	                 sync_status, started_at, completed_at, created_at
	          FROM github_workflow_runs WHERE 1=1`
	var args []interface{}
	idx := 1
	if owner != "" {
		query += fmt.Sprintf(` AND github_owner = $%d`, idx)
		args = append(args, owner)
		idx++
	}
	if repo != "" {
		query += fmt.Sprintf(` AND github_repo = $%d`, idx)
		args = append(args, repo)
		idx++
	}
	if status != "" {
		query += fmt.Sprintf(` AND status = $%d`, idx)
		args = append(args, status)
		idx++
	}
	if workflow != "" {
		query += fmt.Sprintf(` AND workflow_name = $%d`, idx)
		args = append(args, workflow)
		idx++
	}
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, idx)
	args = append(args, limit)

	rows, err := db.QueryContext(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var runs []map[string]interface{}
	for rows.Next() {
		var id int
		var wid, cluster, wfName, ghOwner, ghRepo, branch, sha, st, conclusion, syncSt string
		var ghRunID, ghJobID int64
		var startedAt, completedAt sql.NullTime
		var createdAt time.Time
		rows.Scan(&id, &wid, &cluster, &ghRunID, &ghJobID, &wfName,
			&ghOwner, &ghRepo, &branch, &sha, &st, &conclusion,
			&syncSt, &startedAt, &completedAt, &createdAt)
		run := map[string]interface{}{
			"id": id, "workload_id": wid, "cluster": cluster,
			"github_run_id": ghRunID, "github_job_id": ghJobID, "workflow_name": wfName,
			"github_owner": ghOwner, "github_repo": ghRepo,
			"head_branch": branch, "head_sha": sha,
			"status": st, "conclusion": conclusion, "sync_status": syncSt,
			"created_at": createdAt.Format(time.RFC3339),
		}
		if startedAt.Valid {
			run["started_at"] = startedAt.Time.Format(time.RFC3339)
		}
		if completedAt.Valid {
			run["completed_at"] = completedAt.Time.Format(time.RFC3339)
		}
		runs = append(runs, run)
	}
	c.JSON(200, gin.H{"runs": runs, "count": len(runs)})
}

func handleGetRun(c *gin.Context) {
	id := c.Param("id")
	db := getDB()

	var run map[string]interface{}
	var wid, cluster, wfName, ghOwner, ghRepo, branch, sha, st, conclusion, syncSt string
	var ghRunID, ghJobID int64
	var startedAt, completedAt sql.NullTime
	var createdAt time.Time

	err := db.QueryRowContext(c.Request.Context(), `
		SELECT workload_id, cluster, github_run_id, github_job_id, workflow_name,
		       github_owner, github_repo, head_branch, head_sha, status, conclusion,
		       sync_status, started_at, completed_at, created_at
		FROM github_workflow_runs WHERE id = $1`, id).
		Scan(&wid, &cluster, &ghRunID, &ghJobID, &wfName, &ghOwner, &ghRepo,
			&branch, &sha, &st, &conclusion, &syncSt, &startedAt, &completedAt, &createdAt)
	if err != nil {
		c.JSON(404, gin.H{"error": "run not found"})
		return
	}

	run = map[string]interface{}{
		"id": id, "workload_id": wid, "cluster": cluster,
		"github_run_id": ghRunID, "github_job_id": ghJobID, "workflow_name": wfName,
		"github_owner": ghOwner, "github_repo": ghRepo,
		"head_branch": branch, "head_sha": sha,
		"status": st, "conclusion": conclusion, "sync_status": syncSt,
		"created_at": createdAt.Format(time.RFC3339),
	}
	if startedAt.Valid {
		run["started_at"] = startedAt.Time.Format(time.RFC3339)
	}
	if completedAt.Valid {
		run["completed_at"] = completedAt.Time.Format(time.RFC3339)
	}

	var detailsRaw []byte
	db.QueryRowContext(c.Request.Context(), `SELECT raw_data FROM github_workflow_run_details WHERE run_id = $1`, id).Scan(&detailsRaw)
	if len(detailsRaw) > 0 {
		var details interface{}
		json.Unmarshal(detailsRaw, &details)
		run["github_details"] = details
	}

	c.JSON(200, run)
}

func handleGetRunJobs(c *gin.Context) {
	id := c.Param("id")
	db := getDB()

	rows, err := db.QueryContext(c.Request.Context(), `
		SELECT j.id, j.github_job_id, j.name, j.status, j.conclusion,
		       j.started_at, j.completed_at, j.runner_name
		FROM github_workflow_jobs j WHERE j.run_id = $1
		ORDER BY j.started_at`, id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var jobs []map[string]interface{}
	for rows.Next() {
		var jid int
		var ghJobID int64
		var name, status, conclusion, runnerName string
		var startedAt, completedAt sql.NullTime
		rows.Scan(&jid, &ghJobID, &name, &status, &conclusion, &startedAt, &completedAt, &runnerName)

		job := map[string]interface{}{
			"id": jid, "github_job_id": ghJobID, "name": name,
			"status": status, "conclusion": conclusion, "runner_name": runnerName,
		}
		if startedAt.Valid {
			job["started_at"] = startedAt.Time.Format(time.RFC3339)
		}
		if completedAt.Valid {
			job["completed_at"] = completedAt.Time.Format(time.RFC3339)
		}

		stepRows, _ := db.QueryContext(c.Request.Context(), `
			SELECT step_number, name, status, conclusion, duration_seconds
			FROM github_workflow_steps WHERE job_id = $1 ORDER BY step_number`, jid)
		var steps []map[string]interface{}
		if stepRows != nil {
			for stepRows.Next() {
				var sn, dur int
				var sname, sstatus, sconclusion string
				stepRows.Scan(&sn, &sname, &sstatus, &sconclusion, &dur)
				steps = append(steps, map[string]interface{}{
					"step_number": sn, "name": sname, "status": sstatus,
					"conclusion": sconclusion, "duration_seconds": dur,
				})
			}
			stepRows.Close()
		}
		job["steps"] = steps
		jobs = append(jobs, job)
	}

	c.JSON(200, gin.H{"jobs": jobs, "count": len(jobs)})
}

func handleGetRunMetrics(c *gin.Context) {
	id := c.Param("id")
	db := getDB()

	rows, err := db.QueryContext(c.Request.Context(), `
		SELECT id, config_id, source_file, row_data, timestamp, dimensions, metrics, created_at
		FROM github_workflow_metrics WHERE run_id = $1
		ORDER BY created_at`, id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var metrics []map[string]interface{}
	for rows.Next() {
		var mid int
		var configID sql.NullInt64
		var sourceFile sql.NullString
		var rowDataBytes, dims, mets []byte
		var ts sql.NullTime
		var createdAt time.Time
		rows.Scan(&mid, &configID, &sourceFile, &rowDataBytes, &ts, &dims, &mets, &createdAt)
		m := map[string]interface{}{
			"id":         mid,
			"created_at": createdAt.Format(time.RFC3339),
		}
		if configID.Valid {
			m["config_id"] = configID.Int64
		}
		if sourceFile.Valid {
			m["source_file"] = sourceFile.String
		}
		if len(rowDataBytes) > 0 && string(rowDataBytes) != "{}" {
			var rd interface{}
			json.Unmarshal(rowDataBytes, &rd)
			m["row_data"] = rd
		}
		if ts.Valid {
			m["timestamp"] = ts.Time.Format(time.RFC3339)
		}
		if len(dims) > 0 && string(dims) != "{}" {
			var d interface{}
			json.Unmarshal(dims, &d)
			m["dimensions"] = d
		}
		if len(mets) > 0 && string(mets) != "{}" {
			var v interface{}
			json.Unmarshal(mets, &v)
			m["metrics"] = v
		}
		metrics = append(metrics, m)
	}

	c.JSON(200, gin.H{"metrics": metrics, "count": len(metrics)})
}

func handleGetCommit(c *gin.Context) {
	sha := c.Param("sha")
	db := getDB()

	var message, authorName, authorEmail, ghOwner, ghRepo string
	var authoredAt sql.NullTime
	var additions, deletions, filesChanged int

	err := db.QueryRowContext(c.Request.Context(), `
		SELECT github_owner, github_repo, message, author_name, author_email, authored_at,
		       additions, deletions, files_changed
		FROM github_workflow_commits WHERE sha = $1`, sha).
		Scan(&ghOwner, &ghRepo, &message, &authorName, &authorEmail, &authoredAt,
			&additions, &deletions, &filesChanged)
	if err != nil {
		c.JSON(404, gin.H{"error": "commit not found"})
		return
	}

	result := map[string]interface{}{
		"sha": sha, "github_owner": ghOwner, "github_repo": ghRepo,
		"message": message, "author_name": authorName, "author_email": authorEmail,
		"additions": additions, "deletions": deletions, "files_changed": filesChanged,
	}
	if authoredAt.Valid {
		result["authored_at"] = authoredAt.Time.Format(time.RFC3339)
	}
	c.JSON(200, result)
}

func handleStats(c *gin.Context) {
	db := getDB()
	var total, running, completed, failed int
	db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM github_workflow_runs`).Scan(&total)
	db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM github_workflow_runs WHERE status='running'`).Scan(&running)
	db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM github_workflow_runs WHERE status='completed'`).Scan(&completed)
	db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM github_workflow_runs WHERE conclusion='failure'`).Scan(&failed)

	c.JSON(200, gin.H{
		"total": total, "running": running, "completed": completed, "failed": failed,
	})
}

func handleGetFields(c *gin.Context) {
	configID := c.Param("id")
	db := getDB()

	rows, err := db.QueryContext(c.Request.Context(), `
		SELECT key,
		       count(*) as total,
		       count(*) FILTER (WHERE value ~ '^-?[0-9]+\.?[0-9]*([eE][+-]?[0-9]+)?$') as numeric_count,
		       count(DISTINCT value) as distinct_count
		FROM github_workflow_metrics,
		     jsonb_each_text(row_data) AS kv(key, value)
		WHERE config_id = $1
		  AND created_at > NOW() - INTERVAL '30 days'
		GROUP BY key
		ORDER BY key`, configID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var fields []map[string]interface{}
	for rows.Next() {
		var key string
		var total, numericCount, distinctCount int
		rows.Scan(&key, &total, &numericCount, &distinctCount)

		dataType := "string"
		if numericCount > total/2 {
			dataType = "number"
		}

		hint := "dimension"
		if dataType == "number" && (distinctCount > 20 || float64(distinctCount) > float64(total)*0.1) {
			hint = "metric"
		}

		fields = append(fields, map[string]interface{}{
			"name":           key,
			"data_type":      dataType,
			"distinct_count": distinctCount,
			"total_count":    total,
			"hint":           hint,
		})
	}

	var displaySettings interface{}
	var ds string
	db.QueryRowContext(c.Request.Context(),
		`SELECT COALESCE(display_settings::text, '{}') FROM github_collection_configs WHERE id = $1`, configID).Scan(&ds)
	json.Unmarshal([]byte(ds), &displaySettings)

	c.JSON(200, gin.H{
		"fields":           fields,
		"display_settings": displaySettings,
	})
}

func pgArr(s string) []string {
	s = strings.TrimPrefix(strings.TrimSuffix(s, "}"), "{")
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.Trim(p, "\""))
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func sliceToArr(s []string) string {
	if len(s) == 0 {
		return "{}"
	}
	return "{" + strings.Join(s, ",") + "}"
}
