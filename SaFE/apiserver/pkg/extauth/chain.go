/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package extauth is the pluggable authentication chain of the apiserver, in the
// style of Kubernetes: authenticators are tried in order, the first that accepts
// the credential decides who the caller is, and authorization stays in SaFE.
//
// Three kinds of authenticator exist: OIDC/JWT validation against a configured
// issuer, a webhook TokenReview (authentication.k8s.io/v1) with caching, and the
// built-in mode (the apiserver's own accounts, sessions and API keys), which the
// chain only marks and the caller runs.
package extauth

import (
	"context"
	"errors"
	"time"
)

// Identity is an authenticated caller.
type Identity struct {
	// ID is the SaFE user id. The authenticator decides it; SaFE does not derive it.
	ID    string
	Name  string
	Email string
	// Groups are mapped to roles on every request; they are never stored.
	Groups []string
	// Actor is set when the credential is delegated: the party acting for ID
	// (the "act" claim, RFC 8693). A delegated identity is subject to the
	// capability exclusions in Forbidden.
	Actor string
	// Scopes are the scopes of a delegated credential.
	Scopes []string
	// Jti identifies the credential, for the audit log.
	Jti string
	// Restricted asks SaFE to treat the user as frozen, in addition to the user record.
	Restricted bool
	// Expiry is zero when the authenticator gave none.
	Expiry time.Time
}

// Delegated reports whether the identity acts through a delegated credential.
func (i *Identity) Delegated() bool { return i != nil && i.Actor != "" }

// Credentials are what a request presents.
type Credentials struct {
	Bearer   string
	Cookie   string
	UserType string
	ClientIP string
}

// Empty reports whether the request presents nothing to authenticate.
func (c Credentials) Empty() bool { return c.Bearer == "" && c.Cookie == "" }

var (
	// ErrNotApplicable: this authenticator does not recognise the credential; try the next.
	ErrNotApplicable = errors.New("credential not recognised by this authenticator")
	// ErrUnauthenticated: the credential is recognised and rejected.
	ErrUnauthenticated = errors.New("credential rejected")
	// ErrBuiltin: the chain reached the built-in mode; the caller runs it.
	ErrBuiltin = errors.New("built-in authentication")
)

// UnavailableError means an authenticator could not decide, for example the
// webhook is unreachable. It is never a pass and never cached as a rejection.
type UnavailableError struct {
	Authenticator string
	Err           error
}

func (e *UnavailableError) Error() string {
	return "authenticator " + e.Authenticator + " unavailable: " + e.Err.Error()
}
func (e *UnavailableError) Unwrap() error { return e.Err }

// Authenticator authenticates one kind of credential.
type Authenticator interface {
	Name() string
	Authenticate(ctx context.Context, cred Credentials) (*Identity, error)
}

// Chain tries authenticators in order.
type Chain struct {
	list []Authenticator
}

// NewChain returns a chain of the authenticators.
func NewChain(list ...Authenticator) *Chain { return &Chain{list: list} }

// Len returns the number of authenticators.
func (c *Chain) Len() int { return len(c.list) }

// Authenticate returns the identity from the first authenticator that accepts the
// credential. ErrBuiltin means the built-in mode is next (and is returned even
// after an unavailable authenticator, so a local administrator survives an outage). When none accepts, the
// error is an *UnavailableError if any authenticator could not decide (so the
// caller answers 503, not 401), and ErrUnauthenticated otherwise.
func (c *Chain) Authenticate(ctx context.Context, cred Credentials) (*Identity, error) {
	var unavailable *UnavailableError
	for _, a := range c.list {
		id, err := a.Authenticate(ctx, cred)
		switch {
		case err == nil:
			if id == nil || id.ID == "" {
				continue
			}
			return id, nil
		case errors.Is(err, ErrBuiltin):
			// Even when an earlier authenticator was unavailable: the built-in
			// credentials (a local administrator) must keep working through an outage.
			return nil, ErrBuiltin
		case errors.As(err, &unavailable):
		}
	}
	if unavailable != nil {
		return nil, unavailable
	}
	return nil, ErrUnauthenticated
}

// Builtin marks the place of the built-in mode in a chain.
type Builtin struct{}

// Name implements Authenticator.
func (Builtin) Name() string { return "builtin" }

// Authenticate implements Authenticator.
func (Builtin) Authenticate(context.Context, Credentials) (*Identity, error) { return nil, ErrBuiltin }
