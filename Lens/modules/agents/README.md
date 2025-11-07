# GPU Usage Analysis Agent

基于 LangGraph 的 GPU 使用率分析对话 Agent，能够通过自然语言对话帮助用户分析集群 GPU 使用率的趋势、对比、异常，并下钻到具体的 namespace、用户和 workload 级别。

## 功能特性

### 核心能力

1. **趋势分析**：查询不同维度、不同时间粒度的 GPU 使用率趋势
2. **对比分析**：对比不同时间段、不同实体的使用情况
3. **根因下钻**：从集群 → namespace → 用户 → workload 逐层分析
4. **实时状态**：查询当前的 GPU 分配和使用情况
5. **智能交互** ✨：
   - 主动获取可用集群、namespace、label 列表
   - 当信息不足时友好地反问用户
   - 提供可选项帮助用户选择

### 应用场景

- **场景 1**："最近几天的使用率变化趋势是怎么样的？"
- **场景 2**："为什么这周 ml-team 的使用率比上周低了？是因为哪些 workload 导致的？"
- **场景 3**："当前集群有多少 GPU 在使用？"
- **场景 4**："ml-team 和 cv-team 的 GPU 使用情况对比"
- **场景 5** ✨："我想看看GPU使用情况" → Agent 主动询问集群、提供选项
- **场景 6** ✨："查询某个namespace的使用率" → Agent 展示可用的 namespace 列表

## 架构设计

### 系统架构

```
┌────────────────────────────────────────────────────────────────┐
│                  GPU 使用率分析 Agent 系统                        │
└────────────────────────────────────────────────────────────────┘

┌──────────────┐      ┌─────────────────────────────────────┐
│   用户对话   │────▶│      对话理解层 (NLU)                │
│   前端界面   │      │  - 意图识别（趋势/对比/下钻）        │
└──────────────┘      │  - 实体提取（时间/namespace）        │
                      └───────────────┬─────────────────────┘
                                      │
                      ┌───────────────▼─────────────────────┐
                      │     Agent 编排层 (LangGraph)        │
                      │  - 查询规划                          │
                      │  - 多步推理                          │
                      │  - 上下文管理                        │
                      └───────────────┬─────────────────────┘
                                      │
            ┌────────────────────────┼────────────────────────┐
            │                        │                        │
┌───────────▼──────────┐  ┌─────────▼────────┐  ┌──────────▼───────┐
│   GPU统计查询工具    │  │  Workload分析    │  │  实时快照工具    │
│ - 集群小时统计       │  │  - 历史记录      │  │  - 最新状态      │
│ - Namespace统计      │  │  - 元数据        │  │  - 分配详情      │
│ - Label统计          │  │  - 层级关系      │  │                  │
└──────────┬───────────┘  └─────────┬────────┘  └──────────┬───────┘
           │                        │                        │
┌──────────▼──────────────────────────▼────────────────────▼───────┐
│                     Lens API (数据源层)                            │
│  PostgreSQL    Prometheus    Kubernetes    Redis                 │
└───────────────────────────────────────────────────────────────────┘
```

### Agent 工作流程

```
[用户查询] 
    ↓
┌────────────────┐
│ Understand     │  ← 理解查询：意图识别 + 实体提取
│ (调用 LLM)     │
└────────┬───────┘
         │
         ├─→ [需要澄清?] → 返回澄清问题 → [END]
         │
         ↓
┌────────────────┐
│ Plan           │  ← 规划分析：制定数据收集计划
│ (调用 LLM)     │
└────────┬───────┘
         │
         ↓
┌────────────────┐
│ Execute        │  ← 执行步骤：准备工具调用参数
└────────┬───────┘
         │
         ├─→ [需要调用工具?] ─Yes→ ┌──────────┐
         │                          │ Tools    │ ← 执行工具
         │                          │ (实际查询)│
         │                          └────┬─────┘
         │                               │
         │  ←──────────────────────────┘
         │
         ↓
┌────────────────┐
│ Synthesize     │  ← 综合分析：提取洞察
│ (调用 LLM)     │
└────────┬───────┘
         │
         ↓
┌────────────────┐
│ Respond        │  ← 生成响应：格式化答案
└────────┬───────┘
         │
         ↓
     [END]
```

## 快速开始

### 环境要求

- Python 3.9+
- Lens API 服务运行中
- OpenAI API Key 或其他 LLM 访问权限

### 安装依赖

```bash
cd Lens/modules/agents
pip install -r requirements.txt
```

### 配置

1. 复制配置文件：
```bash
cp config/config.yaml config/config.local.yaml
```

2. 编辑配置文件，设置必要的参数：
```yaml
lens:
  api_url: "http://localhost:8080"

llm:
  provider: "openai"
  model: "gpt-4"
  api_key: "your-api-key"
```

3. 或者使用环境变量：
```bash
export LENS_API_URL="http://localhost:8080"
export LLM_PROVIDER="openai"
export LLM_MODEL="gpt-4"
export LLM_API_KEY="your-api-key"
```

### 启动服务

```bash
cd Lens/modules/agents/api
python main.py
```

服务将在 `http://localhost:8001` 启动。

### API 文档

启动服务后，访问以下地址查看 API 文档：
- Swagger UI: http://localhost:8001/docs
- ReDoc: http://localhost:8001/redoc

## 使用示例

### Python 客户端

```python
import requests

# 发起对话请求
response = requests.post(
    "http://localhost:8001/api/gpu-analysis/chat",
    json={
        "query": "最近7天的GPU使用率趋势如何？",
        "cluster_name": "gpu-cluster-01"
    }
)

result = response.json()
print(result["answer"])
print("洞察:", result["insights"])
```

### cURL

```bash
curl -X POST "http://localhost:8001/api/gpu-analysis/chat" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "最近7天的GPU使用率趋势如何？",
    "cluster_name": "gpu-cluster-01"
  }'
```

### 查询示例

1. **趋势分析**
```
问：最近7天的GPU使用率趋势如何？
答：最近7天集群GPU使用率整体呈上升趋势，从 55% 上升到 68%。
    主要由 ml-team 和 cv-team 贡献...
```

2. **对比分析**
```
问：本周和上周的使用率对比
答：本周平均使用率 65%，比上周的 58% 上升了 7 个百分点...
```

3. **根因下钻**
```
问：为什么 ml-team 的使用率下降了？
答：ml-team 本周使用率为 45.2%，比上周的 62.8% 下降了 28%。
    主要原因是 user_a 的任务大幅减少，从 28 个降至 12 个...
```

4. **实时状态**
```
问：当前集群有多少GPU在使用？
答：当前集群共有 128 个 GPU，已分配 96 个（分配率 75%）...
```

5. **智能交互 - 笼统查询** ✨
```
问：我想看看GPU使用情况
答：要查看 GPU 使用情况，我需要知道您想查询哪个集群。

    当前系统中有以下集群：
    1. gpu-cluster-01
    2. gpu-cluster-02
    3. gpu-cluster-03
    
    请告诉我您想查询哪个集群，或者回复"全部"查看所有集群的汇总情况。
```

6. **智能交互 - 提供选项** ✨
```
问：查询某个namespace的GPU使用率趋势
答：要查询 namespace 的 GPU 使用率趋势，请从以下 namespace 中选择一个：

    最近 7 天有 GPU 使用的 namespaces：
    1. ml-training
    2. ml-inference
    3. data-processing
    4. production
    
    请告诉我您想查询哪个 namespace。
```

## 工具集说明

### 数据查询工具

#### 1. query_gpu_usage_trend

查询 GPU 使用率趋势数据。

**参数：**
- `dimension`: 查询维度（cluster/namespace/label）
- `granularity`: 时间粒度（hour/day）
- `time_range_days`: 时间范围（天数）
- `dimension_value`: 维度的具体值（可选）
- `metric_type`: 指标类型（utilization/allocation_rate）

#### 2. analyze_workload_history

分析 workload 历史记录。

**参数：**
- `time_range_days`: 时间范围（天数）
- `namespace`: 筛选的 namespace（可选）
- `kind`: workload 类型（可选）
- `status`: workload 状态（可选）
- `sort_by`: 排序字段
- `limit`: 返回数量

#### 3. get_latest_snapshot

获取最新的 GPU 分配快照（实时状态）。

#### 4. get_workload_metadata

获取 workload 元数据（所有 namespaces 和 kinds）。

### 元数据发现工具 ✨

#### 5. get_available_clusters

获取所有可用的集群列表。

**使用场景：** 用户没有指定集群或不确定有哪些集群时。

#### 6. get_available_namespaces

获取指定时间范围内有 GPU 分配数据的 namespaces。

**参数：**
- `time_range_days`: 时间范围（默认 7 天）
- `cluster`: 集群名称（可选）

**使用场景：** 展示最近活跃的 namespaces，帮助用户选择。

#### 7. get_available_dimension_keys

获取可用的 label 或 annotation keys。

**参数：**
- `dimension_type`: "label" 或 "annotation"
- `time_range_days`: 时间范围（默认 7 天）
- `cluster`: 集群名称（可选）

**使用场景：** 用户想按 label 筛选但不知道有哪些可用的 keys。

## 开发指南

### 目录结构

```
agents/
├── __init__.py
├── gpu_usage_agent/
│   ├── __init__.py
│   ├── agent.py              # Agent 主逻辑
│   ├── tools.py              # 工具集定义
│   ├── prompts.py            # Prompt 模板
│   ├── state.py              # 状态定义
│   └── data_access.py        # 数据访问层
├── api/
│   └── main.py               # FastAPI 应用
├── config/
│   └── config.yaml           # 配置文件
├── tests/
│   ├── test_agent.py
│   └── test_tools.py
├── requirements.txt
└── README.md
```

### 添加新工具

1. 在 `tools.py` 中定义新工具：
```python
@tool
def your_new_tool(param1: str, param2: int) -> str:
    """工具描述"""
    # 实现逻辑
    return result
```

2. 将工具添加到工具列表：
```python
def get_tools(self) -> List:
    return [
        self.query_gpu_usage_trend,
        self.analyze_workload_history,
        self.your_new_tool,  # 新工具
        # ...
    ]
```

### 自定义 Prompt

编辑 `prompts.py` 文件，修改各阶段的 Prompt 模板。

### 扩展数据访问层

在 `data_access.py` 中添加新的 API 调用方法。

## 测试

```bash
# 运行所有测试
pytest tests/

# 运行特定测试
pytest tests/test_agent.py

# 带覆盖率报告
pytest --cov=gpu_usage_agent tests/
```

## 部署

### Docker 部署

```bash
# 构建镜像
docker build -t gpu-usage-agent:latest .

# 运行容器
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

## 性能优化

### 1. 缓存策略

- Redis 缓存热门查询结果（TTL: 5分钟）
- 语义缓存复用相似查询

### 2. LLM 调用优化

- 简单查询使用规则匹配，不调用 LLM
- 意图识别使用更便宜的模型
- 设置合理的超时时间

### 3. 并行处理

- 并行调用多个工具
- 异步流式输出

## 监控与可观测性

### 业务指标

- 查询响应时间（P50/P95/P99）
- 意图识别准确率
- 工具调用成功率

### 技术指标

- LLM 调用次数和耗时
- API 查询耗时
- 错误率

## 未来优化方向

### 短期（1-2周）

- [x] 实现基础工具集
- [x] 搭建 Agent 框架和状态机
- [x] 完成 API 接口
- [ ] 完成单元测试
- [ ] Docker 化部署

### 中期（1-2月）

- [ ] 引入流式输出和实时反馈
- [ ] 优化 Prompt，提升意图识别准确率
- [ ] 添加对比分析和下钻分析功能
- [ ] 添加可视化图表生成

### 长期（3-6月）

- [ ] 异常检测功能
- [ ] 成本分析功能
- [ ] 知识库检索功能
- [ ] 主动异常告警和建议

## 常见问题

### Q1: 如何保证分析的准确性？

A: 所有结论都基于实际数据，LLM 只负责理解和表达，不做数据计算。关键指标来自可靠的数据源（PostgreSQL、Prometheus）。

### Q2: 如何控制 LLM 成本？

A: 
- 使用语义缓存复用相似查询结果
- 简单查询使用规则匹配
- 意图识别使用更便宜的模型（如 GPT-3.5）

### Q3: 支持哪些语言？

A: 初期支持中文和英文，Prompt 设计支持多语言。

### Q4: 如何处理复杂的多轮对话？

A:
- 维护完整的对话历史
- 使用上下文引用解析（指代消解）
- 支持追问和澄清
- 保持会话状态

### Q5: Agent 如何知道应该询问哪些信息？ ✨

A: Agent 通过增强的意图识别 Prompt 自动判断：
- 检测缺失的必要信息（集群、namespace、时间范围等）
- 自动获取可用选项（调用元数据 API）
- 使用 LLM 生成友好的提问和选项展示
- 详见 [METADATA_INTERACTION.md](METADATA_INTERACTION.md)

## 相关文档

- 设置指南：[SETUP_GUIDE.md](SETUP_GUIDE.md)
- 项目结构：[PROJECT_STRUCTURE.md](PROJECT_STRUCTURE.md)
- 元数据交互：[METADATA_INTERACTION.md](METADATA_INTERACTION.md) ✨
- GPU Aggregation API：[../../docs/api/gpu-aggregation.md](../../docs/api/gpu-aggregation.md)

## License

[MIT License](LICENSE)

## 维护者

AMD-AGI Team

## 更新日志

### v1.1.0 (2025-11-06) ✨

- **新增智能元数据交互功能**
  - 自动获取可用集群列表
  - 展示可选的 namespace 列表
  - 提供可用的 label/annotation keys
  - 友好的反问机制
- 增强的 Prompt 模板
- 新增 3 个元数据工具
- 完善的测试用例
- 详细的交互文档

### v1.0.0 (2025-11-05)

- 初始版本发布
- 实现基础的趋势分析和实时查询功能
- 支持集群、namespace、label 多维度分析
- 提供 REST API 接口

