/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	// ApiKeyPrefix is the prefix for all API keys
	ApiKeyPrefix = "ak-"
	// ApiKeyTokenLength is the length of the random token in bytes (will be base64 encoded)
	ApiKeyTokenLength = 32
	// MaxTTLDays is the maximum allowed TTL for API keys
	MaxTTLDays = 366
	// UserTypeApiKey is the user type for API key authentication
	UserTypeApiKey = "apikey"
)

var (
	apiKeyTokenOnce     sync.Once
	apiKeyTokenInstance *ApiKeyToken
)

// ApiKeyToken implements ApiKeyInterface for API Key authentication
type ApiKeyToken struct {
	dbClient dbclient.Interface
}

// Compile-time check to ensure ApiKeyToken implements ApiKeyInterface
var _ ApiKeyInterface = (*ApiKeyToken)(nil)

// NewApiKeyToken creates and returns a singleton instance of ApiKeyToken
func NewApiKeyToken(dbClient dbclient.Interface) *ApiKeyToken {
	apiKeyTokenOnce.Do(func() {
		apiKeyTokenInstance = &ApiKeyToken{
			dbClient: dbClient,
		}
	})
	return apiKeyTokenInstance
}

// ApiKeyTokenInstance returns the singleton instance of ApiKeyToken
func ApiKeyTokenInstance() *ApiKeyToken {
	return apiKeyTokenInstance
}

// GenerateApiKey generates a new unique API key
func GenerateApiKey() (string, error) {
	bytes := make([]byte, ApiKeyTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	// Use URL-safe base64 encoding without padding
	encoded := base64.RawURLEncoding.EncodeToString(bytes)
	return ApiKeyPrefix + encoded, nil
}

// GetApiKeySecret returns the crypto secret for API key hashing
// Returns the secret bytes or nil if not configured
func GetApiKeySecret() []byte {
	secret := commonconfig.GetCryptoKey()
	if secret == "" {
		return nil
	}
	return []byte(secret)
}

// HashApiKey computes HMAC-SHA256 hash of an API key for secure storage
// The hash is stored in database instead of the plaintext key
// If secret is nil, it falls back to simple SHA-256 hash
func HashApiKey(apiKey string, secret []byte) string {
	if len(secret) == 0 {
		// Fallback to SHA-256 if no secret configured
		hash := sha256.Sum256([]byte(apiKey))
		return hex.EncodeToString(hash[:])
	}
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(apiKey))
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateKeyHint creates a partial key hint for user display
// Format: "XX-YYYY" where XX is first 2 chars and YYYY is last 4 chars after prefix
// Display format will be: "ak-XX****YYYY"
func GenerateKeyHint(apiKey string) string {
	// Remove prefix to get the key body
	keyBody := strings.TrimPrefix(apiKey, ApiKeyPrefix)
	if len(keyBody) < 6 {
		return keyBody // Too short, return as-is
	}
	// Format: first 2 chars + "-" + last 4 chars
	return keyBody[:2] + "-" + keyBody[len(keyBody)-4:]
}

// FormatKeyHint formats the stored hint for display
// Input: "XX-YYYY", Output: "ak-XX****YYYY"
func FormatKeyHint(hint string) string {
	if hint == "" {
		return ""
	}
	parts := strings.Split(hint, "-")
	if len(parts) != 2 {
		return ApiKeyPrefix + hint
	}
	return ApiKeyPrefix + parts[0] + "****" + parts[1]
}

// ValidateApiKey validates an API key and returns user information
// It checks if the key exists, is not deleted, not expired, and the IP is in whitelist
func (a *ApiKeyToken) ValidateApiKey(ctx context.Context, apiKey string, clientIP string) (*UserInfo, error) {
	if a.dbClient == nil {
		return nil, commonerrors.NewInternalError("database client not initialized")
	}

	// Hash the API key before querying (database stores hashed values)
	hashedKey := HashApiKey(apiKey, GetApiKeySecret())

	// Retrieve the API key from database by its hash
	record, err := a.dbClient.GetApiKeyByKey(ctx, hashedKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, commonerrors.NewUnauthorized("invalid API key")
		}
		klog.ErrorS(err, "failed to get API key from database", "apiKey", maskApiKey(apiKey))
		return nil, commonerrors.NewUnauthorized("invalid API key")
	}

	// Check if deleted
	if record.Deleted {
		return nil, commonerrors.NewUnauthorized("Unavailable")
	}

	// Check if expired
	if record.ExpirationTime.Valid && time.Now().UTC().After(record.ExpirationTime.Time) {
		return nil, commonerrors.NewUnauthorized("API key expired")
	}

	// Check IP whitelist
	if err := a.checkIPWhitelist(record.Whitelist, clientIP); err != nil {
		return nil, err
	}

	return &UserInfo{
		Id:   record.UserId,
		Name: record.UserName,
		Exp:  record.ExpirationTime.Time.Unix(),
	}, nil
}

// checkIPWhitelist checks if the client IP is allowed by the whitelist
func (a *ApiKeyToken) checkIPWhitelist(whitelistJSON string, clientIP string) error {
	// Empty whitelist means no restriction
	if whitelistJSON == "" || whitelistJSON == "null" || whitelistJSON == "[]" {
		return nil
	}

	var whitelist []string
	if err := json.Unmarshal([]byte(whitelistJSON), &whitelist); err != nil {
		klog.ErrorS(err, "failed to parse whitelist JSON", "whitelist", whitelistJSON)
		return commonerrors.NewInternalError("invalid whitelist configuration")
	}

	if len(whitelist) == 0 {
		return nil
	}

	// Parse client IP
	clientIPAddr := net.ParseIP(clientIP)
	if clientIPAddr == nil {
		// Try to extract IP from host:port format
		host, _, err := net.SplitHostPort(clientIP)
		if err == nil {
			clientIPAddr = net.ParseIP(host)
		}
	}
	if clientIPAddr == nil {
		klog.Warningf("failed to parse client IP: %s", clientIP)
		return commonerrors.NewForbidden("IP not allowed")
	}

	for _, entry := range whitelist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Check if it's a CIDR range
		if strings.Contains(entry, "/") {
			_, network, err := net.ParseCIDR(entry)
			if err != nil {
				klog.Warningf("invalid CIDR in whitelist: %s", entry)
				continue
			}
			if network.Contains(clientIPAddr) {
				return nil
			}
		} else {
			// Plain IP address
			if net.ParseIP(entry) != nil && net.ParseIP(entry).Equal(clientIPAddr) {
				return nil
			}
		}
	}

	return commonerrors.NewForbidden("IP not allowed")
}

// ValidateAndDeduplicateWhitelist validates the whitelist entries are valid IPs or CIDRs
// and removes duplicates, returning the deduplicated list
func ValidateAndDeduplicateWhitelist(whitelist []string) ([]string, error) {
	seen := make(map[string]bool)
	result := make([]string, 0, len(whitelist))

	for _, entry := range whitelist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Check for duplicates
		if seen[entry] {
			continue
		}

		if strings.Contains(entry, "/") {
			// CIDR format
			_, _, err := net.ParseCIDR(entry)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR format: %s", entry)
			}
		} else {
			// Plain IP address
			if net.ParseIP(entry) == nil {
				return nil, fmt.Errorf("invalid IP address: %s", entry)
			}
		}

		seen[entry] = true
		result = append(result, entry)
	}
	return result, nil
}

// maskApiKey masks an API key for logging purposes
func maskApiKey(apiKey string) string {
	if len(apiKey) <= 12 {
		return "***"
	}
	return apiKey[:8] + "***" + apiKey[len(apiKey)-4:]
}

// IsApiKey checks if a token looks like an API key
func IsApiKey(token string) bool {
	return strings.HasPrefix(token, ApiKeyPrefix)
}

// ExtractApiKeyFromRequest extracts API key from Authorization: Bearer header
func ExtractApiKeyFromRequest(authHeader string) string {
	if authHeader == "" {
		return ""
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
		token := parts[1]
		if IsApiKey(token) {
			return token
		}
	}

	return ""
}
