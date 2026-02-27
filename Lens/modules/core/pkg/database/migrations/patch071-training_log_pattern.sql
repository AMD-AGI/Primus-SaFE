-- training_log_pattern: Dedicated table for log parsing patterns
-- Replaces the framework-coupled pattern storage in system_config
-- All patterns are global and matched against all incoming logs (no framework pre-filtering)

CREATE TABLE IF NOT EXISTS training_log_pattern (
    id              BIGSERIAL PRIMARY KEY,
    pattern         TEXT NOT NULL,                              -- Regex pattern string
    pattern_type    TEXT NOT NULL DEFAULT 'performance',        -- 'performance', 'blacklist', 'training_event', 'checkpoint_event'
    event_subtype   TEXT,                                       -- For events: 'start_training', 'end_training', 'start_saving', 'end_saving', 'loading'
    source          TEXT NOT NULL DEFAULT 'manual',             -- 'manual', 'autodiscovered', 'migration'
    source_workload_uid TEXT,                                   -- Workload that triggered autodiscovery (nullable)
    framework       TEXT,                                       -- Informational: which framework this was derived from (not used for matching)
    name            TEXT,                                       -- Human-readable pattern name
    description     TEXT,
    sample_line     TEXT,                                       -- Example log line that matches this pattern
    enabled         BOOLEAN NOT NULL DEFAULT false,             -- Only enabled patterns are loaded by telemetry-processor
    priority        INTEGER NOT NULL DEFAULT 50,                -- Higher = tried first; allows ordering without framework coupling
    confidence      FLOAT8 NOT NULL DEFAULT 0.5,
    hit_count       BIGINT NOT NULL DEFAULT 0,                  -- Incremented on match (approximate, not transactional)
    last_hit_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Primary query path: load all enabled patterns of a given type, ordered by priority
CREATE INDEX idx_training_log_pattern_type_enabled
    ON training_log_pattern (pattern_type, enabled, priority DESC)
    WHERE enabled = true;

-- Deduplication: prevent exact same regex from being inserted twice
CREATE UNIQUE INDEX idx_training_log_pattern_unique_pattern
    ON training_log_pattern (pattern_type, md5(pattern));

-- Lookup by source workload
CREATE INDEX idx_training_log_pattern_source_workload
    ON training_log_pattern (source_workload_uid)
    WHERE source_workload_uid IS NOT NULL;

-- ============================================================
-- Migrate ALL existing performance patterns from system_config
-- Source: patch015-framework_primus_patterns_init.sql
-- ============================================================

-- 1. Primus ROCm memory variant
INSERT INTO training_log_pattern (pattern, pattern_type, source, framework, name, description, enabled, priority, confidence)
VALUES (
'.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|\s*consumed samples:\s+(?P<ConsumedSamples>\d+)\s*\|\s*elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+rocm\s+mem\s+usage/free/total/usage_ratio:\s+(?P<MemUsage>\d+\.\d+)GB/(?P<MemFree>\d+\.\d+)GB/(?P<MemTotal>\d+\.\d+)GB/(?P<MemUsageRatio>\d+\.\d+)%\s+\|\s+throughput\s+per\s+GPU\s+\(TFLOP/s/GPU\):\s+(?P<TFLOPS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+tokens\s+per\s+GPU\s+\(tokens/s/GPU\):\s+(?P<TokensPerGPU>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s*learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s*\|\s+global\s+batch\s+size:\s+(?P<GlobalBatchSize>\d+(?:\.\d+)*)\s+\|\s+lm\s+loss:\s+(?P<LmLoss>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|\s+loss\s+scale:\s+(?P<LossScale>\d+(?:\.\d+)*)\s+\|\s+grad\s+norm:\s+(?P<GradNorm>\d+(?:\.\d+)*)\s+\|\s+num\s+zeros:\s(?P<NumZeros>\d+(?:\.\d+)*)\s+\|\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\s*\|.*',
 'performance', 'migration', 'primus', 'primus-rocm-memory',
 'Primus performance log with ROCm memory metrics',
 true, 80, 1.0
) ON CONFLICT (pattern_type, md5(pattern)) DO NOTHING;

-- 2. Primus HIP memory variant
INSERT INTO training_log_pattern (pattern, pattern_type, source, framework, name, description, enabled, priority, confidence)
VALUES (
'.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|\s*consumed samples:\s+(?P<ConsumedSamples>\d+)\s*\|\s*elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+hip\s+mem\s+usage/free/total/usage_ratio:\s+(?P<MemUsage>\d+\.\d+)GB/(?P<MemFree>\d+\.\d+)GB/(?P<MemTotal>\d+\.\d+)GB/(?P<MemUsageRatio>\d+\.\d+)%\s+\|\s+throughput\s+per\s+GPU\s+\(TFLOP/s/GPU\):\s+(?P<TFLOPS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+tokens\s+per\s+GPU\s+\(tokens/s/GPU\):\s+(?P<TokensPerGPU>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s*learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s*\|\s+global\s+batch\s+size:\s+(?P<GlobalBatchSize>\d+(?:\.\d+)*)\s+\|\s+lm\s+loss:\s+(?P<LmLoss>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|\s+loss\s+scale:\s+(?P<LossScale>\d+(?:\.\d+)*)\s+\|\s+grad\s+norm:\s+(?P<GradNorm>\d+(?:\.\d+)*)\s+\|\s+num\s+zeros:\s(?P<NumZeros>\d+(?:\.\d+)*)\s+\|\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\s*\|.*',
 'performance', 'migration', 'primus', 'primus-hip-memory',
 'Primus performance log with HIP memory metrics',
 true, 80, 1.0
) ON CONFLICT (pattern_type, md5(pattern)) DO NOTHING;

-- 3. Primus HIP memory v2 (without num_zeros field)
INSERT INTO training_log_pattern (pattern, pattern_type, source, framework, name, description, enabled, priority, confidence)
VALUES (
'.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|\s*consumed samples:\s+(?P<ConsumedSamples>\d+)\s*\|\s*elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+hip\s+mem\s+usage/free/total/usage_ratio:\s+(?P<MemUsage>\d+\.\d+)GB/(?P<MemFree>\d+\.\d+)GB/(?P<MemTotal>\d+\.\d+)GB/(?P<MemUsageRatio>\d+\.\d+)%\s+\|\s+throughput\s+per\s+GPU\s+\(TFLOP/s/GPU\):\s+(?P<TFLOPS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+tokens\s+per\s+GPU\s+\(tokens/s/GPU\):\s+(?P<TokensPerGPU>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s*learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s*\|\s+global\s+batch\s+size:\s+(?P<GlobalBatchSize>\d+(?:\.\d+)*)\s+\|\s+lm\s+loss:\s+(?P<LmLoss>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|\s+loss\s+scale:\s+(?P<LossScale>\d+(?:\.\d+)*)\s+\|\s+grad\s+norm:\s+(?P<GradNorm>\d+(?:\.\d+)*)\s+\|\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\s*\|.*',
 'performance', 'migration', 'primus', 'primus-hip-memory-v2',
 'Primus performance log with HIP memory metrics (v2 - without num zeros)',
 true, 80, 1.0
) ON CONFLICT (pattern_type, md5(pattern)) DO NOTHING;

-- 4. Primus legacy (mem_usages variant, no detailed GPU memory breakdown)
INSERT INTO training_log_pattern (pattern, pattern_type, source, framework, name, description, enabled, priority, confidence)
VALUES (
'.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|\s*consumed samples:\s+(?P<ConsumedSamples>\d+)\s*\|\s*elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+mem\s+usages:\s+(?P<MemUsages>\d+\.\d+)\s+\|\s+throughput\s+per\s+GPU\s+\(TFLOP/s/GPU\):\s+(?P<TFLOPS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+tokens\s+per\s+GPU\s+\(tokens/s/GPU\):\s+(?P<TokensPerGPU>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|\s+global\s+batch\s+size:\s+(?P<GlobalBatchSize>\d+(?:\.\d+)*)\s+\|\s+lm\s+loss:\s+(?P<LmLoss>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|\s+loss\s+scale:\s+(?P<LossScale>\d+(?:\.\d+)*)\s+\|\s+grad\s+norm:\s+(?P<GradNorm>\d+(?:\.\d+)*)\s+\|\s+num\s+zeros:\s(?P<NumZeros>\d+(?:\.\d+)*)\s+\|\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\s*\|.*',
 'performance', 'migration', 'primus', 'primus-legacy',
 'Primus legacy format performance log (mem_usages without detailed GPU memory)',
 true, 80, 0.95
) ON CONFLICT (pattern_type, md5(pattern)) DO NOTHING;

-- ============================================================
-- Migrate training events
-- ============================================================

-- Primus training start
INSERT INTO training_log_pattern (pattern, pattern_type, event_subtype, source, framework, name, description, enabled, priority, confidence)
VALUES (
'training\s+\.\.\.',
 'training_event', 'start_training', 'migration', 'primus', 'primus-training-start',
 'Primus training start marker',
 true, 80, 1.0
) ON CONFLICT (pattern_type, md5(pattern)) DO NOTHING;

-- ============================================================
-- Migrate checkpoint events (from patch014)
-- ============================================================

INSERT INTO training_log_pattern (pattern, pattern_type, event_subtype, source, framework, name, description, enabled, priority, confidence)
VALUES
('saving checkpoint at iteration (?P<Iteration>\d+) to (?P<Path>\S+)',
 'checkpoint_event', 'start_saving', 'migration', 'primus', 'primus-checkpoint-start',
 'Primus checkpoint save start', true, 80, 0.95),
('successfully saved checkpoint at iteration (?P<Iteration>\d+).*?took (?P<DurationMs>\d+)\s*ms',
 'checkpoint_event', 'end_saving', 'migration', 'primus', 'primus-checkpoint-end',
 'Primus checkpoint save completion', true, 80, 0.95)
ON CONFLICT (pattern_type, md5(pattern)) DO NOTHING;

-- ============================================================
-- Migrate inference framework performance patterns (from patch036)
-- Note: these have simple named groups, not TrainingPerformance fields.
-- They are kept for completeness and future inference metric support.
-- ============================================================

-- vLLM throughput
INSERT INTO training_log_pattern (pattern, pattern_type, source, framework, name, description, enabled, priority, confidence)
VALUES (
'Throughput:\s+(?P<SamplesPerSecond>[\d.]+)\s+requests/s',
 'performance', 'migration', 'vllm', 'vllm-throughput',
 'vLLM throughput metrics',
 true, 70, 0.9
) ON CONFLICT (pattern_type, md5(pattern)) DO NOTHING;

-- vLLM latency
INSERT INTO training_log_pattern (pattern, pattern_type, source, framework, name, description, enabled, priority, confidence)
VALUES (
'Avg latency:\s+(?P<ElapsedTimePerIterationMS>[\d.]+)\s*(ms|s)',
 'performance', 'migration', 'vllm', 'vllm-latency',
 'vLLM latency metrics',
 true, 70, 0.9
) ON CONFLICT (pattern_type, md5(pattern)) DO NOTHING;
