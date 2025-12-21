-- Inference Framework Configurations
-- This migration adds configuration for inference frameworks (vLLM, TGI, Triton, etc.)
-- Version: 1.0.0

-- ============================================================================
-- vLLM Inference Framework Configuration
-- ============================================================================

INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.vllm',
    '{
        "name": "vllm",
        "display_name": "vLLM",
        "version": "1.0.0",
        "priority": 90,
        "enabled": true,
        "type": "inference",
        "identify_patterns": [
            {
                "name": "vllm-import",
                "pattern": "vllm|from vllm import|import vllm",
                "description": "vLLM import statement",
                "enabled": true,
                "tags": ["import", "inference"],
                "confidence": 0.9
            },
            {
                "name": "vllm-server-log",
                "pattern": "vLLM|vllm\\.entrypoints|AsyncLLMEngine|LLMEngine",
                "description": "vLLM server log patterns",
                "enabled": true,
                "tags": ["server", "inference"],
                "confidence": 0.85
            }
        ],
        "performance_patterns": [
            {
                "name": "vllm-throughput",
                "pattern": "Throughput:\\s+(?P<throughput>[\\d.]+)\\s+requests/s",
                "description": "vLLM throughput metrics",
                "enabled": true,
                "tags": ["performance", "throughput"],
                "confidence": 0.9
            },
            {
                "name": "vllm-latency",
                "pattern": "Avg latency:\\s+(?P<latency>[\\d.]+)\\s*(ms|s)",
                "description": "vLLM latency metrics",
                "enabled": true,
                "tags": ["performance", "latency"],
                "confidence": 0.9
            }
        ],
        "inference_patterns": {
            "process_patterns": [
                {
                    "name": "vllm-server-process",
                    "pattern": "vllm\\.entrypoints|python.*-m\\s+vllm|vllm\\.engine",
                    "description": "vLLM server process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.95
                }
            ],
            "ports": [8000],
            "env_patterns": [
                {
                    "name": "vllm-env-vars",
                    "pattern": "^VLLM_.*",
                    "description": "vLLM environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.8
                }
            ],
            "image_patterns": [
                {
                    "name": "vllm-official-image",
                    "pattern": "vllm/vllm-openai|vllm/vllm",
                    "description": "vLLM official container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.95
                },
                {
                    "name": "vllm-custom-image",
                    "pattern": ".*vllm.*",
                    "description": "Custom vLLM container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.7
                }
            ],
            "cmdline_patterns": [
                {
                    "name": "vllm-api-server",
                    "pattern": "vllm\\.entrypoints\\.openai\\.api_server|--served-model-name",
                    "description": "vLLM API server command line",
                    "enabled": true,
                    "tags": ["cmdline"],
                    "confidence": 0.9
                }
            ],
            "health_endpoint": "/health"
        },
        "extensions": {
            "model_loading_mode": "eager",
            "supports_openai_api": true,
            "default_port": 8000
        },
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'vLLM inference framework configuration',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- ============================================================================
-- TGI (Text Generation Inference) Framework Configuration
-- ============================================================================

INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.tgi',
    '{
        "name": "tgi",
        "display_name": "Text Generation Inference (TGI)",
        "version": "1.0.0",
        "priority": 85,
        "enabled": true,
        "type": "inference",
        "identify_patterns": [
            {
                "name": "tgi-server-log",
                "pattern": "text-generation-inference|text_generation_server|TGI",
                "description": "TGI server log patterns",
                "enabled": true,
                "tags": ["server", "inference"],
                "confidence": 0.9
            }
        ],
        "performance_patterns": [
            {
                "name": "tgi-request-metrics",
                "pattern": "request_count|batch_size|queue_size",
                "description": "TGI request metrics",
                "enabled": true,
                "tags": ["performance"],
                "confidence": 0.8
            }
        ],
        "inference_patterns": {
            "process_patterns": [
                {
                    "name": "tgi-launcher",
                    "pattern": "text-generation-launcher|text-generation-router",
                    "description": "TGI launcher process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.95
                },
                {
                    "name": "tgi-server",
                    "pattern": "text_generation_server",
                    "description": "TGI server process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.9
                }
            ],
            "ports": [80, 8080, 3000],
            "env_patterns": [
                {
                    "name": "tgi-model-env",
                    "pattern": "^(MODEL_ID|HF_TOKEN|HUGGING_FACE_HUB_TOKEN)",
                    "description": "TGI model environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.7
                },
                {
                    "name": "tgi-config-env",
                    "pattern": "^(MAX_BATCH_PREFILL_TOKENS|MAX_INPUT_LENGTH|MAX_TOTAL_TOKENS)",
                    "description": "TGI configuration environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.85
                }
            ],
            "image_patterns": [
                {
                    "name": "tgi-official-image",
                    "pattern": "ghcr\\.io/huggingface/text-generation-inference",
                    "description": "TGI official container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.95
                },
                {
                    "name": "tgi-custom-image",
                    "pattern": ".*text-generation-inference.*|.*tgi.*",
                    "description": "Custom TGI container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.7
                }
            ],
            "cmdline_patterns": [
                {
                    "name": "tgi-server-cmd",
                    "pattern": "--model-id|--max-batch-prefill-tokens|--quantize",
                    "description": "TGI server command line arguments",
                    "enabled": true,
                    "tags": ["cmdline"],
                    "confidence": 0.85
                }
            ],
            "health_endpoint": "/health"
        },
        "extensions": {
            "supports_flash_attention": true,
            "supports_quantization": true,
            "default_port": 80
        },
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Text Generation Inference (TGI) framework configuration',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- ============================================================================
-- Triton Inference Server Framework Configuration
-- ============================================================================

INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.triton',
    '{
        "name": "triton",
        "display_name": "Triton Inference Server",
        "version": "1.0.0",
        "priority": 85,
        "enabled": true,
        "type": "inference",
        "identify_patterns": [
            {
                "name": "triton-server-log",
                "pattern": "tritonserver|Triton Inference Server|TRITON",
                "description": "Triton server log patterns",
                "enabled": true,
                "tags": ["server", "inference"],
                "confidence": 0.9
            }
        ],
        "performance_patterns": [
            {
                "name": "triton-inference-stats",
                "pattern": "Inference count|execution count|cumulative time",
                "description": "Triton inference statistics",
                "enabled": true,
                "tags": ["performance"],
                "confidence": 0.85
            }
        ],
        "inference_patterns": {
            "process_patterns": [
                {
                    "name": "triton-server-process",
                    "pattern": "tritonserver",
                    "description": "Triton server process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.95
                }
            ],
            "ports": [8000, 8001, 8002],
            "env_patterns": [
                {
                    "name": "triton-env-vars",
                    "pattern": "^TRITON_.*",
                    "description": "Triton environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.85
                }
            ],
            "image_patterns": [
                {
                    "name": "triton-official-image",
                    "pattern": "nvcr\\.io/nvidia/tritonserver",
                    "description": "Triton official container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.95
                },
                {
                    "name": "triton-custom-image",
                    "pattern": ".*tritonserver.*|.*triton.*inference.*",
                    "description": "Custom Triton container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.7
                }
            ],
            "cmdline_patterns": [
                {
                    "name": "triton-server-cmd",
                    "pattern": "--model-repository|--model-control-mode|--strict-model-config",
                    "description": "Triton server command line arguments",
                    "enabled": true,
                    "tags": ["cmdline"],
                    "confidence": 0.9
                }
            ],
            "health_endpoint": "/v2/health/ready"
        },
        "extensions": {
            "supports_grpc": true,
            "supports_http": true,
            "http_port": 8000,
            "grpc_port": 8001,
            "metrics_port": 8002
        },
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Triton Inference Server framework configuration',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- ============================================================================
-- TensorRT-LLM Framework Configuration
-- ============================================================================

INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.tensorrt-llm',
    '{
        "name": "tensorrt-llm",
        "display_name": "TensorRT-LLM",
        "version": "1.0.0",
        "priority": 80,
        "enabled": true,
        "type": "inference",
        "identify_patterns": [
            {
                "name": "trtllm-import",
                "pattern": "tensorrt_llm|from tensorrt_llm import",
                "description": "TensorRT-LLM import statement",
                "enabled": true,
                "tags": ["import", "inference"],
                "confidence": 0.9
            }
        ],
        "performance_patterns": [],
        "inference_patterns": {
            "process_patterns": [
                {
                    "name": "trtllm-process",
                    "pattern": "tensorrt_llm|trtllm",
                    "description": "TensorRT-LLM process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.85
                }
            ],
            "ports": [8000],
            "env_patterns": [
                {
                    "name": "trtllm-env-vars",
                    "pattern": "^TRTLLM_.*",
                    "description": "TensorRT-LLM environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.8
                }
            ],
            "image_patterns": [
                {
                    "name": "trtllm-official-image",
                    "pattern": "nvcr\\.io/nvidia/tensorrt",
                    "description": "TensorRT official container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.9
                }
            ],
            "cmdline_patterns": [],
            "health_endpoint": "/health"
        },
        "extensions": {
            "supports_nvidia_gpu": true
        },
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'TensorRT-LLM inference framework configuration',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- ============================================================================
-- Ray Serve Framework Configuration
-- ============================================================================

INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.ray-serve',
    '{
        "name": "ray-serve",
        "display_name": "Ray Serve",
        "version": "1.0.0",
        "priority": 75,
        "enabled": true,
        "type": "inference",
        "identify_patterns": [
            {
                "name": "ray-serve-import",
                "pattern": "ray\\.serve|from ray import serve",
                "description": "Ray Serve import statement",
                "enabled": true,
                "tags": ["import", "inference"],
                "confidence": 0.85
            }
        ],
        "performance_patterns": [],
        "inference_patterns": {
            "process_patterns": [
                {
                    "name": "ray-serve-process",
                    "pattern": "ray::SERVE|serve\\.run|serve\\.deployment",
                    "description": "Ray Serve process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.8
                }
            ],
            "ports": [8000],
            "env_patterns": [
                {
                    "name": "ray-env-vars",
                    "pattern": "^RAY_.*",
                    "description": "Ray environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.7
                }
            ],
            "image_patterns": [
                {
                    "name": "ray-official-image",
                    "pattern": "rayproject/ray",
                    "description": "Ray official container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.85
                }
            ],
            "cmdline_patterns": [
                {
                    "name": "ray-serve-cmd",
                    "pattern": "serve\\.run|serve\\.deployment|@serve\\.deployment",
                    "description": "Ray Serve command line patterns",
                    "enabled": true,
                    "tags": ["cmdline"],
                    "confidence": 0.8
                }
            ],
            "health_endpoint": "/-/healthz"
        },
        "extensions": {
            "supports_autoscaling": true,
            "supports_batching": true
        },
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Ray Serve inference framework configuration',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

