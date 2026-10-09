/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

// PushToken is a registry bearer token for one staging repository.
type PushToken struct {
	Value  string
	Expiry time.Time
}

type tokenClaims struct {
	Exp    int64 `json:"exp"`
	Access []struct {
		Type    string   `json:"type"`
		Name    string   `json:"name"`
		Actions []string `json:"actions"`
	} `json:"access"`
}

// IssuePushToken asks the registry's token service, with this process's credential, for a
// token that can push to repo and nothing else. The token goes into the user's container,
// so its grant is read back and refused unless it names repo alone, with no action but
// push and pull. It cannot be revoked; it expires when the registry says.
func IssuePushToken(ctx context.Context, repo name.Repository, auth authn.Authenticator, tr http.RoundTripper) (*PushToken, error) {
	challenge, err := transport.Ping(ctx, repo.Registry, tr)
	if err != nil {
		return nil, fmt.Errorf("reaching %s: %w", repo.RegistryStr(), err)
	}
	tok, err := transport.Exchange(ctx, repo.Registry, auth, tr, []string{repo.Scope(transport.PushScope)}, challenge)
	if err != nil {
		return nil, fmt.Errorf("getting a push token for %s: %w", repo, err)
	}
	value := tok.Token
	if value == "" {
		value = tok.AccessToken
	}
	claims, err := parseTokenClaims(value)
	if err != nil {
		return nil, err
	}
	if err := claims.grantsOnlyPushTo(repo.RepositoryStr()); err != nil {
		return nil, err
	}
	if claims.Exp == 0 {
		return nil, fmt.Errorf("the registry's token for %s has no expiry", repo)
	}
	return &PushToken{Value: value, Expiry: time.Unix(claims.Exp, 0)}, nil
}

// parseTokenClaims reads a JWT's claims without verifying its signature: the registry
// verifies it; this only checks what the registry granted.
func parseTokenClaims(token string) (*tokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("the registry's token is not a JWT, so what it grants cannot be checked")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil, fmt.Errorf("the registry's token cannot be decoded: %w", err)
	}
	var c tokenClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("the registry's token cannot be decoded: %w", err)
	}
	return &c, nil
}

func (c *tokenClaims) grantsOnlyPushTo(repo string) error {
	push := false
	for _, a := range c.Access {
		if a.Type != "repository" || a.Name != repo {
			return fmt.Errorf("the registry's token grants access to %s %q, not only to %s", a.Type, a.Name, repo)
		}
		for _, act := range a.Actions {
			switch act {
			case "push":
				push = true
			case "pull":
			default:
				return fmt.Errorf("the registry's token grants %q on %s", act, repo)
			}
		}
	}
	if !push {
		return fmt.Errorf("the registry does not let this platform's credential push to %s", repo)
	}
	return nil
}
