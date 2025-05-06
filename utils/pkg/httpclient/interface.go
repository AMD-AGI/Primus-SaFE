/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package httpclient

import (
	"net/http"
)

type Interface interface {
	Get(url string, headerKVs ...string) (*Result, error)
	Post(url string, body interface{}, headerKVs ...string) (*Result, error)
	Put(url string, body interface{}, headerKVs ...string) (*Result, error)
	Delete(url string, headerKVs ...string) (*Result, error)
	Do(req *http.Request) (*Result, error)
}
