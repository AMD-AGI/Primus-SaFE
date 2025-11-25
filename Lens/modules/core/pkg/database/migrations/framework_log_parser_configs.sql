-- Insert default framework log parser configurations

-- Primus framework configuration
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.primus',
    '{
        "name": "primus",
        "display_name": "Primus",
        "version": "1.0.0",
        "priority": 100,
        "enabled": true,
        "identify_patterns": [
            {
                "name": "primus-identifier",
                "pattern": "primus|Primus|PRIMUS",
                "description": "Identify Primus framework from log content",
                "enabled": true,
                "tags": ["identify"],
                "confidence": 0.7
            }
        ],
        "performance_patterns": [
            {
                "name": "primus-rocm-memory",
                "pattern": "\\\\.*iteration\\\\s+(?P<CurrentIteration>\\\\d+)\\\\s*/\\\\s*(?P<TargetIteration>\\\\d+)\\\\s*\\\\|\\\\s*consumed samples:\\\\s+(?P<ConsumedSamples>\\\\d+)\\\\s*\\\\|\\\\s*elapsed\\\\stime\\\\sper\\\\siteration\\\\s\\\\(ms\\\\):\\\\s+(?P<ElapsedTimePerIterationMS>\\\\d+(?:\\\\.\\\\d+)*)",
                "description": "Primus training performance log",
                "enabled": true,
                "tags": ["performance", "rocm"],
                "confidence": 0.8
            }
        ],
        "training_events": {
            "start_training": [
                {
                    "name": "primus-start-training",
                    "pattern": "training\\\\s*\\\\.\\\\.\\\\.",
                    "description": "Primus training start marker",
                    "enabled": true,
                    "tags": ["lifecycle"],
                    "confidence": 0.9
                }
            ]
        },
        "checkpoint_events": {
            "start_saving": [
                {
                    "name": "primus-checkpoint-start",
                    "pattern": "saving checkpoint at iteration (?P<Iteration>\\\\d+) to (?P<Path>\\\\S+)",
                    "description": "Primus checkpoint save start",
                    "enabled": true,
                    "tags": ["checkpoint"],
                    "confidence": 0.95
                }
            ],
            "end_saving": [
                {
                    "name": "primus-checkpoint-end",
                    "pattern": "successfully saved checkpoint at iteration (?P<Iteration>\\\\d+).*?took (?P<DurationMs>\\\\d+)\\\\s*ms",
                    "description": "Primus checkpoint save completion",
                    "enabled": true,
                    "tags": ["checkpoint"],
                    "confidence": 0.95
                }
            ]
        },
        "extensions": {},
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Log parsing patterns for Primus framework',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- DeepSpeed framework configuration (placeholder)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.deepspeed',
    '{
        "name": "deepspeed",
        "display_name": "DeepSpeed",
        "version": "1.0.0",
        "priority": 90,
        "enabled": true,
        "identify_patterns": [
            {
                "name": "deepspeed-identifier",
                "pattern": "deepspeed|DeepSpeed|DEEPSPEED",
                "description": "Identify DeepSpeed framework from log content",
                "enabled": true,
                "tags": ["identify"],
                "confidence": 0.7
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "checkpoint_events": {
            "start_saving": [],
            "end_saving": []
        },
        "extensions": {},
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Log parsing patterns for DeepSpeed framework',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- Megatron framework configuration (placeholder)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.megatron',
    '{
        "name": "megatron",
        "display_name": "Megatron-LM",
        "version": "1.0.0",
        "priority": 80,
        "enabled": true,
        "identify_patterns": [
            {
                "name": "megatron-identifier",
                "pattern": "megatron|Megatron|MEGATRON",
                "description": "Identify Megatron framework from log content",
                "enabled": true,
                "tags": ["identify"],
                "confidence": 0.7
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "checkpoint_events": {
            "start_saving": [],
            "end_saving": []
        },
        "extensions": {},
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Log parsing patterns for Megatron framework',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

