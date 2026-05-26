/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const gitHubAPIBaseURL = "https://api.github.com"

type gitHubTokenProvider interface {
	Token(ctx context.Context, credential *GitHubCredential) (string, error)
}

type GitHubTokenSource struct {
	baseURL    string
	httpClient *http.Client
	now        func() time.Time
}

func NewGitHubTokenSource() *GitHubTokenSource {
	return &GitHubTokenSource{
		baseURL:    gitHubAPIBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		now:        time.Now,
	}
}

func (s *GitHubTokenSource) Token(ctx context.Context, credential *GitHubCredential) (string, error) {
	if credential == nil {
		return "", fmt.Errorf("github credential is nil")
	}

	switch credential.Type {
	case gitHubAuthTypePAT:
		if strings.TrimSpace(credential.Token) == "" {
			return "", fmt.Errorf("github PAT credential is empty")
		}
		return strings.TrimSpace(credential.Token), nil
	case gitHubAuthTypeApp:
		return s.createInstallationToken(ctx, credential)
	default:
		return "", fmt.Errorf("unsupported github credential type %q", credential.Type)
	}
}

func (s *GitHubTokenSource) createInstallationToken(ctx context.Context, credential *GitHubCredential) (string, error) {
	if strings.TrimSpace(credential.AppID) == "" ||
		strings.TrimSpace(credential.InstallationID) == "" ||
		strings.TrimSpace(credential.PrivateKey) == "" {
		return "", fmt.Errorf("github app credential is incomplete")
	}

	jwt, err := s.createAppJWT(credential)
	if err != nil {
		return "", err
	}

	url := strings.TrimRight(s.baseURL, "/") + "/app/installations/" + credential.InstallationID + "/access_tokens"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(nil))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github app installation token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		partialBody := strings.TrimSpace(string(body))
		if partialBody != "" {
			return "", fmt.Errorf("github app installation token response read: %w: %s", err, partialBody)
		}
		return "", fmt.Errorf("github app installation token response read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("github app installation token: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("github app installation token response: %w", err)
	}
	if strings.TrimSpace(result.Token) == "" {
		return "", fmt.Errorf("github app installation token response has no token")
	}
	return strings.TrimSpace(result.Token), nil
}

func (s *GitHubTokenSource) createAppJWT(credential *GitHubCredential) (string, error) {
	key, err := parseRSAPrivateKey(credential.PrivateKey)
	if err != nil {
		return "", err
	}

	now := s.now()
	claims := map[string]interface{}{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": strings.TrimSpace(credential.AppID),
	}
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := encodedHeader + "." + encodedClaims

	hashed := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("sign github app jwt: %w", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func parseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
	pemText := strings.TrimSpace(raw)
	if !strings.Contains(pemText, "\n") {
		pemText = strings.ReplaceAll(pemText, `\n`, "\n")
	}

	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, fmt.Errorf("decode github app private key PEM")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse github app private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("github app private key is not RSA")
	}
	return key, nil
}
