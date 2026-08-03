/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package metrics

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const tokenURL = "https://example.okta.com/oauth2/default/v1/token"

// urlErr reproduces the shape of an error returned by net/http, which always
// wraps the underlying failure in a *url.Error.
func urlErr(err error) error {
	return &url.Error{Op: "Post", URL: tokenURL, Err: err}
}

func TestClassifyIdPError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil is not an error", nil, ""},
		{
			"reused authorization code",
			&oauth2.RetrieveError{ErrorCode: "invalid_grant"},
			IdPErrInvalidGrant,
		},
		{
			"other oauth error",
			&oauth2.RetrieveError{ErrorCode: "invalid_client"},
			IdPErrOAuth,
		},
		{
			"wrapped oauth error",
			fmt.Errorf("exchange: %w", &oauth2.RetrieveError{ErrorCode: "invalid_grant"}),
			IdPErrInvalidGrant,
		},
		// The browser aborts the request after its own 10s timeout, which
		// cancels the server side context mid-exchange.
		{"caller gave up", urlErr(context.Canceled), IdPErrCanceled},
		{"deadline exceeded", urlErr(context.DeadlineExceeded), IdPErrTimeout},
		// A stalled connection to the provider eventually fails with a reset.
		{
			"reset by peer",
			urlErr(&net.OpError{Op: "read", Err: os.NewSyscallError("read", syscall.ECONNRESET)}),
			IdPErrConnectionReset,
		},
		{
			"connection refused",
			urlErr(&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}),
			IdPErrConnectionRefused,
		},
		{
			"name resolution failure",
			urlErr(&net.DNSError{Err: "no such host", Name: "example.okta.com"}),
			IdPErrDNS,
		},
		{
			"certificate rejected",
			urlErr(&tls.CertificateVerificationError{}),
			IdPErrTLS,
		},
		{
			"handshake failure",
			urlErr(tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}),
			IdPErrTLS,
		},
		{
			"other network failure",
			urlErr(&net.OpError{Op: "read", Err: errors.New("broken")}),
			IdPErrNetwork,
		},
		{"plain error", errors.New("boom"), IdPErrOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifyIdPError(tt.err))
		})
	}
}

func TestClassifyIdPErrorNetworkTimeout(t *testing.T) {
	// The outbound client's own timeout surfaces as a *url.Error that reports
	// Timeout() without wrapping context.DeadlineExceeded.
	assert.Equal(t, IdPErrTimeout, ClassifyIdPError(urlErr(&net.OpError{Op: "dial", Err: &timeoutError{}})))
}

// timeoutError is a net.Error that reports itself as a timeout.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func TestObserveIdPRequest(t *testing.T) {
	const operation = "test_observe_idp"

	ObserveIdPRequest(operation, time.Now(), nil)
	assert.Equal(t, uint64(1), idpRequestCount(t, operation, statusSuccess))

	ObserveIdPRequest(operation, time.Now(), urlErr(context.Canceled))
	assert.Equal(t, uint64(1), idpRequestCount(t, operation, statusError))
	assert.Equal(t, 1.0, testutil.ToFloat64(ssoIdPErrors.WithLabelValues(operation, IdPErrCanceled)))
}

// Verification runs on every authenticated request, so an expired or forged
// token must not be attributed to the identity provider. This is the case that
// would otherwise page someone for ordinary client behaviour.
func TestObserveTokenVerifyDoesNotBlameProvider(t *testing.T) {
	before := testutil.ToFloat64(ssoTokenVerify.WithLabelValues(resultFailure))
	idpBefore := idpErrorTotal(t, OpTokenVerify)

	// The shape go-oidc returns for a token that is simply no longer valid.
	ObserveTokenVerify(errors.New("oidc: token is expired (Token Expiry: 2026-08-03 09:00:00 +0000 UTC)"))

	assert.Equal(t, before+1, testutil.ToFloat64(ssoTokenVerify.WithLabelValues(resultFailure)))
	assert.Equal(t, idpBefore, idpErrorTotal(t, OpTokenVerify), "a bad token is not a provider fault")
}

// A key-set fetch that fails on the network did leave the process, so it is a
// genuine provider signal and must still be counted.
func TestObserveTokenVerifyReportsNetworkFailure(t *testing.T) {
	before := testutil.ToFloat64(ssoIdPErrors.WithLabelValues(OpTokenVerify, IdPErrConnectionReset))

	ObserveTokenVerify(urlErr(&net.OpError{Op: "read", Err: os.NewSyscallError("read", syscall.ECONNRESET)}))

	assert.Equal(t, before+1, testutil.ToFloat64(ssoIdPErrors.WithLabelValues(OpTokenVerify, IdPErrConnectionReset)))
}

// A canceled context means the caller went away, not that the provider failed.
func TestObserveTokenVerifyIgnoresCancellation(t *testing.T) {
	before := idpErrorTotal(t, OpTokenVerify)
	ObserveTokenVerify(urlErr(context.Canceled))
	assert.Equal(t, before, idpErrorTotal(t, OpTokenVerify))
}

func TestObserveTokenVerifySuccess(t *testing.T) {
	before := testutil.ToFloat64(ssoTokenVerify.WithLabelValues(resultSuccess))
	ObserveTokenVerify(nil)
	assert.Equal(t, before+1, testutil.ToFloat64(ssoTokenVerify.WithLabelValues(resultSuccess)))
}

// Verification is excluded from the outbound latency histogram: it is offline
// on the hot path, and its sub-millisecond samples would all pile into the
// first bucket of a histogram whose lower bound is 100ms.
func TestObserveTokenVerifyStaysOutOfLatencyHistogram(t *testing.T) {
	before := testutil.CollectAndCount(ssoIdPRequestDuration, "safe_apiserver_sso_idp_request_duration_seconds")
	ObserveTokenVerify(nil)
	ObserveTokenVerify(errors.New("oidc: malformed jwt"))
	assert.Equal(t, before, testutil.CollectAndCount(ssoIdPRequestDuration, "safe_apiserver_sso_idp_request_duration_seconds"))
}

// idpErrorTotal sums every error kind recorded for one identity provider
// operation.
func idpErrorTotal(t *testing.T, operation string) float64 {
	t.Helper()
	families, err := ctrlmetrics.Registry.Gather()
	require.NoError(t, err)
	total := 0.0
	for _, family := range families {
		if family.GetName() != "safe_apiserver_sso_idp_errors_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "operation" && label.GetValue() == operation {
					total += metric.GetCounter().GetValue()
				}
			}
		}
	}
	return total
}

func TestObserveLogin(t *testing.T) {
	before := testutil.ToFloat64(ssoLogins.WithLabelValues(resultSuccess, StageOK))
	ObserveLoginSuccess()
	assert.Equal(t, before+1, testutil.ToFloat64(ssoLogins.WithLabelValues(resultSuccess, StageOK)))

	// The stage label is what lets an alert say whether the identity provider
	// or the database broke the login.
	ObserveLoginFailure(StageTokenExchange)
	assert.Equal(t, 1.0, testutil.ToFloat64(ssoLogins.WithLabelValues(resultFailure, StageTokenExchange)))
	ObserveLoginFailure(StageDBSession)
	assert.Equal(t, 1.0, testutil.ToFloat64(ssoLogins.WithLabelValues(resultFailure, StageDBSession)))
}

// idpRequestCount reports how many observations landed on one series of
// ssoIdPRequestDuration.
func idpRequestCount(t *testing.T, operation, status string) uint64 {
	t.Helper()
	observer, err := ssoIdPRequestDuration.GetMetricWithLabelValues(operation, status)
	require.NoError(t, err)
	var collected dto.Metric
	require.NoError(t, observer.(prometheus.Metric).Write(&collected))
	return collected.GetHistogram().GetSampleCount()
}
