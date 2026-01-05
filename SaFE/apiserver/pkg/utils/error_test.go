/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gotest.tools/assert"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

func TestError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		errorCode string
		httpCode  int
	}{
		{
			"fmt.error",
			fmt.Errorf("test"),
			commonerrors.InternalError,
			http.StatusInternalServerError,
		},
		{
			"commonErrors.badRequest",
			commonerrors.NewBadRequest("test"),
			commonerrors.BadRequest,
			http.StatusBadRequest,
		},
	}
	gin.SetMode(gin.ReleaseMode)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rsp := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rsp)
			AbortWithApiError(c, test.err)
			assert.Equal(t, rsp.Code, test.httpCode)

			apiErr := &PrimusApiError{}
			err := json.Unmarshal(rsp.Body.Bytes(), apiErr)
			assert.NilError(t, err)
			assert.Equal(t, apiErr.ErrorCode, test.errorCode)
		})
	}
}
