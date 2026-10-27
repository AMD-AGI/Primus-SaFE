/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package identityexchange lets a registered service client act as a platform
// user, with that user's own permissions, using standard protocols:
//
//   - OAuth 2.0 Token Exchange (RFC 8693) at POST /oauth/token,
//   - OIDC discovery at GET /.well-known/openid-configuration,
//   - a JWKS document at GET /.well-known/jwks.json,
//   - short-lived JWT access tokens (RFC 9068 profile, typ "at+jwt").
//
// The package depends only on the narrow Store and UserStore interfaces and on
// a key store, so the authentication logic can later move out of the apiserver
// without changing the protocol surface. Authorization stays with the caller:
// an exchanged token only proves who the user is and which service client is
// acting; roles are never carried in the token and are read on every request.
//
// A service client can act only for users who have personally confirmed an
// identity binding to it, and an exchanged token can never be used for a fixed
// set of capabilities (see Forbidden).
package identityexchange
