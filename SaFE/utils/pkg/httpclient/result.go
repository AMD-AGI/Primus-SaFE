/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package httpclient

import (
	"net/http"
	"strconv"
)

type Result struct {
	StatusCode int
	Body       []byte
	Header     http.Header
}

// IsSuccess returns whether the HTTP response indicates success.
func (r *Result) IsSuccess() bool {
	return r != nil && r.StatusCode/100 == 2
}

// String returns a string representation of the HTTP response.
func (r *Result) String() string {
	return "http code: " + strconv.Itoa(r.StatusCode) + ", body: " + string(r.Body)
}
