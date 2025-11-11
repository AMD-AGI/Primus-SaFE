# GPU 使用率分析示例

本目录包含使用 GPU 使用率分析工具的示例代码。

## 文件说明

- `root_cause_analysis_example.py` - GPU 使用率下降根因分析完整示例

## 前置要求

1. **启动 Lens API 服务**
   ```bash
   cd Lens/modules/api
   go run cmd/primus-lens-api/main.go
   ```

2. **安装 Python 依赖**
   ```bash
   pip install requests langchain-core
   ```

3. **确保数据库中有数据**
   - 需要有 GPU 聚合统计数据
   - 可以通过 GPU aggregation job 生成

## 运行示例

### 根因分析示例

这个示例展示如何使用新增的 `get_available_dimension_values` 功能来分析集群 GPU 使用率下降的根本原因。

```bash
cd Lens/modules/agents/gpu_usage_agent/examples
python root_cause_analysis_example.py
```

**输出示例**:
```
🚀 开始 GPU 使用率下降根因分析...
   API: http://localhost:8080
   集群: default
   时间范围: 最近 7 天

📊 步骤 1: 分析集群整体使用率趋势（最近 7 天）...
   平均使用率: 37.76%
   最高使用率: 46.94%
   最低使用率: 28.52%
   趋势: decreasing

📦 步骤 2: 按 Namespace 分析...
   发现 5 个 namespaces
     - ml-training: 45.23%
     - ml-inference: 38.67%
     - data-processing: 32.45%
     - development: 28.91%
     - test: 15.34%

🏷️  步骤 3: 按 LABEL 分析...
   发现 3 个 label keys

   分析 label key: team
     发现 4 个不同的 values
       - ml-team: 42.15%
       - cv-team: 38.90%
       - nlp-team: 35.67%
       - data-team: 25.43%

   分析 label key: priority
     发现 3 个不同的 values
       - high: 45.78%
       - medium: 35.23%
       - low: 18.56%

🏷️  步骤 4: 按 ANNOTATION 分析...
   发现 2 个 annotation keys

   分析 annotation key: primus-safe.user.name
     发现 10 个不同的 values
       - zhangsan: 48.23%
       - lisi: 42.67%
       - wangwu: 38.45%
       - zhaoliu: 35.12%
       - ...

================================================================================
📈 GPU 使用率下降根因分析报告
================================================================================

【集群整体情况】
  平均使用率: 37.76%
  趋势: decreasing
  ⚠️  使用率呈下降趋势！

【Namespace 使用率最低的前 3 名】
  1. test: 15.34%
  2. development: 28.91%
  3. data-processing: 32.45%

【Label 使用率最低的前 3 名】
  1. priority=low: 18.56%
  2. team=data-team: 25.43%
  3. team=nlp-team: 35.67%

【Annotation 使用率最低的前 3 名】
  1. primus-safe.user.name=user123: 22.34%
  2. primus-safe.user.name=user456: 28.67%
  3. primus-safe.user.name=zhaoliu: 35.12%

【可能的根因】
  1. namespace:test 的平均使用率仅为 15.34%
     建议检查该维度下的任务是否存在资源浪费或配置问题
  2. label:priority=low 的平均使用率仅为 18.56%
     建议检查该维度下的任务是否存在资源浪费或配置问题
  3. annotation:primus-safe.user.name=user123 的平均使用率仅为 22.34%
     建议检查该维度下的任务是否存在资源浪费或配置问题
  4. team=data-team 的平均使用率仅为 25.43%
     建议检查该维度下的任务是否存在资源浪费或配置问题
  5. development 的平均使用率仅为 28.91%
     建议检查该维度下的任务是否存在资源浪费或配置问题

================================================================================

✅ 分析完成！
```

## 自定义配置

可以在代码中修改以下配置：

```python
# API 配置
API_BASE_URL = "http://localhost:8080"  # 修改为你的 API 地址
CLUSTER_NAME = "your-cluster"            # 指定集群名称，或 None 使用默认

# 分析参数
TIME_RANGE_DAYS = 7    # 分析的时间范围（天）
TOP_N = 5              # 显示使用率最低的前 N 个维度
```

## 核心功能展示

### 1. 获取 dimension values（新功能）

```python
# 获取某个 label key 的所有 values
values_result = tools.get_available_dimension_values(
    dimension_type="label",
    dimension_key="team",
    time_range_days=7
)

values_data = json.loads(values_result)
values = values_data.get('dimension_values', [])
print(f"发现 {len(values)} 个团队")
```

### 2. 完整的根因分析流程

示例代码展示了完整的分析流程：

1. **分析集群趋势** - 确认使用率是否在下降
2. **按 namespace 分析** - 找出使用率低的 namespaces
3. **按 label 分析** - 遍历所有 label keys 和 values
4. **按 annotation 分析** - 遍历所有 annotation keys 和 values
5. **生成报告** - 汇总分析结果，给出可能的根因

### 3. 与现有功能集成

示例展示了如何将新功能与现有的工具结合使用：

- `get_available_namespaces` - 获取所有 namespaces
- `get_available_dimension_keys` - 获取所有 keys
- `get_available_dimension_values` - **新增**：获取某个 key 的所有 values
- `query_gpu_usage_trend` - 查询使用率趋势

## 扩展示例

你可以基于这个示例进行扩展：

1. **添加可视化**
   ```python
   import matplotlib.pyplot as plt
   # 绘制使用率趋势图
   ```

2. **导出报告**
   ```python
   import pandas as pd
   # 导出为 CSV 或 Excel
   ```

3. **自动告警**
   ```python
   if avg_utilization < threshold:
       send_alert(dimension, utilization)
   ```

4. **定时分析**
   ```python
   import schedule
   schedule.every().day.at("09:00").do(analyze_cluster)
   ```

## 常见问题

### Q: API 连接失败怎么办？

A: 确保 Lens API 服务正在运行：
```bash
curl http://localhost:8080/v1/gpu-aggregation/clusters
```

### Q: 查询时间太长怎么办？

A: 减少时间范围或限制查询的维度数量：
```python
TIME_RANGE_DAYS = 3  # 改为 3 天
TOP_N = 3            # 只查询前 3 个
```

### Q: 没有数据怎么办？

A: 确保数据库中有 GPU 聚合统计数据。可以手动触发 aggregation job：
```bash
# 运行 GPU aggregation job
cd Lens/modules/jobs
go run cmd/primus-lens-jobs/main.go --job=gpu-aggregation
```

## 相关文档

- [API 文档](../../../docs/api/dimension-values-api.md)
- [实现总结](../../../../IMPLEMENTATION_SUMMARY.md)
- [GPU Aggregation API](../../../docs/api/gpu-aggregation.md)

## 贡献

欢迎提交更多示例代码！请确保：
1. 代码清晰易懂
2. 包含充分的注释
3. 提供使用说明

