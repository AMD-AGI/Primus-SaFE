/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestNewTemplate(t *testing.T) {
	req := &templateRequest{Workspace: "ws", Name: " LoRA zh ", TrainType: "sft", Method: "lora",
		Content: json.RawMessage(`{"choices":{"lr":0.0001}}`), SourceRunID: "pt-1"}
	tpl, err := newTemplate(testContext(), req)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(tpl.TemplateID, "tpl-"))
	require.Equal(t, "LoRA zh", tpl.Name)
	require.Equal(t, "slime", tpl.Framework, "the framework defaults to slime")
	require.Equal(t, 1, tpl.Version)
	require.Equal(t, "u1", tpl.UserID)
	require.Equal(t, "pt-1", tpl.SourceRunID)

	for _, bad := range []templateRequest{
		{Name: "x", TrainType: "sft", Method: "full", Content: json.RawMessage(`{}`)},
		{Workspace: "ws", TrainType: "sft", Method: "full", Content: json.RawMessage(`{}`)},
		{Workspace: "ws", Name: "x", TrainType: "dpo", Method: "full", Content: json.RawMessage(`{}`)},
		{Workspace: "ws", Name: "x", TrainType: "sft", Method: "freeze", Content: json.RawMessage(`{}`)},
		{Workspace: "ws", Name: "x", TrainType: "sft", Method: "full", Content: json.RawMessage(`[1]`)},
		{Workspace: "ws", Name: strings.Repeat("n", 129), TrainType: "sft", Method: "full", Content: json.RawMessage(`{}`)},
	} {
		_, err = newTemplate(testContext(), &bad)
		require.Equal(t, http.StatusBadRequest, statusCode(err), "%+v", bad)
	}
}

func TestTemplateEdits(t *testing.T) {
	fields, err := templateEdits(&templateRequest{Name: "renamed", Content: json.RawMessage(`{"a":1}`)})
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"name": "renamed", "content": `{"a":1}`}, fields)
	_, err = templateEdits(&templateRequest{})
	require.Equal(t, http.StatusBadRequest, statusCode(err), "an edit must change something")
	_, err = templateEdits(&templateRequest{Content: json.RawMessage(`"text"`)})
	require.Equal(t, http.StatusBadRequest, statusCode(err))
}

func TestTemplateViewJSON(t *testing.T) {
	data, err := json.Marshal(viewOf(&dbclient.PostTrainingTemplate{TemplateID: "tpl-1", Content: `{"a":1}`}, 3))
	require.NoError(t, err)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, map[string]interface{}{"a": float64(1)}, got["content"], "the content is JSON, not a string")
	require.Equal(t, float64(3), got["succeededRuns"])
	require.Equal(t, "tpl-1", got["templateId"])
}
