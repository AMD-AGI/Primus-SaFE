-- Migration: migrate_detection_data
-- Description: Migrate existing detection data from ai_workload_metadata to new tables
-- Database: PostgreSQL
-- 
-- IMPORTANT: Run this AFTER patch037-workload_detection_tables.sql
-- 
-- This migration:
-- 1. Creates workload_detection records from ai_workload_metadata
-- 2. Creates workload_detection_evidence records for each detection
-- 3. Does NOT modify or delete data from ai_workload_metadata

-- ============================================================================
-- Step 1: Migrate to workload_detection table
-- Creates aggregated detection records from existing metadata
-- ============================================================================

INSERT INTO workload_detection (
    workload_uid,
    status,
    framework,
    frameworks,
    workload_type,
    confidence,
    framework_layer,
    wrapper_framework,
    base_framework,
    detection_state,
    attempt_count,
    max_attempts,
    evidence_count,
    evidence_sources,
    created_at,
    updated_at,
    confirmed_at
)
SELECT 
    m.workload_uid,
    COALESCE(
        m.metadata->>'detection_status',
        CASE 
            WHEN m.framework IS NOT NULL AND m.framework != '' THEN 'confirmed'
            ELSE 'unknown'
        END
    ) as status,
    m.framework,
    CASE 
        WHEN m.framework IS NOT NULL AND m.framework != '' 
        THEN jsonb_build_array(m.framework)
        ELSE '[]'::jsonb
    END as frameworks,
    m.type as workload_type,
    COALESCE(
        (m.metadata->>'confidence')::numeric,
        CASE 
            WHEN m.framework IS NOT NULL AND m.framework != '' THEN 0.8
            ELSE 0.0
        END
    ) as confidence,
    m.metadata->>'framework_layer' as framework_layer,
    m.metadata->>'wrapper_framework' as wrapper_framework,
    m.metadata->>'base_framework' as base_framework,
    'completed' as detection_state,  -- Migrated records are already completed
    0 as attempt_count,
    5 as max_attempts,
    1 as evidence_count,  -- At least one evidence from migration
    jsonb_build_array('migration') as evidence_sources,
    m.created_at,
    NOW() as updated_at,
    CASE 
        WHEN m.framework IS NOT NULL AND m.framework != '' THEN m.created_at
        ELSE NULL
    END as confirmed_at
FROM ai_workload_metadata m
WHERE NOT EXISTS (
    SELECT 1 FROM workload_detection wd 
    WHERE wd.workload_uid = m.workload_uid
)
ON CONFLICT (workload_uid) DO NOTHING;

-- ============================================================================
-- Step 2: Migrate to workload_detection_evidence table
-- Creates evidence records from existing metadata
-- ============================================================================

-- Create evidence records for each metadata entry with a framework
INSERT INTO workload_detection_evidence (
    workload_uid,
    source,
    source_type,
    framework,
    frameworks,
    workload_type,
    confidence,
    framework_layer,
    wrapper_framework,
    base_framework,
    evidence,
    processed,
    processed_at,
    detected_at,
    created_at
)
SELECT 
    m.workload_uid,
    'migration' as source,
    'passive' as source_type,
    m.framework,
    CASE 
        WHEN m.framework IS NOT NULL AND m.framework != '' 
        THEN jsonb_build_array(m.framework)
        ELSE '[]'::jsonb
    END as frameworks,
    m.type as workload_type,
    COALESCE(
        (m.metadata->>'confidence')::numeric,
        CASE 
            WHEN m.framework IS NOT NULL AND m.framework != '' THEN 0.8
            ELSE 0.0
        END
    ) as confidence,
    m.metadata->>'framework_layer' as framework_layer,
    m.metadata->>'wrapper_framework' as wrapper_framework,
    m.metadata->>'base_framework' as base_framework,
    jsonb_build_object(
        'migrated_from', 'ai_workload_metadata',
        'migrated_at', NOW()::text,
        'original_id', m.id,
        'original_metadata', m.metadata
    ) as evidence,
    true as processed,
    NOW() as processed_at,
    m.created_at as detected_at,
    NOW() as created_at
FROM ai_workload_metadata m
WHERE m.framework IS NOT NULL 
  AND m.framework != ''
  AND NOT EXISTS (
    SELECT 1 FROM workload_detection_evidence wde 
    WHERE wde.workload_uid = m.workload_uid 
      AND wde.source = 'migration'
);

-- ============================================================================
-- Step 3: Create evidence records for WandB detections (if wandb data exists)
-- ============================================================================

INSERT INTO workload_detection_evidence (
    workload_uid,
    source,
    source_type,
    framework,
    frameworks,
    workload_type,
    confidence,
    evidence,
    processed,
    processed_at,
    detected_at,
    created_at
)
SELECT 
    m.workload_uid,
    'wandb' as source,
    'passive' as source_type,
    m.framework,
    CASE 
        WHEN m.framework IS NOT NULL AND m.framework != '' 
        THEN jsonb_build_array(m.framework)
        ELSE '[]'::jsonb
    END as frameworks,
    m.type as workload_type,
    1.0 as confidence,  -- WandB is high confidence
    jsonb_build_object(
        'wandb_data', m.metadata->'wandb',
        'migrated_from', 'ai_workload_metadata',
        'migrated_at', NOW()::text
    ) as evidence,
    true as processed,
    NOW() as processed_at,
    m.created_at as detected_at,
    NOW() as created_at
FROM ai_workload_metadata m
WHERE m.metadata ? 'wandb'
  AND m.metadata->'wandb' IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM workload_detection_evidence wde 
    WHERE wde.workload_uid = m.workload_uid 
      AND wde.source = 'wandb'
);

-- ============================================================================
-- Step 4: Update evidence counts in workload_detection
-- ============================================================================

UPDATE workload_detection wd
SET 
    evidence_count = (
        SELECT COUNT(*) 
        FROM workload_detection_evidence wde 
        WHERE wde.workload_uid = wd.workload_uid
    ),
    evidence_sources = (
        SELECT jsonb_agg(DISTINCT wde.source)
        FROM workload_detection_evidence wde
        WHERE wde.workload_uid = wd.workload_uid
    ),
    updated_at = NOW()
WHERE EXISTS (
    SELECT 1 FROM workload_detection_evidence wde 
    WHERE wde.workload_uid = wd.workload_uid
);

-- ============================================================================
-- Verification queries (run these to check migration results)
-- ============================================================================

-- Check migration counts:
-- SELECT 'ai_workload_metadata' as table_name, COUNT(*) as count FROM ai_workload_metadata
-- UNION ALL
-- SELECT 'workload_detection', COUNT(*) FROM workload_detection
-- UNION ALL
-- SELECT 'workload_detection_evidence', COUNT(*) FROM workload_detection_evidence;

-- Check detection status distribution:
-- SELECT status, COUNT(*) as count 
-- FROM workload_detection 
-- GROUP BY status 
-- ORDER BY count DESC;

-- Check evidence source distribution:
-- SELECT source, COUNT(*) as count 
-- FROM workload_detection_evidence 
-- GROUP BY source 
-- ORDER BY count DESC;

