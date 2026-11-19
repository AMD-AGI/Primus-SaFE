/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
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
	importTokenTTL = 30 * time.Minute
	// importSweepAge is how long after its time a token or scan record is kept, past what any dialog polls.
	importSweepAge    = 24 * time.Hour
	importTokenPrefix = "sst_"
	importBodyLimit   = 16 << 20
	importPurposeCLI  = "cli"
	importPurposeScan = "scan"
	importViaCLI      = "cli"
	importViaScan     = "scan"
	// importTokenKey is where a request keeps the import token it was checked with.
	importTokenKey = "postTrainingImportToken"
	// tokenStates are the states GET /imports/tokens/:id reports.
	tokenWaiting = "waiting"
	tokenUsed    = "used"
	tokenExpired = "expired"
	// importSpentMessage is what the command prints for a token it cannot use.
	importSpentMessage = "Token expired or already used. Get a new command from the Import dialog in SaFE."
	importNoMount      = "Spur's /shared_nfs isn't mounted on this cluster."
	importNoPrefix     = "No directory on the mount is open to you for scanning; an administrator lists them in the " +
		"import mount's allowedPrefixes."
)

// importConfigResponse is GET /post-training/imports/config: whether the mounted-path import works in the workspace's
// cluster, and whether the caller may import.
type importConfigResponse struct {
	MountAvailable bool   `json:"mountAvailable"`
	MountRoot      string `json:"mountRoot,omitempty"`
	MountCluster   string `json:"mountCluster,omitempty"`
	MountReason    string `json:"mountReason,omitempty"`
	CanImport      bool   `json:"canImport"`
	// MountPrefixes are the directories the caller may scan.
	MountPrefixes []string `json:"mountPrefixes,omitempty"`
}

// GetImportConfig returns the import dialog's settings for a workspace.
func (h *Handler) GetImportConfig(c *gin.Context) {
	reg, err, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("the Post Training registry is not loaded: "+err.Error()))
		return
	}
	workspace := c.Query("workspace")
	ws, err := h.getWorkspace(c.Request.Context(), workspace)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	resp := importConfigResponse{CanImport: h.authorize(c, v1.CreateVerb, workspace, "", "", 0) == nil}
	if m, reason := h.importMount(c.Request.Context(), reg, ws.Cluster); m == nil {
		resp.MountReason = reason
	} else if prefixes := m.ScanPrefixes(c.GetString(common.UserName)); len(prefixes) == 0 {
		resp.MountReason = importNoPrefix
	} else if h.authorize(c, v1.CreateVerb, m.Workspace, "", "", 0) != nil {
		resp.MountReason = fmt.Sprintf("Scans run in workspace %s, where you cannot create workloads.", m.Workspace)
	} else {
		resp.MountAvailable, resp.MountRoot, resp.MountCluster, resp.MountPrefixes = true, m.Root, m.Cluster, prefixes
	}
	c.JSON(http.StatusOK, resp)
}

// importMount returns a cluster's import mount when its workspace has a read-only volume at the mount root, or why
// the mounted-path import is not available.
func (h *Handler) importMount(ctx context.Context, reg *registry.Registry, cluster string) (*registry.ImportMount,
	string) {
	m := reg.Imports.MountFor(cluster)
	if m == nil {
		return nil, importNoMount
	}
	ws := &v1.Workspace{}
	if err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: m.Workspace}, ws); err != nil {
		return nil, fmt.Sprintf("The import workspace %s is not available.", m.Workspace)
	}
	for _, vol := range ws.Spec.Volumes {
		if vol.MountPath == m.Root && vol.AccessMode == corev1.ReadOnlyMany && vol.SubPath == "" && !vol.EnableUserDir {
			return m, ""
		}
	}
	return nil, fmt.Sprintf("The import workspace %s has no read-only volume at %s.", m.Workspace, m.Root)
}

// importTokenRequest is the body of POST /post-training/imports/tokens.
type importTokenRequest struct {
	Workspace string `json:"workspace"`
}

// importTokenResponse carries a new token, which appears only in this response; the console builds the command
// that uses it, as the audit log keeps the response with only the token field redacted.
type importTokenResponse struct {
	TokenID   string    `json:"tokenId"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// IssueImportToken issues a one-time token to upload one run directory into a workspace the caller may create
// workloads in.
func (h *Handler) IssueImportToken(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	var req importTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request body: "+err.Error()))
		return
	}
	if _, err := h.getWorkspace(c.Request.Context(), req.Workspace); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if err := h.authorize(c, v1.CreateVerb, req.Workspace, "", "", 0); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	address := externalAddress(c.Request)
	if address == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("the request's host is no address to upload to"))
		return
	}
	t, token, err := h.newImportToken(c, req.Workspace, importPurposeCLI, address)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to issue an import token: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, importTokenResponse{TokenID: t.TokenID, Token: token, ExpiresAt: t.ExpiresAt})
}

// newImportToken stores a token of a purpose for the caller and returns it with its plain text, which is
// sst_<base64url of the SaFE address>.<32 random bytes>; only its SHA-256 is stored.
func (h *Handler) newImportToken(c *gin.Context, workspace, purpose, address string) (*dbclient.PostTrainingImportToken,
	string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, "", err
	}
	token := importTokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(address)) + "." +
		base64.RawURLEncoding.EncodeToString(secret)
	t := &dbclient.PostTrainingImportToken{TokenID: randomID("pti-", 8), TokenHash: tokenHash(token),
		Workspace: workspace, Purpose: purpose, UserID: c.GetString(common.UserId), UserName: c.GetString(common.UserName),
		ExpiresAt: time.Now().UTC().Add(importTokenTTL)}
	if err := h.db.CreatePostTrainingImportToken(c.Request.Context(), t); err != nil {
		return nil, "", err
	}
	if err := h.db.DeleteExpiredPostTrainingImports(c.Request.Context(),
		time.Now().UTC().Add(-importSweepAge)); err != nil {
		klog.ErrorS(err, "failed to delete the expired import tokens and scan records")
	}
	return t, token, nil
}

// tokenHash is the stored form of a token.
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// externalAddress is the SaFE address the command and the scan job post to: the configured system host when there
// is one, which a request cannot choose; without one, the address the caller reached, which is the scheme and host
// the proxy forwarded, or the request's own.
func externalAddress(r *http.Request) string {
	if host := strings.TrimSpace(commonconfig.GetSystemHost()); hostOnly.MatchString(host) {
		return "https://" + host
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); p == "http" || p == "https" {
		scheme = p
	}
	host := r.Host
	if fh := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); hostOnly.MatchString(fh) {
		host = fh
	}
	if !hostOnly.MatchString(host) {
		return ""
	}
	return scheme + "://" + host
}

// hostOnly matches a host with an optional port and nothing else, so a request header cannot add a path, a query or
// a fragment to where the import command and the scan job post.
var hostOnly = regexp.MustCompile(`^(([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+|\[[0-9A-Fa-f:.]+\])(:[0-9]{1,5})?$`)

// importTokenStatus is GET /post-training/imports/tokens/:id: the token's state and, once used, the import it made.
type importTokenStatus struct {
	State       string         `json:"state"`
	ExpiresAt   time.Time      `json:"expiresAt"`
	ImportID    string         `json:"importId,omitempty"`
	RunID       string         `json:"runId,omitempty"`
	DisplayName string         `json:"displayName,omitempty"`
	UploadedAt  *time.Time     `json:"uploadedAt,omitempty"`
	LastError   string         `json:"lastError,omitempty"`
	Preview     *importPreview `json:"preview,omitempty"`
}

// GetImportToken reports whether a token of the caller is waiting, used or expired, for the import dialog's polling.
func (h *Handler) GetImportToken(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	ctx := c.Request.Context()
	t, err := h.db.GetPostTrainingImportToken(ctx, c.Param("id"))
	if err != nil {
		apiutils.AbortWithApiError(c, notFoundOr(err, "import token "+c.Param("id")))
		return
	}
	if err := h.authorize(c, v1.GetVerb, t.Workspace, t.UserID, "", 0); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	resp := importTokenStatus{State: tokenWaiting, ExpiresAt: t.ExpiresAt, LastError: t.LastError}
	switch {
	case t.UsedAt.Valid:
		resp.State, resp.ImportID = tokenUsed, t.ImportID
		if imp, err := h.db.GetPostTrainingImport(ctx, t.ImportID); err == nil {
			resp.RunID = imp.RunID
			if imp.CreatedAt.Valid {
				resp.UploadedAt = &imp.CreatedAt.Time
			}
			s := &importSummary{}
			if json.Unmarshal([]byte(imp.Summary), s) == nil {
				p := previewOf(s)
				resp.Preview, resp.DisplayName = &p, s.Name
			}
		}
	case time.Now().After(t.ExpiresAt):
		resp.State = tokenExpired
	}
	c.JSON(http.StatusOK, resp)
}

// notFoundOr turns a missing record into a 404 and any other database error into a 500.
func notFoundOr(err error, what string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return commonerrors.NewNotFoundWithMessage(what + " not found")
	}
	return commonerrors.NewInternalError(err.Error())
}

// uploadChain returns the handlers of the upload route in their order. The gate stays first: the audit reads the
// whole body, which only the holder of a token may send and only up to the gate's limit.
func (h *Handler) uploadChain(audit gin.HandlerFunc) []gin.HandlerFunc {
	return []gin.HandlerFunc{h.uploadGate, audit, h.UploadImport}
}

// uploadGate runs before the audit of the upload route, which has no sign-in: only the holder of an unspent import
// token gets further, and the body the audit and the upload then read is bounded and already decompressed.
func (h *Handler) uploadGate(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	t := h.uploadToken(c)
	if t == nil {
		return
	}
	raw, err := readImportBody(c.Request)
	if err != nil {
		h.refuseImport(c, t, http.StatusRequestEntityTooLarge,
			gin.H{"errorCode": "RequestEntityTooLarge", "errorMessage": err.Error()})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	c.Request.ContentLength = int64(len(raw))
	c.Request.Header.Del("Content-Encoding")
}

// uploadToken returns the unspent import token an upload carries, which the gate of the route may have read
// already, and names its owner for the audit; it answers 401 and returns nil without one.
func (h *Handler) uploadToken(c *gin.Context) *dbclient.PostTrainingImportToken {
	if t, ok := c.Get(importTokenKey); ok {
		return t.(*dbclient.PostTrainingImportToken)
	}
	token := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	t, err := h.db.GetPostTrainingImportTokenByHash(c.Request.Context(), tokenHash(token))
	if !strings.HasPrefix(token, importTokenPrefix) || err != nil || t.UsedAt.Valid || time.Now().After(t.ExpiresAt) {
		// The audit only sees an upload with a token; one without is at least logged. A token that could not be
		// looked up is not one that is spent.
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			klog.ErrorS(err, "failed to look up the token of an import upload", "client", c.ClientIP())
		} else {
			klog.InfoS("refused an import upload that carries no unspent token", "client", c.ClientIP())
		}
		apiutils.AbortWithApiError(c, commonerrors.NewUnauthorized(importSpentMessage))
		return nil
	}
	c.Set(common.UserId, t.UserID)
	c.Set(common.UserName, t.UserName)
	c.Set(importTokenKey, t)
	return t
}

// refuseImport answers an upload that is refused and records the reason on its token, where the dialog reads it.
func (h *Handler) refuseImport(c *gin.Context, t *dbclient.PostTrainingImportToken, code int, body gin.H) {
	message, _ := body["errorMessage"].(string)
	if err := h.db.SetPostTrainingImportTokenError(c.Request.Context(), t.TokenID, message); err != nil {
		klog.ErrorS(err, "failed to record why an import was refused", "token", t.TokenID)
	}
	c.AbortWithStatusJSON(code, body)
}

// UploadImport takes the summary safe-post-training-import posts with an import token instead of a session: the
// token names the workspace and the user, and is spent by the import it makes. With ?stage=true a scan's token stages
// the summary for the dialog to commit. A refused upload keeps the token, with the reason, so the same command can
// be run again.
func (h *Handler) UploadImport(c *gin.Context) {
	reg, regErr, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	t := h.uploadToken(c)
	if t == nil {
		return
	}
	// Only the holder of a token is told why the registry did not load.
	if regErr != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("the Post Training registry is not loaded: "+regErr.Error()))
		return
	}
	stage := c.Query("stage") == "true"
	if stage != (t.Purpose == importPurposeScan) {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("this token cannot be used for that upload"))
		return
	}
	refuse := func(code int, body gin.H) { h.refuseImport(c, t, code, body) }
	raw, err := readImportBody(c.Request)
	if err != nil {
		refuse(http.StatusRequestEntityTooLarge, gin.H{"errorCode": "RequestEntityTooLarge", "errorMessage": err.Error()})
		return
	}
	s, err := parseImportSummary(raw, reg)
	if err != nil {
		var invalid *importInvalid
		errors.As(err, &invalid)
		refuse(http.StatusUnprocessableEntity, gin.H{"errorCode": "Unprocessable", "errorMessage": invalid.Message,
			"field": invalid.Field})
		return
	}
	if stage {
		h.stageImport(c, raw, s, t)
		return
	}
	run, imp, err := h.saveImport(c, importFramework(reg, s.Format), raw, s, t, "", importViaCLI)
	if errors.Is(err, dbclient.ErrPostTrainingImportExists) {
		existing, _ := h.db.GetPostTrainingImportByPathKey(ctx, importPathKey(t.Workspace, s.Path))
		runID := ""
		if existing != nil {
			runID = existing.RunID
		}
		refuse(http.StatusConflict, gin.H{"errorCode": "Conflict", "runId": runID,
			"errorMessage": fmt.Sprintf("This directory is already imported as %s.", runID)})
		return
	}
	if errors.Is(err, dbclient.ErrPostTrainingImportTokenSpent) {
		apiutils.AbortWithApiError(c, commonerrors.NewUnauthorized(importSpentMessage))
		return
	}
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to save the import: "+err.Error()))
		return
	}
	klog.Infof("imported %s from Spur as post training run %s for user %s", s.Path, run.RunID, t.UserName)
	c.JSON(http.StatusCreated, gin.H{"runId": run.RunID, "importId": imp.ImportID, "displayName": run.DisplayName,
		"workspace": run.Workspace})
}

// readImportBody reads an upload, gzip compressed or not, of at most 16 MB once decompressed.
func readImportBody(r *http.Request) ([]byte, error) {
	var body io.Reader = http.MaxBytesReader(nil, r.Body, importBodyLimit)
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(body)
		if err != nil {
			return nil, fmt.Errorf("the body is not gzip: %v", err)
		}
		defer gz.Close()
		body = gz
	}
	data, err := io.ReadAll(io.LimitReader(body, importBodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("the summary is larger than %d bytes or unreadable: %v", importBodyLimit, err)
	}
	if len(data) > importBodyLimit {
		return nil, fmt.Errorf("the summary is larger than %d bytes once decompressed", importBodyLimit)
	}
	return data, nil
}

// saveImport writes an imported run, its metric points and its import record, spending the token when one is given
// and replacing a staged record when stagedID is set. The run id is spur-<directory>, with a random suffix when that
// id is taken, including by a deleted run.
func (h *Handler) saveImport(c *gin.Context, fw *registry.Framework, raw []byte, s *importSummary,
	t *dbclient.PostTrainingImportToken, stagedID, via string) (*dbclient.PostTrainingRun, *dbclient.PostTrainingImport,
	error) {
	ctx := c.Request.Context()
	start, _ := importTime(s.StartTime)
	tokenID := ""
	if stagedID == "" {
		tokenID = t.TokenID
	}
	for attempt := 0; ; attempt++ {
		suffix := ""
		if attempt > 0 {
			suffix = randomID("", 2)
		} else if existing, err := h.db.GetPostTrainingRun(ctx, importRunID(s.Name, "")); err == nil && existing != nil {
			suffix = randomID("", 2)
		}
		runID := importRunID(s.Name, suffix)
		points := importPoints(s, fw, start)
		run := importRun(s, fw, t, runID, points)
		imp, err := importRecord(raw, s, t, randomID("imp-", 8), runID, via, dbclient.PostTrainingImportImported)
		if err != nil {
			return nil, nil, err
		}
		err = h.db.SavePostTrainingImport(ctx, dbclient.PostTrainingImportWrite{Import: imp, Run: run, Points: points,
			TokenID: tokenID, StagedID: stagedID})
		if err != nil && attempt == 0 && strings.Contains(err.Error(), "duplicate key value") {
			continue
		}
		return run, imp, err
	}
}

// stageImport keeps a scan's summary as a staged record for the dialog to commit, spending the scan's token.
func (h *Handler) stageImport(c *gin.Context, raw []byte, s *importSummary, t *dbclient.PostTrainingImportToken) {
	imp, err := importRecord(raw, s, t, randomID("imp-", 8), "", importViaScan, dbclient.PostTrainingImportStaged)
	if err == nil {
		err = h.db.SavePostTrainingImport(c.Request.Context(), dbclient.PostTrainingImportWrite{Import: imp,
			TokenID: t.TokenID})
	}
	if errors.Is(err, dbclient.ErrPostTrainingImportTokenSpent) {
		apiutils.AbortWithApiError(c, commonerrors.NewUnauthorized(importSpentMessage))
		return
	}
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to stage the scan: "+err.Error()))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"importId": imp.ImportID})
}

// randomID returns prefix followed by n random bytes in lowercase hex.
func randomID(prefix string, n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
