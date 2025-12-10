# Model Check - Multi-GPU LLaMA Training System

A highly efficient multi-GPU training framework designed specifically for LLaMA models, supporting distributed training, automatic error detection, and intelligent resource management.

## 📋 Prerequisites

### Environment Requirements

1. **Docker Release Environment**: This system must be run in a Docker release environment.

2. **HuggingFace Token**: You need to export your HuggingFace token to download models:
   ```bash
   export HF_TOKEN=your_huggingface_token_here
   ```
   
   > **Note**: Get your token from [HuggingFace Settings](https://huggingface.co/settings/tokens)

## 🚀 Quick Start

```bash
# 1. Set HuggingFace token (required for model downloads)
export HF_TOKEN=your_huggingface_token_here

# 2. Ensure you're in the Docker release environment

# 3. Run training on all available GPUs
bash run.sh

# Use debug mode
bash run.sh --debug

# Use custom configuration
bash run.sh --config my_config.json
```

## ✨ Core Features

### 1. Modular Architecture Design
- **run.sh**: Main launcher script, automatically manages multi-GPU processes (285 lines)
- **pretrain_main.py**: Training entry point with flexible configuration support (271 lines)
- **prepare_dataset.py**: Dataset preprocessing and caching (48 lines)
- **config.py**: Unified configuration management system (305 lines)

### 2. Intelligent Dataset Caching
- **One-time** dataset preparation before training
- All GPU processes share cached data
- Cache location: `.cache/datasets/` (configurable)
- Automatic cache key generation based on:
  - Model/tokenizer name
  - Dataset name
  - Context length
  - Sample count

### 3. Advanced GPU Process Management
- Automatic GPU detection and allocation
- Isolated CUDA environment for each process
- Log output with GPU ID prefix
- Real-time process monitoring and error detection
- Graceful error handling and cleanup mechanism
- Flexible switching between single and multi-GPU modes

### 4. Clear Log Output Format
Each GPU process is identified with an ID prefix:
```
[GPU0] Step [0/100] | Loss: 12.2340 | LR: 1.00e-05 | Grad Norm: 2.7565 | ETA: 5:30
[GPU1] Step [0/100] | Loss: 12.2340 | LR: 1.00e-05 | Grad Norm: 2.7565 | ETA: 5:45
```

Logs are automatically saved to: `/tmp/model_check_logs_<PID>_<timestamp>/`

### 5. Robust Error Handling
- Automatic termination of all processes when any GPU fails
- Proper exit code propagation
- Clear error messages with GPU identification
- NaN/Inf detection and debug support
- Automatic error checkpoint saving

## ⚙️ Configuration System

### Preset Configurations

The system provides two preset configuration modes:

| Mode | Layers | Training Steps | Context Length | Learning Rate | Purpose |
|------|--------|----------------|----------------|---------------|----------|
| **Debug** | 2 | 10 | 512 | 1e-7 | Quick testing and debugging |
| **Production** | 4 | 100 | 8192 | 1e-5 | Full training |

```bash
# Use debug mode
python3 pretrain_main.py --debug
# or
python3 pretrain_main.py --preset debug

# Use production mode (default)
python3 pretrain_main.py
# or
python3 pretrain_main.py --preset production
```

### Custom Configuration

Create a custom configuration file `custom_config.json`:

```json
{
  "model": {
    "model_path": "meta-llama/Llama-3.1-8B-Instruct",
    "num_layers": 8,
    "attention_backend": "flash_attn"
  },
  "training": {
    "batch_size": 4,
    "learning_rate": 5e-6,
    "max_steps": 500,
    "context_length": 4096
  },
  "system": {
    "device": "cuda",
    "seed": 42
  }
}
```

Use custom configuration:
```bash
python3 pretrain_main.py --config custom_config.json
```

## 📂 Project Structure

```
model_check/
├── config/
│   ├── presets/            # Preset configuration files
│   │   ├── debug.json      # Debug configuration
│   │   └── production.json # Production configuration
│   └── README.md          # Configuration documentation
├── data/
│   ├── __init__.py
│   └── build_dataset.py   # Dataset builder
├── engine/
│   ├── __init__.py
│   └── trainer.py        # Training engine
├── models/
│   ├── __init__.py
│   ├── basic_llama.py    # LLaMA model implementation
│   ├── build_model.py    # Model builder
│   └── rope.py           # RoPE position encoding
├── utils/                 # Utility functions
├── .cache/               # Cache directory (auto-generated)
├── .checkpoints/         # Checkpoint directory (auto-generated)
├── .logs/                # Log directory (auto-generated)
├── .outputs/             # Output directory (auto-generated)
├── config.py             # Configuration management
├── prepare_dataset.py    # Dataset preprocessing
├── pretrain_main.py      # Main training script
├── requirements.txt      # Dependencies
├── run.sh               # Multi-GPU launcher
└── README.md            # This document
```

## 🔧 GPU Selection Scripts

### Quick GPU Selection
Create a helper script `train_gpu.sh`:
```bash
#!/bin/bash
# Usage: ./train_gpu.sh <gpu_id> [extra_args]
GPU_ID=${1:-0}  # Default to GPU 0 if not specified
shift
CUDA_VISIBLE_DEVICES=$GPU_ID GPU_RANK=$GPU_ID python3 pretrain_main.py "$@"
```

Use it:
```bash
# Train on GPU 0
./train_gpu.sh 0

# Train on GPU 3 with debug
./train_gpu.sh 3 --debug

# Train on GPU 5 with custom config
./train_gpu.sh 5 --config my_config.json
```

### Auto-Select Free GPU
Create `train_auto_gpu.sh`:
```bash
#!/bin/bash
# Auto-select GPU with lowest utilization
GPU_ID=$(nvidia-smi --query-gpu=index,utilization.gpu --format=csv,noheader,nounits | \
         sort -t',' -k2 -n | head -1 | cut -d',' -f1 | tr -d ' ')
echo "Using GPU $GPU_ID (lowest utilization)"
CUDA_VISIBLE_DEVICES=$GPU_ID GPU_RANK=$GPU_ID python3 pretrain_main.py "$@"
```

## 📦 Requirements

### System Requirements
- Python >= 3.8
- CUDA >= 11.0 (for GPU training)
- Linux/Unix system (recommended)
- Docker release environment (required)
- HuggingFace account and API token (required for model downloads)

### Python Packages
```txt
torch>=2.0.0
transformers>=4.30.0
datasets>=2.0.0
tqdm
numpy
tensorboard
# flash-attn>=2.0.0  # Optional, for performance boost
```

### Automatic Installation
The run.sh script automatically detects and installs missing dependencies:
```bash
# Make sure HF_TOKEN is exported before running
export HF_TOKEN=your_huggingface_token_here
bash run.sh  # Automatically installs dependencies and starts training
```

## 📊 Dataset Preparation

### Automatic Preparation
The run.sh script automatically prepares datasets, no manual operation required.

### Manual Pre-caching
To prepare dataset separately:
```bash
python3 prepare_dataset.py
```

### Supported Datasets
- wikitext (default)
- c4
- Custom datasets (specify through configuration file)

## 🐛 Debug Mode

### Enabling Debug Mode

Debug mode provides:
- Small model (2 layers) for quick iteration
- Detailed logging and NaN detection
- Tensor value dumps on errors
- Conservative settings to avoid numerical issues
- Shorter training steps (10 steps)
- Smaller context length (512)

```bash
# Single GPU debug
python3 pretrain_main.py --debug

# Multi-GPU debug
bash run.sh --debug

# Explicitly use debug preset
python3 pretrain_main.py --preset debug
```

### Debug Output Example

Debug output when NaN is detected:
```
================================================================================
NaN GRADIENT DETECTED [GPU 6]
================================================================================
Step: 0
Loss: 12.6087
Gradient Norm: nan

========================================
PROBLEMATIC TENSORS:
========================================
⚠️  Gradients with NaN (15 layers):
  • tok_embed.weight (shape: torch.Size([128256, 4096]))
    Grad values: ['NaN', 'NaN', 'NaN', 'NaN', 'NaN', ...]
    NaN count: 525336576/525336576
  • layers.0.attention.wq.weight (shape: torch.Size([4096, 4096]))
    Grad values: ['NaN', 'NaN', 'NaN', 'NaN', 'NaN', ...]
    NaN count: 16777216/16777216

Total layers with NaN gradients: 15
Affected layers: ['tok_embed.weight', 'layers.0.attention.wq.weight', ...]
========================================

Checkpoint saved: .checkpoints/nan_grad_step_0.pt
Debug tensors saved: .checkpoints/debug/nan_debug_step_0.pkl
```

### Loading Debug Checkpoint

Analyze saved debug data:
```python
import pickle
import numpy as np

# Load debug tensors
with open('.checkpoints/debug/nan_debug_step_0.pkl', 'rb') as f:
    debug_data = pickle.load(f)

# Check which parameters have NaN
for name, param in debug_data['parameters'].items():
    if np.isnan(param).any():
        print(f"{name} has NaN values")

# Check gradients
for name, grad in debug_data['gradients'].items():
    if np.isnan(grad).any():
        nan_ratio = np.isnan(grad).sum() / grad.size
        print(f"{name}: {nan_ratio*100:.1f}% NaN")
```

## 💻 Single GPU Training

**Note**: Remember to export your HuggingFace token before running:
```bash
export HF_TOKEN=your_huggingface_token_here
```

### Method 1: Specify GPU via Environment Variable
```bash
# Use GPU 0
CUDA_VISIBLE_DEVICES=0 GPU_RANK=0 python3 pretrain_main.py

# Use GPU 3
CUDA_VISIBLE_DEVICES=3 GPU_RANK=3 python3 pretrain_main.py

# Use the last GPU (e.g., GPU 7 in 8-GPU system)
CUDA_VISIBLE_DEVICES=7 GPU_RANK=7 python3 pretrain_main.py
```

### Method 2: Specify GPU in Configuration File
Create a single GPU configuration file `single_gpu.json`:
```json
{
    "system": {
        "device": "cuda",
        "device_id": 0
    },
    "training": {
        "batch_size": 4,
        "grad_accum_nums": 8
    }
}
```

Then run:
```bash
CUDA_VISIBLE_DEVICES=2 python3 pretrain_main.py --config single_gpu.json
```

### Method 3: Auto-select Best Available GPU
```bash
# Use nvidia-smi to check GPU utilization
nvidia-smi --query-gpu=index,name,memory.used,memory.total --format=csv

# Choose GPU with most free memory
# Example: If GPU 5 has most free memory
CUDA_VISIBLE_DEVICES=5 GPU_RANK=5 python3 pretrain_main.py
```

### Single GPU Debug Mode
```bash
# Debug on single GPU for faster iteration
CUDA_VISIBLE_DEVICES=0 GPU_RANK=0 python3 pretrain_main.py --debug

# With custom config
CUDA_VISIBLE_DEVICES=1 GPU_RANK=1 python3 pretrain_main.py --config debug_config.json
```

### Advantages of Single GPU Training
- **Faster debugging**: No multi-process complexity
- **Lower memory usage**: Can use larger batch sizes
- **Simpler logs**: No interleaved output from multiple GPUs
- **Easier profiling**: Can use standard profiling tools

### Single GPU Optimization
When using a single GPU, you can adjust parameters for better efficiency:
```json
{
    "training": {
        "batch_size": 8,         // Larger batch since only one GPU
        "grad_accum_nums": 4,    // Less accumulation needed
        "num_workers": 8         // More data workers for single GPU
    }
}
```

### Single GPU Testing Before Multi-GPU
```bash
# Step 1: Test on single GPU with small batch
CUDA_VISIBLE_DEVICES=0 python3 pretrain_main.py \
    --config test_config.json \
    --max-steps 10

# Step 2: If successful, run on all GPUs
bash run.sh --config test_config.json
```

## 📈 Monitoring and Logging

### View All GPU Logs in Real-time
```bash
bash run.sh 2>&1 | tee training.log
```

### Filter Specific GPU Logs
```bash
bash run.sh 2>&1 | grep "GPU2"
```

### Log File Locations
- Temporary logs: `/tmp/model_check_logs_<PID>_<timestamp>/`
- Persistent logs: `./.logs/` (if configured)
- Checkpoints: `./.checkpoints/`
- Output files: `./.outputs/`

## ❓ Troubleshooting

### Authentication Errors

If you encounter HuggingFace authentication errors:

1. **Verify HF_TOKEN is set**:
   ```bash
   echo $HF_TOKEN  # Should show your token
   ```

2. **Set the token if missing**:
   ```bash
   export HF_TOKEN=your_huggingface_token_here
   ```

3. **Ensure token has read permissions** for the model repositories you're accessing

4. **Check Docker environment**: Make sure you're running in the Docker release environment

### NaN/Inf Errors

If training fails with NaN or Inf errors:

1. **Enable debug mode** for detailed information:
   ```bash
   python3 pretrain_main.py --debug
   ```

2. **Check common causes**:
   - Learning rate too high → reduce `learning_rate`
   - Poor initialization → reduce `init_std` 
   - Gradient explosion → reduce `max_grad_norm`
   - Numerical instability → disable `use_amp`

3. **Use stable configuration**:
   ```bash
   python3 pretrain_main.py --preset debug
   ```

4. **Analyze debug output**:
   - Check which layers have NaN
   - Review gradient magnitudes
   - Examine tensor values before NaN

### Memory Issues

If running out of GPU memory:
- Reduce `batch_size`
- Reduce `context_length`
- Increase `grad_accum_nums`
- Enable gradient checkpointing (if implemented)
- Use fewer model layers

### Process Errors

If processes fail to start:
- Check CUDA installation: `nvidia-smi`
- Verify Python packages: `pip list | grep torch`
- Clear cache: `rm -rf .cache __pycache__`
- Check available GPUs: `python3 -c "import torch; print(torch.cuda.device_count())"`
- Check environment variables: `echo $CUDA_VISIBLE_DEVICES`

## 🎯 Best Practices

1. **Development Workflow**
   - Test in debug mode first
   - Validate on single GPU before multi-GPU
   - Gradually increase model size

2. **Performance Optimization**
   - Use Flash Attention (if available)
   - Set batch size and gradient accumulation appropriately
   - Enable mixed precision training (AMP)

3. **Error Prevention**
   - Save checkpoints regularly
   - Monitor GPU memory usage
   - Use conservative learning rates

## 📝 License

This project is licensed under the MIT License. See LICENSE file for details.

## 🤝 Contributing

Contributions are welcome! Feel free to submit Issues and Pull Requests.

## 📧 Contact

For questions, please contact the project maintainers.
