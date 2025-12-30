# AI Gateway Phase 1-5 End-to-End Testing Guide

This document records the complete end-to-end testing process for the AI Agent Webhook System (Phase 1-5), used to verify the functionality of AI Gateway, AI Registry, AI Client, and other core components.

## Test Environment

- **Kubernetes Cluster**: test-env
- **Namespace**: primus-lens
- **Test Date**: 2025-12-29

## Prerequisites

1. Kubernetes cluster access
2. Properly configured kubeconfig
3. AI Gateway deployment already deployed

```bash
export KUBECONFIG=/wekafs/haiskong/.kube/config
kubectl config use-context test-env
```

## Step 1: Create Database Tables

AI Gateway requires two database tables: `ai_agent_registrations` and `ai_tasks`.

### 1.1 Check if Tables Exist

```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- \
  psql -d primus-lens -c "SELECT table_name FROM information_schema.tables WHERE table_name LIKE 'ai_%';"
```

### 1.2 Create ai_agent_registrations Table

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

### 1.3 Create ai_tasks Table

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

### 1.4 Grant Table Access Permissions

```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c '
GRANT ALL PRIVILEGES ON TABLE ai_agent_registrations TO "primus-lens";
GRANT ALL PRIVILEGES ON TABLE ai_tasks TO "primus-lens";
'
```

## Step 2: Fix Deployment Probe Configuration

AI Gateway uses `/v1/health` as the health check path by default, but this endpoint is not implemented. It needs to be changed to `/metrics`.

### 2.1 Check Current Probe Configuration

```bash
kubectl get deployment ai-gateway -n primus-lens -o yaml | grep -A 10 "livenessProbe\|readinessProbe"
```

### 2.2 Fix Probe Configuration

```bash
kubectl patch deployment ai-gateway -n primus-lens --type='json' -p='[
  {"op": "replace", "path": "/spec/template/spec/containers/0/livenessProbe/httpGet/path", "value": "/metrics"},
  {"op": "replace", "path": "/spec/template/spec/containers/0/livenessProbe/httpGet/port", "value": 8004},
  {"op": "replace", "path": "/spec/template/spec/containers/0/readinessProbe/httpGet/path", "value": "/metrics"},
  {"op": "replace", "path": "/spec/template/spec/containers/0/readinessProbe/httpGet/port", "value": 8004}
]'
```

### 2.3 Wait for Pod to be Ready

```bash
kubectl rollout status deployment/ai-gateway -n primus-lens
kubectl get pods -n primus-lens -l app=ai-gateway
```

Expected output:
```
NAME                          READY   STATUS    RESTARTS   AGE
ai-gateway-xxxxxxxxx-xxxxx    1/1     Running   0          xxs
```

## Step 3: Verify AI Gateway API

### 3.1 Start Port Forwarding

```bash
kubectl port-forward -n primus-lens svc/ai-gateway 58003:8003 &
sleep 3
```

### 3.2 Test Agent List API

```bash
curl -s http://localhost:58003/v1/ai/agents
```

Expected output:
```json
{"agents":[],"total":0}
```

### 3.3 Test Agent Registration

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

Expected output:
```json
{"message":"Agent registered successfully","name":"test-agent"}
```

### 3.4 Test Get Agent Details

```bash
curl -s http://localhost:58003/v1/ai/agents/test-agent
```

### 3.5 Test Get Statistics

```bash
curl -s http://localhost:58003/v1/ai/stats
```

Expected output:
```json
{"agents":{"total":1,"healthy":0,"unhealthy":0,"unknown":1},"tasks":{"pending":0,"processing":0,"completed":0,"failed":0,"cancelled":0,"total":0}}
```

### 3.6 Test Delete Agent

```bash
curl -s -X DELETE http://localhost:58003/v1/ai/agents/test-agent
```

### 3.7 Clean Up Port Forwarding

```bash
pkill -f "port-forward.*58003"
```

## Step 4: Test Message Sending/Receiving

This is the most important test, verifying that the Mock Agent can properly receive calls from the platform.

### 4.1 Create a Simple Mock Agent

Create file `simple_mock_agent.py`:

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

### 4.2 Start Mock Agent

```bash
python3 simple_mock_agent.py &
```

### 4.3 Verify Mock Agent Health Check

```bash
curl -s http://localhost:8002/health
```

Expected output:
```json
{"status":"healthy"}
```

### 4.4 Get Local IP Address

```bash
MY_IP=$(hostname -I | awk '{print $1}')
echo "My IP: $MY_IP"
```

### 4.5 Register Mock Agent with AI Gateway

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

### 4.6 Simulate Platform Calling Agent (aiclient Behavior)

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

Expected output:
```json
{"status": "success", "code": 0, "message": "Mock response", "payload": {"received": true}}
```

### 4.7 Check Mock Agent Logs

In the Mock Agent terminal, you should see output similar to:

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

### 4.8 Verify AI Gateway Health Check

After waiting about 10 seconds, check if the Agent status changes to healthy:

```bash
curl -s http://localhost:58003/v1/ai/agents/my-mock-agent
```

Expected output (note that status should be "healthy"):
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

Mock Agent logs should show health check requests from AI Gateway:
```
[GET] /health
172.16.20.48 - - [29/Dec/2025 16:50:25] "GET /health HTTP/1.1" 200 -
```

### 4.9 Clean Up Test Environment

```bash
# Delete test Agent
curl -s -X DELETE http://localhost:58003/v1/ai/agents/my-mock-agent

# Stop port forwarding
pkill -f "port-forward.*58003"

# Stop Mock Agent
pkill -f "simple_mock_agent.py"
```

## Verification Checklist

| Feature | Verification Method | Expected Result |
|---------|---------------------|-----------------|
| Database table creation | `\dt ai_*` | Shows two tables |
| Agent registration | `POST /v1/ai/agents/register` | Returns success message |
| Agent list | `GET /v1/ai/agents` | Returns Agent list |
| Agent details | `GET /v1/ai/agents/:name` | Returns Agent info |
| Agent deletion | `DELETE /v1/ai/agents/:name` | Returns success message |
| Task list | `GET /v1/ai/tasks` | Returns task list |
| Statistics | `GET /v1/ai/stats` | Returns statistics data |
| Mock Agent health check | AI Gateway background job | Agent status becomes healthy |
| Message sending/receiving | `POST /invoke` | Mock Agent prints received request |

## Troubleshooting

### Pod Keeps Staying in CrashLoopBackOff

1. Check logs:
   ```bash
   kubectl logs -n primus-lens -l app=ai-gateway --tail=50
   ```

2. Common causes:
   - Database tables do not exist
   - Insufficient table permissions
   - Probe endpoint does not exist

### Agent Status Stays as Unknown

1. Check if Mock Agent is running
2. Check network connectivity (can AI Gateway Pod access Mock Agent IP)
3. Check AI Gateway logs for health check errors

### Database Permission Error

```
ERROR: permission denied for table ai_agent_registrations (SQLSTATE 42501)
```

Solution:
```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c '
GRANT ALL PRIVILEGES ON TABLE ai_agent_registrations TO "primus-lens";
GRANT ALL PRIVILEGES ON TABLE ai_tasks TO "primus-lens";
'
```

## Related Files

- Design document: `/wekafs/haiskong/code/docs/lens/ai-agents/ai-agent-webhook-design.md`
- AI Gateway code: `/wekafs/haiskong/code/Primus-SaFE/Lens/modules/ai-gateway/`
- AI Client SDK: `/wekafs/haiskong/code/Primus-SaFE/Lens/modules/core/pkg/aiclient/`
- AI Registry SDK: `/wekafs/haiskong/code/Primus-SaFE/Lens/modules/core/pkg/airegistry/`
- Task Queue SDK: `/wekafs/haiskong/code/Primus-SaFE/Lens/modules/core/pkg/aitaskqueue/`
