# Training Performance API 快速开始

## 🚀 5 分钟快速上手

### 前置条件

- API 服务已启动（默认端口 8080）
- 有可用的 workload UID
- （可选）curl 或 Postman

### 步骤 1：查看可用指标

首先查看 workload 有哪些可用的训练指标：

```bash
curl -X GET "http://localhost:8080/api/v1/workloads/YOUR_WORKLOAD_UID/metrics/available"
```

**响应示例：**

```json
{
  "workload_uid": "YOUR_WORKLOAD_UID",
  "metrics": [
    {
      "name": "train/loss",
      "data_source": ["wandb", "log"],
      "count": 1000
    },
    {
      "name": "train/accuracy",
      "data_source": ["wandb"],
      "count": 500
    },
    {
      "name": "train/learning_rate",
      "data_source": ["log"],
      "count": 1000
    }
  ],
  "total_count": 3
}
```

### 步骤 2：获取指标数据

选择感兴趣的指标，查询具体数据：

```bash
# 获取 loss 和 accuracy 指标
curl -X GET "http://localhost:8080/api/v1/workloads/YOUR_WORKLOAD_UID/metrics/data?metrics=train/loss,train/accuracy"
```

**响应示例：**

```json
{
  "workload_uid": "YOUR_WORKLOAD_UID",
  "data": [
    {
      "metric_name": "train/loss",
      "value": 2.3456,
      "timestamp": 1704067200000,
      "iteration": 1,
      "data_source": "wandb"
    },
    {
      "metric_name": "train/accuracy",
      "value": 0.1234,
      "timestamp": 1704067200000,
      "iteration": 1,
      "data_source": "wandb"
    },
    {
      "metric_name": "train/loss",
      "value": 1.8765,
      "timestamp": 1704067260000,
      "iteration": 2,
      "data_source": "wandb"
    }
  ],
  "total_count": 3
}
```

### 步骤 3：添加过滤条件

#### 按数据源过滤

只查看来自 wandb 的数据：

```bash
curl -X GET "http://localhost:8080/api/v1/workloads/YOUR_WORKLOAD_UID/metrics/data?data_source=wandb"
```

#### 按时间范围过滤

查询最近 1 小时的数据：

```bash
# 计算时间戳（当前时间 - 1小时）
END_TIME=$(date +%s)000
START_TIME=$((END_TIME - 3600000))

curl -X GET "http://localhost:8080/api/v1/workloads/YOUR_WORKLOAD_UID/metrics/data?start=${START_TIME}&end=${END_TIME}"
```

#### 组合过滤

```bash
# wandb 来源 + 特定指标 + 时间范围
curl -X GET "http://localhost:8080/api/v1/workloads/YOUR_WORKLOAD_UID/metrics/data?data_source=wandb&metrics=train/loss&start=${START_TIME}&end=${END_TIME}"
```

## 📊 可视化数据（前端示例）

### JavaScript/Fetch API

```javascript
// 1. 获取可用指标
async function getAvailableMetrics(workloadUid) {
  const response = await fetch(
    `/api/v1/workloads/${workloadUid}/metrics/available`
  );
  return await response.json();
}

// 2. 获取指标数据
async function getMetricsData(workloadUid, options = {}) {
  const params = new URLSearchParams();
  
  if (options.dataSource) {
    params.append('data_source', options.dataSource);
  }
  
  if (options.metrics) {
    params.append('metrics', options.metrics.join(','));
  }
  
  if (options.start && options.end) {
    params.append('start', options.start);
    params.append('end', options.end);
  }
  
  const response = await fetch(
    `/api/v1/workloads/${workloadUid}/metrics/data?${params}`
  );
  return await response.json();
}

// 3. 使用示例
const workloadUid = 'YOUR_WORKLOAD_UID';

// 获取所有可用指标
const available = await getAvailableMetrics(workloadUid);
console.log('可用指标:', available.metrics.map(m => m.name));

// 获取 loss 数据
const lossData = await getMetricsData(workloadUid, {
  metrics: ['train/loss'],
  dataSource: 'wandb'
});

// 绘制图表（使用 Chart.js）
const chartData = {
  labels: lossData.data.map(d => d.iteration),
  datasets: [{
    label: 'Training Loss',
    data: lossData.data.map(d => d.value),
    borderColor: 'rgb(75, 192, 192)',
    tension: 0.1
  }]
};
```

### Python/Requests

```python
import requests
import pandas as pd
import matplotlib.pyplot as plt

BASE_URL = "http://localhost:8080/api/v1"
WORKLOAD_UID = "YOUR_WORKLOAD_UID"

# 1. 获取可用指标
def get_available_metrics(workload_uid):
    url = f"{BASE_URL}/workloads/{workload_uid}/metrics/available"
    response = requests.get(url)
    return response.json()

# 2. 获取指标数据
def get_metrics_data(workload_uid, **kwargs):
    url = f"{BASE_URL}/workloads/{workload_uid}/metrics/data"
    response = requests.get(url, params=kwargs)
    return response.json()

# 3. 使用示例
# 查看所有可用指标
available = get_available_metrics(WORKLOAD_UID)
print("可用指标:")
for metric in available['metrics']:
    print(f"  - {metric['name']} (来源: {', '.join(metric['data_source'])})")

# 获取 loss 数据
data = get_metrics_data(
    WORKLOAD_UID,
    metrics='train/loss',
    data_source='wandb'
)

# 转换为 DataFrame
df = pd.DataFrame(data['data'])

# 绘制图表
plt.figure(figsize=(10, 6))
plt.plot(df['iteration'], df['value'], marker='o')
plt.xlabel('Iteration')
plt.ylabel('Loss')
plt.title('Training Loss')
plt.grid(True)
plt.show()
```

## 🔧 常见问题

### Q1: 如何获取 workload_uid？

```bash
# 列出所有 workload
curl -X GET "http://localhost:8080/api/v1/workloads"

# 或者从 K8s 资源中获取
kubectl get workloads -o jsonpath='{.items[*].metadata.uid}'
```

### Q2: 时间戳格式是什么？

- 使用**毫秒级时间戳**（13 位数字）
- 示例：`1704067200000`（2024-01-01 00:00:00 UTC）

```javascript
// JavaScript 获取当前时间戳
const now = Date.now(); // 1704067200000

// Python 获取当前时间戳
import time
now = int(time.time() * 1000) # 1704067200000
```

### Q3: 如何获取最近 N 条数据？

API 不直接支持 limit，但可以：

1. 使用时间范围限制
2. 在客户端截取数据

```javascript
// 获取最近 100 条
const allData = await getMetricsData(workloadUid);
const last100 = allData.data.slice(-100);
```

### Q4: 数据源有哪些？

当前支持：
- `log`: 从训练日志解析
- `wandb`: 从 W&B API 获取
- `tensorflow`: 从 TensorFlow/TensorBoard 获取

### Q5: 如何获取多个指标？

使用逗号分隔：

```bash
curl -X GET "...?metrics=train/loss,train/accuracy,train/lr"
```

## 📈 使用场景

### 场景 1：监控训练进度

```bash
#!/bin/bash
# monitor_training.sh

WORKLOAD_UID=$1
INTERVAL=60  # 每 60 秒刷新一次

while true; do
  # 获取最新的 loss 值
  DATA=$(curl -s "http://localhost:8080/api/v1/workloads/${WORKLOAD_UID}/metrics/data?metrics=train/loss" | jq '.data | last')
  
  ITERATION=$(echo $DATA | jq '.iteration')
  LOSS=$(echo $DATA | jq '.value')
  
  echo "$(date) - Iteration: ${ITERATION}, Loss: ${LOSS}"
  
  sleep $INTERVAL
done
```

### 场景 2：对比不同数据源

```python
# 对比 log 和 wandb 的数据
log_data = get_metrics_data(uid, metrics='train/loss', data_source='log')
wandb_data = get_metrics_data(uid, metrics='train/loss', data_source='wandb')

# 绘制对比图
plt.plot(log_df['iteration'], log_df['value'], label='Log', alpha=0.7)
plt.plot(wandb_df['iteration'], wandb_df['value'], label='WandB', alpha=0.7)
plt.legend()
plt.show()
```

### 场景 3：导出到 CSV

```python
import pandas as pd

# 获取所有数据
data = get_metrics_data(WORKLOAD_UID)
df = pd.DataFrame(data['data'])

# 导出为 CSV
df.to_csv('training_metrics.csv', index=False)
print(f"导出了 {len(df)} 条数据到 training_metrics.csv")
```

## 🎓 更多资源

- [完整 API 文档](./training_performance_api.md)
- [实现总结](./training_performance_api_summary_zh.md)
- [示例代码](../pkg/api/training_performance_test.go)

## 💡 提示

1. **性能优化**：对于大数据量，务必使用时间范围和指标过滤
2. **数据一致性**：不同数据源可能有轻微差异，这是正常的
3. **时间戳**：使用毫秒级时间戳（13 位）
4. **迭代次数**：`iteration` 字段对应训练的 step 数

## 🆘 需要帮助？

- 查看 [API 文档](./training_performance_api.md)
- 提交 Issue 到项目仓库
- 联系开发团队

