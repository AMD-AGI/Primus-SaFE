# GPU Usage Analysis Agent

A LangGraph-based GPU utilization analysis conversational agent that helps users analyze GPU usage trends, comparisons, and anomalies through natural language dialogue, with drill-down capabilities to namespace, user, and workload levels.

## Features

### Core Capabilities

1. **Trend Analysis**: Query GPU utilization trends across different dimensions and time granularities
2. **Comparison Analysis**: Compare usage across different time periods and entities
3. **Root Cause Drill-down**: Layer-by-layer analysis from cluster → namespace → user → workload
4. **Real-time Status**: Query current GPU allocation and usage
5. **Intelligent Interaction** ✨:
   - Proactively fetch available clusters, namespaces, and label lists
   - Friendly follow-up questions when information is insufficient
   - Provide options to help users choose

### Use Cases

- **Scenario 1**: "What's the utilization trend over the past few days?"
- **Scenario 2**: "Why is ml-team's utilization lower this week than last week? Which workloads caused this?"
- **Scenario 3**: "How many GPUs are currently in use in the cluster?"
- **Scenario 4**: "Compare GPU usage between ml-team and cv-team"
- **Scenario 5** ✨: "I want to check GPU usage" → Agent proactively asks about cluster and provides options
- **Scenario 6** ✨: "Query utilization of a namespace" → Agent displays available namespace list

## Architecture Design

### System Architecture

```
┌────────────────────────────────────────────────────────────────┐
│              GPU Usage Analysis Agent System                      │
└────────────────────────────────────────────────────────────────┘

┌──────────────┐      ┌─────────────────────────────────────┐
│  User Dialog │────▶│      Dialog Understanding (NLU)      │
│  Front-end   │      │  - Intent Recognition (trend/compare)│
└──────────────┘      │  - Entity Extraction (time/namespace)│
                      └───────────────┬─────────────────────┘
                                      │
                      ┌───────────────▼─────────────────────┐
                      │   Agent Orchestration (LangGraph)   │
                      │  - Query Planning                    │
                      │  - Multi-step Reasoning              │
                      │  - Context Management                │
                      └───────────────┬─────────────────────┘
                                      │
            ┌────────────────────────┼────────────────────────┐
            │                        │                        │
┌───────────▼──────────┐  ┌─────────▼────────┐  ┌──────────▼───────┐
│   GPU Stats Query    │  │  Workload        │  │  Real-time       │
│   Tools              │  │  Analysis        │  │  Snapshot Tools  │
│ - Cluster hourly     │  │  - History       │  │  - Latest state  │
│ - Namespace stats    │  │  - Metadata      │  │  - Allocation    │
│ - Label stats        │  │  - Hierarchy     │  │    details       │
└──────────┬───────────┘  └─────────┬────────┘  └──────────┬───────┘
           │                        │                        │
┌──────────▼──────────────────────────▼────────────────────▼───────┐
│                    Lens API (Data Source Layer)                   │
│  PostgreSQL    Prometheus    Kubernetes    Redis                  │
└───────────────────────────────────────────────────────────────────┘
```

### Agent Workflow

```
[User Query] 
    ↓
┌────────────────┐
│ Understand     │  ← Understand query: Intent recognition + Entity extraction
│ (Call LLM)     │
└────────┬───────┘
         │
         ├─→ [Need clarification?] → Return clarification question → [END]
         │
         ↓
┌────────────────┐
│ Plan           │  ← Plan analysis: Develop data collection plan
│ (Call LLM)     │
└────────┬───────┘
         │
         ↓
┌────────────────┐
│ Execute        │  ← Execute steps: Prepare tool call parameters
└────────┬───────┘
         │
         ├─→ [Need to call tools?] ─Yes→ ┌──────────┐
         │                                │ Tools    │ ← Execute tools
         │                                │ (Actual  │
         │                                │  query)  │
         │                                └────┬─────┘
         │                                     │
         │  ←──────────────────────────────────┘
         │
         ↓
┌────────────────┐
│ Synthesize     │  ← Synthesize analysis: Extract insights
│ (Call LLM)     │
└────────┬───────┘
         │
         ↓
┌────────────────┐
│ Respond        │  ← Generate response: Format answer
└────────┬───────┘
         │
         ↓
     [END]
```

## Quick Start

### Requirements

- Python 3.9+
- Lens API service running
- OpenAI API Key or other LLM access

### Install Dependencies

```bash
cd Lens/modules/agents
pip install -r requirements.txt
```

### Configuration

1. Copy configuration file:
```bash
cp config/config.yaml config/config.local.yaml
```

2. Edit configuration file and set necessary parameters:
```yaml
lens:
  api_url: "http://localhost:8080"

llm:
  provider: "openai"
  model: "gpt-4"
  api_key: "your-api-key"

# Chat history storage configuration (optional)
storage:
  enabled: true
  backend: "file"  # Supports: file, db, pg
  
  # File storage (default)
  file:
    storage_dir: ".storage/conversations"
  
  # PostgreSQL storage (recommended for enterprise)
  pg:
    host: "localhost"
    port: 5432
    database: "agents"
    user: "postgres"
    password: ""
    schema: "public"
    min_connections: 1
    max_connections: 10
    sslmode: "prefer"  # disable, allow, prefer, require, verify-ca, verify-full
```

3. Or use environment variables:
```bash
export LENS_API_URL="http://localhost:8080"
export LLM_PROVIDER="openai"
export LLM_MODEL="gpt-4"
export LLM_API_KEY="your-api-key"

# Storage configuration
export STORAGE_BACKEND="pg"
export PG_HOST="localhost"
export PG_DATABASE="agents"
export PG_USER="postgres"
export PG_PASSWORD="your_password"
export PG_SCHEMA="public"
export PG_SSLMODE="prefer"  # disable, allow, prefer, require, verify-ca, verify-full
```

> 💡 **PostgreSQL Storage**: Production environments are recommended to use PostgreSQL storage for better performance and scalability. See [POSTGRES_SETUP.md](POSTGRES_SETUP.md) for details

### Start Service

```bash
cd Lens/modules/agents/api
python main.py
```

Service will start at `http://localhost:8001`.

### API Documentation

After starting the service, visit the following URLs to view API documentation:
- Swagger UI: http://localhost:8001/docs
- ReDoc: http://localhost:8001/redoc

## Usage Examples

### Python Client

```python
import requests

# Send chat request
response = requests.post(
    "http://localhost:8001/api/gpu-analysis/chat",
    json={
        "query": "What's the GPU utilization trend over the last 7 days?",
        "cluster_name": "gpu-cluster-01"
    }
)

result = response.json()
print(result["answer"])
print("Insights:", result["insights"])
```

### cURL

```bash
curl -X POST "http://localhost:8001/api/gpu-analysis/chat" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What is the GPU utilization trend over the last 7 days?",
    "cluster_name": "gpu-cluster-01"
  }'
```

### Query Examples

1. **Trend Analysis**
```
Q: What's the GPU utilization trend over the last 7 days?
A: Over the last 7 days, cluster GPU utilization has been trending upward, 
   rising from 55% to 68%. This is mainly contributed by ml-team and cv-team...
```

2. **Comparison Analysis**
```
Q: Compare this week's and last week's utilization
A: This week's average utilization is 65%, up 7 percentage points from last week's 58%...
```

3. **Root Cause Drill-down**
```
Q: Why has ml-team's utilization decreased?
A: ml-team's utilization this week is 45.2%, down 28% from last week's 62.8%. 
   The main reason is user_a's tasks significantly decreased from 28 to 12...
```

4. **Real-time Status**
```
Q: How many GPUs are currently in use in the cluster?
A: The cluster has a total of 128 GPUs, with 96 allocated (75% allocation rate)...
```

5. **Intelligent Interaction - General Query** ✨
```
Q: I want to check GPU usage
A: To view GPU usage, I need to know which cluster you want to query.

   The system has the following clusters:
   1. gpu-cluster-01
   2. gpu-cluster-02
   3. gpu-cluster-03
   
   Please tell me which cluster you want to query, or reply "all" to see a summary of all clusters.
```

6. **Intelligent Interaction - Providing Options** ✨
```
Q: Query GPU utilization trend for a namespace
A: To query GPU utilization trend for a namespace, please select from the following namespaces:

   Namespaces with GPU usage in the last 7 days:
   1. ml-training
   2. ml-inference
   3. data-processing
   4. production
   
   Please tell me which namespace you want to query.
```

## Tool Set Description

### Data Query Tools

#### 1. query_gpu_usage_trend

Query GPU utilization trend data.

**Parameters:**
- `dimension`: Query dimension (cluster/namespace/label)
- `granularity`: Time granularity (hour/day)
- `time_range_days`: Time range (days)
- `dimension_value`: Specific dimension value (optional)
- `metric_type`: Metric type (utilization/allocation_rate)

#### 2. analyze_workload_history

Analyze workload history records.

**Parameters:**
- `time_range_days`: Time range (days)
- `namespace`: Filter by namespace (optional)
- `kind`: Workload type (optional)
- `status`: Workload status (optional)
- `sort_by`: Sort field
- `limit`: Return count

#### 3. get_latest_snapshot

Get the latest GPU allocation snapshot (real-time status).

#### 4. get_workload_metadata

Get workload metadata (all namespaces and kinds).

### Metadata Discovery Tools ✨

#### 5. get_available_clusters

Get list of all available clusters.

**Use Case:** When user hasn't specified a cluster or is unsure which clusters exist.

#### 6. get_available_namespaces

Get namespaces with GPU allocation data within the specified time range.

**Parameters:**
- `time_range_days`: Time range (default 7 days)
- `cluster`: Cluster name (optional)

**Use Case:** Show recently active namespaces to help user choose.

#### 7. get_available_dimension_keys

Get available label or annotation keys.

**Parameters:**
- `dimension_type`: "label" or "annotation"
- `time_range_days`: Time range (default 7 days)
- `cluster`: Cluster name (optional)

**Use Case:** When user wants to filter by label but doesn't know which keys are available.

## Development Guide

### Directory Structure

```
agents/
├── __init__.py
├── gpu_usage_agent/
│   ├── __init__.py
│   ├── agent.py              # Agent main logic
│   ├── tools.py              # Tool set definitions
│   ├── prompts.py            # Prompt templates
│   ├── state.py              # State definitions
│   └── data_access.py        # Data access layer
├── api/
│   └── main.py               # FastAPI application
├── config/
│   └── config.yaml           # Configuration file
├── tests/
│   ├── test_agent.py
│   └── test_tools.py
├── requirements.txt
└── README.md
```

### Adding New Tools

1. Define new tool in `tools.py`:
```python
@tool
def your_new_tool(param1: str, param2: int) -> str:
    """Tool description"""
    # Implementation logic
    return result
```

2. Add tool to tool list:
```python
def get_tools(self) -> List:
    return [
        self.query_gpu_usage_trend,
        self.analyze_workload_history,
        self.your_new_tool,  # New tool
        # ...
    ]
```

### Custom Prompts

Edit the `prompts.py` file to modify prompt templates for each phase.

### Extend Data Access Layer

Add new API call methods in `data_access.py`.

## Testing

```bash
# Run all tests
pytest tests/

# Run specific test
pytest tests/test_agent.py

# With coverage report
pytest --cov=gpu_usage_agent tests/
```

## Deployment

### Docker Deployment

```bash
# Build image
docker build -t gpu-usage-agent:latest .

# Run container
docker run -d \
  -p 8001:8001 \
  -e LENS_API_URL=http://lens-api:8080 \
  -e LLM_API_KEY=your-api-key \
  gpu-usage-agent:latest
```

### Docker Compose

```yaml
services:
  gpu-analysis-agent:
    image: gpu-usage-agent:latest
    ports:
      - "8001:8001"
    environment:
      - LENS_API_URL=http://lens-api:8080
      - LLM_PROVIDER=openai
      - LLM_MODEL=gpt-4
      - LLM_API_KEY=${OPENAI_API_KEY}
    depends_on:
      - lens-api
```

## Performance Optimization

### 1. Caching Strategy

- Redis cache for popular query results (TTL: 5 minutes)
- Semantic cache to reuse similar queries

### 2. LLM Call Optimization

- Use rule matching for simple queries without calling LLM
- Use cheaper models for intent recognition
- Set reasonable timeout periods

### 3. Parallel Processing

- Parallel calls to multiple tools
- Asynchronous streaming output

## Monitoring and Observability

### Business Metrics

- Query response time (P50/P95/P99)
- Intent recognition accuracy
- Tool call success rate

### Technical Metrics

- LLM call count and duration
- API query duration
- Error rate

## Future Optimization Directions

### Short-term (1-2 weeks)

- [x] Implement basic tool set
- [x] Build agent framework and state machine
- [x] Complete API interface
- [ ] Complete unit tests
- [ ] Dockerize deployment

### Mid-term (1-2 months)

- [ ] Introduce streaming output and real-time feedback
- [ ] Optimize prompts to improve intent recognition accuracy
- [ ] Add comparison analysis and drill-down analysis
- [ ] Add visualization chart generation

### Long-term (3-6 months)

- [ ] Anomaly detection functionality
- [ ] Cost analysis functionality
- [ ] Knowledge base retrieval functionality
- [ ] Proactive anomaly alerts and recommendations

## FAQ

### Q1: How to ensure analysis accuracy?

A: All conclusions are based on actual data. LLM is only responsible for understanding and expression, not data calculation. Key metrics come from reliable data sources (PostgreSQL, Prometheus).

### Q2: How to control LLM costs?

A: 
- Use semantic cache to reuse similar query results
- Use rule matching for simple queries
- Use cheaper models for intent recognition (e.g., GPT-3.5)

### Q3: Which languages are supported?

A: Initially supports Chinese and English. Prompt design supports multilingual.

### Q4: How to handle complex multi-turn dialogues?

A:
- Maintain complete conversation history
- Use context reference resolution (anaphora resolution)
- Support follow-up questions and clarifications
- Maintain session state

### Q5: How does the agent know what information to ask for? ✨

A: The agent automatically determines through enhanced intent recognition prompts:
- Detects missing necessary information (cluster, namespace, time range, etc.)
- Automatically fetches available options (calls metadata API)
- Uses LLM to generate friendly questions and option displays
- See [METADATA_INTERACTION.md](METADATA_INTERACTION.md) for details

## Related Documentation

- Setup Guide: [SETUP_GUIDE.md](SETUP_GUIDE.md)
- Project Structure: [PROJECT_STRUCTURE.md](PROJECT_STRUCTURE.md)
- Metadata Interaction: [METADATA_INTERACTION.md](METADATA_INTERACTION.md) ✨
- GPU Aggregation API: [../../docs/api/gpu-aggregation.md](../../docs/api/gpu-aggregation.md)

## License

[MIT License](LICENSE)

## Maintainers

AMD-AGI Team

## Changelog

### v1.1.0 (2025-11-06) ✨

- **New Intelligent Metadata Interaction Features**
  - Automatically fetch available cluster list
  - Display optional namespace list
  - Provide available label/annotation keys
  - Friendly follow-up question mechanism
- Enhanced prompt templates
- Added 3 metadata tools
- Complete test cases
- Detailed interaction documentation

### v1.0.0 (2025-11-05)

- Initial version release
- Implemented basic trend analysis and real-time query functionality
- Support multi-dimensional analysis across cluster, namespace, and label
- Provided REST API interface
