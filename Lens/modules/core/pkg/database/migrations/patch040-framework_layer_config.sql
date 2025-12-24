-- ============================================================================
-- Migration: Add layer field to framework configurations for Detection V2
-- 
-- Detection V2 Multi-Layer Framework Architecture:
--   L1 (wrapper):       High-level training abstraction (primus, lightning)
--   L2 (orchestration): Distributed training / optimization (megatron, deepspeed)
--   L3 (runtime):       Base DL framework (pytorch, tensorflow, jax)
--   inference:          Inference serving (vllm, triton, tgi)
--
-- Layer Hierarchy Rules:
--   - Cross-layer combinations are valid (primus + megatron + pytorch)
--   - Same-layer combinations are conflicts (primus + lightning)
-- ============================================================================

-- ============================================================================
-- Part 1: Update existing inference framework configs with layer field
-- ============================================================================

-- Update vllm
UPDATE system_config 
SET value = jsonb_set(value::jsonb, '{layer}', '"inference"'),
    updated_at = NOW()
WHERE key = 'training.log.parser.framework.vllm';

-- Update triton
UPDATE system_config 
SET value = jsonb_set(value::jsonb, '{layer}', '"inference"'),
    updated_at = NOW()
WHERE key = 'training.log.parser.framework.triton';

-- Update tgi
UPDATE system_config 
SET value = jsonb_set(value::jsonb, '{layer}', '"inference"'),
    updated_at = NOW()
WHERE key = 'training.log.parser.framework.tgi';

-- Update tensorrt-llm
UPDATE system_config 
SET value = jsonb_set(value::jsonb, '{layer}', '"inference"'),
    updated_at = NOW()
WHERE key = 'training.log.parser.framework.tensorrt-llm';

-- Update ray-serve
UPDATE system_config 
SET value = jsonb_set(value::jsonb, '{layer}', '"inference"'),
    updated_at = NOW()
WHERE key = 'training.log.parser.framework.ray-serve';

-- ============================================================================
-- Part 2: Add Layer 1 (Wrapper) Framework Configs
-- ============================================================================

-- Primus (L1: wrapper)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.primus',
    '{
        "name": "primus",
        "display_name": "Primus Training Framework",
        "type": "training",
        "layer": "wrapper",
        "priority": 100,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "primus_cmdline",
                "pattern": "primus|primus-train|primus\\.train",
                "enabled": true,
                "confidence": 0.85,
                "description": "Primus command line patterns",
                "tags": ["cmdline", "training"]
            },
            {
                "name": "primus_import",
                "pattern": "from primus import|import primus",
                "enabled": true,
                "confidence": 0.9,
                "description": "Primus import statement",
                "tags": ["import", "training"]
            }
        ],
        "performance_patterns": [
            {
                "name": "primus_step",
                "pattern": "\\[Primus\\].*step\\s*(\\d+)",
                "enabled": true,
                "confidence": 0.8,
                "description": "Primus training step",
                "tags": ["performance"]
            }
        ],
        "training_events": {
            "start_training": [
                {
                    "name": "primus_start",
                    "pattern": "\\[Primus\\].*Training started|primus.*begin training",
                    "enabled": true,
                    "confidence": 0.85,
                    "description": "Primus training start"
                }
            ],
            "end_training": [
                {
                    "name": "primus_end",
                    "pattern": "\\[Primus\\].*Training completed|primus.*training finished",
                    "enabled": true,
                    "confidence": 0.85,
                    "description": "Primus training end"
                }
            ]
        },
        "extensions": {
            "supports_megatron": true,
            "supports_deepspeed": true
        },
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'Primus training framework configuration (wrapper layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"wrapper"'),
    updated_at = NOW();

-- PyTorch Lightning (L1: wrapper)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.lightning',
    '{
        "name": "lightning",
        "display_name": "PyTorch Lightning",
        "type": "training",
        "layer": "wrapper",
        "priority": 95,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "lightning_cmdline",
                "pattern": "lightning|pytorch_lightning|pl\\.",
                "enabled": true,
                "confidence": 0.8,
                "description": "Lightning command line patterns",
                "tags": ["cmdline", "training"]
            },
            {
                "name": "lightning_import",
                "pattern": "import pytorch_lightning|from pytorch_lightning import|import lightning",
                "enabled": true,
                "confidence": 0.85,
                "description": "Lightning import statement",
                "tags": ["import", "training"]
            }
        ],
        "performance_patterns": [
            {
                "name": "lightning_epoch",
                "pattern": "Epoch\\s*(\\d+).*Step\\s*(\\d+)",
                "enabled": true,
                "confidence": 0.75,
                "description": "Lightning epoch/step progress",
                "tags": ["performance"]
            }
        ],
        "training_events": {
            "start_training": [
                {
                    "name": "lightning_start",
                    "pattern": "GPU available.*TPU available|Trainer\\.fit\\(\\)",
                    "enabled": true,
                    "confidence": 0.7,
                    "description": "Lightning training start"
                }
            ]
        },
        "extensions": {},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'PyTorch Lightning framework configuration (wrapper layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"wrapper"'),
    updated_at = NOW();

-- HuggingFace Trainer (L1: wrapper)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.transformers_trainer',
    '{
        "name": "transformers_trainer",
        "display_name": "HuggingFace Trainer",
        "type": "training",
        "layer": "wrapper",
        "priority": 90,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "hf_trainer_import",
                "pattern": "from transformers import.*Trainer|TrainingArguments",
                "enabled": true,
                "confidence": 0.8,
                "description": "HuggingFace Trainer import",
                "tags": ["import", "training"]
            },
            {
                "name": "hf_trainer_config",
                "pattern": "transformers\\.Trainer|Seq2SeqTrainer|TrainingArguments",
                "enabled": true,
                "confidence": 0.75,
                "description": "HuggingFace Trainer usage",
                "tags": ["cmdline", "training"]
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "extensions": {},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'HuggingFace Trainer framework configuration (wrapper layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"wrapper"'),
    updated_at = NOW();

-- ============================================================================
-- Part 3: Add Layer 2 (Orchestration) Framework Configs
-- ============================================================================

-- Megatron (L2: orchestration)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.megatron',
    '{
        "name": "megatron",
        "display_name": "Megatron-LM",
        "type": "training",
        "layer": "orchestration",
        "priority": 90,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "megatron_cmdline",
                "pattern": "megatron|pretrain_gpt|megatron-lm|megatron_lm",
                "enabled": true,
                "confidence": 0.85,
                "description": "Megatron command line patterns",
                "tags": ["cmdline", "training"]
            },
            {
                "name": "megatron_import",
                "pattern": "from megatron import|import megatron|megatron\\.core",
                "enabled": true,
                "confidence": 0.9,
                "description": "Megatron import statement",
                "tags": ["import", "training"]
            }
        ],
        "performance_patterns": [
            {
                "name": "megatron_iteration",
                "pattern": "iteration\\s*(\\d+).*consumed samples",
                "enabled": true,
                "confidence": 0.85,
                "description": "Megatron iteration progress",
                "tags": ["performance"]
            },
            {
                "name": "megatron_tflops",
                "pattern": "TFLOPs:\\s*([\\d.]+)",
                "enabled": true,
                "confidence": 0.9,
                "description": "Megatron TFLOPs metric",
                "tags": ["performance"]
            }
        ],
        "training_events": {
            "start_training": [
                {
                    "name": "megatron_start",
                    "pattern": "training starting|iteration.*0",
                    "enabled": true,
                    "confidence": 0.7,
                    "description": "Megatron training start"
                }
            ]
        },
        "extensions": {
            "supports_tensor_parallel": true,
            "supports_pipeline_parallel": true
        },
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'Megatron-LM framework configuration (orchestration layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"orchestration"'),
    updated_at = NOW();

-- DeepSpeed (L2: orchestration)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.deepspeed',
    '{
        "name": "deepspeed",
        "display_name": "DeepSpeed",
        "type": "training",
        "layer": "orchestration",
        "priority": 85,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "deepspeed_cmdline",
                "pattern": "deepspeed|ds_config|deepspeed_config",
                "enabled": true,
                "confidence": 0.85,
                "description": "DeepSpeed command line patterns",
                "tags": ["cmdline", "training"]
            },
            {
                "name": "deepspeed_import",
                "pattern": "import deepspeed|from deepspeed import",
                "enabled": true,
                "confidence": 0.9,
                "description": "DeepSpeed import statement",
                "tags": ["import", "training"]
            }
        ],
        "performance_patterns": [
            {
                "name": "deepspeed_step",
                "pattern": "\\[DeepSpeed\\].*step\\s*(\\d+)",
                "enabled": true,
                "confidence": 0.8,
                "description": "DeepSpeed step progress",
                "tags": ["performance"]
            }
        ],
        "training_events": {
            "start_training": [
                {
                    "name": "deepspeed_init",
                    "pattern": "DeepSpeed.*initialized|deepspeed\\.initialize",
                    "enabled": true,
                    "confidence": 0.8,
                    "description": "DeepSpeed initialization"
                }
            ]
        },
        "extensions": {
            "supports_zero": true,
            "supports_offload": true
        },
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'DeepSpeed framework configuration (orchestration layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"orchestration"'),
    updated_at = NOW();

-- ColossalAI (L2: orchestration)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.colossalai',
    '{
        "name": "colossalai",
        "display_name": "ColossalAI",
        "type": "training",
        "layer": "orchestration",
        "priority": 80,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "colossalai_cmdline",
                "pattern": "colossalai|colossal",
                "enabled": true,
                "confidence": 0.8,
                "description": "ColossalAI command line patterns",
                "tags": ["cmdline", "training"]
            },
            {
                "name": "colossalai_import",
                "pattern": "import colossalai|from colossalai import",
                "enabled": true,
                "confidence": 0.85,
                "description": "ColossalAI import statement",
                "tags": ["import", "training"]
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "extensions": {},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'ColossalAI framework configuration (orchestration layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"orchestration"'),
    updated_at = NOW();

-- FSDP (L2: orchestration)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.fsdp',
    '{
        "name": "fsdp",
        "display_name": "PyTorch FSDP",
        "type": "training",
        "layer": "orchestration",
        "priority": 75,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "fsdp_cmdline",
                "pattern": "FullyShardedDataParallel|FSDP",
                "enabled": true,
                "confidence": 0.8,
                "description": "FSDP command line patterns",
                "tags": ["cmdline", "training"]
            },
            {
                "name": "fsdp_import",
                "pattern": "from torch.distributed.fsdp import|FullyShardedDataParallel",
                "enabled": true,
                "confidence": 0.85,
                "description": "FSDP import statement",
                "tags": ["import", "training"]
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "extensions": {},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'PyTorch FSDP framework configuration (orchestration layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"orchestration"'),
    updated_at = NOW();

-- Horovod (L2: orchestration)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.horovod',
    '{
        "name": "horovod",
        "display_name": "Horovod",
        "type": "training",
        "layer": "orchestration",
        "priority": 70,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "horovod_cmdline",
                "pattern": "horovodrun|horovod",
                "enabled": true,
                "confidence": 0.8,
                "description": "Horovod command line patterns",
                "tags": ["cmdline", "training"]
            },
            {
                "name": "horovod_import",
                "pattern": "import horovod|from horovod import|horovod\\.torch|horovod\\.tensorflow",
                "enabled": true,
                "confidence": 0.85,
                "description": "Horovod import statement",
                "tags": ["import", "training"]
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "extensions": {},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'Horovod framework configuration (orchestration layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"orchestration"'),
    updated_at = NOW();

-- ============================================================================
-- Part 4: Add Layer 3 (Runtime) Framework Configs
-- ============================================================================

-- PyTorch (L3: runtime)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.pytorch',
    '{
        "name": "pytorch",
        "display_name": "PyTorch",
        "type": "training",
        "layer": "runtime",
        "priority": 50,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "pytorch_cmdline",
                "pattern": "torch\\.distributed|torchrun|python.*-m.*torch",
                "enabled": true,
                "confidence": 0.7,
                "description": "PyTorch command line patterns",
                "tags": ["cmdline", "training"]
            },
            {
                "name": "pytorch_import",
                "pattern": "import torch|from torch import",
                "enabled": true,
                "confidence": 0.6,
                "description": "PyTorch import statement",
                "tags": ["import", "training"]
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "extensions": {},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'PyTorch framework configuration (runtime layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"runtime"'),
    updated_at = NOW();

-- TensorFlow (L3: runtime)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.tensorflow',
    '{
        "name": "tensorflow",
        "display_name": "TensorFlow",
        "type": "training",
        "layer": "runtime",
        "priority": 50,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "tensorflow_cmdline",
                "pattern": "tensorflow|tf\\.distribute",
                "enabled": true,
                "confidence": 0.7,
                "description": "TensorFlow command line patterns",
                "tags": ["cmdline", "training"]
            },
            {
                "name": "tensorflow_import",
                "pattern": "import tensorflow|from tensorflow import|import tf",
                "enabled": true,
                "confidence": 0.6,
                "description": "TensorFlow import statement",
                "tags": ["import", "training"]
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "extensions": {},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'TensorFlow framework configuration (runtime layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"runtime"'),
    updated_at = NOW();

-- JAX (L3: runtime)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.jax',
    '{
        "name": "jax",
        "display_name": "JAX",
        "type": "training",
        "layer": "runtime",
        "priority": 50,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "jax_cmdline",
                "pattern": "jax\\.|flax\\.",
                "enabled": true,
                "confidence": 0.7,
                "description": "JAX command line patterns",
                "tags": ["cmdline", "training"]
            },
            {
                "name": "jax_import",
                "pattern": "import jax|from jax import|import flax",
                "enabled": true,
                "confidence": 0.6,
                "description": "JAX import statement",
                "tags": ["import", "training"]
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "extensions": {},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'JAX framework configuration (runtime layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"runtime"'),
    updated_at = NOW();

-- PaddlePaddle (L3: runtime)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.paddle',
    '{
        "name": "paddle",
        "display_name": "PaddlePaddle",
        "type": "training",
        "layer": "runtime",
        "priority": 50,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "paddle_cmdline",
                "pattern": "paddle\\.distributed|paddlepaddle",
                "enabled": true,
                "confidence": 0.7,
                "description": "PaddlePaddle command line patterns",
                "tags": ["cmdline", "training"]
            },
            {
                "name": "paddle_import",
                "pattern": "import paddle|from paddle import",
                "enabled": true,
                "confidence": 0.6,
                "description": "PaddlePaddle import statement",
                "tags": ["import", "training"]
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "extensions": {},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'PaddlePaddle framework configuration (runtime layer)',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"runtime"'),
    updated_at = NOW();

-- ============================================================================
-- Part 5: Add additional inference frameworks with layer field
-- ============================================================================

-- SGLang (inference)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.sglang',
    '{
        "name": "sglang",
        "display_name": "SGLang",
        "type": "inference",
        "layer": "inference",
        "priority": 80,
        "enabled": true,
        "version": "1.0.0",
        "identify_patterns": [
            {
                "name": "sglang_import",
                "pattern": "sglang|from sglang import",
                "enabled": true,
                "confidence": 0.85,
                "description": "SGLang import statement",
                "tags": ["import", "inference"]
            }
        ],
        "inference_patterns": {
            "ports": [30000],
            "env_patterns": [],
            "image_patterns": [
                {
                    "name": "sglang_image",
                    "pattern": ".*sglang.*",
                    "enabled": true,
                    "confidence": 0.8,
                    "description": "SGLang container image"
                }
            ],
            "health_endpoint": "/health",
            "cmdline_patterns": [],
            "process_patterns": [
                {
                    "name": "sglang_process",
                    "pattern": "sglang\\.launch_server|python.*sglang",
                    "enabled": true,
                    "confidence": 0.9,
                    "description": "SGLang server process"
                }
            ]
        },
        "performance_patterns": [],
        "extensions": {},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
    }'::jsonb,
    'SGLang inference framework configuration',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = jsonb_set(EXCLUDED.value::jsonb, '{layer}', '"inference"'),
    updated_at = NOW();

-- ============================================================================
-- Verification query (can be removed in production)
-- ============================================================================
-- SELECT key, value->>'layer' as layer, value->>'type' as type, value->>'priority' as priority
-- FROM system_config 
-- WHERE key LIKE 'training.log.parser.framework.%'
-- ORDER BY value->>'layer', (value->>'priority')::int DESC;

