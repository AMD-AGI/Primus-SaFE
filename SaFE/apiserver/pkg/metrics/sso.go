/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package metrics defines apiserver business metrics (SSO login, identity
// provider access). They are registered on the controller-runtime registry so
// they are exposed on the existing /metrics endpoint and collected via pull.
package metrics

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/oauth2"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Operations performed against the identity provider.
const (
	// OpTokenExchange is the authorization-code-for-token call made on login.
	OpTokenExchange = "token_exchange"
	// OpTokenVerify is ID token verification, which reaches the provider when
	// the signing keys are not cached.
	OpTokenVerify = "token_verify"
	// OpProviderDiscovery is the OIDC discovery call made at startup.
	OpProviderDiscovery = "provider_discovery"
)

// Stages of a login attempt, reported on safe_apiserver_sso_login_total so that
// an identity provider outage can be told apart from a database outage.
const (
	StageOK            = "ok"
	StageTokenExchange = "token_exchange"
	StageTokenVerify   = "token_verify"
	StageUserSync      = "user_sync"
	StageDBSession     = "db_session"
	StageBadRequest    = "bad_request"
)

// Error kinds reported for identity provider calls.
const (
	// IdPErrTimeout is a deadline exceeded while talking to the provider.
	IdPErrTimeout = "timeout"
	// IdPErrCanceled means the caller went away before the provider answered.
	// This is what the browser aborting a slow login looks like server side.
	IdPErrCanceled = "canceled"
	// IdPErrConnectionReset is a TCP reset from the provider or a middlebox,
	// which is how a stalled connection eventually fails.
	IdPErrConnectionReset = "connection_reset"
	// IdPErrConnectionRefused is a refused connection.
	IdPErrConnectionRefused = "connection_refused"
	// IdPErrDNS is a name resolution failure.
	IdPErrDNS = "dns"
	// IdPErrTLS is a TLS handshake or certificate failure.
	IdPErrTLS = "tls"
	// IdPErrNetwork is any other transport level failure.
	IdPErrNetwork = "network"
	// IdPErrInvalidGrant means the provider rejected the authorization code,
	// typically because it was already used or has expired. It is a client
	// problem rather than an outage.
	IdPErrInvalidGrant = "invalid_grant"
	// IdPErrOAuth is any other structured OAuth2 error response.
	IdPErrOAuth = "oauth"
	// IdPErrOther is anything not recognised above.
	IdPErrOther = "other"
)

const (
	statusSuccess = "success"
	statusError   = "error"

	resultSuccess = "success"
	resultFailure = "failure"
)

var (
	ssoIdPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "safe_apiserver_sso_idp_request_duration_seconds",
		Help: "Latency of outbound calls to the SSO identity provider.",
		// The outbound HTTP client allows 30s per attempt and the OAuth2
		// library may probe a second auth style, so a stalled call can reach
		// 60s. The buckets deliberately span that whole range.
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 45, 60},
	}, []string{"operation", "status"})

	ssoIdPErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_apiserver_sso_idp_errors_total",
		Help: "Failed outbound calls to the SSO identity provider, by error kind.",
	}, []string{"operation", "kind"})

	ssoLogins = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_apiserver_sso_login_total",
		Help: "SSO login attempts, by outcome and the stage that decided it.",
	}, []string{"result", "stage"})
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		ssoIdPRequestDuration,
		ssoIdPErrors,
		ssoLogins,
	)
}

// ObserveIdPRequest records the latency and outcome of one call to the
// identity provider.
func ObserveIdPRequest(operation string, start time.Time, err error) {
	status := statusSuccess
	if err != nil {
		status = statusError
		ssoIdPErrors.WithLabelValues(operation, ClassifyIdPError(err)).Inc()
	}
	ssoIdPRequestDuration.WithLabelValues(operation, status).Observe(time.Since(start).Seconds())
}

// ObserveLoginSuccess records a login that completed end to end.
func ObserveLoginSuccess() {
	ssoLogins.WithLabelValues(resultSuccess, StageOK).Inc()
}

// ObserveLoginFailure records a login that failed, tagged with the stage it
// failed at so that alerts can point at the responsible dependency.
func ObserveLoginFailure(stage string) {
	ssoLogins.WithLabelValues(resultFailure, stage).Inc()
}

// ClassifyIdPError maps an error from an identity provider call onto one of the
// IdPErr constants. It returns an empty string for a nil error.
func ClassifyIdPError(err error) string {
	if err == nil {
		return ""
	}

	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		if retrieveErr.ErrorCode == "invalid_grant" {
			return IdPErrInvalidGrant
		}
		return IdPErrOAuth
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return IdPErrTimeout
	case errors.Is(err, context.Canceled):
		return IdPErrCanceled
	case errors.Is(err, syscall.ECONNRESET):
		return IdPErrConnectionReset
	case errors.Is(err, syscall.ECONNREFUSED):
		return IdPErrConnectionRefused
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return IdPErrDNS
	}

	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return IdPErrTLS
	}
	var recordErr tls.RecordHeaderError
	if errors.As(err, &recordErr) {
		return IdPErrTLS
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return IdPErrTimeout
		}
		return IdPErrNetwork
	}

	return IdPErrOther
}
