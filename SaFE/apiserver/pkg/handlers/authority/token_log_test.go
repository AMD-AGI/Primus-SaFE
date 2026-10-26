/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"bytes"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"
)

// captureKlog redirects klog output into a buffer for the duration of fn.
func captureKlog(t *testing.T, fn func()) string {
	t.Helper()
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	assert.NoError(t, fs.Set("logtostderr", "false"))
	assert.NoError(t, fs.Set("alsologtostderr", "false"))
	assert.NoError(t, fs.Set("stderrthreshold", "FATAL"))
	var buf bytes.Buffer
	klog.SetOutput(&buf)
	defer func() {
		klog.Flush()
		klog.SetOutput(os.Stderr)
		assert.NoError(t, fs.Set("logtostderr", "true"))
		assert.NoError(t, fs.Set("alsologtostderr", "false"))
		assert.NoError(t, fs.Set("stderrthreshold", "ERROR"))
	}()
	fn()
	klog.Flush()
	return buf.String()
}

func TestParseTokenDoesNotLogRawToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldInstance := defaultTokenInstance
	defaultTokenInstance = &defaultToken{}
	defer func() { defaultTokenInstance = oldInstance }()

	const secret = "SECRET-bearer-token-value-0123456789"
	tests := []struct {
		name string
		set  func(r *http.Request)
	}{
		{"bearer header", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+secret)
		}},
		{"cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: CookieToken, Value: secret})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			out := captureKlog(t, func() {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
				tt.set(c.Request)
				err = parseTokenFromRequest(c)
			})
			assert.Error(t, err)
			assert.Contains(t, out, "failed to validate user token", "failure must still be logged")
			assert.NotContains(t, out, secret)
		})
	}
}

func TestInternalAuthValidate(t *testing.T) {
	tests := []struct {
		name  string
		auth  *InternalAuth
		token string
		want  bool
	}{
		{"match", &InternalAuth{token: "s3cret"}, "s3cret", true},
		{"mismatch", &InternalAuth{token: "s3cret"}, "s3creT", false},
		{"prefix", &InternalAuth{token: "s3cret"}, "s3cre", false},
		{"longer", &InternalAuth{token: "s3cret"}, "s3cret-extra", false},
		{"empty input", &InternalAuth{token: "s3cret"}, "", false},
		{"empty stored", &InternalAuth{}, "", false},
		{"empty stored non-empty input", &InternalAuth{}, "x", false},
		{"nil receiver", nil, "x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.auth.Validate(tt.token))
		})
	}
}
