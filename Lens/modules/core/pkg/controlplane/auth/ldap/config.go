// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package ldap

import (
	"fmt"
	"os"
)

// Config contains LDAP provider configuration
type Config struct {
	// Connection settings
	Host          string `json:"host" yaml:"host"`
	Port          int    `json:"port" yaml:"port"`
	UseSSL        bool   `json:"useSsl" yaml:"useSsl"`
	UseStartTLS   bool   `json:"useStartTls" yaml:"useStartTls"`
	SkipTLSVerify bool   `json:"skipTlsVerify" yaml:"skipTlsVerify"`

	// Bind credentials
	BindDN          string `json:"bindDn" yaml:"bindDn"`
	BindPassword    string `json:"bindPassword" yaml:"bindPassword"`
	BindPasswordEnv string `json:"bindPasswordEnv" yaml:"bindPasswordEnv"`

	// Search settings
	BaseDN           string `json:"baseDn" yaml:"baseDn"`
	UserSearchBase   string `json:"userSearchBase" yaml:"userSearchBase"`
	UserSearchFilter string `json:"userSearchFilter" yaml:"userSearchFilter"` // e.g., "(uid=%s)" or "(sAMAccountName=%s)"

	// Attribute mappings
	UsernameAttr    string `json:"usernameAttr" yaml:"usernameAttr"`       // default: uid
	EmailAttr       string `json:"emailAttr" yaml:"emailAttr"`             // default: mail
	DisplayNameAttr string `json:"displayNameAttr" yaml:"displayNameAttr"` // default: displayName
	MemberOfAttr    string `json:"memberOfAttr" yaml:"memberOfAttr"`       // default: memberOf

	// Group settings
	AdminGroupDN string `json:"adminGroupDn" yaml:"adminGroupDn"` // Users in this group are admins

	// Connection pool settings
	PoolSize    int `json:"poolSize" yaml:"poolSize"`       // default: 5
	ConnTimeout int `json:"connTimeout" yaml:"connTimeout"` // seconds, default: 10
}

// GetPort returns the port, using defaults based on SSL setting
func (c *Config) GetPort() int {
	if c.Port > 0 {
		return c.Port
	}
	if c.UseSSL {
		return 636
	}
	return 389
}

// GetBindPassword returns the bind password from config or environment
func (c *Config) GetBindPassword() string {
	if c.BindPassword != "" {
		return c.BindPassword
	}
	if c.BindPasswordEnv != "" {
		return os.Getenv(c.BindPasswordEnv)
	}
	return ""
}

// GetUserSearchFilter returns the user search filter with defaults
func (c *Config) GetUserSearchFilter() string {
	if c.UserSearchFilter != "" {
		return c.UserSearchFilter
	}
	return "(uid=%s)"
}

// GetUserSearchBase returns the user search base, defaulting to BaseDN
func (c *Config) GetUserSearchBase() string {
	if c.UserSearchBase != "" {
		return c.UserSearchBase
	}
	return c.BaseDN
}

// GetUsernameAttr returns the username attribute with default
func (c *Config) GetUsernameAttr() string {
	if c.UsernameAttr != "" {
		return c.UsernameAttr
	}
	return "uid"
}

// GetEmailAttr returns the email attribute with default
func (c *Config) GetEmailAttr() string {
	if c.EmailAttr != "" {
		return c.EmailAttr
	}
	return "mail"
}

// GetDisplayNameAttr returns the display name attribute with default
func (c *Config) GetDisplayNameAttr() string {
	if c.DisplayNameAttr != "" {
		return c.DisplayNameAttr
	}
	return "displayName"
}

// GetMemberOfAttr returns the member of attribute with default
func (c *Config) GetMemberOfAttr() string {
	if c.MemberOfAttr != "" {
		return c.MemberOfAttr
	}
	return "memberOf"
}

// GetPoolSize returns the pool size with default
func (c *Config) GetPoolSize() int {
	if c.PoolSize > 0 {
		return c.PoolSize
	}
	return 5
}

// GetConnTimeout returns the connection timeout in seconds with default
func (c *Config) GetConnTimeout() int {
	if c.ConnTimeout > 0 {
		return c.ConnTimeout
	}
	return 10
}

// Validate validates the LDAP configuration
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("LDAP host is required")
	}
	if c.BindDN == "" {
		return fmt.Errorf("LDAP bind DN is required")
	}
	if c.GetBindPassword() == "" {
		return fmt.Errorf("LDAP bind password is required")
	}
	if c.BaseDN == "" {
		return fmt.Errorf("LDAP base DN is required")
	}
	return nil
}

// ConfigFromMap creates a Config from a map (typically from database)
func ConfigFromMap(m map[string]interface{}) *Config {
	config := &Config{}

	if v, ok := m["host"].(string); ok {
		config.Host = v
	}
	if v, ok := m["port"].(float64); ok {
		config.Port = int(v)
	}
	if v, ok := m["useSsl"].(bool); ok {
		config.UseSSL = v
	}
	if v, ok := m["useStartTls"].(bool); ok {
		config.UseStartTLS = v
	}
	if v, ok := m["skipTlsVerify"].(bool); ok {
		config.SkipTLSVerify = v
	}
	if v, ok := m["bindDn"].(string); ok {
		config.BindDN = v
	}
	if v, ok := m["bindPassword"].(string); ok {
		config.BindPassword = v
	}
	if v, ok := m["bindPasswordEnv"].(string); ok {
		config.BindPasswordEnv = v
	}
	if v, ok := m["baseDn"].(string); ok {
		config.BaseDN = v
	}
	if v, ok := m["userSearchBase"].(string); ok {
		config.UserSearchBase = v
	}
	if v, ok := m["userSearchFilter"].(string); ok {
		config.UserSearchFilter = v
	}
	if v, ok := m["usernameAttr"].(string); ok {
		config.UsernameAttr = v
	}
	if v, ok := m["emailAttr"].(string); ok {
		config.EmailAttr = v
	}
	if v, ok := m["displayNameAttr"].(string); ok {
		config.DisplayNameAttr = v
	}
	if v, ok := m["memberOfAttr"].(string); ok {
		config.MemberOfAttr = v
	}
	if v, ok := m["adminGroupDn"].(string); ok {
		config.AdminGroupDN = v
	}
	if v, ok := m["poolSize"].(float64); ok {
		config.PoolSize = int(v)
	}
	if v, ok := m["connTimeout"].(float64); ok {
		config.ConnTimeout = int(v)
	}

	return config
}
