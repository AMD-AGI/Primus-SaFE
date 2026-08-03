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
