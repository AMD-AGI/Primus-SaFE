/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The export project is created private when it is missing, and an existing one is never
// modified: an administrator's choice of visibility stands.
func TestEnsureProjectExists(t *testing.T) {
	for _, tc := range []struct {
		name    string
		get     int
		calls   []string
		created string
	}{
		{name: "missing", get: http.StatusNotFound, calls: []string{"GET", "POST"},
			created: `{"metadata":{"public":"false"},"project_name":"custom"}`},
		{name: "exists private", get: http.StatusOK, calls: []string{"GET"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			var body string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method)
				if r.Method == http.MethodGet {
					w.WriteHeader(tc.get)
					if tc.get == http.StatusOK {
						_, _ = w.Write([]byte(`{"name":"custom","public":false}`))
					}
					return
				}
				b, _ := io.ReadAll(r.Body)
				body = string(b)
				w.WriteHeader(http.StatusCreated)
			}))
			defer ts.Close()
			h := &ImageHandler{}
			assert.NoError(t, h.ensureProjectExists(context.Background(), hostFromServer(ts), "admin", "x", "custom"))
			assert.Equal(t, tc.calls, calls)
			if tc.created != "" {
				assert.JSONEq(t, tc.created, body)
			}
		})
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	h := &ImageHandler{}
	assert.Error(t, h.ensureProjectExists(context.Background(), hostFromServer(ts), "admin", "x", "custom"))
}
