/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// platformKeyResponse is the JSON body of GET /api/v1/apikeys/platform.
type platformKeyResponse struct {
	UserID string `json:"userId"`
	APIKey string `json:"apiKey"`
}

// FetchPlatformAPIKey returns userId's sealed platform API key from the identity service.
// The call uses the introspection service client and the user's credential: the Bearer (session,
// SSO token or ak- key; an ak- key needs the client to hold identity.platform_keys) when set,
// else the cookie Token. A key identity answers for another user is refused.
func FetchPlatformAPIKey(ctx context.Context, userId, sessionBearer, cookieToken, userType string) (string, error) {
	sessionBearer = strings.TrimSpace(sessionBearer)
	cookieToken = strings.TrimSpace(cookieToken)
	if sessionBearer == "" && cookieToken == "" {
		return "", commonerrors.NewUnauthorized("no session credential to fetch the platform API key")
	}
	base, err := ResolveBase()
	if err != nil {
		return "", err
	}
	clientID, secret, err := introspectionSecret()
	if err != nil {
		return "", err
	}
	endpoint, err := url.JoinPath(base, "api/v1/apikeys/platform")
	if err != nil {
		return "", &ConfigError{Msg: "cannot build the platform API key URL"}
	}
	cli, err := httpClient()
	if err != nil {
		return "", err
	}
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Internal-Token", clientID+":"+secret)
	if sessionBearer != "" {
		req.Header.Set("Authorization", "Bearer "+sessionBearer)
		if userType != "" {
			req.Header.Set("UserType", userType)
		}
	} else {
		cookie := "Token=" + cookieToken
		if userType != "" {
			cookie += "; userType=" + userType
		}
		req.Header.Set("Cookie", cookie)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", mapCall("fetching the platform API key", resp.StatusCode, nil)
	}
	var out platformKeyResponse
	if err = json.Unmarshal(body, &out); err != nil {
		return "", badGateway("identity returned an invalid platform API key body")
	}
	if strings.TrimSpace(out.APIKey) == "" {
		return "", badGateway("identity returned an empty platform API key")
	}
	if out.UserID != userId {
		return "", badGateway("identity returned the platform API key of another user")
	}
	return out.APIKey, nil
}
