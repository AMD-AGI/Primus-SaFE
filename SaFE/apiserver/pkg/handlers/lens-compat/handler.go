/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package lenscompat exposes the legacy `/lens/v1/*` URL prefix that the SaFE
// web frontend still uses for gpu-aggregation, training, workload and log
// endpoints.
//
// Historically those endpoints were served by a standalone primus-lens-api
// Deployment. That service was removed and its routes now live inside
// robust-analyzer on each data cluster (under the `/api/v1/...` prefix).
// Instead of hard-coding a single robust-analyzer URL in the apiserver
// ConfigMap's `proxy.services` list (which does not support multi-cluster
// routing and bakes environment-specific endpoints into the chart values),
// this handler dynamically picks the target robust-analyzer based on the
// `?cluster=<name>` query parameter and the Cluster CR annotation
// `primus-safe.amd.com/robust-api-endpoint` that was populated by the
// SaFE addon controller when primus-robust was installed.
package lenscompat

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

// Prefix is the URL prefix served by this handler. Keep the "v1" suffix so
// future Lens URL versions can be routed separately if/when they appear.
const (
	Prefix              = "/lens/v1"
	robustRoutingPrefix = "/api/v1"
	clusterQueryParam   = "cluster"
)

// Handler proxies Lens-compat requests to the data-plane robust-analyzer
// that is registered in the shared robustclient for the cluster selected by
// the incoming ?cluster=<name> query parameter.
//
// The handler is stateless with respect to the request but maintains a
// cache of reverse-proxy instances keyed by resolved target URL so we don't
// re-parse and reallocate the proxy on every call.
type Handler struct {
	rc *robustclient.Client

	mu      sync.RWMutex
	proxies map[string]*httputil.ReverseProxy
}

// NewHandler constructs a lens-compat handler bound to the given robust
// client. The client should be populated by the robustclient.Discovery
// that watches Cluster CRs; an empty client produces 503 responses until
// some cluster is discovered.
func NewHandler(rc *robustclient.Client) *Handler {
	return &Handler{
		rc:      rc,
		proxies: make(map[string]*httputil.ReverseProxy),
	}
}

// Register wires the `/lens/v1/*path` catch-all onto the given gin engine
// and applies the standard authentication middleware so unauthenticated
// callers are rejected before we resolve a backend — same contract as the
// ConfigMap-driven proxy_handler.
func (h *Handler) Register(engine *gin.Engine) {
	if h.rc == nil {
		klog.Info("[lens-compat] robust client is nil, skipping /lens/v1 routes")
		return
	}
	group := engine.Group(Prefix)
	group.Use(func(c *gin.Context) {
		if err := authority.ParseToken(c); err != nil {
			apiutils.AbortWithApiError(c, err)
			return
		}
		c.Next()
	})
	group.Any("/*path", h.proxy)
	klog.Infof("Registered dynamic proxy route: %s/* -> robustclient.ForCluster(?%s)",
		Prefix, clusterQueryParam)
}

// proxy is the single request handler for Lens-compat traffic.
func (h *Handler) proxy(c *gin.Context) {
	cluster := strings.TrimSpace(c.Query(clusterQueryParam))
	if cluster == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest(
			fmt.Sprintf("%s query parameter is required for lens-compat requests", clusterQueryParam)))
		return
	}

	cc := h.rc.ForCluster(cluster)
	if cc == nil {
		// The cluster exists in K8s but robustclient discovery either hasn't
		// seen it yet or its Cluster CR is missing the robust-api endpoint
		// annotation (i.e. the primus-robust addon was never installed on
		// this cluster). Surface that as a distinct error code so the
		// frontend can render "Robust not installed on this cluster"
		// instead of a generic red-toast error.
		apiutils.AbortWithApiError(c, commonerrors.NewRobustAddonNotInstalled(cluster))
		return
	}

	baseURL := cc.BaseURL()
	proxy, err := h.getOrCreateProxy(baseURL, cluster)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError(
			fmt.Sprintf("lens-compat parse target URL %q: %v", baseURL, err)))
		return
	}

	// Forward the authenticated user identity so the data-plane logs who
	// actually made the call (audit parity with ConfigMap proxy_handler).
	if userID, ok := c.Get(common.UserId); ok {
		if s, ok := userID.(string); ok {
			c.Request.Header.Set(common.UserId, s)
		}
	}
	if userName, ok := c.Get(common.UserName); ok {
		if s, ok := userName.(string); ok {
			c.Request.Header.Set(common.UserName, s)
		}
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

// getOrCreateProxy returns a reverse proxy for the given robust-analyzer
// base URL, creating one lazily on first use.
func (h *Handler) getOrCreateProxy(baseURL, cluster string) (*httputil.ReverseProxy, error) {
	h.mu.RLock()
	if p, ok := h.proxies[baseURL]; ok {
		h.mu.RUnlock()
		return p, nil
	}
	h.mu.RUnlock()

	target, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		// Rewrite  /lens/v1/<rest>  →  /api/v1/<rest>
		// We deliberately recompute from the request URL each call because
		// Gin may have already populated req.URL.Path from the route match.
		rest := strings.TrimPrefix(req.URL.Path, Prefix)
		if !strings.HasPrefix(rest, "/") {
			rest = "/" + rest
		}
		req.URL.Path = singleJoiningSlash(robustRoutingPrefix, rest)

		// Let the stdlib director fill Scheme/Host from the target URL.
		originalDirector(req)

		klog.V(4).Infof("[lens-compat] %s %s cluster=%s -> %s",
			req.Method, Prefix+rest, cluster, req.URL.String())
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		klog.ErrorS(err, "[lens-compat] upstream error",
			"cluster", cluster, "target", baseURL, "path", r.URL.Path)
		http.Error(w, fmt.Sprintf("lens-compat upstream %q: %v", baseURL, err), http.StatusBadGateway)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	// Double-check in case a concurrent caller beat us to it.
	if p, ok := h.proxies[baseURL]; ok {
		return p, nil
	}
	h.proxies[baseURL] = proxy
	return proxy, nil
}

// singleJoiningSlash mirrors the stdlib helper in net/http/httputil to
// avoid double slashes when concatenating a prefix and a sub-path.
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}
