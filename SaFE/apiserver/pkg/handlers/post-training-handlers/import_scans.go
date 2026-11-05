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
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

const (
	// importScanTimeout bounds a scan job from its dispatch, in seconds; its token lives as long.
	importScanTimeout = 30 * 60
	// importScanTTL keeps a finished scan job for its status until its staged summary expires.
	importScanTTL = 3600
	// importStagedTTL is how long a staged summary waits for the dialog's commit.
	importStagedTTL = time.Hour
	// importScanEntrypoint runs the import command the preprocess init container copies into /shared-data; the
	// directory and the root come from the environment, so no user input reaches the shell.
	importScanEntrypoint = `PYTHONPATH=/shared-data python3 -m safe_post_training.spur_import --stage ` +
		`--root "$PT_IMPORT_ROOT" "$PT_IMPORT_DIR"`
	// scan states GET /imports/scans/:id reports.
	scanRunning  = "running"
	scanReady    = "ready"
	scanFailed   = "failed"
	scanImported = "imported"
)

// importScanRequest is the body of POST /post-training/imports/scans.
type importScanRequest struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
}

// CreateImportScan starts a CPU job in the cluster's import workspace that parses a run directory under the mount
// root and stages its summary for the dialog to preview and commit. The scan's id is its token's.
func (h *Handler) CreateImportScan(c *gin.Context) {
	reg, err, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("the Post Training registry is not loaded: "+err.Error()))
		return
	}
	if err = pausedError(reg); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	var req importScanRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request body: "+err.Error()))
		return
	}
	ctx := c.Request.Context()
	ws, err := h.getWorkspace(ctx, req.Workspace)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if err = h.authorize(c, v1.CreateVerb, req.Workspace, "", "", 0); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	m, reason := h.importMount(ctx, reg, ws.Cluster)
	if m == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest(reason))
		return
	}
	// The scan job reads Spur's storage in the import workspace, so the caller must be able to run workloads there.
	if err = h.authorize(c, v1.CreateVerb, m.Workspace, "", "", 0); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	dir, err := scanPath(req.Path, m.Root, m.ScanPrefixes(c.GetString(common.UserName)))
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	t, token, err := h.newImportToken(c, req.Workspace, importPurposeScan, externalAddress(c.Request))
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to issue the scan's token: "+err.Error()))
		return
	}
	w := scanWorkload(m, t, token, dir)
	if err = h.client.Create(ctx, w); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to start the scan: "+err.Error()))
		return
	}
	klog.Infof("scan %s of %s for workspace %s by user %s", w.Name, dir, req.Workspace, t.UserName)
	c.JSON(http.StatusAccepted, gin.H{"scanId": t.TokenID, "workloadId": w.Name})
}

// scanPath checks a directory to scan: an absolute path without . or .. parts or control characters, under root and
// within one of the prefixes the user may scan. The scan job checks it again after resolving symlinks.
func scanPath(p, root string, prefixes []string) (string, error) {
	p = strings.TrimRight(strings.TrimSpace(p), "/")
	if !path.IsAbs(p) || path.Clean(p) != p || strings.ContainsFunc(p, unicode.IsControl) {
		return "", commonerrors.NewBadRequest("The run directory must be an absolute path without . or .. parts.")
	}
	if !strings.HasPrefix(p, strings.TrimRight(root, "/")+"/") {
		return "", commonerrors.NewBadRequest(fmt.Sprintf("The run directory must be under %s.", root))
	}
	if len(prefixes) == 0 {
		return "", commonerrors.NewBadRequest(importNoPrefix)
	}
	for _, prefix := range prefixes {
		if prefix = strings.TrimRight(prefix, "/"); p == prefix || strings.HasPrefix(p, prefix+"/") {
			return p, nil
		}
	}
	return "", commonerrors.NewBadRequest(fmt.Sprintf("The run directory must be under %s.", strings.Join(prefixes, " or ")))
}

// scanWorkload builds the low-priority CPU job, named after the scan, that runs the import command with --stage on
// dir in the import workspace.
func scanWorkload(m *registry.ImportMount, t *dbclient.PostTrainingImportToken, token, dir string) *v1.Workload {
	w := &v1.Workload{}
	w.Name = t.TokenID
	w.Labels = map[string]string{
		v1.UserIdLabel:           t.UserID,
		v1.PostTrainingRoleLabel: v1.PostTrainingRoleImportScan,
	}
	if name := "import-scan-" + path.Base(dir); len(validation.IsValidLabelValue(name)) == 0 {
		w.Labels[v1.DisplayNameLabel] = name
	}
	w.Annotations = map[string]string{
		v1.UserNameAnnotation:    t.UserName,
		v1.DescriptionAnnotation: fmt.Sprintf("Spur import scan of %s for workspace %s", dir, t.Workspace),
	}
	timeout, ttl := importScanTimeout, importScanTTL
	w.Spec = v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.JobKind, Version: common.DefaultVersion},
		Workspace:        m.Workspace,
		Images:           []string{commonconfig.GetDownloadJoImage()},
		EntryPoints:      []string{base64.StdEncoding.EncodeToString([]byte(importScanEntrypoint))},
		Env: map[string]string{"SAFE_IMPORT_TOKEN": token, "PT_IMPORT_DIR": dir, "PT_IMPORT_ROOT": m.Root,
			"PYTHONUNBUFFERED": "1"},
		Resources:               []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "4Gi", EphemeralStorage: "1Gi"}},
		Priority:                common.LowPriorityInt,
		Timeout:                 &timeout,
		TTLSecondsAfterFinished: &ttl,
	}
	return w
}

// importScanStatus is GET /post-training/imports/scans/:id: running, ready with the preview and the run the
// directory is already imported as, failed with the reason, or imported as runId.
type importScanStatus struct {
	State         string         `json:"state"`
	WorkloadID    string         `json:"workloadId"`
	Message       string         `json:"message,omitempty"`
	Path          string         `json:"path,omitempty"`
	Preview       *importPreview `json:"preview,omitempty"`
	ExistingRunID string         `json:"existingRunId,omitempty"`
	RunID         string         `json:"runId,omitempty"`
	ExpiresAt     *time.Time     `json:"expiresAt,omitempty"`
}

// GetImportScan reports a scan of the caller for the dialog's polling.
func (h *Handler) GetImportScan(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	t, err := h.scanToken(c, v1.GetVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	resp, err := h.scanStatus(c.Request.Context(), t)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// scanToken reads the token of the scan in the path, for a caller who may act on the issuer's workloads.
func (h *Handler) scanToken(c *gin.Context, verb v1.RoleVerb) (*dbclient.PostTrainingImportToken, error) {
	t, err := h.db.GetPostTrainingImportToken(c.Request.Context(), c.Param("id"))
	if err == nil && t.Purpose != importPurposeScan {
		err = gorm.ErrRecordNotFound
	}
	if err != nil {
		return nil, notFoundOr(err, "import scan "+c.Param("id"))
	}
	if err = h.authorize(c, verb, t.Workspace, t.UserID, "", 0); err != nil {
		return nil, err
	}
	return t, nil
}

// scanStatus reads a scan's state from its staged summary once the job uploaded one, and from its job before.
func (h *Handler) scanStatus(ctx context.Context, t *dbclient.PostTrainingImportToken) (*importScanStatus, error) {
	resp := &importScanStatus{State: scanRunning, WorkloadID: t.TokenID}
	if !t.UsedAt.Valid {
		resp.State, resp.Message = h.scanJobState(ctx, t)
		return resp, nil
	}
	imp, err := h.db.GetPostTrainingImport(ctx, t.ImportID)
	if err != nil {
		return nil, notFoundOr(err, "the staged scan "+t.ImportID)
	}
	resp.Path = imp.SourcePath
	s := &importSummary{}
	if json.Unmarshal([]byte(imp.Summary), s) == nil {
		p := previewOf(s)
		resp.Preview = &p
	}
	switch {
	case imp.State == dbclient.PostTrainingImportCommitted:
		resp.State, resp.RunID = scanImported, imp.RunID
	case stagedExpired(imp):
		resp.State, resp.Message, resp.Preview = scanFailed, "The scan expired. Scan the directory again.", nil
	default:
		resp.State = scanReady
		expires := imp.CreatedAt.Time.Add(importStagedTTL)
		resp.ExpiresAt = &expires
		if existing, err := h.db.GetPostTrainingImportByPathKey(ctx, importPathKey(t.Workspace,
			imp.SourcePath)); err == nil && existing != nil {
			resp.ExistingRunID = existing.RunID
		}
	}
	return resp, nil
}

// stagedExpired tells whether a staged summary is too old to commit.
func stagedExpired(imp *dbclient.PostTrainingImport) bool {
	return imp.State != dbclient.PostTrainingImportStaged ||
		(imp.CreatedAt.Valid && time.Since(imp.CreatedAt.Time) > importStagedTTL)
}

// scanJobState reports a scan that has not staged a summary: running while its job runs, and failed with the
// reason once the upload was refused, the job ended or the token expired.
func (h *Handler) scanJobState(ctx context.Context, t *dbclient.PostTrainingImportToken) (string, string) {
	if t.LastError != "" {
		return scanFailed, t.LastError
	}
	w := &v1.Workload{}
	err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: t.TokenID}, w)
	switch {
	case apierrors.IsNotFound(err):
		if t.CreatedAt.Valid && time.Since(t.CreatedAt.Time) < time.Minute {
			return scanRunning, ""
		}
		return scanFailed, "The scan job is gone. Scan the directory again."
	case err != nil:
		return scanRunning, ""
	case w.IsEnd():
		return scanFailed, scanFailure(w)
	case time.Now().After(t.ExpiresAt):
		return scanFailed, "The scan did not finish within 30 minutes."
	}
	return scanRunning, ""
}

// scanFailure is why a scan job ended without a summary: the import command's termination message when it left
// one, or the job's own message.
func scanFailure(w *v1.Workload) string {
	for i := len(w.Status.Pods) - 1; i >= 0; i-- {
		for _, ct := range w.Status.Pods[i].Containers {
			if msg := strings.TrimSpace(ct.Message); msg != "" {
				return msg
			}
		}
	}
	if w.Status.Phase == v1.WorkloadSucceeded {
		return "The scan finished without a summary."
	}
	if msg := strings.TrimSpace(w.Status.Message); msg != "" {
		return "The scan failed: " + msg
	}
	return "The scan failed."
}

// CommitImportScan writes a staged scan as an imported run of the scan's workspace.
func (h *Handler) CommitImportScan(c *gin.Context) {
	reg, err, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("the Post Training registry is not loaded: "+err.Error()))
		return
	}
	t, err := h.scanToken(c, v1.CreateVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	h.commitScan(c, reg, t)
}

// commitScan writes the staged summary of a scan's token as an imported run, refusing a scan still running, expired
// or committed, and a directory the workspace already imported.
func (h *Handler) commitScan(c *gin.Context, reg *registry.Registry, t *dbclient.PostTrainingImportToken) {
	ctx := c.Request.Context()
	if !t.UsedAt.Valid {
		apiutils.AbortWithApiError(c, commonerrors.NewConflict("The scan has not finished."))
		return
	}
	imp, err := h.db.GetPostTrainingImport(ctx, t.ImportID)
	if err != nil {
		apiutils.AbortWithApiError(c, notFoundOr(err, "the staged scan "+t.ImportID))
		return
	}
	if imp.State == dbclient.PostTrainingImportCommitted {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"errorCode": "Conflict", "runId": imp.RunID,
			"errorMessage": fmt.Sprintf("This scan is already imported as %s.", imp.RunID)})
		return
	}
	if stagedExpired(imp) {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("The scan expired. Scan the directory again."))
		return
	}
	raw := []byte(imp.Summary)
	s, err := parseImportSummary(raw, reg)
	if err != nil {
		var invalid *importInvalid
		errors.As(err, &invalid)
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"errorCode": "Unprocessable",
			"errorMessage": invalid.Message, "field": invalid.Field})
		return
	}
	run, created, err := h.saveImport(c, importFramework(reg, s.Format), raw, s, t, imp.ImportID, importViaScan)
	if errors.Is(err, dbclient.ErrPostTrainingImportExists) {
		runID := ""
		if existing, _ := h.db.GetPostTrainingImportByPathKey(ctx, importPathKey(t.Workspace, s.Path)); existing != nil {
			runID = existing.RunID
		}
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"errorCode": "Conflict", "runId": runID,
			"errorMessage": fmt.Sprintf("This directory is already imported as %s.", runID)})
		return
	}
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to save the import: "+err.Error()))
		return
	}
	klog.Infof("imported %s from Spur as post training run %s for user %s", s.Path, run.RunID, t.UserName)
	c.JSON(http.StatusCreated, gin.H{"runId": run.RunID, "importId": created.ImportID,
		"displayName": run.DisplayName, "workspace": run.Workspace})
}
