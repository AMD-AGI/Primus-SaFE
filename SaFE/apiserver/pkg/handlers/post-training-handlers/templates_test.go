/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
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

// templateHandler returns a handler with Post Training on whose user u1 is a member of workspace ws1.
func templateHandler(t *testing.T, db dbclient.Interface) *Handler {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	member := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1", Labels: map[string]string{v1.UserIdLabel: "u1"}},
		Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.DefaultRole},
			Resources: map[string][]string{common.UserWorkspaces: {"ws1"}}}}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: common.PrimusSafeNamespace,
		Name: registry.ConfigMapName}, Data: map[string]string{registry.DataKey: testRegistry}}
	objs := append(authtest.ChartRoles(t), member, cm, &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}})
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	h := NewHandler(cli, db)
	h.access = &authority.AccessController{Client: cli}
	return h
}

// templateCall runs handle as user u1 on template tpl-1 with body and returns the status and body.
func templateCall(handle gin.HandlerFunc, body string) (int, string) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "tpl-1"}}
	c.Set(common.UserId, "u1")
	c.Set(common.UserName, "alice")
	handle(c)
	return w.Code, w.Body.String()
}

func TestAMemberReadsTheTemplatesOfTheWorkspace(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	db.EXPECT().GetPostTrainingTemplate(gomock.Any(), "tpl-1").Return(&dbclient.PostTrainingTemplate{
		TemplateID: "tpl-1", Workspace: "ws1", UserID: "u2", Version: 1, Content: "{}"}, nil)
	db.EXPECT().CountPostTrainingRunsByTemplate(gomock.Any(), []string{"tpl-1"}).Return(nil, nil)
	code, body := templateCall(templateHandler(t, db).GetTemplate, "")
	require.Equal(t, http.StatusOK, code, "a member may use the templates the list shows: %s", body)
}

func TestTheTemplateListIsBounded(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	db.EXPECT().CountPostTrainingRunsByTemplate(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	h := templateHandler(t, db)
	for query, want := range map[string]int{"limit=0&offset=-5": 100, "limit=100000": maxListLimit} {
		db.EXPECT().ListPostTrainingTemplates(gomock.Any(), dbclient.PostTrainingTemplateFilter{Workspace: "ws1",
			Limit: want}).Return(nil, int64(0), nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/?workspace=ws1&"+query, nil)
		c.Set(common.UserId, "u1")
		h.ListTemplates(c)
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", query, w.Body.String())
	}
}

func TestUpdateTemplateNeedsTheVersionItWasReadAt(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	db.EXPECT().GetPostTrainingTemplate(gomock.Any(), "tpl-1").Return(&dbclient.PostTrainingTemplate{
		TemplateID: "tpl-1", Workspace: "ws1", UserID: "u1", Version: 3, Content: "{}"}, nil).AnyTimes()
	h := templateHandler(t, db)
	code, _ := templateCall(h.UpdateTemplate, `{"name":"renamed"}`)
	require.Equal(t, http.StatusBadRequest, code, "an edit says which version it changes")

	db.EXPECT().UpdatePostTrainingTemplate(gomock.Any(), "tpl-1", 2, map[string]interface{}{"name": "renamed"}).
		Return(0, dbclient.ErrStaleVersion)
	code, _ = templateCall(h.UpdateTemplate, `{"name":"renamed","version":2}`)
	require.Equal(t, http.StatusConflict, code, "someone else saved version 3 meanwhile")

	db.EXPECT().UpdatePostTrainingTemplate(gomock.Any(), "tpl-1", 3, map[string]interface{}{"name": "renamed"}).
		Return(4, nil)
	code, body := templateCall(h.UpdateTemplate, `{"name":"renamed","version":3}`)
	require.Equal(t, http.StatusOK, code, body)
	require.JSONEq(t, `{"templateId":"tpl-1","version":4}`, body)
}
