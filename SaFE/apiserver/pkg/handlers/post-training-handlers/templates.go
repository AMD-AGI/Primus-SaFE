/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	maxTemplateNameLen      = 128
	maxTemplateContentBytes = 64 << 10
)

// templateRequest creates a template, or edits one when only some fields are set.
type templateRequest struct {
	Workspace   string          `json:"workspace"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	TrainType   string          `json:"trainType"`
	Method      string          `json:"method"`
	Framework   string          `json:"framework"`
	Spec        string          `json:"spec"`
	Content     json.RawMessage `json:"content"`
	SourceRunID string          `json:"sourceRunId"`
}

// templateView is a template with its content as JSON and how many of its runs succeeded.
type templateView struct {
	*dbclient.PostTrainingTemplate
	Content       json.RawMessage `json:"content"`
	SucceededRuns int             `json:"succeededRuns"`
}

// templateList is the body of the template list.
type templateList struct {
	Total int64          `json:"total"`
	Items []templateView `json:"items"`
}

// checkTemplateContent requires the template content to be a JSON object of at most 64 KB.
func checkTemplateContent(content json.RawMessage) error {
	var obj map[string]interface{}
	if len(content) == 0 || len(content) > maxTemplateContentBytes || json.Unmarshal(content, &obj) != nil {
		return commonerrors.NewBadRequest(fmt.Sprintf("content must be a JSON object of at most %d bytes",
			maxTemplateContentBytes))
	}
	return nil
}

// checkTemplateName requires a name of 1 to 128 characters.
func checkTemplateName(name string) error {
	if name = strings.TrimSpace(name); name == "" || len(name) > maxTemplateNameLen {
		return commonerrors.NewBadRequest(fmt.Sprintf("name must have 1 to %d characters", maxTemplateNameLen))
	}
	return nil
}

// newTemplate checks a create request and returns the template row it makes.
func newTemplate(c *gin.Context, req *templateRequest) (*dbclient.PostTrainingTemplate, error) {
	if req.Workspace == "" {
		return nil, commonerrors.NewBadRequest("workspace is required")
	}
	if err := checkTemplateName(req.Name); err != nil {
		return nil, err
	}
	if req.TrainType != registry.TrainTypeSFT && req.TrainType != registry.TrainTypeRL {
		return nil, commonerrors.NewBadRequest("trainType must be sft or rl")
	}
	if req.Method != registry.MethodFull && req.Method != registry.MethodLora {
		return nil, commonerrors.NewBadRequest("method must be full or lora")
	}
	if err := checkTemplateContent(req.Content); err != nil {
		return nil, err
	}
	framework := req.Framework
	if framework == "" {
		framework = "slime"
	}
	return &dbclient.PostTrainingTemplate{TemplateID: commonutils.GenerateName("tpl"), Workspace: req.Workspace,
		Name: strings.TrimSpace(req.Name), Description: req.Description, TrainType: req.TrainType, Method: req.Method,
		Framework: framework, Spec: req.Spec, Version: 1, Content: string(req.Content), SourceRunID: req.SourceRunID,
		UserID: c.GetString(common.UserId), UserName: c.GetString(common.UserName)}, nil
}

// templateEdits returns the columns an edit request changes: name, description and content.
func templateEdits(req *templateRequest) (map[string]interface{}, error) {
	fields := map[string]interface{}{}
	if req.Name != "" {
		if err := checkTemplateName(req.Name); err != nil {
			return nil, err
		}
		fields["name"] = strings.TrimSpace(req.Name)
	}
	if req.Description != "" {
		fields["description"] = req.Description
	}
	if len(req.Content) > 0 {
		if err := checkTemplateContent(req.Content); err != nil {
			return nil, err
		}
		fields["content"] = string(req.Content)
	}
	if len(fields) == 0 {
		return nil, commonerrors.NewBadRequest("nothing to change")
	}
	return fields, nil
}

// viewOf returns the API view of a template.
func viewOf(t *dbclient.PostTrainingTemplate, succeeded int) templateView {
	return templateView{PostTrainingTemplate: t, Content: rawJSON(t.Content), SucceededRuns: succeeded}
}

// ListTemplates returns a workspace's templates, optionally of one train type or matching a name.
func (h *Handler) ListTemplates(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	workspace := c.Query("workspace")
	if workspace == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("workspace is required"))
		return
	}
	if err := h.authorize(c, v1.ListVerb, workspace, "", "", 0); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	ctx := c.Request.Context()
	items, total, err := h.db.ListPostTrainingTemplates(ctx, dbclient.PostTrainingTemplateFilter{Workspace: workspace,
		TrainType: c.Query("trainType"), Search: c.Query("search"), Limit: limit, Offset: offset})
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to list templates: "+err.Error()))
		return
	}
	ids := make([]string, 0, len(items))
	for _, t := range items {
		ids = append(ids, t.TemplateID)
	}
	counts, err := h.db.CountPostTrainingRunsByTemplate(ctx, ids)
	if err != nil {
		klog.ErrorS(err, "failed to count the successful runs of templates")
	}
	out := templateList{Total: total, Items: make([]templateView, 0, len(items))}
	for _, t := range items {
		out.Items = append(out.Items, viewOf(t, counts[t.TemplateID]))
	}
	c.JSON(http.StatusOK, out)
}

// GetTemplate returns one template.
func (h *Handler) GetTemplate(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	t, err := h.getTemplate(c, v1.GetVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	counts, _ := h.db.CountPostTrainingRunsByTemplate(c.Request.Context(), []string{t.TemplateID})
	c.JSON(http.StatusOK, viewOf(t, counts[t.TemplateID]))
}

// CreateTemplate saves a recipe template in a workspace.
func (h *Handler) CreateTemplate(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	var req templateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request body: "+err.Error()))
		return
	}
	t, err := newTemplate(c, &req)
	if err == nil {
		err = h.authorize(c, v1.CreateVerb, req.Workspace, "", "", 0)
	}
	if err == nil {
		err = h.db.CreatePostTrainingTemplate(c.Request.Context(), t)
	}
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	klog.Infof("create post training template %s by user %s", t.TemplateID, c.GetString(common.UserName))
	c.JSON(http.StatusOK, viewOf(t, 0))
}

// UpdateTemplate edits a template's name, description or content and increments its version.
func (h *Handler) UpdateTemplate(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	var req templateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request body: "+err.Error()))
		return
	}
	t, err := h.getTemplate(c, v1.UpdateVerb)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	fields, err := templateEdits(&req)
	if err == nil {
		err = h.db.UpdatePostTrainingTemplate(c.Request.Context(), t.TemplateID, fields)
	}
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	klog.Infof("update post training template %s by user %s", t.TemplateID, c.GetString(common.UserName))
	c.JSON(http.StatusOK, gin.H{"templateId": t.TemplateID, "version": t.Version + 1})
}

// DeleteTemplate soft-deletes a template; runs keep the template id and version they recorded.
func (h *Handler) DeleteTemplate(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	t, err := h.getTemplate(c, v1.DeleteVerb)
	if err == nil {
		err = h.db.SetPostTrainingTemplateDeleted(c.Request.Context(), t.TemplateID)
	}
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	klog.Infof("delete post training template %s by user %s", t.TemplateID, c.GetString(common.UserName))
	c.JSON(http.StatusOK, gin.H{"templateId": t.TemplateID})
}

// getTemplate loads the template of the :id parameter and checks the caller may act on it with verb; changes are
// checked against its owner.
func (h *Handler) getTemplate(c *gin.Context, verb v1.RoleVerb) (*dbclient.PostTrainingTemplate, error) {
	t, err := h.db.GetPostTrainingTemplate(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("template %s not found", c.Param("id")))
		}
		return nil, commonerrors.NewInternalError("failed to read the template: " + err.Error())
	}
	owner := ""
	if verb == v1.UpdateVerb || verb == v1.DeleteVerb {
		owner = t.UserID
	}
	if err = h.authorize(c, verb, t.Workspace, owner, t.TemplateID, 0); err != nil {
		return nil, err
	}
	return t, nil
}
