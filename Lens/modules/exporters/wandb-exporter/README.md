# Primus Lens WandB Exporter

自动劫持 WandB 上报的工具 - **零代码侵入，pip install 即用**

## ✨ 核心特性

- 🚀 **零代码修改**：pip install 后自动生效，训练代码无需任何改动
- 📊 **自动增强**：自动为 wandb.log() 添加系统级指标（CPU、内存、GPU等）
- 💾 **本地备份**：自动保存所有 wandb 指标到本地文件
- 🌐 **API 异步上报**：自动上报框架检测和训练指标到 telemetry-processor（不阻塞训练）
- 🎯 **智能框架检测**：采集环境变量、WandB 配置等多源证据，生成预判断 hints
- 🔧 **灵活配置**：通过环境变量控制所有功能
- 🎯 **分布式支持**：完美支持多节点、多卡训练场景

## 🎯 快速开始

### 安装

```bash
pip install -e .
```

就这么简单！安装后，当你的 Python 程序导入 `wandb` 时，Primus Lens 会自动劫持并增强它。

### 使用

**你的训练代码完全不需要修改**：

```python
import wandb

# 正常使用 wandb - Primus Lens 会自动劫持
wandb.init(project="my-project")

for epoch in range(10):
    # wandb.log 会被自动增强
    wandb.log({"loss": 0.5, "accuracy": 0.9})
```

运行训练脚本：

```bash
# 设置环境变量（可选）
export PRIMUS_LENS_WANDB_HOOK=true                # 启用劫持（默认启用）
export PRIMUS_LENS_WANDB_ENHANCE_METRICS=true     # 添加系统指标
export PRIMUS_LENS_WANDB_OUTPUT_PATH=/tmp/metrics # 本地保存路径
export PRIMUS_LENS_WANDB_SAVE_LOCAL=true          # 保存到本地

# API 上报配置（可选，在 Kubernetes 环境中自动注入）
export WORKLOAD_UID="your-workload-uid"           # Workload 标识
export POD_UID="your-pod-uid"                     # Pod 标识
export PRIMUS_LENS_WANDB_API_REPORTING=true       # 启用 API 上报
export PRIMUS_LENS_API_BASE_URL="http://primus-lens-telemetry-processor:8080/api/v1"

# 运行训练脚本（无需修改）
python train.py
```

## 🔧 配置选项

通过环境变量控制行为：

### 基础配置

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `PRIMUS_LENS_WANDB_HOOK` | `true` | 是否启用 WandB 劫持 |
| `PRIMUS_LENS_WANDB_ENHANCE_METRICS` | `false` | 是否自动添加系统指标（CPU、内存、GPU） |
| `PRIMUS_LENS_WANDB_SAVE_LOCAL` | `true` | 是否保存指标到本地文件 |
| `PRIMUS_LENS_WANDB_OUTPUT_PATH` | `None` | 本地指标输出路径 |

### API 上报配置 (NEW!)

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `PRIMUS_LENS_WANDB_API_REPORTING` | `true` | 是否启用 API 异步上报 |
| `PRIMUS_LENS_API_BASE_URL` | `http://primus-lens-telemetry-processor:8080/api/v1` | API 基础 URL |
| `WORKLOAD_UID` | 必需 | Workload 唯一标识（由 Adapter 注入） |
| `POD_UID` | 必需 | Pod 唯一标识 |

**详细配置请参考**：[API 上报文档](API_REPORTING.md)

### 分布式训练配置

支持以下环境变量自动识别节点和进程：

- `NODE_RANK` / `SLURM_NODEID` / `PET_NODE_RANK` - 节点 rank
- `RANK` / `LOCAL_RANK` / `WORLD_SIZE` - 进程 rank 信息

## 🛠️ 工作原理

1. **安装时**：`pip install` 会在 `site-packages` 创建 `primus_lens_wandb_hook.pth` 文件
2. **Python 启动时**：自动读取并执行 `.pth` 文件，注册 Import Hook
3. **导入劫持**：当用户 `import wandb` 时，Import Hook 触发
4. **Monkey Patching**：自动 patch `wandb.init()` 和 `wandb.log()` 方法
5. **透明增强**：原有功能保持不变，同时添加额外能力

```
pip install 
    ↓
创建 .pth 文件
    ↓
Python 启动时自动导入
    ↓
注册 Import Hook
    ↓
import wandb 触发劫持
    ↓
自动 Patch wandb
    ↓
透明增强 wandb 功能
```

## 📁 输出格式

当设置 `PRIMUS_LENS_WANDB_OUTPUT_PATH` 时，指标会保存到：

```
{OUTPUT_PATH}/
  └── node_0/
      ├── rank_0/
      │   └── wandb_metrics.jsonl
      ├── rank_1/
      │   └── wandb_metrics.jsonl
      └── ...
```

每行 JSON 格式：

```json
{
  "timestamp": 1700000000.123,
  "step": 10,
  "data": {
    "loss": 0.5,
    "accuracy": 0.9,
    "_primus_lens_enabled": true,
    "_primus_sys_cpu_percent": 45.2,
    "_primus_sys_memory_percent": 60.5
  }
}
```

## 🧪 验证安装

检查 `.pth` 文件是否已安装：

```bash
python install_hook.py check
```

测试劫持是否生效：

```python
import os
os.environ['PRIMUS_LENS_WANDB_HOOK'] = 'true'

import wandb

# 检查是否被 patch
print(f"WandB patched: {hasattr(wandb, '_primus_lens_patched')}")
```

## 🚀 高级用法

### 仅保存到本地，不上报到 wandb

```bash
# 设置 wandb 为 offline 模式
export WANDB_MODE=offline
export PRIMUS_LENS_WANDB_SAVE_LOCAL=true
export PRIMUS_LENS_WANDB_OUTPUT_PATH=/data/metrics

python train.py
```

### 多节点训练示例

```bash
# 节点 0
export NODE_RANK=0
export PRIMUS_LENS_WANDB_OUTPUT_PATH=/shared/metrics
python -m torch.distributed.launch train.py

# 节点 1
export NODE_RANK=1
export PRIMUS_LENS_WANDB_OUTPUT_PATH=/shared/metrics
python -m torch.distributed.launch train.py
```

### 临时禁用劫持

```bash
export PRIMUS_LENS_WANDB_HOOK=false
python train.py  # wandb 正常工作，不会被劫持
```

## 🗑️ 卸载

```bash
# 方法 1：使用脚本
python install_hook.py uninstall

# 方法 2：手动删除
python -c "import site, os; pth=os.path.join(site.getsitepackages()[0], 'primus_lens_wandb_hook.pth'); os.remove(pth) if os.path.exists(pth) else None"

# 方法 3：临时禁用（不删除）
export PRIMUS_LENS_WANDB_HOOK=false
```

## 📊 性能影响

- **启动开销**：< 10ms（加载 .pth 文件）
- **每次 log 开销**：< 1ms（dict.copy() + 指标收集）
- **对训练影响**：< 0.1%

## 🤝 与其他 Primus Lens 模块集成

WandB Exporter 可以与其他 Primus Lens 模块配合使用：

- **workload-exporter**：收集 GPU/RDMA 底层指标
- **node-exporter**：收集节点级别资源
- **network-exporter**：收集网络流量

## ⚠️ 注意事项

1. **虚拟环境**：需要在每个虚拟环境中单独安装
2. **权限问题**：可能需要 sudo/管理员权限来创建 .pth 文件
3. **Import 顺序**：必须在导入 wandb 之前完成 hook 注册（自动完成）
4. **兼容性**：已测试 wandb >= 0.12.0

## 📖 更多文档

- [安装指南](INSTALL.md) - 详细安装说明和故障排除
- **[API 上报文档](API_REPORTING.md)** - 异步 API 上报功能详解（NEW!）
- [使用指南](使用指南.md) - 中文详细使用说明
- [示例脚本](example_api_reporting.py) - API 上报示例代码

## 📄 许可证

Apache-2.0

