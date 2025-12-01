# Multi-GPU Training System

## Quick Start

```bash
# Run training on all available GPUs
bash run_training.sh
```

## Key Features

### 1. Simplified Architecture
- **run_training.sh**: Main launcher script (48 lines)
- **prepare_dataset.py**: Dataset preparation and caching
- **pretrain_main.py**: Training entry point

### 2. Dataset Caching & Reuse
- Dataset is prepared **once** before multi-GPU training starts
- All GPU processes share the same cached dataset
- Cache location: `./cache/datasets/`
- Automatic cache key generation based on:
  - Model/tokenizer name
  - Dataset name
  - Context length
  - Sample count

### 3. GPU Process Management
- Automatic GPU detection
- Each GPU runs in isolated CUDA environment
- GPU ID logging for easy debugging
- Process monitoring with automatic failure detection
- Clean shutdown on error

### 4. Log Output Format
Each GPU process logs with its ID prefix:
```
GPU0:INFO | [GPU 0] Step [0/100] | Loss: 12.2340 | LR: 1.00e-05 | Grad Norm: 2.7565 | ETA: 5:30
GPU1:INFO | [GPU 1] Step [0/100] | Loss: 12.2340 | LR: 1.00e-05 | Grad Norm: 2.7565 | ETA: 5:45
```

### 5. Error Handling
- If any GPU fails, all processes are terminated
- Exit codes are properly propagated
- Clear error messages with GPU identification

## Configuration

Two configuration modes are available:

- **Debug Mode**: For testing and development (2 layers, 10 steps, debug logging)
- **Production Mode**: For real training (32 layers, 10000 steps, optimized settings)

```bash
# Debug mode
python3 pretrain_main.py --debug

# Production mode (default)
python3 pretrain_main.py

# Or specify explicitly
python3 pretrain_main.py --preset production
```

## GPU Selection Scripts

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

## Manual Dataset Preparation

To pre-cache dataset without training:
```bash
python3 prepare_dataset.py
```

## Debug Mode

### Enabling Debug Mode

Debug mode provides:
- Small model (2 layers) for quick iteration
- Detailed logging and NaN detection
- Tensor value dumps when errors occur
- Conservative settings to avoid numerical issues

```bash
# Single GPU debug
python3 pretrain_main.py --debug

# Multi-GPU debug
bash run_training.sh --debug

# Or explicitly use debug preset
python3 pretrain_main.py --preset debug
```

### Debug Output Example

When NaN is detected with debug enabled:
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

To analyze saved debug data:
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

## Single GPU Training

### Method 1: Specify GPU via Environment Variable
```bash
# Use GPU 0
CUDA_VISIBLE_DEVICES=0 GPU_RANK=0 python3 pretrain_main.py

# Use GPU 3
CUDA_VISIBLE_DEVICES=3 GPU_RANK=3 python3 pretrain_main.py

# Use the last GPU (e.g., GPU 7 in 8-GPU system)
CUDA_VISIBLE_DEVICES=7 GPU_RANK=7 python3 pretrain_main.py
```

### Method 2: Specify GPU in Config File
Create a single GPU config file `single_gpu.json`:
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

### Method 3: Select Best Available GPU
```bash
# Use nvidia-smi to check GPU utilization
nvidia-smi --query-gpu=index,name,memory.used,memory.total --format=csv

# Choose GPU with most free memory
# Example: If GPU 5 has most free memory
CUDA_VISIBLE_DEVICES=5 GPU_RANK=5 python3 pretrain_main.py
```

### Single GPU with Debug Mode
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

### Optimizing for Single GPU
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

### Testing on Single GPU Before Multi-GPU
```bash
# Step 1: Test on single GPU with small batch
CUDA_VISIBLE_DEVICES=0 python3 pretrain_main.py \
    --config test_config.json \
    --max-steps 10

# Step 2: If successful, run on all GPUs
bash run_training.sh --config test_config.json
```

## Monitoring

View real-time logs from all GPUs:
```bash
bash run_training.sh 2>&1 | tee training.log
```

Filter logs for specific GPU:
```bash
bash run_training.sh 2>&1 | grep "GPU2:"
```

## Troubleshooting

### NaN/Inf Errors

If training fails with NaN or Inf errors:

1. **Enable debug mode** to get detailed information:
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
   python3 pretrain_main.py --config config_stable.json
   ```

4. **Analyze debug output**:
   - Check which layers have NaN
   - Look at gradient magnitudes
   - Review tensor values before NaN

### Memory Issues

If running out of GPU memory:
- Reduce `batch_size`
- Reduce `context_length`
- Increase `grad_accum_nums`
- Enable gradient checkpointing (if implemented)

### Process Errors

If processes fail to start:
- Check CUDA installation: `nvidia-smi`
- Verify Python packages: `pip list | grep torch`
- Clear cache: `rm -rf .cache __pycache__`
- Check available GPUs: `python3 -c "import torch; print(torch.cuda.device_count())"`
