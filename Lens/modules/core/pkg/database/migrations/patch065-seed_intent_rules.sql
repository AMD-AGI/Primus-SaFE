-- =============================================================================
-- Patch 065: Seed intent_rule table with detection heuristic patterns
-- =============================================================================
-- Migrates hardcoded image/cmdline/env/pip patterns from evaluator.go into
-- the intent_rule table as promoted rules. This enables rule management via
-- DB without code redeployment.
-- =============================================================================

-- Guard: only insert if table is empty (first-time seeding)
DO $$
BEGIN
  IF (SELECT count(*) FROM intent_rule) > 0 THEN
    RAISE NOTICE 'intent_rule already has data, skipping seed';
    RETURN;
  END IF;

  -- =========================================================================
  -- Image-based rules (dimension = 'image')
  -- =========================================================================
  INSERT INTO intent_rule (detects_field, detects_value, dimension, pattern, confidence, status, reasoning) VALUES
    ('category', 'inference',      'image', '(?i)vllm',                           0.7, 'promoted', 'vLLM serving framework image'),
    ('category', 'inference',      'image', '(?i)text-generation-inference|tgi',   0.7, 'promoted', 'HuggingFace TGI image'),
    ('category', 'inference',      'image', '(?i)sglang',                          0.6, 'promoted', 'SGLang serving framework image'),
    ('category', 'inference',      'image', '(?i)triton.?server',                  0.6, 'promoted', 'NVIDIA Triton Inference Server image'),
    ('category', 'serving',        'image', '(?i)torchserve',                      0.5, 'promoted', 'TorchServe serving image'),
    ('category', 'pre_training',   'image', '(?i)megatron',                        0.6, 'promoted', 'Megatron training framework image'),
    ('category', 'pre_training',   'image', '(?i)nemo.*training',                  0.5, 'promoted', 'NVIDIA NeMo training image'),
    ('category', 'pre_training',   'image', '(?i)deepspeed',                       0.4, 'promoted', 'DeepSpeed training framework image'),
    ('category', 'evaluation',     'image', '(?i)lm.?eval',                        0.7, 'promoted', 'LM Evaluation Harness image');

  -- =========================================================================
  -- Cmdline-based rules (dimension = 'cmdline')
  -- =========================================================================
  INSERT INTO intent_rule (detects_field, detects_value, dimension, pattern, confidence, status, reasoning) VALUES
    -- Serving / inference frameworks
    ('category', 'inference',      'cmdline', '(?i)vllm\.entrypoints|python\s+-m\s+vllm',  0.8, 'promoted', 'vLLM entrypoint in cmdline'),
    ('category', 'inference',      'cmdline', '(?i)text-generation-launcher',               0.8, 'promoted', 'HuggingFace TGI launcher'),
    ('category', 'inference',      'cmdline', '(?i)sglang\.launch_server',                  0.7, 'promoted', 'SGLang server launch command'),

    -- Training frameworks (general)
    ('category', 'pre_training',   'cmdline', '(?i)torchrun|torch\.distributed\.launch',    0.4, 'promoted', 'PyTorch distributed launcher (general training)'),
    ('category', 'pre_training',   'cmdline', '(?i)deepspeed\s',                            0.5, 'promoted', 'DeepSpeed launcher'),
    ('category', 'fine_tuning',    'cmdline', '(?i)accelerate\s+launch',                    0.4, 'promoted', 'HuggingFace Accelerate launcher'),
    ('category', 'fine_tuning',    'cmdline', '(?i)--do_train|--training_args',              0.6, 'promoted', 'HuggingFace Trainer --do_train flag'),
    ('category', 'evaluation',     'cmdline', '(?i)--do_eval\b',                            0.5, 'promoted', 'HuggingFace Trainer --do_eval flag'),

    -- Megatron-style commands
    ('category', 'pre_training',   'cmdline', '(?i)\bmegatron\s+(pt|pretrain|sft|finetune|train)\b', 0.7, 'promoted', 'Megatron CLI subcommand'),
    ('category', 'pre_training',   'cmdline', '(?i)megatron.*--model\b',                    0.5, 'promoted', 'Megatron model training argument'),

    -- Primus CLI
    ('category', 'pre_training',   'cmdline', '(?i)primus/cli/main\.py\s+train\s+pretrain', 0.7, 'promoted', 'Primus CLI pretrain command'),
    ('category', 'fine_tuning',    'cmdline', '(?i)primus/cli/main\.py\s+train\s+sft',      0.7, 'promoted', 'Primus CLI SFT command'),
    ('category', 'fine_tuning',    'cmdline', '(?i)primus/cli/main\.py\s+train',            0.5, 'promoted', 'Primus CLI generic train command'),

    -- ms-swift
    ('category', 'fine_tuning',    'cmdline', '(?i)\bswift\s+(sft|pt|pretrain|finetune)\b', 0.6, 'promoted', 'ms-swift training command'),
    ('category', 'inference',      'cmdline', '(?i)\bswift\s+infer\b',                      0.6, 'promoted', 'ms-swift inference command'),

    -- Fine-tuning indicators
    ('category', 'fine_tuning',    'cmdline', '(?i)--lora_r|--use_peft|--peft_type',        0.7, 'promoted', 'PEFT/LoRA training arguments'),
    ('category', 'fine_tuning',    'cmdline', '(?i)sft_trainer|dpo_trainer|rlhf',            0.7, 'promoted', 'SFT/DPO/RLHF trainer module'),

    -- HuggingFace training arguments
    ('category', 'fine_tuning',    'cmdline', '(?i)--num_train_epochs|--per_device_train_batch_size', 0.5, 'promoted', 'HuggingFace TrainingArguments (epochs/batch_size)'),
    ('category', 'fine_tuning',    'cmdline', '(?i)--gradient_accumulation_steps|--warmup_steps',     0.4, 'promoted', 'HuggingFace TrainingArguments (grad_accum/warmup)'),

    -- Evaluation
    ('category', 'evaluation',     'cmdline', '(?i)lm_eval|lm-eval|evaluate\s+--model',     0.7, 'promoted', 'LM evaluation harness command'),

    -- Data processing
    ('category', 'data_processing','cmdline', '(?i)tokenize|preprocess.*dataset|data.*pipeline', 0.4, 'promoted', 'Data preprocessing/tokenization pipeline'),

    -- Benchmark / profiling / stress-testing
    ('category', 'benchmark',      'cmdline', '(?i)test_internode\.py|test_intranode\.py',   0.8, 'promoted', 'Network internode/intranode benchmark script'),
    ('category', 'benchmark',      'cmdline', '(?i)pressure.?test.?mode',                   0.8, 'promoted', 'Network pressure/stress test mode flag'),
    ('category', 'benchmark',      'cmdline', '(?i)ansible-playbook.*primusbench|ansible-playbook.*bench', 0.8, 'promoted', 'Ansible-driven benchmark automation (primusbench)'),
    ('category', 'benchmark',      'cmdline', '(?i)nccl.?test|rccl.?test|all_reduce_perf',  0.8, 'promoted', 'NCCL/RCCL collective communication test'),
    ('category', 'benchmark',      'cmdline', '(?i)superbench|gpu.?burn|cuda.?memcheck',    0.7, 'promoted', 'GPU burn/memcheck/superbench stress test'),
    ('category', 'benchmark',      'cmdline', '(?i)\bbenchmark\b|node_check\.yaml',         0.6, 'promoted', 'Generic benchmark keyword or node_check playbook');

  -- =========================================================================
  -- Pip-based rules (dimension = 'pip')
  -- =========================================================================
  INSERT INTO intent_rule (detects_field, detects_value, dimension, pattern, confidence, status, reasoning) VALUES
    ('category', 'fine_tuning',    'pip', '(?m)^trl==',                  0.6, 'promoted', 'TRL (transformer reinforcement learning) package installed'),
    ('category', 'fine_tuning',    'pip', '(?m)^peft==',                 0.5, 'promoted', 'PEFT (parameter-efficient fine-tuning) package installed'),
    ('category', 'inference',      'pip', '(?m)^vllm==',                 0.7, 'promoted', 'vLLM package installed'),
    ('category', 'inference',      'pip', '(?m)^text-generation-inference==', 0.7, 'promoted', 'TGI package installed'),
    ('category', 'inference',      'pip', '(?m)^sglang==',               0.6, 'promoted', 'SGLang package installed'),
    ('category', 'inference',      'pip', '(?m)^triton==',               0.4, 'promoted', 'Triton package installed'),
    ('category', 'pre_training',   'pip', '(?m)^deepspeed==',            0.4, 'promoted', 'DeepSpeed package installed'),
    ('category', 'pre_training',   'pip', '(?m)^megatron-core==',        0.6, 'promoted', 'Megatron-Core package installed'),
    ('category', 'fine_tuning',    'pip', '(?m)^lightning==',            0.3, 'promoted', 'PyTorch Lightning package installed'),
    ('category', 'evaluation',     'pip', '(?m)^lm-eval-harness==',     0.6, 'promoted', 'LM Evaluation Harness package installed'),
    ('category', 'fine_tuning',    'pip', '(?m)^transformers==',         0.3, 'promoted', 'HuggingFace Transformers package installed (general)'),
    ('category', 'evaluation',     'pip', '(?m)^evaluate==',            0.3, 'promoted', 'HuggingFace evaluate package installed');

  -- =========================================================================
  -- Env-key-based rules (dimension = 'env_key')
  -- =========================================================================
  INSERT INTO intent_rule (detects_field, detects_value, dimension, pattern, confidence, status, reasoning) VALUES
    ('category', 'pre_training',   'env_key', '^DEEPSPEED_ZERO_STAGE$',  0.5, 'promoted', 'DeepSpeed ZeRO stage environment variable set');

  RAISE NOTICE 'Seeded % intent rules', (SELECT count(*) FROM intent_rule);
END $$;
