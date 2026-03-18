-- =============================================================================
-- Patch 070: Image Analyzer schema additions
-- =============================================================================
-- 1. Add status workflow columns to image_registry_cache for async analysis
-- 2. Create image_layer_cache table for layer-level deduplication
-- =============================================================================

-- -------------------------------------------------------------------------
-- 1. Extend image_registry_cache with analysis status workflow
-- -------------------------------------------------------------------------
ALTER TABLE image_registry_cache ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'completed';
ALTER TABLE image_registry_cache ADD COLUMN IF NOT EXISTS error_message TEXT DEFAULT NULL;
ALTER TABLE image_registry_cache ADD COLUMN IF NOT EXISTS analyzed_at TIMESTAMPTZ DEFAULT NULL;
ALTER TABLE image_registry_cache ADD COLUMN IF NOT EXISTS namespace VARCHAR(128) DEFAULT NULL;

-- Index for the image-analyzer worker to poll pending/processing tasks
CREATE INDEX IF NOT EXISTS idx_irc_status
    ON image_registry_cache(status)
    WHERE status IN ('pending', 'processing');

-- Index for looking up by image_ref + status (used by ai-advisor hybrid poll)
CREATE INDEX IF NOT EXISTS idx_irc_image_ref_status
    ON image_registry_cache(image_ref, status);

-- -------------------------------------------------------------------------
-- 2. Create image_layer_cache for layer-level deduplication
-- -------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS image_layer_cache (
    id               BIGSERIAL PRIMARY KEY,
    layer_digest     VARCHAR(128) NOT NULL,
    compressed_size  BIGINT NOT NULL DEFAULT 0,
    media_type       VARCHAR(128) DEFAULT NULL,
    file_count       INTEGER NOT NULL DEFAULT 0,
    packages         JSONB DEFAULT '[]'::jsonb,
    framework_hints  JSONB DEFAULT '{}'::jsonb,
    notable_paths    JSONB DEFAULT '[]'::jsonb,
    analyzed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ilc_digest
    ON image_layer_cache(layer_digest);
