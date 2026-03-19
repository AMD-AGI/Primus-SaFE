# GitHub Workflow 功能上线计划 — tw-project1

## 版本信息

| 组件 | 新版本 Tag | 旧版本（回滚用） |
|------|-----------|-----------------|
| apiserver | `202603191519` | 上线前记录当前版本 |
| job-manager | `202603191501` | 上线前记录当前版本 |

镜像仓库：`docker.io/primussafe/`

## 前置条件

- [x] 代码已合并 main 最新提交（`31c53cc3`）
- [x] CI 构建通过
- [x] WorkflowTracker 初始化 + SyncJob 启动 + panic 保护已修复
- [ ] tw-project1 SSH 隧道可用
- [ ] kubectl 可访问 tw-project1

## 上线步骤

### 第一步：记录当前版本（用于回滚）

```bash
KUBECONFIG=~/workspace/kubeconfig kubectl config use-context tw-project1

# 记录当前镜像版本
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe get deployment primus-safe-apiserver \
  -o jsonpath='{.spec.template.spec.containers[0].image}' ; echo
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe get deployment primus-safe-job-manager \
  -o jsonpath='{.spec.template.spec.containers[0].image}' ; echo
```

将输出保存为 `ROLLBACK_APISERVER_IMAGE` 和 `ROLLBACK_JOB_MANAGER_IMAGE`。

### 第二步：执行数据库变更

在 SaFE 数据库中创建 7 张新的 GitHub Workflow 表。这些是新增表，不影响现有表。

```bash
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe run github-migration --rm -it --restart=Never \
  --image=postgres:16-alpine \
  --env="PGPASSWORD=<SaFE DB 密码>" \
  -- psql -h <SaFE DB Host> -U <SaFE DB User> -d <SaFE DB Name> -c "
-- 1. 采集配置表
create table if not exists github_collection_configs (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    github_owner VARCHAR(255) NOT NULL,
    github_repo VARCHAR(255) NOT NULL,
    workflow_patterns TEXT[] DEFAULT '{}',
    branch_patterns TEXT[] DEFAULT '{}',
    file_patterns TEXT[] DEFAULT '{}',
    enabled BOOLEAN DEFAULT TRUE,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
create index if not exists idx_gcc_owner_repo on github_collection_configs (github_owner, github_repo);
alter table github_collection_configs owner to \"primus-safe\";

-- 2. Workflow Run 记录表
create table if not exists github_workflow_runs (
    id SERIAL PRIMARY KEY,
    workload_id VARCHAR(64),
    cluster VARCHAR(64),
    github_run_id BIGINT,
    github_job_id BIGINT,
    workflow_name VARCHAR(500),
    github_owner VARCHAR(255),
    github_repo VARCHAR(255),
    head_branch VARCHAR(255),
    head_sha VARCHAR(64),
    status VARCHAR(50) DEFAULT 'running',
    conclusion VARCHAR(50),
    collection_status VARCHAR(50) DEFAULT 'pending',
    sync_status VARCHAR(50) DEFAULT 'pending',
    config_id INT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
create index if not exists idx_gwr_run_id on github_workflow_runs (github_run_id);
create index if not exists idx_gwr_workload on github_workflow_runs (workload_id);
create index if not exists idx_gwr_status on github_workflow_runs (status);
create index if not exists idx_gwr_sync on github_workflow_runs (sync_status);
alter table github_workflow_runs owner to \"primus-safe\";

-- 3. Run 详情表
create table if not exists github_workflow_run_details (
    id SERIAL PRIMARY KEY,
    run_id INT NOT NULL,
    github_run_id BIGINT NOT NULL,
    html_url TEXT,
    jobs_url TEXT,
    logs_url TEXT,
    event VARCHAR(50),
    trigger_actor VARCHAR(255),
    pull_request_number INT,
    workflow_path TEXT,
    raw_data JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
create index if not exists idx_gwrd_run on github_workflow_run_details (run_id);
alter table github_workflow_run_details owner to \"primus-safe\";

-- 4. Jobs 表
create table if not exists github_workflow_jobs (
    id SERIAL PRIMARY KEY,
    run_id INT NOT NULL,
    github_job_id BIGINT NOT NULL,
    name VARCHAR(500),
    status VARCHAR(50),
    conclusion VARCHAR(50),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    runner_name VARCHAR(255),
    runner_group_name VARCHAR(255),
    needs TEXT[],
    created_at TIMESTAMPTZ DEFAULT NOW()
);
create index if not exists idx_gwj_run on github_workflow_jobs (run_id);
create unique index if not exists idx_gwj_github_job on github_workflow_jobs (github_job_id);
alter table github_workflow_jobs owner to \"primus-safe\";

-- 5. Steps 表
create table if not exists github_workflow_steps (
    id SERIAL PRIMARY KEY,
    job_id INT NOT NULL,
    step_number INT NOT NULL,
    name VARCHAR(500),
    status VARCHAR(50),
    conclusion VARCHAR(50),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_seconds INT
);
create index if not exists idx_gws_job on github_workflow_steps (job_id);
alter table github_workflow_steps owner to \"primus-safe\";

-- 6. Commits 表
create table if not exists github_workflow_commits (
    id SERIAL PRIMARY KEY,
    sha VARCHAR(64) NOT NULL,
    github_owner VARCHAR(255),
    github_repo VARCHAR(255),
    message TEXT,
    author_name VARCHAR(255),
    author_email VARCHAR(255),
    authored_at TIMESTAMPTZ,
    additions INT,
    deletions INT,
    files_changed INT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
create unique index if not exists idx_gwc_sha on github_workflow_commits (sha, github_owner, github_repo);
alter table github_workflow_commits owner to \"primus-safe\";

-- 7. Metrics 表
create table if not exists github_workflow_metrics (
    id SERIAL PRIMARY KEY,
    config_id INT,
    run_id INT NOT NULL,
    timestamp TIMESTAMPTZ,
    dimensions JSONB DEFAULT '{}',
    metrics JSONB DEFAULT '{}',
    raw_data JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
create index if not exists idx_gwm_run on github_workflow_metrics (run_id);
create index if not exists idx_gwm_config on github_workflow_metrics (config_id);
alter table github_workflow_metrics owner to \"primus-safe\";

SELECT '7 tables created successfully' as result;
"
```

### 第三步：更新 job-manager 镜像

先更新 job-manager（它包含 WorkflowTracker 和 SyncJob）：

```bash
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe set image \
  deployment/primus-safe-job-manager \
  job-manager=primussafe/job-manager:202603191501

# 等待 rollout 完成
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe rollout status \
  deployment/primus-safe-job-manager --timeout=120s
```

### 第四步：验证 job-manager

```bash
# 检查 Pod 状态
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe get pods -l app=primus-safe-job-manager

# 检查日志中是否有 GitHub tracker 初始化成功
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe logs deployment/primus-safe-job-manager --tail=30 | grep -i github

# 预期看到：
# [github] workflow tracker initialized
# [github-sync] starting sync job
```

如果日志中出现 `workflow tracker disabled`，说明 DB 配置未启用或连接失败，需要检查。

### 第五步：更新 apiserver 镜像

```bash
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe set image \
  deployment/primus-safe-apiserver \
  apiserver=primussafe/apiserver:202603191519

# 等待 rollout 完成
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe rollout status \
  deployment/primus-safe-apiserver --timeout=120s
```

### 第六步：验证 apiserver

```bash
# 端口转发
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe port-forward deployment/primus-safe-apiserver 18080:8080 &
sleep 3

# 验证新 API 端点
# 1. 采集配置列表（应为空）
curl -s http://localhost:18080/api/v1/github-workflow/collection-configs | python3 -m json.tool

# 2. Workflow Runs 列表（应为空或有新数据）
curl -s http://localhost:18080/api/v1/github-workflow/runs | python3 -m json.tool

# 3. 统计信息
curl -s http://localhost:18080/api/v1/github-workflow/stats | python3 -m json.tool

# 4. 确认不影响现有功能 — 测试 workload API
curl -s http://localhost:18080/api/v1/workloads | python3 -m json.tool | head -20

# 清理端口转发
kill %1
```

### 第七步：功能验证

等待有新的 EphemeralRunner 被创建（GitHub Actions 触发），然后检查：

```bash
# 查看 job-manager 是否跟踪到了 workflow run
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe logs deployment/primus-safe-job-manager --tail=50 | grep github-tracker

# 查看数据库中是否有新记录
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe port-forward deployment/primus-safe-apiserver 18080:8080 &
sleep 3
curl -s http://localhost:18080/api/v1/github-workflow/runs?limit=5 | python3 -m json.tool
curl -s http://localhost:18080/api/v1/github-workflow/stats | python3 -m json.tool
kill %1
```

## 回滚方案

### 快速回滚（1 分钟内）

如果出现以下任何问题，立即回滚：
- apiserver 启动失败或 CrashLoop
- job-manager 启动失败或 CrashLoop
- 现有功能（workload 调度、CI/CD 调度）受影响

```bash
KUBECONFIG=~/workspace/kubeconfig kubectl config use-context tw-project1

# 回滚 apiserver
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe set image \
  deployment/primus-safe-apiserver \
  apiserver=${ROLLBACK_APISERVER_IMAGE}

# 回滚 job-manager
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe set image \
  deployment/primus-safe-job-manager \
  job-manager=${ROLLBACK_JOB_MANAGER_IMAGE}

# 等待 rollout
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe rollout status deployment/primus-safe-apiserver --timeout=60s
KUBECONFIG=~/workspace/kubeconfig kubectl -n primus-safe rollout status deployment/primus-safe-job-manager --timeout=60s
```

### 数据库回滚（可选）

新增的 7 张 GitHub 表与现有表完全独立，**不需要删除**。如果确实需要清理：

```sql
DROP TABLE IF EXISTS github_workflow_metrics;
DROP TABLE IF EXISTS github_workflow_steps;
DROP TABLE IF EXISTS github_workflow_jobs;
DROP TABLE IF EXISTS github_workflow_run_details;
DROP TABLE IF EXISTS github_workflow_runs;
DROP TABLE IF EXISTS github_workflow_commits;
DROP TABLE IF EXISTS github_collection_configs;
```

## 风险评估

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| job-manager WorkflowTracker 初始化失败 | 低 | 无影响（graceful skip） | `commonconfig.IsDBEnable()` 检查 + nil tracker |
| trackGithubWorkflow panic | 极低 | 无影响（defer recover） | panic 被捕获，不影响调度 |
| SyncJob DB 写入失败 | 低 | GitHub 数据不同步 | 日志告警，不影响核心调度 |
| DB 表创建失败 | 低 | API 返回空数据 | 手动执行 SQL 修复 |
| apiserver 新端点与现有路由冲突 | 极低 | 404 | `/github-workflow/` 是全新路径 |

## 核心安全保障

1. **GitHub 跟踪完全可选**：`workflowTracker == nil` 时所有 GitHub 逻辑静默跳过
2. **不修改现有调度逻辑**：`handleJobImpl` 的返回值不受 GitHub hook 影响
3. **panic 保护**：`trackGithubWorkflow` 内部有 `defer recover()`
4. **新增表独立**：7 张新表与 SaFE 现有表无外键关联
5. **新增 API 独立**：`/api/v1/github-workflow/` 路径完全独立
