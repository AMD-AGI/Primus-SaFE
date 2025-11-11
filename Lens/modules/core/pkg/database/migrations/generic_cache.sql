-- Generic Cache Database Schema
-- This file defines the database schema for a generic key-value cache system
-- that can store arbitrary JSON data

-- Table: generic_cache
-- Stores key-value pairs with JSONB values for flexible data caching
CREATE TABLE IF NOT EXISTS generic_cache (
    id BIGSERIAL PRIMARY KEY,
    key VARCHAR(255) NOT NULL UNIQUE,
    value JSONB NOT NULL,
    
    -- Expiration support
    expires_at TIMESTAMP,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for generic_cache
CREATE UNIQUE INDEX IF NOT EXISTS idx_generic_cache_key ON generic_cache(key);
CREATE INDEX IF NOT EXISTS idx_generic_cache_expires_at ON generic_cache(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_generic_cache_updated_at ON generic_cache(updated_at DESC);

-- GIN index for JSONB value queries (optional, for advanced JSON queries)
CREATE INDEX IF NOT EXISTS idx_generic_cache_value ON generic_cache USING GIN(value);

-- Comments for documentation
COMMENT ON TABLE generic_cache IS 'Generic key-value cache system with JSONB values for flexible data storage';
COMMENT ON COLUMN generic_cache.key IS 'Unique cache key identifier';
COMMENT ON COLUMN generic_cache.value IS 'JSONB value containing cached data';
COMMENT ON COLUMN generic_cache.expires_at IS 'Optional expiration timestamp, NULL means no expiration';
COMMENT ON COLUMN generic_cache.created_at IS 'Timestamp when the cache entry was first created';
COMMENT ON COLUMN generic_cache.updated_at IS 'Timestamp when the cache entry was last updated';

