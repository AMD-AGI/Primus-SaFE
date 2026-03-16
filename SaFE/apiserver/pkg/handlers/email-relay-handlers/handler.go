/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package emailrelayhandlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	dbClient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbModel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/channel"
)

type Handler struct {
	dbClient *dbClient.Client
}

func NewHandler() (*Handler, error) {
	client := dbClient.NewClient()
	if client == nil {
		return nil, fmt.Errorf("database client not available for email relay handler")
	}
	return &Handler{dbClient: client}, nil
}

// Stream handles the SSE long-connection for relaying pending emails.
// GET /api/v1/email-relay/stream
func (h *Handler) Stream(c *gin.Context) {
	if !h.validateInternalToken(c) {
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, _ := c.Writer.(interface{ Flush() })
	ctx := c.Request.Context()

	relay := channel.GetEmailRelayInstance()
	var sub chan *dbModel.EmailOutbox
	if relay != nil {
		sub = relay.Subscribe()
		defer relay.Unsubscribe(sub)
	}

	// Send backlog first
	pending, err := h.dbClient.ListPendingEmailOutbox(ctx, 100)
	if err != nil {
		klog.Errorf("failed to list pending outbox: %v", err)
	} else {
		for _, item := range pending {
			h.sendSSEEvent(c, flusher, item)
		}
	}

	// Heartbeat ticker
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.Info("Email relay SSE client disconnected")
			return
		case item, ok := <-sub:
			if !ok {
				return
			}
			h.sendSSEEvent(c, flusher, item)
		case <-ticker.C:
			c.SSEvent("heartbeat", "ping")
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// Ack acknowledges that an outbox entry was successfully sent.
// POST /api/v1/email-relay/:id/ack
func (h *Handler) Ack(c *gin.Context) {
	if !h.validateInternalToken(c) {
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.dbClient.AckEmailOutbox(c.Request.Context(), int32(id)); err != nil {
		klog.Errorf("failed to ack outbox id=%d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	klog.Infof("Email outbox id=%d acknowledged as sent", id)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Fail marks an outbox entry as failed.
// POST /api/v1/email-relay/:id/fail
func (h *Handler) Fail(c *gin.Context) {
	if !h.validateInternalToken(c) {
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.Error = "unknown error"
	}

	if err := h.dbClient.FailEmailOutbox(c.Request.Context(), int32(id), body.Error); err != nil {
		klog.Errorf("failed to mark outbox id=%d as failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	klog.Infof("Email outbox id=%d marked as failed: %s", id, body.Error)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// SubmitRequest is the request body for the Submit endpoint.
type SubmitRequest struct {
	Source     string   `json:"source"`
	Recipients []string `json:"recipients"`
	Subject    string   `json:"subject"`
	Content    string   `json:"content"`
}

// Submit allows external services (e.g., Lens) to push emails into the outbox.
// POST /api/v1/email-relay/submit
func (h *Handler) Submit(c *gin.Context) {
	if !h.validateInternalToken(c) {
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}
	defer c.Request.Body.Close()

	var req SubmitRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	if len(req.Recipients) == 0 || req.Subject == "" || req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recipients, subject, and content are required"})
		return
	}

	source := req.Source
	if source == "" {
		source = dbModel.EmailOutboxSourceLens
	}

	outbox := &dbModel.EmailOutbox{
		Source:      source,
		Recipients:  dbModel.StringArray(req.Recipients),
		Subject:     req.Subject,
		HTMLContent: req.Content,
		Status:      dbModel.EmailOutboxStatusPending,
	}

	if err := h.dbClient.CreateEmailOutbox(c.Request.Context(), outbox); err != nil {
		klog.Errorf("failed to submit email to outbox: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Notify SSE listeners
	relay := channel.GetEmailRelayInstance()
	if relay != nil {
		relay.Send(c.Request.Context(), nil)
	}

	klog.Infof("External email submitted to outbox (id=%d, source=%s)", outbox.ID, source)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "id": outbox.ID})
}

func (h *Handler) sendSSEEvent(c *gin.Context, flusher interface{ Flush() }, item *dbModel.EmailOutbox) {
	data, err := json.Marshal(item)
	if err != nil {
		klog.Errorf("failed to marshal outbox item: %v", err)
		return
	}
	c.SSEvent("email", string(data))
	if flusher != nil {
		flusher.Flush()
	}
}

func (h *Handler) validateInternalToken(c *gin.Context) bool {
	internalAuth := authority.InternalAuthInstance()
	if internalAuth == nil {
		c.JSON(http.StatusInternalServerError, commonerrors.NewInternalError("internal auth not initialized"))
		c.Abort()
		return false
	}
	token := c.GetHeader(authority.InternalAuthTokenHeader)
	if !internalAuth.Validate(token) {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid internal token"})
		c.Abort()
		return false
	}
	return true
}
