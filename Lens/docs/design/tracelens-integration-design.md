# TraceLens Integration Design Document

| Field | Value |
|-------|-------|
| **Author** | AI-Advisor Team |
| **Status** | Draft |
| **Created** | 2025-12-17 |
| **Last Updated** | 2025-12-17 |

---

## 1. Overview

### 1.1 Background

The Primus-SaFE Lens system collects PyTorch profiler trace files (`.pt.trace.json.gz`) from training workloads. These files contain detailed GPU kernel execution information, memory usage patterns, and communication overhead data. To provide actionable insights to users, we need to integrate TraceLens - an AMD-developed Python library for automated trace analysis.

### 1.2 Goals

1. **On-Demand Analysis**: Provide TraceLens analysis UI for any collected profiler file
2. **Seamless Integration**: Users access TraceLens through the existing Lens API gateway
3. **Resource Efficiency**: Create analysis environments only when needed
4. **Multi-User Support**: Allow concurrent analysis sessions for different workloads

### 1.3 Non-Goals

1. Modifying TraceLens core library code
2. Re-implementing TraceLens UI in a different framework
3. Real-time streaming analysis during training

---

## 2. Architecture

### 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              User Browser                                │
│                                   │                                      │
│                    ┌──────────────▼──────────────┐                      │
│                    │       Frontend (React)       │                      │
│                    └──────────────┬──────────────┘                      │
└───────────────────────────────────┼─────────────────────────────────────┘
                                    │
                    ┌───────────────▼───────────────┐
                    │         Ingress/Gateway       │
                    └───────────────┬───────────────┘
                                    │
┌───────────────────────────────────┼─────────────────────────────────────┐
│  Kubernetes Cluster (primus-lens) │                                      │
│                    ┌──────────────▼──────────────┐                      │
│                    │          API Server          │                      │
│                    │            (Go)              │                      │
│                    │  ┌─────────────────────────┐│                      │
│                    │  │  TraceLens Controller   ││                      │
│                    │  │  - Session Manager      ││                      │
│                    │  │  - Pod Lifecycle        ││                      │
│                    │  │  - Reverse Proxy        ││                      │
│                    │  └─────────────────────────┘│                      │
│                    └──────────────┬──────────────┘                      │
│                                   │                                      │
│           ┌───────────────────────┼───────────────────────┐             │
│           │                       │                       │             │
│           ▼                       ▼                       ▼             │
│   ┌───────────────┐       ┌───────────────┐       ┌───────────────┐    │
│   │ TraceLens Pod │       │ TraceLens Pod │       │ TraceLens Pod │    │
│   │  Session A    │       │  Session B    │       │  Session C    │    │
│   │ ┌───────────┐ │       │ ┌───────────┐ │       │ ┌───────────┐ │    │
│   │ │ Streamlit │ │       │ │ Streamlit │ │       │ │ Streamlit │ │    │
│   │ │   :8501   │ │       │ │   :8501   │ │       │ │   :8501   │ │    │
│   │ └───────────┘ │       │ └───────────┘ │       │ └───────────┘ │    │
│   └───────┬───────┘       └───────┬───────┘       └───────┬───────┘    │
│           │                       │                       │             │
│           └───────────────────────┴───────────────────────┘             │
│                                   │                                      │
│                    ┌──────────────▼──────────────┐                      │
│                    │    Shared Storage (WekaFS)  │                      │
│                    │    /wekafs/.../profiler/    │                      │
│                    └─────────────────────────────┘                      │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           API Server Module                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────┐ │
│  │   HTTP Router   │  │  WebSocket      │  │   Kubernetes Client     │ │
│  │   (Gin)         │  │  Proxy          │  │   (client-go)           │ │
│  └────────┬────────┘  └────────┬────────┘  └────────────┬────────────┘ │
│           │                    │                        │               │
│           └────────────────────┼────────────────────────┘               │
│                                │                                         │
│                    ┌───────────▼───────────┐                            │
│                    │  TraceLens Controller │                            │
│                    ├───────────────────────┤                            │
│                    │ - CreateSession()     │                            │
│                    │ - GetSession()        │                            │
│                    │ - DeleteSession()     │                            │
│                    │ - ProxyRequest()      │                            │
│                    │ - CleanupExpired()    │                            │
│                    └───────────┬───────────┘                            │
│                                │                                         │
│                    ┌───────────▼───────────┐                            │
│                    │   Session Repository  │                            │
│                    │   (PostgreSQL)        │                            │
│                    └───────────────────────┘                            │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Detailed Design

### 3.1 Data Model

#### 3.1.1 TraceLens Session Table

```sql
CREATE TABLE tracelens_sessions (
    -- Primary key
    id SERIAL PRIMARY KEY,
    
    -- Session identification
    session_id VARCHAR(64) UNIQUE NOT NULL,
    
    -- Association with profiler data
    workload_uid VARCHAR(64) NOT NULL,
    profiler_file_id INTEGER REFERENCES profiler_files(id),
    
    -- User tracking
    user_id VARCHAR(64),
    user_email VARCHAR(256),
    
    -- Kubernetes resources
    pod_name VARCHAR(128),
    pod_namespace VARCHAR(64) DEFAULT 'primus-lens',
    pod_ip VARCHAR(64),
    pod_port INTEGER DEFAULT 8501,
    
    -- Session status
    status VARCHAR(32) DEFAULT 'pending',
    status_message TEXT,
    
    -- Lifecycle management
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    ready_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    last_accessed_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Configuration
    config JSONB DEFAULT '{}',
    
    -- Indexes
    CONSTRAINT valid_status CHECK (status IN (
        'pending',      -- Session created, pod not yet scheduled
        'creating',     -- Pod is being created
        'initializing', -- Pod running, TraceLens starting up
        'ready',        -- TraceLens UI is accessible
        'failed',       -- Pod creation or startup failed
        'expired',      -- Session expired, pending cleanup
        'deleted'       -- Session and pod deleted
    ))
);

-- Indexes for efficient queries
CREATE INDEX idx_tracelens_sessions_status ON tracelens_sessions(status);
CREATE INDEX idx_tracelens_sessions_workload ON tracelens_sessions(workload_uid);
CREATE INDEX idx_tracelens_sessions_expires ON tracelens_sessions(expires_at) 
    WHERE status NOT IN ('deleted', 'expired');
CREATE INDEX idx_tracelens_sessions_user ON tracelens_sessions(user_id);
```

#### 3.1.2 Session Status State Machine

```
                    ┌─────────┐
                    │ pending │
                    └────┬────┘
                         │ Pod scheduled
                         ▼
                    ┌──────────┐
         ┌─────────│ creating │─────────┐
         │         └────┬─────┘         │
         │ Pod failed   │ Pod running   │ Timeout
         │              ▼               │
         │       ┌──────────────┐       │
         │       │ initializing │───────┤
         │       └──────┬───────┘       │
         │              │ Health check  │
         │              │ passed        │
         │              ▼               ▼
         │         ┌─────────┐     ┌────────┐
         └────────▶│  failed │     │ ready  │
                   └─────────┘     └───┬────┘
                                       │ TTL expired
                                       ▼
                                  ┌─────────┐
                                  │ expired │
                                  └────┬────┘
                                       │ Cleanup
                                       ▼
                                  ┌─────────┐
                                  │ deleted │
                                  └─────────┘
```

### 3.2 API Design

#### 3.2.1 RESTful API Endpoints

```yaml
# ============================================================
# TraceLens Session Management API
# ============================================================

# Create a new TraceLens analysis session
POST /api/v1/tracelens/sessions
Content-Type: application/json
Authorization: Bearer {token}

Request Body:
{
    "workload_uid": "0512be05-0c7f-4a39-b577-624b13c8533f",
    "profiler_file_id": 369,
    "ttl_minutes": 60,           # Optional, default: 60
    "resource_profile": "medium"  # Optional: small, medium, large
}

Response (201 Created):
{
    "session_id": "tls-0512be05-369-1702814400",
    "status": "creating",
    "ui_path": "/api/v1/tracelens/sessions/tls-0512be05-369-1702814400/ui/",
    "created_at": "2025-12-17T12:00:00Z",
    "expires_at": "2025-12-17T13:00:00Z",
    "estimated_ready_seconds": 30
}

# ============================================================

# Get session status
GET /api/v1/tracelens/sessions/{session_id}
Authorization: Bearer {token}

Response (200 OK):
{
    "session_id": "tls-0512be05-369-1702814400",
    "status": "ready",
    "status_message": "TraceLens UI is ready",
    "workload_uid": "0512be05-0c7f-4a39-b577-624b13c8533f",
    "profiler_file": {
        "id": 369,
        "file_name": "primus-megatron-exp[...].pt.trace.json.gz",
        "file_size": 121808875
    },
    "ui_path": "/api/v1/tracelens/sessions/tls-0512be05-369-1702814400/ui/",
    "created_at": "2025-12-17T12:00:00Z",
    "ready_at": "2025-12-17T12:00:25Z",
    "expires_at": "2025-12-17T13:00:00Z",
    "last_accessed_at": "2025-12-17T12:05:00Z"
}

# ============================================================

# List user's sessions
GET /api/v1/tracelens/sessions?status=ready&workload_uid={uid}
Authorization: Bearer {token}

Response (200 OK):
{
    "sessions": [...],
    "total": 3,
    "page": 1,
    "page_size": 20
}

# ============================================================

# Extend session TTL
PATCH /api/v1/tracelens/sessions/{session_id}
Content-Type: application/json
Authorization: Bearer {token}

Request Body:
{
    "extend_minutes": 30
}

Response (200 OK):
{
    "session_id": "tls-0512be05-369-1702814400",
    "expires_at": "2025-12-17T13:30:00Z"
}

# ============================================================

# Delete session
DELETE /api/v1/tracelens/sessions/{session_id}
Authorization: Bearer {token}

Response (204 No Content)

# ============================================================

# Proxy to TraceLens UI (all methods)
ANY /api/v1/tracelens/sessions/{session_id}/ui/*path
Authorization: Bearer {token}

-> Reverse proxy to Pod's Streamlit server
-> Handles HTTP and WebSocket connections
```

#### 3.2.2 WebSocket Proxy

Streamlit uses WebSocket for real-time updates. The proxy must handle:

```
Client                      API Server                    TraceLens Pod
   │                            │                              │
   │  GET /ui/_stcore/stream    │                              │
   │  Upgrade: websocket        │                              │
   │ ──────────────────────────▶│                              │
   │                            │  GET /_stcore/stream         │
   │                            │  Upgrade: websocket          │
   │                            │ ────────────────────────────▶│
   │                            │                              │
   │                            │◀──── 101 Switching ─────────│
   │◀──── 101 Switching ────────│                              │
   │                            │                              │
   │◀═══════════════════════════╪══════════════════════════════│
   │       Bidirectional WebSocket Tunnel                      │
   │◀═══════════════════════════╪══════════════════════════════│
```

### 3.3 Kubernetes Resources

#### 3.3.1 TraceLens Pod Template

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: tracelens-session-${SESSION_ID}
  namespace: primus-lens
  labels:
    app: tracelens-session
    session-id: "${SESSION_ID}"
    workload-uid: "${WORKLOAD_UID}"
    managed-by: lens-api
  annotations:
    # Auto-cleanup annotation
    lens.amd.com/expires-at: "${EXPIRES_AT}"
    lens.amd.com/created-by: "${USER_ID}"
spec:
  # Prevent restarts - session is disposable
  restartPolicy: Never
  
  # Auto-terminate after TTL
  activeDeadlineSeconds: 3600  # 1 hour max
  
  # Scheduling preferences
  nodeSelector:
    kubernetes.io/os: linux
  
  tolerations:
  - key: "workload-type"
    operator: "Equal"
    value: "analysis"
    effect: "NoSchedule"
  
  # Security context
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    fsGroup: 1000
  
  containers:
  - name: tracelens
    image: harbor.tw325.primus-safe.amd.com/primussafe/tracelens:${VERSION}
    
    command:
    - /bin/bash
    - -c
    - |
      # Wait for trace file to be accessible
      while [ ! -f "${TRACE_FILE_PATH}" ]; do
        echo "Waiting for trace file..."
        sleep 1
      done
      
      # Start Streamlit with baseUrlPath for transparent proxy support
      # This ensures all generated URLs include the correct prefix
      streamlit run /app/analyze_trace.py \
        --server.port=8501 \
        --server.headless=true \
        --server.enableCORS=false \
        --server.enableXsrfProtection=false \
        --server.baseUrlPath="${BASE_URL_PATH}" \
        --browser.gatherUsageStats=false \
        -- \
        --trace-file="${TRACE_FILE_PATH}"
    
    env:
    - name: TRACE_FILE_PATH
      value: "${TRACE_FILE_PATH}"
    - name: SESSION_ID
      value: "${SESSION_ID}"
    - name: BASE_URL_PATH
      value: "/api/v1/tracelens/sessions/${SESSION_ID}/ui"
    - name: PYTHONUNBUFFERED
      value: "1"
    
    ports:
    - name: http
      containerPort: 8501
      protocol: TCP
    
    # Health checks
    startupProbe:
      httpGet:
        path: /_stcore/health
        port: 8501
      initialDelaySeconds: 5
      periodSeconds: 2
      failureThreshold: 30  # 60 seconds max startup
    
    readinessProbe:
      httpGet:
        path: /_stcore/health
        port: 8501
      periodSeconds: 5
      failureThreshold: 3
    
    livenessProbe:
      httpGet:
        path: /_stcore/health
        port: 8501
      periodSeconds: 30
      failureThreshold: 3
    
    # Resource limits based on profile
    resources:
      requests:
        cpu: "${CPU_REQUEST}"      # small: 500m, medium: 1, large: 2
        memory: "${MEM_REQUEST}"   # small: 2Gi, medium: 4Gi, large: 8Gi
      limits:
        cpu: "${CPU_LIMIT}"        # small: 1, medium: 2, large: 4
        memory: "${MEM_LIMIT}"     # small: 4Gi, medium: 8Gi, large: 16Gi
    
    volumeMounts:
    - name: profiler-storage
      mountPath: /data/profiler
      readOnly: true
    - name: tmp
      mountPath: /tmp
  
  volumes:
  # Mount WekaFS for profiler file access
  - name: profiler-storage
    hostPath:
      path: /wekafs
      type: Directory
  
  # Temp storage for Streamlit
  - name: tmp
    emptyDir:
      sizeLimit: 1Gi
  
  # Image pull secret
  imagePullSecrets:
  - name: primus-lens-image
```

#### 3.3.2 Resource Profiles

| Profile | CPU Request | CPU Limit | Memory Request | Memory Limit | Use Case |
|---------|-------------|-----------|----------------|--------------|----------|
| small   | 500m        | 1         | 2Gi            | 4Gi          | Traces < 50MB |
| medium  | 1           | 2         | 4Gi            | 8Gi          | Traces 50-200MB |
| large   | 2           | 4         | 8Gi            | 16Gi         | Traces > 200MB |

### 3.4 TraceLens Container Image

#### 3.4.1 Dockerfile

```dockerfile
# TraceLens Analysis Container
FROM python:3.10-slim

LABEL maintainer="AMD AGI Team"
LABEL description="TraceLens Profiler Analysis Service"

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd -m -u 1000 tracelens
WORKDIR /app

# Install TraceLens and dependencies
RUN pip install --no-cache-dir \
    git+https://github.com/AMD-AGI/TraceLens.git \
    streamlit \
    openpyxl \
    plotly

# Copy analysis script
COPY analyze_trace.py /app/

# Set ownership
RUN chown -R tracelens:tracelens /app

USER tracelens

# Expose Streamlit port
EXPOSE 8501

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8501/_stcore/health || exit 1

# Default command
CMD ["streamlit", "run", "/app/analyze_trace.py", "--server.port=8501"]
```

#### 3.4.2 Analysis Script (analyze_trace.py)

```python
#!/usr/bin/env python3
"""
TraceLens Analysis Script for Lens Integration
"""

import argparse
import os
import streamlit as st
from TraceLens import TreePerfAnalyzer, NcclAnalyser
from TraceLens.Reporting import generate_perf_report_pytorch

def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument('--trace-file', required=True, help='Path to trace file')
    return parser.parse_args()

@st.cache_resource
def load_analyzer(trace_path: str) -> TreePerfAnalyzer:
    """Load and cache the trace analyzer"""
    return TreePerfAnalyzer.from_file(trace_path)

def main():
    args = parse_args()
    trace_path = args.trace_file
    
    st.set_page_config(
        page_title="TraceLens Analysis",
        page_icon="🔍",
        layout="wide"
    )
    
    st.title("🔍 TraceLens Performance Analysis")
    st.caption(f"Analyzing: `{os.path.basename(trace_path)}`")
    
    # Load analyzer
    with st.spinner("Loading trace file..."):
        analyzer = load_analyzer(trace_path)
    
    # Create tabs for different analyses
    tabs = st.tabs([
        "📊 GPU Timeline",
        "⚡ Kernel Summary",
        "🔗 Communication",
        "📈 Performance Metrics",
        "📥 Export"
    ])
    
    with tabs[0]:  # GPU Timeline
        st.subheader("GPU Timeline Breakdown")
        df_timeline = analyzer.get_df_gpu_timeline()
        st.dataframe(df_timeline, use_container_width=True)
        
        # Visualization
        import plotly.express as px
        fig = px.pie(
            df_timeline[df_timeline['type'] != 'total_time'],
            values='time ms',
            names='type',
            title='GPU Time Distribution'
        )
        st.plotly_chart(fig, use_container_width=True)
    
    with tabs[1]:  # Kernel Summary
        st.subheader("Kernel Performance Summary")
        df_kernels = analyzer.get_df_kernel_launchers()
        df_summary = analyzer.get_df_kernel_launchers_summary(df_kernels)
        st.dataframe(df_summary, use_container_width=True)
        
        # Category breakdown
        df_category = analyzer.get_df_kernel_launchers_summary_by_category(df_kernels)
        st.subheader("By Category")
        st.dataframe(df_category, use_container_width=True)
    
    with tabs[2]:  # Communication
        st.subheader("Collective Communication Analysis")
        try:
            nccl = NcclAnalyser([trace_path], None)
            df_nccl = nccl.build_df_summary_long()
            if not df_nccl.empty:
                st.dataframe(df_nccl, use_container_width=True)
            else:
                st.info("No collective operations found in this trace.")
        except Exception as e:
            st.warning(f"Could not analyze collectives: {e}")
    
    with tabs[3]:  # Performance Metrics
        st.subheader("Detailed Performance Metrics")
        df_unique = analyzer.get_df_kernel_launchers_unique_args(
            df_kernels,
            agg_metrics=['mean', 'std', 'min', 'max'],
            include_pct=True
        )
        st.dataframe(df_unique, use_container_width=True)
    
    with tabs[4]:  # Export
        st.subheader("Export Analysis Results")
        
        if st.button("Generate Excel Report"):
            with st.spinner("Generating report..."):
                output_path = f"/tmp/report_{os.path.basename(trace_path)}.xlsx"
                generate_perf_report_pytorch(
                    profile_json_path=trace_path,
                    output_xlsx_path=output_path
                )
                
                with open(output_path, 'rb') as f:
                    st.download_button(
                        label="📥 Download Excel Report",
                        data=f,
                        file_name=os.path.basename(output_path),
                        mime="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                    )

if __name__ == "__main__":
    main()
```

### 3.5 Session Lifecycle Management

#### 3.5.1 Session Creation Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Session Creation Flow                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Client              API Server           K8s API         TraceLens Pod │
│    │                     │                   │                  │        │
│    │ POST /sessions      │                   │                  │        │
│    │────────────────────▶│                   │                  │        │
│    │                     │                   │                  │        │
│    │                     │ Validate request  │                  │        │
│    │                     │ Check profiler    │                  │        │
│    │                     │ file exists       │                  │        │
│    │                     │                   │                  │        │
│    │                     │ Create session    │                  │        │
│    │                     │ record (pending)  │                  │        │
│    │                     │                   │                  │        │
│    │                     │ Create Pod        │                  │        │
│    │                     │──────────────────▶│                  │        │
│    │                     │                   │                  │        │
│    │                     │ Update session    │                  │        │
│    │                     │ (creating)        │                  │        │
│    │                     │                   │                  │        │
│    │◀────────────────────│                   │                  │        │
│    │ 201 Created         │                   │                  │        │
│    │ {session_id, ...}   │                   │                  │        │
│    │                     │                   │                  │        │
│    │                     │                   │ Pod Scheduled    │        │
│    │                     │                   │─────────────────▶│        │
│    │                     │                   │                  │        │
│    │                     │ Watch Pod status  │                  │        │
│    │                     │◀──────────────────│                  │        │
│    │                     │                   │                  │        │
│    │                     │ Pod Running       │ Container Start  │        │
│    │                     │ (initializing)    │─────────────────▶│        │
│    │                     │                   │                  │        │
│    │                     │                   │ Startup Probe OK │        │
│    │                     │◀──────────────────│◀─────────────────│        │
│    │                     │                   │                  │        │
│    │                     │ Update session    │                  │        │
│    │                     │ (ready)           │                  │        │
│    │                     │                   │                  │        │
│    │ (Poll for status)   │                   │                  │        │
│    │────────────────────▶│                   │                  │        │
│    │◀────────────────────│                   │                  │        │
│    │ {status: ready}     │                   │                  │        │
│    │                     │                   │                  │        │
└─────────────────────────────────────────────────────────────────────────┘
```

#### 3.5.2 Session Cleanup Strategy

```go
// Cleanup runs every 5 minutes
func (c *Controller) CleanupExpiredSessions(ctx context.Context) error {
    // 1. Find expired sessions
    expiredSessions, err := c.repo.FindExpiredSessions(ctx)
    if err != nil {
        return err
    }
    
    for _, session := range expiredSessions {
        // 2. Delete the Pod
        err := c.k8sClient.DeletePod(ctx, session.PodName, session.PodNamespace)
        if err != nil && !errors.IsNotFound(err) {
            log.Errorf("Failed to delete pod %s: %v", session.PodName, err)
            continue
        }
        
        // 3. Update session status
        session.Status = "deleted"
        session.DeletedAt = time.Now()
        c.repo.Update(ctx, session)
    }
    
    // 4. Clean up orphan pods (pods without session records)
    c.cleanupOrphanPods(ctx)
    
    return nil
}
```

### 3.6 Streamlit baseUrlPath Configuration

To enable transparent proxy without URL rewriting, we configure Streamlit's `baseUrlPath` parameter. This ensures all generated URLs (static assets, WebSocket endpoints, etc.) include the correct API gateway prefix.

#### 3.6.1 How It Works

```
Without baseUrlPath:
  Streamlit generates: <script src="/static/js/main.js">
  Browser requests:    https://gateway/static/js/main.js  ❌ Wrong path!

With baseUrlPath="/api/v1/tracelens/sessions/{id}/ui":
  Streamlit generates: <script src="/api/v1/tracelens/sessions/{id}/ui/static/js/main.js">
  Browser requests:    https://gateway/api/v1/tracelens/sessions/{id}/ui/static/js/main.js  ✅
```

#### 3.6.2 Configuration

```bash
# Pod startup command
streamlit run /app/analyze_trace.py \
  --server.port=8501 \
  --server.headless=true \
  --server.baseUrlPath="/api/v1/tracelens/sessions/${SESSION_ID}/ui" \
  --server.enableCORS=false \
  --server.enableXsrfProtection=false
```

#### 3.6.3 Proxy Behavior

With `baseUrlPath` configured:

| Request Type | Generated Path | Proxy Action |
|--------------|----------------|--------------|
| HTML Page | `/api/v1/.../ui/` | Forward as-is |
| Static JS/CSS | `/api/v1/.../ui/static/*` | Forward as-is |
| WebSocket | `/api/v1/.../ui/_stcore/stream` | Upgrade + Forward |
| Health Check | `/api/v1/.../ui/_stcore/health` | Forward as-is |

The proxy becomes a **transparent tunnel** - no URL rewriting needed in either direction.

### 3.7 Reverse Proxy Implementation

#### 3.6.1 HTTP Proxy Handler

```go
package tracelens

import (
    "net/http"
    "net/http/httputil"
    "net/url"
    "strings"
    
    "github.com/gin-gonic/gin"
    "nhooyr.io/websocket"
)

type ProxyHandler struct {
    sessionRepo SessionRepository
}

func (h *ProxyHandler) HandleProxy(c *gin.Context) {
    sessionID := c.Param("session_id")
    
    // Get session info
    session, err := h.sessionRepo.GetByID(c, sessionID)
    if err != nil {
        c.JSON(404, gin.H{"error": "Session not found"})
        return
    }
    
    if session.Status != "ready" {
        c.JSON(503, gin.H{"error": "Session not ready", "status": session.Status})
        return
    }
    
    // Update last accessed time
    h.sessionRepo.UpdateLastAccessed(c, sessionID)
    
    // Build target URL
    targetURL := &url.URL{
        Scheme: "http",
        Host:   fmt.Sprintf("%s:%d", session.PodIP, session.PodPort),
    }
    
    // Check if WebSocket upgrade
    if isWebSocketRequest(c.Request) {
        h.handleWebSocket(c, session, targetURL)
        return
    }
    
    // HTTP reverse proxy
    proxy := httputil.NewSingleHostReverseProxy(targetURL)
    
    // Modify request
    originalDirector := proxy.Director
    proxy.Director = func(req *http.Request) {
        originalDirector(req)
        
        // Strip the prefix path
        // /api/v1/tracelens/sessions/{id}/ui/foo -> /foo
        prefix := fmt.Sprintf("/api/v1/tracelens/sessions/%s/ui", sessionID)
        req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
        if req.URL.Path == "" {
            req.URL.Path = "/"
        }
        
        // Forward headers
        req.Header.Set("X-Forwarded-For", c.ClientIP())
        req.Header.Set("X-Forwarded-Proto", "https")
    }
    
    // Modify response
    proxy.ModifyResponse = func(resp *http.Response) error {
        // Rewrite redirect locations
        if location := resp.Header.Get("Location"); location != "" {
            newLocation := rewriteLocation(location, sessionID)
            resp.Header.Set("Location", newLocation)
        }
        return nil
    }
    
    proxy.ServeHTTP(c.Writer, c.Request)
}

func (h *ProxyHandler) handleWebSocket(c *gin.Context, session *Session, targetURL *url.URL) {
    // Upgrade client connection
    clientConn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
        InsecureSkipVerify: true,
    })
    if err != nil {
        log.Errorf("Failed to accept WebSocket: %v", err)
        return
    }
    defer clientConn.Close(websocket.StatusNormalClosure, "")
    
    // Connect to backend
    wsURL := fmt.Sprintf("ws://%s:%d%s", session.PodIP, session.PodPort, c.Request.URL.Path)
    backendConn, _, err := websocket.Dial(c, wsURL, nil)
    if err != nil {
        log.Errorf("Failed to connect to backend WebSocket: %v", err)
        return
    }
    defer backendConn.Close(websocket.StatusNormalClosure, "")
    
    // Bidirectional copy
    errc := make(chan error, 2)
    
    go func() {
        errc <- copyWebSocket(clientConn, backendConn)
    }()
    
    go func() {
        errc <- copyWebSocket(backendConn, clientConn)
    }()
    
    <-errc // Wait for either direction to fail
}
```

---

## 4. Security Considerations

### 4.1 Authentication & Authorization

| Check | Description |
|-------|-------------|
| **API Authentication** | All endpoints require valid JWT token |
| **Session Ownership** | Users can only access their own sessions |
| **Workload Access** | Verify user has access to the workload |
| **File Access** | Validate file belongs to the workload |

### 4.2 Pod Security

```yaml
# Pod Security Standards (Restricted)
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  fsGroup: 1000
  seccompProfile:
    type: RuntimeDefault

# Container security
containers:
- name: tracelens
  securityContext:
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    capabilities:
      drop:
        - ALL
```

### 4.3 Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: tracelens-session-policy
  namespace: primus-lens
spec:
  podSelector:
    matchLabels:
      app: tracelens-session
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Only allow from API server
  - from:
    - podSelector:
        matchLabels:
          app: api-server
    ports:
    - port: 8501
  egress:
  # Allow DNS
  - to:
    - namespaceSelector: {}
    ports:
    - port: 53
      protocol: UDP
  # Block all other egress
```

---

## 5. Monitoring & Observability

### 5.1 Metrics

```go
// Prometheus metrics
var (
    sessionsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "lens",
            Subsystem: "tracelens",
            Name:      "sessions_total",
            Help:      "Total number of TraceLens sessions created",
        },
        []string{"status"},
    )
    
    sessionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "lens",
            Subsystem: "tracelens",
            Name:      "session_duration_seconds",
            Help:      "Duration of TraceLens sessions",
            Buckets:   []float64{60, 300, 600, 1800, 3600},
        },
        []string{"resource_profile"},
    )
    
    activeSessions = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Namespace: "lens",
            Subsystem: "tracelens",
            Name:      "active_sessions",
            Help:      "Number of currently active TraceLens sessions",
        },
    )
    
    podStartupDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Namespace: "lens",
            Subsystem: "tracelens",
            Name:      "pod_startup_seconds",
            Help:      "Time taken for TraceLens pod to become ready",
            Buckets:   []float64{5, 10, 20, 30, 45, 60, 90, 120},
        },
    )
)
```

### 5.2 Logging

```go
// Structured logging for key events
log.WithFields(log.Fields{
    "session_id":   sessionID,
    "workload_uid": workloadUID,
    "user_id":      userID,
    "file_id":      fileID,
    "action":       "create_session",
}).Info("Creating TraceLens session")
```

### 5.3 Alerts

```yaml
# Prometheus AlertManager rules
groups:
- name: tracelens
  rules:
  - alert: TraceLensSessionCreationFailed
    expr: rate(lens_tracelens_sessions_total{status="failed"}[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "TraceLens session creation failures"
      
  - alert: TraceLensHighPodStartupTime
    expr: histogram_quantile(0.95, lens_tracelens_pod_startup_seconds_bucket) > 60
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "TraceLens pod startup time is high"
      
  - alert: TraceLensTooManyActiveSessions
    expr: lens_tracelens_active_sessions > 20
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Too many active TraceLens sessions"
```

---

## 6. Implementation Plan

### Phase 1: Foundation (Week 1)

| Task | Effort | Owner |
|------|--------|-------|
| Create database schema | 2h | Backend |
| Build TraceLens Docker image | 4h | DevOps |
| Implement session repository | 4h | Backend |
| Basic CRUD API endpoints | 8h | Backend |

### Phase 2: Pod Management (Week 2)

| Task | Effort | Owner |
|------|--------|-------|
| Kubernetes client integration | 4h | Backend |
| Pod template and creation | 8h | Backend |
| Pod status watching | 4h | Backend |
| Session lifecycle management | 8h | Backend |
| Cleanup CronJob | 4h | Backend |

### Phase 3: Proxy Implementation (Week 3)

| Task | Effort | Owner |
|------|--------|-------|
| HTTP reverse proxy | 8h | Backend |
| WebSocket proxy | 8h | Backend |
| Path rewriting | 4h | Backend |
| Error handling | 4h | Backend |

### Phase 4: Testing & Polish (Week 4)

| Task | Effort | Owner |
|------|--------|-------|
| Integration tests | 8h | QA |
| Load testing | 4h | QA |
| Documentation | 4h | All |
| Security review | 4h | Security |
| Deployment to staging | 4h | DevOps |

---

## 7. Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Pod startup too slow | Medium | Medium | Pod pool pre-warming |
| WebSocket proxy instability | High | Low | Use battle-tested library |
| Resource exhaustion | High | Medium | Strict limits, cleanup |
| File access issues | Medium | Low | Proper volume mounts |
| Security vulnerabilities | High | Low | Security review, network policies |

---

## 8. Future Enhancements

1. **Pod Pool Pre-warming**: Keep idle pods ready for instant assignment
2. **Session Sharing**: Allow sharing analysis results via URL
3. **Batch Analysis**: Queue multiple files for analysis
4. **Custom Analysis Scripts**: User-provided analysis extensions
5. **Result Caching**: Cache analysis results to avoid re-computation

---

## 9. References

- [TraceLens GitHub Repository](https://github.com/AMD-AGI/TraceLens)
- [Streamlit Documentation](https://docs.streamlit.io/)
- [Kubernetes Pod API](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/)
- [WebSocket Proxy Best Practices](https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API)

