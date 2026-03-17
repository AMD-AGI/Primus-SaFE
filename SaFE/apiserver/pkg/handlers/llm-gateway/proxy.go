/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"crypto/tls"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"

	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

const llmProxyPrefix = "/api/v1/llm-proxy"

// newLLMProxy creates a reverse proxy targeting the LiteLLM endpoint.
// It strips the /api/v1/llm-proxy prefix so that
// /api/v1/llm-proxy/v1/chat/completions is forwarded as /v1/chat/completions to LiteLLM.
func newLLMProxy(endpoint string) (*httputil.ReverseProxy, error) {
	targetURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Required for SSE streaming — flush response bytes immediately.
	proxy.FlushInterval = -1

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = strings.TrimPrefix(req.URL.Path, llmProxyPrefix)
		if !strings.HasPrefix(req.URL.Path, "/") {
			req.URL.Path = "/" + req.URL.Path
		}
		req.Host = targetURL.Host
		klog.V(4).Infof("LLM Proxy: %s %s -> %s", req.Method, llmProxyPrefix, req.URL.String())
	}

	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // nolint:gosec
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		klog.ErrorS(err, "LLM Proxy error", "url", r.URL.String())
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"LiteLLM service unavailable"}`))
	}

	return proxy, nil
}

// ProxyLLMRequest handles /llm-gateway/v1/* requests.
// It resolves the user's Virtual Key from the DB, replaces the Authorization
// header, and reverse-proxies the request to LiteLLM.
func (h *Handler) ProxyLLMRequest(c *gin.Context) {
	email := h.getUserEmail(c)
	if email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unable to identify user"})
		c.Abort()
		return
	}

	binding, err := h.dbClient.GetLLMBindingByEmail(c.Request.Context(), email)
	if err != nil {
		klog.ErrorS(err, "LLM Proxy: DB error", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("internal error"))
		return
	}
	if binding == nil {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "No APIM Key binding found. Please upload your APIM Key on the SaFE platform first.",
		})
		c.Abort()
		return
	}

	virtualKey, err := h.crypto.Decrypt(binding.LiteLLMVirtualKey)
	if err != nil {
		klog.ErrorS(err, "LLM Proxy: failed to decrypt VKey", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("internal error"))
		return
	}

	c.Request.Header.Set("Authorization", "Bearer "+virtualKey)

	h.proxy.ServeHTTP(c.Writer, c.Request)
}
