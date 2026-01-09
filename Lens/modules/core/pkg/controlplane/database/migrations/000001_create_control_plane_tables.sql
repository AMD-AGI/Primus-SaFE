-- Control Plane Database Migration
-- Version: 000001
-- Description: Create all Control Plane tables

-- 1. Create users table
CREATE TABLE IF NOT EXISTS lens_users (
    id              VARCHAR(64) PRIMARY KEY,
    username        VARCHAR(128) NOT NULL,
    email           VARCHAR(256),
    display_name    VARCHAR(256),
    auth_type       VARCHAR(32) NOT NULL DEFAULT 'local',
    ldap_dn         VARCHAR(512),
    status          VARCHAR(32) NOT NULL DEFAULT 'active',
    is_admin        BOOLEAN NOT NULL DEFAULT FALSE,
    is_root         BOOLEAN NOT NULL DEFAULT FALSE,
    password_hash   VARCHAR(256),
    must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at   TIMESTAMPTZ,
    CONSTRAINT uq_lens_users_username UNIQUE (username)
);

CREATE INDEX IF NOT EXISTS idx_lens_users_username ON lens_users(username);
CREATE INDEX IF NOT EXISTS idx_lens_users_email ON lens_users(email);
CREATE INDEX IF NOT EXISTS idx_lens_users_auth_type ON lens_users(auth_type);
CREATE INDEX IF NOT EXISTS idx_lens_users_is_admin ON lens_users(is_admin);

-- 2. Create sessions table
CREATE TABLE IF NOT EXISTS lens_sessions (
    id              VARCHAR(64) PRIMARY KEY,
    user_id         VARCHAR(64) NOT NULL,
    token_hash      VARCHAR(128) NOT NULL,
    user_agent      VARCHAR(512),
    ip_address      VARCHAR(64),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    last_accessed   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_revoked      BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at      TIMESTAMPTZ,
    revoke_reason   VARCHAR(256),
    safe_session_id VARCHAR(64),
    sync_source     VARCHAR(32) DEFAULT 'local'
);

CREATE INDEX IF NOT EXISTS idx_lens_sessions_user_id ON lens_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_lens_sessions_token_hash ON lens_sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_lens_sessions_expires_at ON lens_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_lens_sessions_safe_session_id ON lens_sessions(safe_session_id);

-- 3. Create system_configs table
CREATE TABLE IF NOT EXISTS lens_system_configs (
    key             VARCHAR(256) PRIMARY KEY,
    value           JSONB NOT NULL,
    description     VARCHAR(512),
    category        VARCHAR(64) NOT NULL,
    is_secret       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lens_system_configs_category ON lens_system_configs(category);

-- 4. Create auth_providers table
CREATE TABLE IF NOT EXISTS lens_auth_providers (
    id              VARCHAR(64) PRIMARY KEY,
    name            VARCHAR(128) NOT NULL,
    type            VARCHAR(32) NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    priority        INT NOT NULL DEFAULT 0,
    config          JSONB NOT NULL,
    status          VARCHAR(32) NOT NULL DEFAULT 'active',
    last_check_at   TIMESTAMPTZ,
    last_error      VARCHAR(512),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by      VARCHAR(64),
    CONSTRAINT uq_lens_auth_providers_name UNIQUE (name)
);

CREATE INDEX IF NOT EXISTS idx_lens_auth_providers_type ON lens_auth_providers(type);
CREATE INDEX IF NOT EXISTS idx_lens_auth_providers_enabled ON lens_auth_providers(enabled);

-- 5. Create login_audit table
CREATE TABLE IF NOT EXISTS lens_login_audit (
    id              VARCHAR(64) PRIMARY KEY,
    username        VARCHAR(128) NOT NULL,
    user_id         VARCHAR(64),
    event_type      VARCHAR(32) NOT NULL,
    auth_type       VARCHAR(32),
    ip_address      VARCHAR(64),
    user_agent      VARCHAR(512),
    success         BOOLEAN NOT NULL,
    failure_reason  VARCHAR(256),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lens_login_audit_username ON lens_login_audit(username);
CREATE INDEX IF NOT EXISTS idx_lens_login_audit_user_id ON lens_login_audit(user_id);
CREATE INDEX IF NOT EXISTS idx_lens_login_audit_created_at ON lens_login_audit(created_at);

-- 6. Insert default configs
INSERT INTO lens_system_configs (key, value, description, category) VALUES
    ('auth.mode', '"none"', 'Authentication mode: none, ldap, sso, safe', 'auth'),
    ('auth.initialized', 'false', 'Whether auth has been initialized', 'auth'),
    ('system.initialized', 'false', 'Whether system has been initialized', 'system'),
    ('safe.integration.enabled', 'false', 'Whether SaFE integration is enabled', 'auth'),
    ('safe.integration.auto_detected', 'false', 'Whether SaFE was auto-detected', 'auth')
ON CONFLICT (key) DO NOTHING;
