# 安装指南

## 快速安装

### 方法 1：使用 pip（推荐）

```bash
cd Lens/modules/exporters/wandb-exporter
pip install -e .
```

安装完成后，`.pth` 文件会自动创建到 `site-packages` 目录。

### 方法 2：手动安装 .pth 文件

如果自动安装失败，可以手动安装：

```bash
# 进入项目目录
cd Lens/modules/exporters/wandb-exporter

# 运行安装脚本
python install_hook.py install
```

### 方法 3：使用 Makefile

```bash
make install
```

## 验证安装

```bash
# 检查安装状态
python install_hook.py check

# 或使用 Makefile
make check
```

或者运行测试脚本：

```bash
python test_wandb_hook.py

# 或使用 Makefile
make test
```

## 使用示例

安装后，你的训练脚本**无需任何修改**：

```python
import wandb

# 正常使用 wandb - Primus Lens 会自动劫持
wandb.init(project="my-project")
wandb.log({"loss": 0.5})
```

运行脚本：

```bash
# 设置环境变量
export PRIMUS_LENS_WANDB_HOOK=true
export PRIMUS_LENS_WANDB_ENHANCE_METRICS=true
export PRIMUS_LENS_WANDB_OUTPUT_PATH=/tmp/metrics

# 运行训练脚本
python your_training.py
```

## 环境变量配置

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `PRIMUS_LENS_WANDB_HOOK` | `true` | 启用/禁用 WandB 劫持 |
| `PRIMUS_LENS_WANDB_ENHANCE_METRICS` | `false` | 在 wandb.log 中自动添加系统指标 |
| `PRIMUS_LENS_WANDB_SAVE_LOCAL` | `true` | 保存指标到本地文件 |
| `PRIMUS_LENS_WANDB_OUTPUT_PATH` | `None` | 指标输出路径 |

## 卸载

```bash
# 使用脚本卸载
python install_hook.py uninstall

# 或使用 Makefile
make uninstall

# 或手动删除
python -c "import site, os; pth=os.path.join(site.getsitepackages()[0], 'primus_lens_wandb_hook.pth'); os.remove(pth) if os.path.exists(pth) else None"
```

## 故障排除

### 问题 1：.pth 文件未创建

**原因**：权限不足

**解决**：使用 sudo 安装（Linux/Mac）或以管理员身份运行（Windows）

```bash
# Linux/Mac
sudo pip install -e .

# Windows - 以管理员身份运行 PowerShell/CMD
pip install -e .
```

### 问题 2：劫持未生效

**检查步骤**：

1. 确认 .pth 文件存在：
```bash
python install_hook.py check
```

2. 确认环境变量设置：
```bash
# Linux/Mac
echo $PRIMUS_LENS_WANDB_HOOK

# Windows
echo %PRIMUS_LENS_WANDB_HOOK%
```

3. 确认导入顺序：确保在导入 wandb 之前，劫持器已加载（自动完成）

4. 检查 Python 环境：确保在正确的虚拟环境中

### 问题 3：虚拟环境中不工作

**原因**：.pth 文件安装到了全局 site-packages

**解决**：在虚拟环境中重新安装

```bash
# 激活虚拟环境
source venv/bin/activate  # Linux/Mac
# 或
venv\Scripts\activate  # Windows

# 重新安装
pip install -e .

# 验证
python install_hook.py check
```

### 问题 4：与其他 wandb hooks 冲突

**解决**：调整环境变量或修改加载顺序

```bash
# 临时禁用 Primus Lens
export PRIMUS_LENS_WANDB_HOOK=false
```

### 问题 5：找不到 psutil

**原因**：依赖未安装

**解决**：安装依赖

```bash
pip install psutil

# 如果需要 GPU 指标
pip install nvidia-ml-py3
```

## 高级配置

### 自定义劫持逻辑

编辑 `src/primus_lens_wandb_exporter/wandb_hook.py`：

```python
def intercepted_log(data: Dict[str, Any], step: Optional[int] = None, *args, **kwargs):
    """自定义 log 拦截"""
    enhanced_data = data.copy()
    
    # 添加你的自定义指标
    enhanced_data["custom_metric"] = your_custom_function()
    
    return self.original_log(enhanced_data, step=step, *args, **kwargs)
```

### 仅在特定条件下启用

```bash
# 仅在 rank 0 启用
if [ "$RANK" = "0" ]; then
    export PRIMUS_LENS_WANDB_HOOK=true
else
    export PRIMUS_LENS_WANDB_HOOK=false
fi
```

## 依赖要求

### 必需依赖
- Python >= 3.7
- psutil >= 5.8.0

### 可选依赖
- wandb (被劫持的目标，如果未安装则劫持器不会激活)
- nvidia-ml-py3 >= 7.352.0 (用于 GPU 指标收集)

## 性能影响

- **启动时间**：增加 < 10ms（加载 .pth 文件）
- **运行时开销**：每次 `wandb.log()` 增加 < 1ms
- **内存占用**：增加 < 5MB

对训练性能的影响可忽略不计。

## 兼容性

- **Python**: 3.7+
- **WandB**: 0.12.0+ (理论上支持所有版本)
- **操作系统**: Linux, macOS, Windows
- **训练框架**: PyTorch, TensorFlow, JAX, 等（框架无关）

## 开发者安装

如果你想修改源码：

```bash
# 克隆仓库
cd Lens/modules/exporters/wandb-exporter

# 安装开发依赖
pip install -e ".[dev]"

# 运行测试
pytest test_wandb_hook.py

# 或使用 Makefile
make test
```

## 更多帮助

查看完整文档：
- [README.md](README.md) - 英文完整文档
- [使用指南.md](使用指南.md) - 中文使用指南

运行示例代码：
```bash
python example_usage.py
# 或
make example
```

