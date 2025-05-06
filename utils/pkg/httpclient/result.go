/*
 * Copyright © AMD. 2025-2026. All rights reserved.
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

func (r *Result) IsSuccess() bool {
	return r != nil && r.StatusCode/100 == 2
}

func (r *Result) String() string {
	return "http code: " + strconv.Itoa(r.StatusCode) + ", body: " + string(r.Body)
}
