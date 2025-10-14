/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gotest.tools/assert"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
)

func TestParseLogQuery(t *testing.T) {
	body := `{
    "since": "2006-01-02T15:04:05.000Z",
	"until": "2006-01-03T15:04:05.000Z",
    "keywords": ["key1", "key2"],
    "nodeNames": "node1,node2"}`

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads/test-workload/logs", strings.NewReader(body))
	query, err := parseLogQuery(c.Request, time.Time{}, time.Time{})
	assert.NilError(t, err)
	assert.Equal(t, query.Offset, 0)
	assert.Equal(t, query.Limit, types.DefaultQueryLimit)
	assert.Equal(t, query.SinceTime.IsZero(), false)
	assert.Equal(t, query.UntilTime.IsZero(), false)
	assert.Equal(t, query.UntilTime.Sub(query.SinceTime).Hours(), float64(24))
	assert.Equal(t, slice.EqualIgnoreOrder(query.Keywords, []string{"key1", "key2"}), true)
	assert.Equal(t, query.NodeNames, "node1,node2")
	assert.Equal(t, query.Order, dbclient.ASC)
	assert.Equal(t, query.DispatchCount, 0)
}
