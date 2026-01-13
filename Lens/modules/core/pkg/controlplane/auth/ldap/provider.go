// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package ldap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/go-ldap/ldap/v3"
)

// Credentials represents authentication credentials
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResult represents the result of authentication
type AuthResult struct {
	Success    bool      `json:"success"`
	User       *UserInfo `json:"user,omitempty"`
	FailReason string    `json:"failReason,omitempty"`
}

// UserInfo represents user information from LDAP
type UserInfo struct {
	ID          string            `json:"id"`
	Username    string            `json:"username"`
	Email       string            `json:"email"`
	DisplayName string            `json:"displayName"`
	Groups      []string          `json:"groups"`
	Attributes  map[string]string `json:"attributes"`
	AuthType    string            `json:"authType"`
	ExternalID  string            `json:"externalId"` // LDAP DN
	IsAdmin     bool              `json:"isAdmin"`
}

// Provider implements LDAP authentication
type Provider struct {
	config *Config
	pool   *ConnectionPool
}

// NewProvider creates a new LDAP provider
func NewProvider(config *Config) (*Provider, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid LDAP config: %w", err)
	}

	pool, err := NewConnectionPool(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create LDAP connection pool: %w", err)
	}

	return &Provider{
		config: config,
		pool:   pool,
	}, nil
}

// NewProviderFromMap creates a new LDAP provider from a config map
func NewProviderFromMap(m map[string]interface{}) (*Provider, error) {
	config := ConfigFromMap(m)
	return NewProvider(config)
}

// Type returns the provider type
func (p *Provider) Type() string {
	return "ldap"
}

// Authenticate validates credentials against LDAP
func (p *Provider) Authenticate(ctx context.Context, creds *Credentials) (*AuthResult, error) {
	if creds.Username == "" || creds.Password == "" {
		return &AuthResult{
			Success:    false,
			FailReason: "username and password are required",
		}, nil
	}

	conn, err := p.pool.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get LDAP connection: %w", err)
	}
	defer p.pool.Put(conn)

	// Step 1: Search for user DN
	userDN, err := p.searchUserDN(conn, creds.Username)
	if err != nil {
		log.Debugf("LDAP user search failed for %s: %v", creds.Username, err)
		return &AuthResult{
			Success:    false,
			FailReason: "user not found",
		}, nil
	}

	log.Debugf("Found user DN: %s", userDN)

	// Step 2: Bind with user credentials to verify password
	err = conn.Bind(userDN, creds.Password)
	if err != nil {
		log.Debugf("LDAP bind failed for %s: %v", userDN, err)
		return &AuthResult{
			Success:    false,
			FailReason: "invalid credentials",
		}, nil
	}

	// Step 3: Re-bind with service account to fetch user attributes
	err = conn.Bind(p.config.BindDN, p.config.GetBindPassword())
	if err != nil {
		return nil, fmt.Errorf("failed to rebind with service account: %w", err)
	}

	// Step 4: Get user attributes
	userInfo, err := p.getUserAttributes(conn, userDN, creds.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to get user attributes: %w", err)
	}

	log.Infof("LDAP authentication successful for user: %s", creds.Username)

	return &AuthResult{
		Success: true,
		User:    userInfo,
	}, nil
}

// searchUserDN searches for a user's DN by username
func (p *Provider) searchUserDN(conn *ldap.Conn, username string) (string, error) {
	searchBase := p.config.GetUserSearchBase()
	filter := fmt.Sprintf(p.config.GetUserSearchFilter(), ldap.EscapeFilter(username))

	searchRequest := ldap.NewSearchRequest(
		searchBase,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1,  // size limit
		30, // time limit (seconds)
		false,
		filter,
		[]string{"dn"},
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(result.Entries) == 0 {
		return "", fmt.Errorf("user not found")
	}

	return result.Entries[0].DN, nil
}

// getUserAttributes fetches user attributes from LDAP
func (p *Provider) getUserAttributes(conn *ldap.Conn, userDN, username string) (*UserInfo, error) {
	attrs := []string{
		p.config.GetUsernameAttr(),
		p.config.GetEmailAttr(),
		p.config.GetDisplayNameAttr(),
		p.config.GetMemberOfAttr(),
	}

	searchRequest := ldap.NewSearchRequest(
		userDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1,
		30,
		false,
		"(objectClass=*)",
		attrs,
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("attribute search failed: %w", err)
	}

	if len(result.Entries) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	entry := result.Entries[0]

	userInfo := &UserInfo{
		ID:          generateUserID(username),
		Username:    username,
		Email:       entry.GetAttributeValue(p.config.GetEmailAttr()),
		DisplayName: entry.GetAttributeValue(p.config.GetDisplayNameAttr()),
		Groups:      entry.GetAttributeValues(p.config.GetMemberOfAttr()),
		AuthType:    "ldap",
		ExternalID:  userDN,
		Attributes:  make(map[string]string),
	}

	// Store additional attributes
	for _, attr := range entry.Attributes {
		if len(attr.Values) > 0 {
			userInfo.Attributes[attr.Name] = attr.Values[0]
		}
	}

	// Check if user is admin based on group membership
	if p.config.AdminGroupDN != "" {
		userInfo.IsAdmin = p.isUserInGroup(userInfo.Groups, p.config.AdminGroupDN)
	}

	return userInfo, nil
}

// isUserInGroup checks if a user is in a specific group
func (p *Provider) isUserInGroup(groups []string, groupDN string) bool {
	normalizedGroupDN := strings.ToLower(groupDN)
	for _, g := range groups {
		if strings.ToLower(g) == normalizedGroupDN {
			return true
		}
	}
	return false
}

// GetUserInfo retrieves user information by username without authentication
func (p *Provider) GetUserInfo(ctx context.Context, username string) (*UserInfo, error) {
	conn, err := p.pool.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get LDAP connection: %w", err)
	}
	defer p.pool.Put(conn)

	userDN, err := p.searchUserDN(conn, username)
	if err != nil {
		return nil, err
	}

	return p.getUserAttributes(conn, userDN, username)
}

// IsAvailable checks if LDAP server is reachable
func (p *Provider) IsAvailable(ctx context.Context) bool {
	conn, err := p.pool.Get()
	if err != nil {
		return false
	}
	defer p.pool.Put(conn)
	return true
}

// TestConnection tests the LDAP connection and returns details
func (p *Provider) TestConnection(ctx context.Context) (*TestResult, error) {
	result := &TestResult{
		Success: false,
		Details: make(map[string]interface{}),
	}

	conn, err := p.pool.Get()
	if err != nil {
		result.Message = fmt.Sprintf("connection failed: %v", err)
		return result, nil
	}
	defer p.pool.Put(conn)

	// Test base DN search
	searchRequest := ldap.NewSearchRequest(
		p.config.BaseDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1,
		10,
		false,
		"(objectClass=*)",
		[]string{"namingContexts", "subschemaSubentry", "supportedLDAPVersion"},
		nil,
	)

	searchResult, err := conn.Search(searchRequest)
	if err != nil {
		result.Message = fmt.Sprintf("base DN search failed: %v", err)
		return result, nil
	}

	result.Success = true
	result.Message = "LDAP connection successful"
	result.Details["baseDnFound"] = len(searchResult.Entries) > 0
	result.Details["host"] = p.config.Host
	result.Details["port"] = p.config.GetPort()
	result.Details["useSSL"] = p.config.UseSSL
	result.Details["useStartTLS"] = p.config.UseStartTLS

	// Try to count users (limited search)
	userCountRequest := ldap.NewSearchRequest(
		p.config.GetUserSearchBase(),
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		100, // limit to 100 for count
		10,
		false,
		"(objectClass=person)",
		[]string{"dn"},
		nil,
	)

	userResult, err := conn.Search(userCountRequest)
	if err == nil {
		result.Details["userCount"] = len(userResult.Entries)
	}

	return result, nil
}

// TestResult represents the result of a connection test
type TestResult struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Close closes the provider and its connection pool
func (p *Provider) Close() {
	if p.pool != nil {
		p.pool.Close()
	}
}

// generateUserID generates a deterministic user ID from username
func generateUserID(username string) string {
	hash := sha256.Sum256([]byte("ldap:" + username))
	return "ldap-" + hex.EncodeToString(hash[:8])
}
