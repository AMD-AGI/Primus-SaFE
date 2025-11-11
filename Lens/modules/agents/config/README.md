# Configuration Guide

## Configuration File Loading Priority

Configuration loading follows the following priority (from high to low):

1. **Environment Variables** - Highest priority
2. **config.local.yaml** - Local configuration (not committed to Git)
3. **config.yaml** - Default configuration

## Usage

### Method 1: Using Configuration File (Recommended)

Create a local configuration file:

```bash
cd Lens/modules/agents
cp config/config.yaml config/config.local.yaml
```

Edit `config.local.yaml` and modify the required configuration items:

```yaml
# Lens API Configuration
lens:
  api_url: "http://localhost:30182"
  cluster_name: "x-flannel"
  timeout: 30

# LLM Configuration
llm:
  provider: "openai"
  model: "deepseek-ai/DeepSeek-V3-0324"
  api_key: "EMPTY"
  base_url: "https://tw325.primus-safe.amd.com/deepseekv3/v1"
  temperature: 0
  max_tokens: 2000

# Agent Configuration
agent:
  max_iterations: 10
  timeout: 120
```

Then start directly:

```bash
python -m api.main
```

### Method 2: Using Environment Variables

Environment variables will override values in the configuration file:

```bash
# Linux/Mac
export LENS_API_URL="http://localhost:30182"
export CLUSTER_NAME="x-flannel"
export LLM_MODEL="deepseek-ai/DeepSeek-V3-0324"
export LLM_API_KEY="EMPTY"
export LLM_BASE_URL="https://tw325.primus-safe.amd.com/deepseekv3/v1"

# Windows PowerShell
$env:LENS_API_URL="http://localhost:30182"
$env:CLUSTER_NAME="x-flannel"
$env:LLM_MODEL="deepseek-ai/DeepSeek-V3-0324"
$env:LLM_API_KEY="EMPTY"
$env:LLM_BASE_URL="https://tw325.primus-safe.amd.com/deepseekv3/v1"

# Then start
python -m api.main
```

### Method 3: Specifying Configuration File Path

Use the `CONFIG_FILE` environment variable to specify the configuration file:

```bash
export CONFIG_FILE="/path/to/your/config.yaml"
python -m api.main
```

## Supported Environment Variables

### API Configuration
- `API_HOST` - API listening address (default: 0.0.0.0)
- `API_PORT` - API port (default: 8001)

### Lens API Configuration
- `LENS_API_URL` - Lens API address
- `CLUSTER_NAME` - Cluster name
- `LENS_TIMEOUT` - Request timeout (seconds)

### LLM Configuration
- `LLM_PROVIDER` - LLM provider (openai/anthropic)
- `LLM_MODEL` - Model name
- `LLM_API_KEY` - API key
- `LLM_BASE_URL` - Custom API endpoint
- `LLM_TEMPERATURE` - Temperature parameter
- `LLM_MAX_TOKENS` - Maximum token count

### Agent Configuration
- `AGENT_MAX_ITERATIONS` - Maximum iteration count
- `AGENT_TIMEOUT` - Timeout (seconds)

### Logging Configuration
- `LOG_LEVEL` - Log level (DEBUG/INFO/WARNING/ERROR)

## Configuration File Description

### Complete Configuration Example

See `config.yaml` for complete configuration item descriptions.

### Main Configuration Items

#### API Configuration
```yaml
api:
  host: "0.0.0.0"        # Listening address
  port: 8001             # Port
  title: "GPU Usage Analysis Agent API"
  version: "1.0.0"
  cors:
    enabled: true        # Enable CORS
    origins:
      - "*"              # Allowed origins
```

#### Lens API Configuration
```yaml
lens:
  api_url: "http://localhost:30182"    # Lens API address
  cluster_name: "x-flannel"            # Default cluster name
  timeout: 30                          # Request timeout (seconds)
```

#### LLM Configuration
```yaml
llm:
  provider: "openai"                   # openai, anthropic, local
  model: "deepseek-ai/DeepSeek-V3-0324"
  api_key: "EMPTY"                     # API key
  base_url: "https://tw325.primus-safe.amd.com/deepseekv3/v1"
  temperature: 0                       # Temperature parameter
  max_tokens: 2000                     # Maximum token count
```

#### Agent Configuration
```yaml
agent:
  max_iterations: 10                   # Maximum iteration count
  timeout: 120                         # Timeout (seconds)
  
  features:
    trend_analysis: true               # Trend analysis
    comparison: true                   # Comparison analysis
    drill_down: true                   # Root cause drill-down
    anomaly_detection: false           # Anomaly detection (not implemented)
    cost_analysis: false               # Cost analysis (not implemented)
```

## Best Practices

1. **Development Environment**: Use `config.local.yaml` to configure local development environment
2. **Production Environment**: Use environment variables or Docker secrets to manage sensitive configuration
3. **Don't Commit Sensitive Information**: Add `config.local.yaml` to `.gitignore`
4. **Use Different Configuration Files**: Create different configuration files for different environments

## Example Scenarios

### Scenario 1: Local Development

Create `config.local.yaml`:
```yaml
lens:
  api_url: "http://localhost:30182"
  cluster_name: "x-flannel"

llm:
  provider: "openai"
  model: "deepseek-ai/DeepSeek-V3-0324"
  api_key: "your-dev-key"
  base_url: "https://tw325.primus-safe.amd.com/deepseekv3/v1"
```

Start:
```bash
python -m api.main
```

### Scenario 2: Testing Different LLMs

Temporarily switch via environment variables:
```bash
export LLM_MODEL="gpt-4-turbo"
export LLM_BASE_URL=""
python -m api.main
```

### Scenario 3: Docker Deployment

Use environment variables:
```bash
docker run -d \
  -e LENS_API_URL="http://lens-api:8080" \
  -e LLM_API_KEY="your-key" \
  -e LLM_MODEL="gpt-4" \
  -p 8001:8001 \
  gpu-usage-agent
```

### Scenario 4: Multi-Cluster Support

Don't set default cluster, specify in request:
```yaml
lens:
  api_url: "http://localhost:30182"
  cluster_name: null  # Don't set default value
```

Specify in request:
```bash
curl -X POST "http://localhost:8001/api/gpu-analysis/chat" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What is the current GPU utilization of the cluster?",
    "cluster_name": "x-flannel"
  }'
```

## Troubleshooting

### Configuration File Not Found
If no configuration file is found, the program will use default configuration, but necessary values (like API_KEY) need to be provided via environment variables.

### Environment Variables Not Taking Effect
Make sure to set environment variables before starting the program, or set them in the same command:
```bash
LENS_API_URL="http://localhost:30182" python -m api.main
```

### View Current Configuration
Configuration information will be printed at startup. Check the logs to confirm if the configuration is loaded correctly.
