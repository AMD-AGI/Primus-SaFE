-- 假数据：用于在不调用 Claw 创建链路的情况下，验证 GET /optimization/tasks 等读接口。
-- 数据库：primus-safe-db（以集群内实际 PG 实例为准；以下为 core42 管理面 mj62 示例）。
-- 幂等：同一 id 重复执行会先删再插。
--
-- 在跳板机上执行示例：
--   kubectl exec -n primus-safe primus-safe-db-mj62-0 -c database -- \
--     psql -U postgres -d primus-safe-db -f - < seed-optimization-fake-task.sql

BEGIN;

DELETE FROM optimization_event WHERE task_id = 'opt-fake-core42-001';
DELETE FROM optimization_task WHERE id = 'opt-fake-core42-001';

INSERT INTO optimization_task (
  id,
  display_name,
  workspace,
  user_id,
  user_name,
  model_id,
  model_path,
  mode,
  framework,
  precision,
  tp,
  ep,
  gpu_type,
  isl,
  osl,
  concurrency,
  kernel_backends,
  geak_step_limit,
  claw_session_id,
  prompt,
  status,
  current_phase,
  message,
  created_at,
  updated_at,
  is_deleted
) VALUES (
  'opt-fake-core42-001',
  'Fake seed (API smoke test)',
  'core42-sandbox',
  'seed',
  'seed',
  'minimax-m2-5-nvfp4-gjcv5',
  '/wekafs/models/nvidia-MiniMax-M2.5-NVFP4',
  'claw',
  'sglang',
  'bf16',
  1,
  1,
  'MI300',
  0,
  0,
  0,
  '[]',
  0,
  '',
  '',
  'Succeeded',
  10,
  'manually seeded; claw_session_id empty so artifacts/list will return 400',
  NOW(),
  NOW(),
  FALSE
);

-- 可选：一条持久化事件，便于 GET /tasks/:id/events 重放片段测试
INSERT INTO optimization_event (
  event_id,
  task_id,
  type,
  payload,
  seq,
  timestamp,
  created_at
) VALUES (
  'evt-fake-001',
  'opt-fake-core42-001',
  'status',
  '{"note":"fake replay row"}',
  1,
  (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT,
  NOW()
);

COMMIT;
