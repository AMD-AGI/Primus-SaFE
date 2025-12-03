# Configuration System

## Two-Mode Configuration

The system supports two configuration modes that are **identical except for debug logging**:

1. **Debug Mode** - Same as production with debug logging enabled
2. **Production Mode** - Standard training without debug output

## Structure

```
config.py                # Main configuration module (in project root)
config/                  # Configuration folder
├── README.md           # This file
└── presets/            # Pre-configured settings
    ├── debug.json      # Debug mode (debug.enabled = true)
    └── production.json # Production mode (debug.enabled = false)
```

## Configuration Modes

Both `debug.json` and `production.json` use **identical training settings**:
- **Model**: 32 layers, full size
- **Context**: 8192 tokens
- **Training**: 10000 steps
- **Learning Rate**: 1e-4
- **Batch Size**: 4 with gradient accumulation
- **Mixed Precision**: Enabled (bfloat16)
- **Flash Attention**: Enabled

The **only difference** is:
- **Debug Mode**: `debug.enabled = true` and `log_level = "DEBUG"`
- **Production Mode**: `debug.enabled = false` and `log_level = "INFO"`

## Usage

### Command Line

```bash
# Debug mode (verbose logging for troubleshooting)
python3 pretrain_main.py --preset debug

# Production mode (standard logging)
python3 pretrain_main.py --preset production

# Or use the --debug flag (automatically uses debug preset)
python3 pretrain_main.py --debug
```

### Multi-GPU Training

```bash
# Debug mode on all GPUs
bash run_training.sh --preset debug

# Production training on all GPUs
bash run_training.sh --preset production
```

### Programmatic Usage

```python
from config import get_default_config

# Debug mode
config = get_default_config(preset='debug')
# or
config = get_default_config(debug_mode=True)

# Production mode
config = get_default_config(preset='production')
# or
config = get_default_config()  # defaults to production
```

## What Debug Mode Provides

When `debug.enabled = true`:
- Detailed tensor value dumps when NaN occurs
- Gradient checking and analysis
- Step-by-step logging of operations
- Memory usage tracking
- Attention score visualization
- Call stack tracing on errors

## Custom Configuration

If you need different settings:

```bash
# Copy and modify a preset
cp config/presets/production.json my_config.json
# Edit my_config.json...
python3 pretrain_main.py --config my_config.json
```

## Configuration Sections

Each configuration file contains:

- **debug**: Debug settings (enabled, log_level)
- **model**: Model architecture (layers, attention, dimensions)
- **training**: Training hyperparameters (batch_size, learning_rate, steps)
- **data**: Dataset configuration (dataset_name, num_workers)
- **system**: System settings (device, seed, paths)