/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestGenerateEvaluationJobValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	cases := []struct {
		name string
		body string
	}{
		{
			name: "missing serviceId",
			body: `{"name":"eval","type":"evaluation","inputs":[{"name":"eval.service.type","value":"remote_api"}]}`,
		},
		{
			name: "missing serviceType",
			body: `{"name":"eval","type":"evaluation","inputs":[{"name":"eval.service.id","value":"svc-1"}]}`,
		},
		{
			name: "missing benchmarks",
			body: `{"name":"eval","type":"evaluation","inputs":[{"name":"eval.service.id","value":"svc-1"},{"name":"eval.service.type","value":"remote_api"}]}`,
		},
		{
			name: "invalid benchmarks json",
			body: `{"name":"eval","type":"evaluation","inputs":[{"name":"eval.service.id","value":"svc-1"},{"name":"eval.service.type","value":"remote_api"},{"name":"eval.benchmarks","value":"not-json"}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newOpsJobCtx(user.Name, tc.body)
			_, err := h.generateEvaluationJob(c, []byte(tc.body))
			assert.Error(t, err)
		})
	}
}

func TestGenerateEvaluationJobDatasetNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, user := newAdminHandlerWithObjects()
	mockDB := mock_client.NewMockInterface(ctrl)
	h.dbClient = mockDB
	mockDB.EXPECT().GetDataset(gomock.Any(), "ds-1").Return(nil, assert.AnError)

	body := `{"name":"eval","type":"evaluation","inputs":[` +
		`{"name":"eval.service.id","value":"svc-1"},` +
		`{"name":"eval.service.type","value":"remote_api"},` +
		`{"name":"eval.benchmarks","value":"[{\"datasetId\":\"ds-1\"}]"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	_, err := h.generateEvaluationJob(c, []byte(body))
	assert.Error(t, err)
}
