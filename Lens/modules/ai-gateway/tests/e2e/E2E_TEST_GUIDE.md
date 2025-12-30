# AI Gateway Phase 1-5 端到端测试指南

本文档记录了 AI Agent Webhook System (Phase 1-5) 的完整端到端测试流程，用于验证 AI Gateway、AI Registry、AI Client 等核心组件的功能。

## 测试环境

- **Kubernetes 集群**: x-flannel
- **命名空间**: primus-lens
- **测试日期**: 2025-12-29

## 前置条件

1. 有 Kubernetes 集群访问权限
2. 配置正确的 kubeconfig
3. AI Gateway deployment 已部署

```bash
export KUBECONFIG=/wekafs/haiskong/.kube/config
kubectl config use-context x-flannel
```

## 步骤 1: 创建数据库表

AI Gateway 需要两个数据库表：`ai_agent_registrations` 和 `ai_tasks`。

### 1.1 检查表是否存在

```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- \
  psql -d primus-lens -c "SELECT table_name FROM information_schema.tables WHERE table_name LIKE 'ai_%';"
```

### 1.2 创建 ai_agent_registrations 表

```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c "
CREATE TABLE IF NOT EXISTS ai_agent_registrations (
    name VARCHAR(128) PRIMARY KEY,
    endpoint VARCHAR(512) NOT NULL,
    topics JSONB NOT NULL DEFAULT '[]'::jsonb,
    health_check_path VARCHAR(256) DEFAULT '/health',
    timeout_secs INT DEFAULT 60,
    status VARCHAR(32) DEFAULT 'unknown',
    last_health_check TIMESTAMPTZ,
    failure_count INT DEFAULT 0,
    metadata JSONB DEFAULT '{}'::jsonb,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ai_agent_reg_status ON ai_agent_registrations(status);
CREATE INDEX IF NOT EXISTS idx_ai_agent_reg_topics ON ai_agent_registrations USING GIN(topics);
"
```

### 1.3 创建 ai_tasks 表

```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c "
CREATE TABLE IF NOT EXISTS ai_tasks (
    id VARCHAR(64) PRIMARY KEY,
    topic VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    priority INT DEFAULT 0,
    input_payload JSONB NOT NULL,
    output_payload JSONB,
    error_message VARCHAR(1024),
    error_code INT,
    retry_count INT DEFAULT 0,
    max_retries INT DEFAULT 3,
    agent_id VARCHAR(128),
    context JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    timeout_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ai_tasks_pending_priority ON ai_tasks(status, priority DESC, created_at ASC) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_ai_tasks_topic_status ON ai_tasks(topic, status);
CREATE INDEX IF NOT EXISTS idx_ai_tasks_timeout ON ai_tasks(timeout_at) WHERE status = 'processing';
CREATE INDEX IF NOT EXISTS idx_ai_tasks_created_at ON ai_tasks(created_at DESC);
"
```

### 1.4 授权表访问权限

```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c '
GRANT ALL PRIVILEGES ON TABLE ai_agent_registrations TO "primus-lens";
GRANT ALL PRIVILEGES ON TABLE ai_tasks TO "primus-lens";
'
```

## 步骤 2: 修复 Deployment Probe 配置

AI Gateway 默认使用 `/v1/health` 作为健康检查路径，但该端点未实现。需要修改为 `/metrics`。

### 2.1 检查当前 probe 配置

```bash
kubectl get deployment ai-gateway -n primus-lens -o yaml | grep -A 10 "livenessProbe\|readinessProbe"
```

### 2.2 修复 probe 配置

```bash
kubectl patch deployment ai-gateway -n primus-lens --type='json' -p='[
  {"op": "replace", "path": "/spec/template/spec/containers/0/livenessProbe/httpGet/path", "value": "/metrics"},
  {"op": "replace", "path": "/spec/template/spec/containers/0/livenessProbe/httpGet/port", "value": 8004},
  {"op": "replace", "path": "/spec/template/spec/containers/0/readinessProbe/httpGet/path", "value": "/metrics"},
  {"op": "replace", "path": "/spec/template/spec/containers/0/readinessProbe/httpGet/port", "value": 8004}
]'
```

### 2.3 等待 Pod 就绪

```bash
kubectl rollout status deployment/ai-gateway -n primus-lens
kubectl get pods -n primus-lens -l app=ai-gateway
```

预期输出：
```
NAME                          READY   STATUS    RESTARTS   AGE
ai-gateway-xxxxxxxxx-xxxxx    1/1     Running   0          xxs
```

## 步骤 3: 验证 AI Gateway API

### 3.1 启动端口转发

```bash
kubectl port-forward -n primus-lens svc/ai-gateway 58003:8003 &
sleep 3
```

### 3.2 测试 Agent 列表 API

```bash
curl -s http://localhost:58003/v1/ai/agents
```

预期输出：
```json
{"agents":[],"total":0}
```

### 3.3 测试注册 Agent

```bash
curl -s -X POST http://localhost:58003/v1/ai/agents/register \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-agent",
    "endpoint": "http://test-agent.example.com:8080",
    "topics": [".alert.advisor.aggregate_workloads"],
    "healthCheckPath": "/health",
    "timeout": 60
  }'
```

预期输出：
```json
{"message":"Agent registered successfully","name":"test-agent"}
```

### 3.4 测试获取 Agent 详情

```bash
curl -s http://localhost:58003/v1/ai/agents/test-agent
```

### 3.5 测试获取统计信息

```bash
curl -s http://localhost:58003/v1/ai/stats
```

预期输出：
```json
{"agents":{"total":1,"healthy":0,"unhealthy":0,"unknown":1},"tasks":{"pending":0,"processing":0,"completed":0,"failed":0,"cancelled":0,"total":0}}
```

### 3.6 测试删除 Agent

```bash
curl -s -X DELETE http://localhost:58003/v1/ai/agents/test-agent
```

### 3.7 清理端口转发

```bash
pkill -f "port-forward.*58003"
```

## 步骤 4: 测试消息发送/接收

这是最重要的测试，验证 Mock Agent 能否正常接收来自平台的调用。

### 4.1 创建简单的 Mock Agent

创建文件 `simple_mock_agent.py`：

```python
#!/usr/bin/env python3
"""Simple Mock Agent - just prints received requests"""
from http.server import HTTPServer, BaseHTTPRequestHandler
import json

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"[GET] {self.path}")
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{"status":"healthy"}')
        else:
            self.send_error(404)
    
    def do_POST(self):
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length).decode()
        print(f"\n{'='*60}")
        print(f"[POST] {self.path}")
        print(f"Headers: {dict(self.headers)}")
        print(f"Body: {body}")
        print(f"{'='*60}\n")
        
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        result = {"status": "success", "code": 0, "message": "Mock response", "payload": {"received": True}}
        self.wfile.write(json.dumps(result).encode())

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8002), Handler)
    print("Mock Agent listening on port 8002...")
    server.serve_forever()
```

### 4.2 启动 Mock Agent

```bash
python3 simple_mock_agent.py &
```

### 4.3 验证 Mock Agent 健康检查

```bash
curl -s http://localhost:8002/health
```

预期输出：
```json
{"status":"healthy"}
```

### 4.4 获取本机 IP

```bash
MY_IP=$(hostname -I | awk '{print $1}')
echo "My IP: $MY_IP"
```

### 4.5 注册 Mock Agent 到 AI Gateway

```bash
kubectl port-forward -n primus-lens svc/ai-gateway 58003:8003 &
sleep 3

curl -s -X POST http://localhost:58003/v1/ai/agents/register \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"my-mock-agent\",
    \"endpoint\": \"http://${MY_IP}:8002\",
    \"topics\": [\".alert.advisor.aggregate_workloads\"],
    \"healthCheckPath\": \"/health\",
    \"timeout\": 60
  }"
```

### 4.6 模拟平台调用 Agent (aiclient 行为)

```bash
curl -s -X POST http://${MY_IP}:8002/invoke \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: test-req-001" \
  -H "X-Topic: .alert.advisor.aggregate_workloads" \
  -d '{
    "request_id": "test-req-001",
    "topic": ".alert.advisor.aggregate_workloads",
    "context": {"tenant_id": "test-tenant", "user_id": "test-user"},
    "payload": {"workloads": [{"name": "nginx", "namespace": "default"}]}
  }'
```

预期输出：
```json
{"status": "success", "code": 0, "message": "Mock response", "payload": {"received": true}}
```

### 4.7 检查 Mock Agent 日志

在 Mock Agent 的终端中应该能看到类似输出：

```
============================================================
[POST] /invoke
Headers: {'Host': '172.16.65.34:8002', 'User-Agent': 'curl/8.5.0', 'Accept': '*/*', 'Content-Type': 'application/json', 'X-Request-ID': 'test-req-001', 'X-Topic': '.alert.advisor.aggregate_workloads', 'Content-Length': '233'}
Body: {
    "request_id": "test-req-001",
    "topic": ".alert.advisor.aggregate_workloads",
    "context": {"tenant_id": "test-tenant", "user_id": "test-user"},
    "payload": {"workloads": [{"name": "nginx", "namespace": "default"}]}
  }
============================================================
```

### 4.8 验证 AI Gateway 健康检查

等待约 10 秒后，检查 Agent 状态是否变为 healthy：

```bash
curl -s http://localhost:58003/v1/ai/agents/my-mock-agent
```

预期输出（注意 status 应为 "healthy"）：
```json
{
  "name": "my-mock-agent",
  "endpoint": "http://172.16.65.34:8002",
  "topics": [".alert.advisor.aggregate_workloads"],
  "status": "healthy",
  "health_check_path": "/health",
  "failure_count": 0,
  ...
}
```

Mock Agent 日志应该显示来自 AI Gateway 的健康检查请求：
```
[GET] /health
172.16.20.48 - - [29/Dec/2025 16:50:25] "GET /health HTTP/1.1" 200 -
```

### 4.9 清理测试环境

```bash
# 删除测试 Agent
curl -s -X DELETE http://localhost:58003/v1/ai/agents/my-mock-agent

# 停止端口转发
pkill -f "port-forward.*58003"

# 停止 Mock Agent
pkill -f "simple_mock_agent.py"
```

## 验证检查清单

| 功能 | 验证方式 | 预期结果 |
|------|---------|---------|
| 数据库表创建 | `\dt ai_*` | 显示两个表 |
| Agent 注册 | `POST /v1/ai/agents/register` | 返回成功消息 |
| Agent 列表 | `GET /v1/ai/agents` | 返回 Agent 列表 |
| Agent 详情 | `GET /v1/ai/agents/:name` | 返回 Agent 信息 |
| Agent 删除 | `DELETE /v1/ai/agents/:name` | 返回成功消息 |
| 任务列表 | `GET /v1/ai/tasks` | 返回任务列表 |
| 统计信息 | `GET /v1/ai/stats` | 返回统计数据 |
| Mock Agent 健康检查 | AI Gateway 后台 Job | Agent status 变为 healthy |
| 消息发送/接收 | `POST /invoke` | Mock Agent 打印收到的请求 |

## 故障排除

### Pod 一直处于 CrashLoopBackOff

1. 检查日志：
   ```bash
   kubectl logs -n primus-lens -l app=ai-gateway --tail=50
   ```

2. 常见原因：
   - 数据库表不存在
   - 表权限不足
   - Probe 端点不存在

### Agent 状态一直是 unknown

1. 检查 Mock Agent 是否在运行
2. 检查网络连通性（AI Gateway Pod 能否访问 Mock Agent IP）
3. 检查 AI Gateway 日志是否有健康检查错误

### 数据库权限错误

```
ERROR: permission denied for table ai_agent_registrations (SQLSTATE 42501)
```

解决方案：
```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c '
GRANT ALL PRIVILEGES ON TABLE ai_agent_registrations TO "primus-lens";
GRANT ALL PRIVILEGES ON TABLE ai_tasks TO "primus-lens";
'
```

## 相关文件

- 设计文档: `/wekafs/haiskong/code/docs/lens/ai-agents/ai-agent-webhook-design.md`
- AI Gateway 代码: `/wekafs/haiskong/code/Primus-SaFE/Lens/modules/ai-gateway/`
- AI Client SDK: `/wekafs/haiskong/code/Primus-SaFE/Lens/modules/core/pkg/aiclient/`
- AI Registry SDK: `/wekafs/haiskong/code/Primus-SaFE/Lens/modules/core/pkg/airegistry/`
- Task Queue SDK: `/wekafs/haiskong/code/Primus-SaFE/Lens/modules/core/pkg/aitaskqueue/`


