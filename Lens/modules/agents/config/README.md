# 配置说明

## 配置文件加载优先级

配置加载遵循以下优先级（从高到低）：

1. **环境变量** - 优先级最高
2. **config.local.yaml** - 本地配置（不会提交到 Git）
3. **config.yaml** - 默认配置

## 使用方法

### 方法 1: 使用配置文件（推荐）

创建本地配置文件：

```bash
cd Lens/modules/agents
cp config/config.yaml config/config.local.yaml
```

编辑 `config.local.yaml`，修改需要的配置项：

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

然后直接启动：

```bash
python -m api.main
```

### 方法 2: 使用环境变量

环境变量会覆盖配置文件中的值：

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

# 然后启动
python -m api.main
```

### 方法 3: 指定配置文件路径

使用 `CONFIG_FILE` 环境变量指定配置文件：

```bash
export CONFIG_FILE="/path/to/your/config.yaml"
python -m api.main
```

## 支持的环境变量

### API 配置
- `API_HOST` - API 监听地址（默认: 0.0.0.0）
- `API_PORT` - API 端口（默认: 8001）

### Lens API 配置
- `LENS_API_URL` - Lens API 地址
- `CLUSTER_NAME` - 集群名称
- `LENS_TIMEOUT` - 请求超时时间（秒）

### LLM 配置
- `LLM_PROVIDER` - LLM 提供商（openai/anthropic）
- `LLM_MODEL` - 模型名称
- `LLM_API_KEY` - API 密钥
- `LLM_BASE_URL` - 自定义 API 端点
- `LLM_TEMPERATURE` - 温度参数
- `LLM_MAX_TOKENS` - 最大 token 数

### Agent 配置
- `AGENT_MAX_ITERATIONS` - 最大迭代次数
- `AGENT_TIMEOUT` - 超时时间（秒）

### 日志配置
- `LOG_LEVEL` - 日志级别（DEBUG/INFO/WARNING/ERROR）

## 配置文件说明

### 完整配置示例

查看 `config.yaml` 获取完整的配置项说明。

### 主要配置项

#### API 配置
```yaml
api:
  host: "0.0.0.0"        # 监听地址
  port: 8001             # 端口
  title: "GPU Usage Analysis Agent API"
  version: "1.0.0"
  cors:
    enabled: true        # 是否启用 CORS
    origins:
      - "*"              # 允许的来源
```

#### Lens API 配置
```yaml
lens:
  api_url: "http://localhost:30182"    # Lens API 地址
  cluster_name: "x-flannel"            # 默认集群名称
  timeout: 30                          # 请求超时时间（秒）
```

#### LLM 配置
```yaml
llm:
  provider: "openai"                   # openai, anthropic, local
  model: "deepseek-ai/DeepSeek-V3-0324"
  api_key: "EMPTY"                     # API 密钥
  base_url: "https://tw325.primus-safe.amd.com/deepseekv3/v1"
  temperature: 0                       # 温度参数
  max_tokens: 2000                     # 最大 token 数
```

#### Agent 配置
```yaml
agent:
  max_iterations: 10                   # 最大迭代次数
  timeout: 120                         # 超时时间（秒）
  
  features:
    trend_analysis: true               # 趋势分析
    comparison: true                   # 对比分析
    drill_down: true                   # 根因下钻
    anomaly_detection: false           # 异常检测（未实现）
    cost_analysis: false               # 成本分析（未实现）
```

## 最佳实践

1. **开发环境**: 使用 `config.local.yaml` 配置本地开发环境
2. **生产环境**: 使用环境变量或 Docker secrets 管理敏感配置
3. **不要提交敏感信息**: 将 `config.local.yaml` 加入 `.gitignore`
4. **使用不同的配置文件**: 为不同环境创建不同的配置文件

## 示例场景

### 场景 1: 本地开发

创建 `config.local.yaml`:
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

启动：
```bash
python -m api.main
```

### 场景 2: 测试不同的 LLM

通过环境变量临时切换：
```bash
export LLM_MODEL="gpt-4-turbo"
export LLM_BASE_URL=""
python -m api.main
```

### 场景 3: Docker 部署

使用环境变量：
```bash
docker run -d \
  -e LENS_API_URL="http://lens-api:8080" \
  -e LLM_API_KEY="your-key" \
  -e LLM_MODEL="gpt-4" \
  -p 8001:8001 \
  gpu-usage-agent
```

### 场景 4: 多集群支持

不设置默认集群，在请求中指定：
```yaml
lens:
  api_url: "http://localhost:30182"
  cluster_name: null  # 不设置默认值
```

请求时指定：
```bash
curl -X POST "http://localhost:8001/api/gpu-analysis/chat" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "当前集群GPU使用率？",
    "cluster_name": "x-flannel"
  }'
```

## 故障排查

### 配置文件未找到
如果没有配置文件，程序会使用默认配置，但需要通过环境变量提供必要的值（如 API_KEY）。

### 环境变量不生效
确保在启动程序之前设置环境变量，或在同一命令中设置：
```bash
LENS_API_URL="http://localhost:30182" python -m api.main
```

### 查看当前配置
启动时会打印配置信息，检查日志确认配置是否正确加载。

