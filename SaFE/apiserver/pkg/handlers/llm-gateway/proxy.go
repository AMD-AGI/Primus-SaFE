/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strconv"
	"strings"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"

	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

const llmProxyPrefix = "/api/v1/llm-proxy"

// apimKeyHeader selects the APIM subscription the upstream request is billed to.
const apimKeyHeader = "Ocp-Apim-Subscription-Key"

// upstreamUserHeader names the person APIM attributes the request to.
const upstreamUserHeader = "user"

// legacyUpstreamUserHeader is the header this proxy used to stamp the NTID in.
// LiteLLM never forwarded it, so it only ever reached LiteLLM itself; it is
// still deleted so a client cannot pass its own value through.
const legacyUpstreamUserHeader = "USER-NTID"

// extraHeadersField is the request body field LiteLLM sends to the provider as
// HTTP headers. It does not forward the client's own HTTP headers under these
// names, so this field is the only way to reach APIM through it.
const extraHeadersField = "extra_headers"

// forwardedForHeader carries the caller's address to LiteLLM, which persists it
// verbatim as LiteLLM_SpendLogs.requester_ip_address without splitting the
// chain, so it must hold exactly one address.
const forwardedForHeader = "X-Forwarded-For"

// accelBufferingHeader tells an nginx hop to stream a response rather than
// buffer it. It is per-response and overrides a location's proxy_buffering,
// which is what FlushInterval alone cannot reach: flushing here only gets the
// bytes as far as the next hop, and a buffering one accumulates them and hands
// the client silence while the model is streaming normally.
const accelBufferingHeader = "X-Accel-Buffering"

// eventStreamContentType marks the SSE responses the header above applies to.
const eventStreamContentType = "text/event-stream"

// newLLMProxy creates a reverse proxy targeting the LiteLLM endpoint.
// It strips the /api/v1/llm-proxy prefix and prepends the target's base path, so that
// /api/v1/llm-proxy/v1/chat/completions → <endpoint>/v1/chat/completions.
// For example, if endpoint is "https://host/llm-gateway", the result is
// "https://host/llm-gateway/v1/chat/completions".
func newLLMProxy(endpoint string) (*httputil.ReverseProxy, error) {
	targetURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	basePath := strings.TrimSuffix(targetURL.Path, "/")

	proxy := &httputil.ReverseProxy{
		// Required for SSE streaming — flush response bytes immediately.
		FlushInterval: -1,

		// Rewrite rather than Director: with Director, net/http appends this hop's
		// address to X-Forwarded-For, which would leave LiteLLM recording
		// "<caller>, <apiserver>" instead of the caller alone.
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = targetURL.Scheme
			pr.Out.URL.Host = targetURL.Host
			pr.Out.Host = targetURL.Host

			// Strip SaFE proxy prefix, prepend target's base path
			trimmed := strings.TrimPrefix(pr.Out.URL.Path, llmProxyPrefix)
			if !strings.HasPrefix(trimmed, "/") {
				trimmed = "/" + trimmed
			}
			pr.Out.URL.Path = basePath + trimmed

			// Rewrite strips inbound X-Forwarded-* from pr.Out, so carry over the
			// single address pinned by applyProxyClientIPHeader.
			if clientIP := pr.In.Header.Get(forwardedForHeader); clientIP != "" {
				pr.Out.Header.Set(forwardedForHeader, clientIP)
			}

			klog.Infof("LLM Proxy: %s -> %s", pr.Out.Method, pr.Out.URL.String())
		},

		// Only streaming replies get the header, so a buffering hop still gets
		// to batch the unary ones.
		ModifyResponse: func(resp *http.Response) error {
			if strings.HasPrefix(resp.Header.Get("Content-Type"), eventStreamContentType) {
				resp.Header.Set(accelBufferingHeader, "no")
			}
			return nil
		},

		// Cloned rather than built fresh: a bare &http.Transport{} keeps none
		// of DefaultTransport's bounds, so this hop would have no dial or TLS
		// handshake timeout and would hold idle connections forever -- which
		// reuses one the peer has already closed, and a POST with a body is
		// not retried.
		Transport: clonedDefaultTransport(),

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			klog.ErrorS(err, "LLM Proxy error", "url", r.URL.String())
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"LiteLLM service unavailable"}`))
		},
	}

	return proxy, nil
}

// clonedDefaultTransport keeps DefaultTransport's timeouts and connection
// bounds while skipping upstream certificate verification.
func clonedDefaultTransport() *http.Transport {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // nolint:gosec
		}
	}
	cloned := transport.Clone()
	cloned.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // nolint:gosec
	return cloned
}

// ProxyLLMRequest handles /llm-gateway/v1/* requests.
// It resolves the user's Virtual Key from the DB, replaces the Authorization
// header, and reverse-proxies the request to LiteLLM.
func (h *Handler) ProxyLLMRequest(c *gin.Context) {
	email, ntid := h.getUserIdentity(c)
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

	apimKey, err := h.crypto.Decrypt(binding.ApimKey)
	if err != nil {
		klog.ErrorS(err, "LLM Proxy: failed to decrypt APIM Key", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("internal error"))
		return
	}

	applyProxyVirtualKeyHeader(c, virtualKey)
	if err := applyUpstreamHeaders(c, apimKey, upstreamUser(binding.ApimKeyHash, ntid)); err != nil {
		klog.ErrorS(err, "LLM Proxy: failed to read request body", "email", email)
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("failed to read request body"))
		return
	}
	applyProxyClientIPHeader(c)

	defer recoverReverseProxyAbort(c)
	h.proxy.ServeHTTP(c.Writer, c.Request)
}

// upstreamUser picks who APIM attributes the request to. A shared APIM key is
// attributed to the account configured for it, whoever made the call; any
// other key to the caller's NTID.
func upstreamUser(apimKeyHash, ntid string) string {
	if apimKeyHash != "" && slices.Contains(commonconfig.GetLLMGatewaySharedApimKeyHashes(), apimKeyHash) {
		if user := commonconfig.GetLLMGatewaySharedApimUser(); user != "" {
			return user
		}
	}
	return ntid
}

// applyUpstreamHeaders writes the APIM key and the upstream user into the JSON
// body's extra_headers, which LiteLLM sends to APIM as HTTP headers.
//
// Both are replaced unconditionally, and that is the point of the function: this
// is a reverse proxy, so a client can put these names into extra_headers itself.
// Keeping the client's value whenever the gateway has none would forward it
// upstream as though the gateway had established it. A body that is not a JSON
// object carries no extra_headers LiteLLM could read, so it is forwarded as is.
func applyUpstreamHeaders(c *gin.Context, apimKey, user string) error {
	c.Request.Header.Del(legacyUpstreamUserHeader)
	if user == "" {
		// Every way the user can go unresolved converges here, so this is the one
		// place that observes the outcome itself: the request is about to reach
		// APIM with no identity to attribute it to.
		klog.Warningf("LLM Proxy: no NTID resolved, forwarding %s without %s",
			c.Request.URL.Path, upstreamUserHeader)
	}

	req := c.Request
	if req.Body == nil || req.Body == http.NoBody || !isJSONContentType(req.Header.Get("Content-Type")) {
		return nil
	}
	raw, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return err
	}
	body, err := withUpstreamHeaders(raw, map[string]string{apimKeyHeader: apimKey, upstreamUserHeader: user})
	if err != nil {
		body = raw
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}

// withUpstreamHeaders returns raw with extra_headers carrying headers, dropping
// any entry the client put there under one of those names in any case. An empty
// value removes the name without setting it. raw must be a JSON object.
func withUpstreamHeaders(raw []byte, headers map[string]string) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return nil, errBodyNotJSONObject
	}

	extra := map[string]json.RawMessage{}
	if existing, ok := body[extraHeadersField]; ok {
		// A non-object extra_headers is replaced: LiteLLM could not send it anyway.
		var parsed map[string]json.RawMessage
		if json.Unmarshal(existing, &parsed) == nil && parsed != nil {
			extra = parsed
		}
	}
	for name := range extra {
		for header := range headers {
			if strings.EqualFold(name, header) {
				delete(extra, name)
			}
		}
	}
	for header, value := range headers {
		if value == "" {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		extra[header] = encoded
	}

	if len(extra) == 0 {
		delete(body, extraHeadersField)
	} else {
		encoded, err := marshalJSON(extra)
		if err != nil {
			return nil, err
		}
		body[extraHeadersField] = encoded
	}
	return marshalJSON(body)
}

var errBodyNotJSONObject = errors.New("request body is not a JSON object")

// marshalJSON encodes v without HTML escaping, so the strings the client sent
// keep their characters rather than turning into \u escapes.
func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func isJSONContentType(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	return strings.EqualFold(strings.TrimSpace(mediaType), "application/json")
}

// recoverReverseProxyAbort absorbs the panic ReverseProxy raises when it can no
// longer copy a response body.
//
// The cause is not observable here: net/http raises the same ErrAbortHandler
// whether the client went away or the upstream read failed mid-body. Naming
// only the client would attribute a LiteLLM failure to the caller, and at V(4)
// neither cause is recorded at all -- which is why a stalled stream could only
// be diagnosed from the caller's own timeout.
func recoverReverseProxyAbort(c *gin.Context) {
	if r := recover(); r != nil {
		if r == http.ErrAbortHandler {
			klog.InfoS("LLM Proxy: response stream aborted (client disconnect or upstream failure)",
				"path", c.Request.URL.Path, "status", c.Writer.Status())
			c.Abort()
			return
		}
		panic(r)
	}
}

func applyProxyVirtualKeyHeader(c *gin.Context, virtualKey string) {
	style, _ := c.Get(llmProxyAuthHeaderStyleKey)
	mapping := proxyAuthHeaderMappingByStyle(style)
	if mapping.style == "" {
		mapping = llmProxyAuthHeaderMappings[0]
	}

	clearProxyAuthHeaders(c)
	if mapping.style == llmProxyAuthHeaderAuth {
		c.Request.Header.Set(mapping.header, "Bearer "+virtualKey)
		return
	}
	c.Request.Header.Set(mapping.header, virtualKey)
}

// applyProxyClientIPHeader stamps the caller's address for LiteLLM attribution,
// so a spend log names the real client rather than the apiserver pod, and names
// it the same way the apiserver access log already does. Collapsing the chain to
// a single hop is what keeps requester_ip_address groupable.
//
// Unlike applyUpstreamHeaders this cannot replace the value up front, because
// gin derives the address from it; deleting first would degrade the result to the
// immediate peer, which is the apiserver's own ingress.
func applyProxyClientIPHeader(c *gin.Context) {
	clientIP := c.ClientIP()
	if clientIP == "" {
		c.Request.Header.Del(forwardedForHeader)
		return
	}
	c.Request.Header.Set(forwardedForHeader, clientIP)
}

func proxyAuthHeaderMappingByStyle(style any) llmProxyAuthHeaderMapping {
	styleString, ok := style.(string)
	if !ok {
		return llmProxyAuthHeaderMapping{}
	}
	for _, mapping := range llmProxyAuthHeaderMappings {
		if mapping.style == styleString {
			return mapping
		}
	}
	return llmProxyAuthHeaderMapping{}
}

func clearProxyAuthHeaders(c *gin.Context) {
	for _, mapping := range llmProxyAuthHeaderMappings {
		c.Request.Header.Del(mapping.header)
	}
}
