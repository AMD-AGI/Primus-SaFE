/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	// TokenType is the JWS "typ" header of exchanged access tokens (RFC 9068).
	TokenType = "at+jwt"
	// clockLeeway tolerates small clock differences between replicas and consumers.
	clockLeeway = 30 * time.Second
)

// Actor is the RFC 8693 "act" claim: the party acting on behalf of the subject.
type Actor struct {
	Sub string `json:"sub"`
}

// AccessClaims are the claims of an exchanged access token. Roles and
// permissions are deliberately absent: they are read from the user record on
// every request, so a revoked role takes effect immediately.
type AccessClaims struct {
	jwt.Claims
	ClientId string `json:"client_id"`
	Act      *Actor `json:"act,omitempty"`
	Scope    string `json:"scope,omitempty"`
}

// issueToken signs the claims.
func (k *KeySet) issueToken(c AccessClaims) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return k.Sign(payload)
}

// unverifiedIssuer reads the issuer of a compact JWS without verifying it. It
// is only used to decide which authentication branch a bearer token belongs to.
func unverifiedIssuer(raw string) (iss string, ok bool) {
	if strings.Count(raw, ".") != 2 {
		return "", false
	}
	tok, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.ES256, jose.RS256})
	if err != nil || len(tok.Headers) != 1 {
		return "", false
	}
	if typ, _ := tok.Headers[0].ExtraHeaders[jose.HeaderType].(string); !strings.EqualFold(typ, TokenType) {
		return "", false
	}
	var c jwt.Claims
	if err = tok.UnsafeClaimsWithoutVerification(&c); err != nil {
		return "", false
	}
	return c.Issuer, true
}

// verifyToken checks signature, type, issuer, audience and time bounds.
func (k *KeySet) verifyToken(raw, issuer, audience string, now time.Time) (*AccessClaims, error) {
	tok, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.ES256, jose.RS256})
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if len(tok.Headers) != 1 {
		return nil, errors.New("unexpected signature count")
	}
	h := tok.Headers[0]
	if typ, _ := h.ExtraHeaders[jose.HeaderType].(string); !strings.EqualFold(typ, TokenType) {
		return nil, errors.New("wrong token type")
	}
	key, found := k.verificationKey(h.KeyID)
	if !found {
		return nil, errors.New("unknown key id")
	}
	if key.Algorithm != h.Algorithm {
		return nil, errors.New("algorithm does not match the key")
	}
	var c AccessClaims
	if err = tok.Claims(key.Key, &c); err != nil {
		return nil, errors.New("bad signature")
	}
	exp := jwt.Expected{Issuer: issuer, AnyAudience: []string{audience}, Time: now}
	if err = c.Claims.ValidateWithLeeway(exp, clockLeeway); err != nil {
		return nil, err
	}
	if c.Expiry == nil || c.IssuedAt == nil || c.Subject == "" || c.ID == "" || c.ClientId == "" ||
		c.Act == nil || c.Act.Sub != c.ClientId {
		return nil, errors.New("incomplete claims")
	}
	return &c, nil
}
