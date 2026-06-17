package config

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestApplyHeaders verifies upstream auth headers are set based on credential precedence.
func TestApplyHeaders(t *testing.T) {
	// outbound token takes precedence and uses Bearer auth
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	AuthConfig{Outbound: "tok"}.ApplyHeaders(req)
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer tok")
	}

	// internal token is used only when outbound is absent
	req, _ = http.NewRequest(http.MethodGet, "http://example.com", nil)
	AuthConfig{Internal: "itok"}.ApplyHeaders(req)
	if got := req.Header.Get("X-Internal-Token"); got != "itok" {
		t.Fatalf("X-Internal-Token = %q, want %q", got, "itok")
	}

	// no credentials means no headers
	req, _ = http.NewRequest(http.MethodGet, "http://example.com", nil)
	AuthConfig{}.ApplyHeaders(req)
	if req.Header.Get("Authorization") != "" || req.Header.Get("X-Internal-Token") != "" {
		t.Fatalf("expected no auth headers")
	}
}

// writeConfig writes content to a temp file and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// TestLoad verifies config loading, defaults, env overrides and validation errors.
func TestLoad(t *testing.T) {
	// missing file returns an error
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}

	// invalid yaml returns an error
	if _, err := Load(writeConfig(t, "::::not yaml")); err == nil {
		t.Fatal("expected error for invalid yaml")
	}

	// missing smtp.host returns an error
	if _, err := Load(writeConfig(t, "smtp:\n  from: a@b.com\n")); err == nil {
		t.Fatal("expected error for missing smtp.host")
	}

	// missing smtp.from returns an error
	if _, err := Load(writeConfig(t, "smtp:\n  host: mail.local\n")); err == nil {
		t.Fatal("expected error for missing smtp.from")
	}

	// cluster without name returns an error
	if _, err := Load(writeConfig(t, "smtp:\n  host: mail.local\n  from: a@b.com\nclusters:\n  - base_url: http://x\n")); err == nil {
		t.Fatal("expected error for missing cluster name")
	}

	// cluster without base_url returns an error
	if _, err := Load(writeConfig(t, "smtp:\n  host: mail.local\n  from: a@b.com\nclusters:\n  - name: c1\n")); err == nil {
		t.Fatal("expected error for missing cluster base_url")
	}

	// valid config applies defaults and env overrides
	t.Setenv("EMAIL_RELAY_SMTP_USER", "user1")
	t.Setenv("EMAIL_RELAY_SMTP_CREDENTIAL", "secret")
	t.Setenv("EMAIL_RELAY_CLUSTER_0_OUTBOUND", "obtok")
	t.Setenv("EMAIL_RELAY_CLUSTER_0_INTERNAL", "intok")
	content := "smtp:\n  host: mail.local\n  from: a@b.com\nclusters:\n  - name: c1\n    base_url: http://x/\n"
	cfg, err := Load(writeConfig(t, content))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SMTP.Port != 25 {
		t.Errorf("default smtp port = %d, want 25", cfg.SMTP.Port)
	}
	if cfg.APIPort != 8090 {
		t.Errorf("default api port = %d, want 8090", cfg.APIPort)
	}
	if cfg.SMTP.User != "user1" || cfg.SMTP.Credential != "secret" {
		t.Errorf("smtp creds not loaded from env")
	}
	c0 := cfg.Clusters[0]
	if c0.BaseURL != "http://x" {
		t.Errorf("base_url not trimmed: %q", c0.BaseURL)
	}
	if c0.APIPath != "/api/v1/email-relay" {
		t.Errorf("default api_path = %q", c0.APIPath)
	}
	if c0.ReconnectInterval != 5*time.Second {
		t.Errorf("default reconnect = %v", c0.ReconnectInterval)
	}
	if c0.Auth.Outbound != "obtok" || c0.Auth.Internal != "intok" {
		t.Errorf("cluster auth not loaded from env")
	}
}
