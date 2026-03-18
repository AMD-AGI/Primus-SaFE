-- patch064: Add workload intent analysis system
-- Part 1: Extend workload_detection with intent analysis fields
-- Part 2: Create intent_rule table for distilled detection rules
-- Part 3: Create workload_code_snapshot table for container code snapshots
-- Part 4: Create image_registry_cache table for Harbor image metadata
-- Part 5: Seed new detection source priorities

-- =============================================================================
-- Part 1: ALTER TABLE workload_detection - add intent analysis columns
-- =============================================================================

-- Intent analysis: workload category (fine-grained workload_type)
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS category VARCHAR(50);

-- Intent analysis: expected behavior pattern
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS expected_behavior VARCHAR(50);

-- Intent analysis: model information
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS model_path TEXT;
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS model_family VARCHAR(100);
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS model_scale VARCHAR(50);
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS model_variant VARCHAR(50);

-- Intent analysis: runtime framework (completes the 3-layer stack: wrapper + base + runtime)
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS runtime_framework VARCHAR(100);

-- Intent analysis: detailed parameters (JSONB for flexibility)
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS intent_detail JSONB DEFAULT '{}'::jsonb;

-- Intent analysis: metadata
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS intent_confidence DECIMAL(4,3);
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS intent_source VARCHAR(50);
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS intent_reasoning TEXT;
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS intent_field_sources JSONB DEFAULT '{}'::jsonb;
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS intent_analysis_mode VARCHAR(20);
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS intent_matched_rules JSONB DEFAULT '[]'::jsonb;

-- Intent analysis: lifecycle state (independent from detection_state)
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS intent_state VARCHAR(50) DEFAULT 'pending';
ALTER TABLE workload_detection ADD COLUMN IF NOT EXISTS intent_analyzed_at TIMESTAMPTZ;

-- Indexes for intent fields
CREATE INDEX IF NOT EXISTS idx_wd_category ON workload_detection(category);
CREATE INDEX IF NOT EXISTS idx_wd_model_family ON workload_detection(model_family);
CREATE INDEX IF NOT EXISTS idx_wd_intent_state ON workload_detection(intent_state);
CREATE INDEX IF NOT EXISTS idx_wd_intent_confidence ON workload_detection(intent_confidence);

-- =============================================================================
-- Part 2: CREATE TABLE intent_rule
-- =============================================================================

CREATE TABLE IF NOT EXISTS intent_rule (
    id              BIGSERIAL PRIMARY KEY,
    detects_field   VARCHAR(50) NOT NULL,
    detects_value   VARCHAR(100) NOT NULL,
    dimension       VARCHAR(50) NOT NULL,
    pattern         TEXT NOT NULL,
    confidence      DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    reasoning       TEXT,
    derived_from    JSONB DEFAULT '[]'::jsonb,
    status          VARCHAR(20) NOT NULL DEFAULT 'proposed',
    backtest_result JSONB DEFAULT '{}'::jsonb,
    last_backtested_at TIMESTAMPTZ,
    match_count     INTEGER NOT NULL DEFAULT 0,
    correct_count   INTEGER NOT NULL DEFAULT 0,
    false_positive_count INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_intent_rule_status ON intent_rule(status);
CREATE INDEX IF NOT EXISTS idx_intent_rule_detects ON intent_rule(detects_field, detects_value);
CREATE INDEX IF NOT EXISTS idx_intent_rule_dimension ON intent_rule(dimension);

-- =============================================================================
-- Part 3: CREATE TABLE workload_code_snapshot
-- =============================================================================

CREATE TABLE IF NOT EXISTS workload_code_snapshot (
    id              BIGSERIAL PRIMARY KEY,
    workload_uid    VARCHAR(255) NOT NULL,
    entry_script    JSONB DEFAULT '{}'::jsonb,
    config_files    JSONB DEFAULT '[]'::jsonb,
    local_modules   JSONB DEFAULT '[]'::jsonb,
    import_graph    JSONB DEFAULT '{}'::jsonb,
    pip_freeze      TEXT,
    working_dir_tree TEXT,
    fingerprint     VARCHAR(64),
    total_size      INTEGER NOT NULL DEFAULT 0,
    file_count      INTEGER NOT NULL DEFAULT 0,
    captured_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_wcs_workload_uid ON workload_code_snapshot(workload_uid);
CREATE INDEX IF NOT EXISTS idx_wcs_fingerprint ON workload_code_snapshot(fingerprint);

-- =============================================================================
-- Part 4: CREATE TABLE image_registry_cache
-- =============================================================================

CREATE TABLE IF NOT EXISTS image_registry_cache (
    id                  BIGSERIAL PRIMARY KEY,
    image_ref           VARCHAR(512) NOT NULL,
    digest              VARCHAR(128) NOT NULL,
    registry            VARCHAR(255),
    repository          VARCHAR(255),
    tag                 VARCHAR(128),
    base_image          VARCHAR(512),
    layer_count         INTEGER NOT NULL DEFAULT 0,
    layer_history       JSONB DEFAULT '[]'::jsonb,
    image_labels        JSONB DEFAULT '{}'::jsonb,
    image_env           JSONB DEFAULT '{}'::jsonb,
    image_entrypoint    TEXT,
    installed_packages  JSONB DEFAULT '[]'::jsonb,
    framework_hints     JSONB DEFAULT '{}'::jsonb,
    total_size          BIGINT NOT NULL DEFAULT 0,
    image_created_at    TIMESTAMPTZ,
    cached_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at          TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_irc_digest ON image_registry_cache(digest);
CREATE INDEX IF NOT EXISTS idx_irc_registry_repo_tag ON image_registry_cache(registry, repository, tag);
CREATE INDEX IF NOT EXISTS idx_irc_base_image ON image_registry_cache(base_image);

-- =============================================================================
-- Part 5: Seed new detection source priorities
-- =============================================================================

INSERT INTO detection_source_priority (source_name, display_name, priority, base_confidence, description) VALUES
    ('spec', 'Workload Spec', 50, 0.50, 'Image name, cmdline, env, labels from workload spec'),
    ('code_snapshot', 'Code Snapshot', 90, 0.90, 'Code and config files from container'),
    ('image_registry', 'Image Registry', 55, 0.65, 'Image layer analysis from Harbor'),
    ('intent_analysis', 'Intent Analysis', 95, 0.90, 'LLM-powered intent analysis')
ON CONFLICT (source_name) DO NOTHING;
