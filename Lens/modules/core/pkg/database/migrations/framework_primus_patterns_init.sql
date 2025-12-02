-- Primus Framework Log Patterns Initialization
-- This script initializes the default Primus framework log patterns configuration
-- Version: 1.0.0

BEGIN;

-- Insert Primus framework patterns configuration into system_config
INSERT INTO system_config (key, value, description, created_at, updated_at) VALUES
('training.log.parser.framework.primus', '{
  "name": "primus",
  "display_name": "Primus",
  "version": "1.0.0",
  "priority": 80,
  "enabled": true,
  "identify_patterns": [
    {
      "name": "primus-iteration-log",
      "pattern": "iteration\\\\s+\\\\d+\\\\s*/\\\\s*\\\\d+.*throughput\\\\s+per\\\\s+GPU",
      "description": "Primus iteration performance log identifier",
      "enabled": true,
      "tags": ["performance", "iteration"],
      "confidence": 0.9
    },
    {
      "name": "primus-trainer",
      "pattern": "PrimusTrainer|primus\\\\.distributed",
      "description": "Primus trainer initialization",
      "enabled": true,
      "tags": ["framework"],
      "confidence": 0.85
    }
  ],
  "performance_patterns": [
    {
      "name": "primus-rocm-memory",
      "pattern": "\\\\..*iteration\\\\s+(?P<CurrentIteration>\\\\d+)\\\\s*/\\\\s*(?P<TargetIteration>\\\\d+)\\\\s*\\\\|\\\\s*consumed samples:\\\\s+(?P<ConsumedSamples>\\\\d+)\\\\s*\\\\|\\\\s*elapsed\\\\stime\\\\sper\\\\siteration\\\\s\\\\(ms\\\\):\\\\s+(?P<ElapsedTimePerIterationMS>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s+rocm\\\\s+mem\\\\s+usage/free/total/usage_ratio:\\\\s+(?P<MemUsage>\\\\d+\\\\.\\\\d+)GB/(?P<MemFree>\\\\d+\\\\.\\\\d+)GB/(?P<MemTotal>\\\\d+\\\\.\\\\d+)GB/(?P<MemUsageRatio>\\\\d+\\\\.\\\\d+)%\\\\s+\\\\|\\\\s+throughput\\\\s+per\\\\s+GPU\\\\s+\\\\(TFLOP/s/GPU\\\\):\\\\s+(?P<TFLOPS>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s+tokens\\\\s+per\\\\s+GPU\\\\s+\\\\(tokens/s/GPU\\\\):\\\\s+(?P<TokensPerGPU>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s*learning\\\\s+rate:\\\\s+(?P<LearningRate>[+-]?\\\\d+(?:\\\\.\\\\d+)?(?:[Ee][+-]?\\\\d+)?)\\\\s*\\\\|\\\\s+global\\\\s+batch\\\\s+size:\\\\s+(?P<GlobalBatchSize>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+lm\\\\s+loss:\\\\s+(?P<LmLoss>[+-]?\\\\d+(?:\\\\.\\\\d+)?(?:[Ee][+-]?\\\\d+)?)\\\\s+\\\\|\\\\s+loss\\\\s+scale:\\\\s+(?P<LossScale>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+grad\\\\s+norm:\\\\s+(?P<GradNorm>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+num\\\\s+zeros:\\\\s(?P<NumZeros>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+number\\\\s+of\\\\s+skipped\\\\s+iterations:\\\\s+(?P<SkippedIterationsNumber>\\\\d+)\\\\s+\\\\|\\\\s+number\\\\s+of\\\\s+nan\\\\s+iterations:\\\\s+(?P<NanIterationsNumber>\\\\d+)\\\\..*",
      "description": "Primus performance log with ROCm memory metrics",
      "enabled": true,
      "tags": ["performance", "rocm", "memory"],
      "confidence": 1.0
    },
    {
      "name": "primus-hip-memory",
      "pattern": "\\\\..*iteration\\\\s+(?P<CurrentIteration>\\\\d+)\\\\s*/\\\\s*(?P<TargetIteration>\\\\d+)\\\\s*\\\\|\\\\s*consumed samples:\\\\s+(?P<ConsumedSamples>\\\\d+)\\\\s*\\\\|\\\\s*elapsed\\\\stime\\\\sper\\\\siteration\\\\s\\\\(ms\\\\):\\\\s+(?P<ElapsedTimePerIterationMS>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s+hip\\\\s+mem\\\\s+usage/free/total/usage_ratio:\\\\s+(?P<MemUsage>\\\\d+\\\\.\\\\d+)GB/(?P<MemFree>\\\\d+\\\\.\\\\d+)GB/(?P<MemTotal>\\\\d+\\\\.\\\\d+)GB/(?P<MemUsageRatio>\\\\d+\\\\.\\\\d+)%\\\\s+\\\\|\\\\s+throughput\\\\s+per\\\\s+GPU\\\\s+\\\\(TFLOP/s/GPU\\\\):\\\\s+(?P<TFLOPS>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s+tokens\\\\s+per\\\\s+GPU\\\\s+\\\\(tokens/s/GPU\\\\):\\\\s+(?P<TokensPerGPU>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s*learning\\\\s+rate:\\\\s+(?P<LearningRate>[+-]?\\\\d+(?:\\\\.\\\\d+)?(?:[Ee][+-]?\\\\d+)?)\\\\s*\\\\|\\\\s+global\\\\s+batch\\\\s+size:\\\\s+(?P<GlobalBatchSize>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+lm\\\\s+loss:\\\\s+(?P<LmLoss>[+-]?\\\\d+(?:\\\\.\\\\d+)?(?:[Ee][+-]?\\\\d+)?)\\\\s+\\\\|\\\\s+loss\\\\s+scale:\\\\s+(?P<LossScale>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+grad\\\\s+norm:\\\\s+(?P<GradNorm>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+num\\\\s+zeros:\\\\s(?P<NumZeros>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+number\\\\s+of\\\\s+skipped\\\\s+iterations:\\\\s+(?P<SkippedIterationsNumber>\\\\d+)\\\\s+\\\\|\\\\s+number\\\\s+of\\\\s+nan\\\\s+iterations:\\\\s+(?P<NanIterationsNumber>\\\\d+)\\\\..*",
      "description": "Primus performance log with HIP memory metrics",
      "enabled": true,
      "tags": ["performance", "hip", "memory"],
      "confidence": 1.0
    },
    {
      "name": "primus-hip-memory-v2",
      "pattern": "\\\\..*iteration\\\\s+(?P<CurrentIteration>\\\\d+)\\\\s*/\\\\s*(?P<TargetIteration>\\\\d+)\\\\s*\\\\|\\\\s*consumed samples:\\\\s+(?P<ConsumedSamples>\\\\d+)\\\\s*\\\\|\\\\s*elapsed\\\\stime\\\\sper\\\\siteration\\\\s\\\\(ms\\\\):\\\\s+(?P<ElapsedTimePerIterationMS>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s+hip\\\\s+mem\\\\s+usage/free/total/usage_ratio:\\\\s+(?P<MemUsage>\\\\d+\\\\.\\\\d+)GB/(?P<MemFree>\\\\d+\\\\.\\\\d+)GB/(?P<MemTotal>\\\\d+\\\\.\\\\d+)GB/(?P<MemUsageRatio>\\\\d+\\\\.\\\\d+)%\\\\s+\\\\|\\\\s+throughput\\\\s+per\\\\s+GPU\\\\s+\\\\(TFLOP/s/GPU\\\\):\\\\s+(?P<TFLOPS>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s+tokens\\\\s+per\\\\s+GPU\\\\s+\\\\(tokens/s/GPU\\\\):\\\\s+(?P<TokensPerGPU>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s*learning\\\\s+rate:\\\\s+(?P<LearningRate>[+-]?\\\\d+(?:\\\\.\\\\d+)?(?:[Ee][+-]?\\\\d+)?)\\\\s*\\\\|\\\\s+global\\\\s+batch\\\\s+size:\\\\s+(?P<GlobalBatchSize>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+lm\\\\s+loss:\\\\s+(?P<LmLoss>[+-]?\\\\d+(?:\\\\.\\\\d+)?(?:[Ee][+-]?\\\\d+)?)\\\\s+\\\\|\\\\s+loss\\\\s+scale:\\\\s+(?P<LossScale>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+grad\\\\s+norm:\\\\s+(?P<GradNorm>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+number\\\\s+of\\\\s+skipped\\\\s+iterations:\\\\s+(?P<SkippedIterationsNumber>\\\\d+)\\\\s+\\\\|\\\\s+number\\\\s+of\\\\s+nan\\\\s+iterations:\\\\s+(?P<NanIterationsNumber>\\\\d+)\\\\s*\\\\|.*",
      "description": "Primus performance log with HIP memory metrics (v2 - without num zeros field)",
      "enabled": true,
      "tags": ["performance", "hip", "memory"],
      "confidence": 1.0
    },
    {
      "name": "primus-legacy",
      "pattern": "\\\\..*iteration\\\\s+(?P<CurrentIteration>\\\\d+)\\\\s*/\\\\s*(?P<TargetIteration>\\\\d+)\\\\s*\\\\|\\\\s*consumed samples:\\\\s+(?P<ConsumedSamples>\\\\d+)\\\\s*\\\\|\\\\s*elapsed\\\\stime\\\\sper\\\\siteration\\\\s\\\\(ms\\\\):\\\\s+(?P<ElapsedTimePerIterationMS>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s+mem\\\\s+usages:\\\\s+(?P<MemUsages>\\\\d+\\\\.\\\\d+)\\\\s+\\\\|\\\\s+throughput\\\\s+per\\\\s+GPU\\\\s+\\\\(TFLOP/s/GPU\\\\):\\\\s+(?P<TFLOPS>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s+tokens\\\\s+per\\\\s+GPU\\\\s+\\\\(tokens/s/GPU\\\\):\\\\s+(?P<TokensPerGPU>\\\\d+(?:\\\\.\\\\d+)*)/\\\\d+(?:\\\\.\\\\d+)*\\\\s+\\\\|\\\\s+learning\\\\s+rate:\\\\s+(?P<LearningRate>[+-]?\\\\d+(?:\\\\.\\\\d+)?(?:[Ee][+-]?\\\\d+)?)\\\\s+\\\\|\\\\s+global\\\\s+batch\\\\s+size:\\\\s+(?P<GlobalBatchSize>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+lm\\\\s+loss:\\\\s+(?P<LmLoss>[+-]?\\\\d+(?:\\\\.\\\\d+)?(?:[Ee][+-]?\\\\d+)?)\\\\s+\\\\|\\\\s+loss\\\\s+scale:\\\\s+(?P<LossScale>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+grad\\\\s+norm:\\\\s+(?P<GradNorm>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+num\\\\s+zeros:\\\\s(?P<NumZeros>\\\\d+(?:\\\\.\\\\d+)*)\\\\s+\\\\|\\\\s+number\\\\s+of\\\\s+skipped\\\\s+iterations:\\\\s+(?P<SkippedIterationsNumber>\\\\d+)\\\\s+\\\\|\\\\s+number\\\\s+of\\\\s+nan\\\\s+iterations:\\\\s+(?P<NanIterationsNumber>\\\\d+)\\\\..*",
      "description": "Primus legacy format performance log (without detailed memory metrics)",
      "enabled": true,
      "tags": ["performance", "legacy"],
      "confidence": 0.95
    }
  ],
  "training_events": {
    "start_training": [
      {
        "name": "training-start",
        "pattern": "training\\\\s+\\\\.\\\\.\\\\.",
        "description": "Training started marker",
        "enabled": true,
        "tags": ["lifecycle", "start"],
        "confidence": 1.0
      }
    ],
    "end_training": [],
    "pause_training": [],
    "resume_training": []
  },
  "checkpoint_events": {
    "start_saving": [],
    "end_saving": [],
    "loading": []
  },
  "extensions": {
    "supports_rocm": true,
    "supports_hip": true,
    "memory_tracking": true
  },
  "updated_at": "2024-01-01T00:00:00Z",
  "created_at": "2024-01-01T00:00:00Z"
}'::jsonb, 'Primus framework log patterns configuration', NOW(), NOW())
ON CONFLICT (key) 
DO UPDATE SET 
  value = EXCLUDED.value,
  updated_at = NOW(),
  description = EXCLUDED.description;

COMMIT;

