/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import "testing"

// TestListNodeRequestGetWorkspaceId covers nil receiver, nil field and set value.
func TestListNodeRequestGetWorkspaceId(t *testing.T) {
	var nilReq *ListNodeRequest
	if got := nilReq.GetWorkspaceId(); got != "" {
		t.Errorf("nil receiver: expected empty, got %q", got)
	}

	req := &ListNodeRequest{}
	if got := req.GetWorkspaceId(); got != "" {
		t.Errorf("nil field: expected empty, got %q", got)
	}

	ws := "ws-1"
	req.WorkspaceId = &ws
	if got := req.GetWorkspaceId(); got != "ws-1" {
		t.Errorf("expected ws-1, got %q", got)
	}
}

// TestListNodeRequestGetClusterId covers nil receiver, nil field and set value.
func TestListNodeRequestGetClusterId(t *testing.T) {
	var nilReq *ListNodeRequest
	if got := nilReq.GetClusterId(); got != "" {
		t.Errorf("nil receiver: expected empty, got %q", got)
	}

	req := &ListNodeRequest{}
	if got := req.GetClusterId(); got != "" {
		t.Errorf("nil field: expected empty, got %q", got)
	}

	cl := "cluster-1"
	req.ClusterId = &cl
	if got := req.GetClusterId(); got != "cluster-1" {
		t.Errorf("expected cluster-1, got %q", got)
	}
}
