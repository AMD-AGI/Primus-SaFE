#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Megatron-Bridge LoRA for slime.

slime's --custom-model-provider-path names model_provider: it builds slime's GPT model and wraps the target modules
with Bridge's LoRA, configured by the pt_lora argument plugins.yaml sets. Loading needs three fixes, installed once per
process. A base checkpoint has no adapter entries: while one loads, every AdapterWrapper lists only the wrapped
module in its sharded state dict, since the adapters' extra state and fc1 split would otherwise break the read. A
pre-hook moves base keys under to_wrap. and fills the adapter keys a base checkpoint lacks, so the strict write-back
succeeds and a real mismatch still fails. Loading from HF reads only the base parameters, under their unwrapped names.
"""
import argparse
import importlib.util
import logging
import os
import sys
import types

logger = logging.getLogger(__name__)

ADAPTER = ".adapter."
WRAPPED = ".to_wrap."
_state = {"installed": False, "base_load": False}


def import_bridge():
    """Return Bridge's LoRA and AdapterWrapper; when the package's __init__ does not import, for example with another
    transformers version, load only its peft subpackage."""
    try:
        from megatron.bridge.peft.adapter_wrapper import AdapterWrapper
        from megatron.bridge.peft.lora import LoRA
        return LoRA, AdapterWrapper
    except Exception as e:  # noqa: BLE001 - any import failure falls back to the subpackage
        logger.info("lora: loading megatron.bridge.peft without the package __init__: %s", e)
    for name in [m for m in sys.modules if m == "megatron.bridge" or m.startswith("megatron.bridge.")]:
        del sys.modules[name]
    import megatron

    spec = importlib.util.find_spec("megatron.bridge")
    if spec is None or not spec.submodule_search_locations:
        raise RuntimeError("Megatron-Bridge is not installed in the image")
    pkg = types.ModuleType("megatron.bridge")
    pkg.__path__ = list(spec.submodule_search_locations)
    sys.modules["megatron.bridge"] = pkg
    megatron.bridge = pkg
    from megatron.bridge.peft.adapter_wrapper import AdapterWrapper
    from megatron.bridge.peft.lora import LoRA
    return LoRA, AdapterWrapper


def remap_keys(state_dict, prefix, adapter_state):
    """Move a wrapper's base keys under to_wrap. and add the adapter entries the checkpoint lacks."""
    for key in [k for k in state_dict if k.startswith(prefix)]:
        rest = key[len(prefix):]
        if not rest.startswith(("to_wrap.", "adapter.")):
            state_dict[prefix + "to_wrap." + rest] = state_dict.pop(key)
    for name, value in adapter_state.items():
        state_dict.setdefault(prefix + "adapter." + name, value)


def remap_hook(module, state_dict, prefix, local_metadata, strict, missing_keys, unexpected_keys, error_msgs):
    """load_state_dict pre-hook of an AdapterWrapper; adapters missing from the checkpoint keep their values."""
    import torch

    adapter = {n: v.detach() if torch.is_tensor(v) else v
               for n, v in module.adapter.state_dict(keep_vars=True).items()}
    remap_keys(state_dict, prefix, adapter)


def checkpoint_path(load_dir):
    """Return the checkpoint directory slime loads from load_dir: the latest iteration named by its tracker file,
    or load_dir itself."""
    tracker = os.path.join(load_dir, "latest_checkpointed_iteration.txt")
    if not os.path.exists(tracker):
        return load_dir
    with open(tracker) as f:
        iteration = f.read().strip()
    return os.path.join(load_dir, "release" if iteration == "release" else "iter_%07d" % int(iteration))


def has_adapters(load_dir):
    """Tell whether the torch_dist checkpoint in load_dir holds adapter entries, as one saved by a LoRA run does."""
    if not load_dir:
        return False
    path = checkpoint_path(load_dir)
    if not os.path.exists(os.path.join(path, ".metadata")):
        return False
    from torch.distributed.checkpoint import FileSystemReader

    return any(ADAPTER in key for key in FileSystemReader(path).read_metadata().state_dict_metadata)


def base_names(named):
    """Yield the base parameters of (name, tensor) pairs under their unwrapped names, without the adapters."""
    for name, tensor in named:
        if ADAPTER not in name:
            yield name.replace(WRAPPED, "."), tensor


def install(adapter_wrapper):
    """Patch AdapterWrapper and slime's checkpoint and HF loaders once per process."""
    if _state["installed"]:
        return
    _state["installed"] = True

    sharded = adapter_wrapper.sharded_state_dict

    def sharded_state_dict(self, prefix="", sharded_offsets=(), metadata=None):
        """List only the wrapped module while a base checkpoint loads."""
        if _state["base_load"]:
            return self.to_wrap.sharded_state_dict(prefix, sharded_offsets, metadata)
        return sharded(self, prefix, sharded_offsets, metadata)

    adapter_wrapper.sharded_state_dict = sharded_state_dict

    from slime.backends.megatron_utils import hf_to_megatron
    from slime.backends.megatron_utils import model as slime_model

    load_checkpoint = slime_model.load_checkpoint

    def load_with_lora(*args, **kwargs):
        """Load the checkpoint, treating one without adapter entries as the base model."""
        from megatron.training.global_vars import get_args

        _state["base_load"] = not has_adapters(get_args().load)
        try:
            return load_checkpoint(*args, **kwargs)
        finally:
            _state["base_load"] = False

    slime_model.load_checkpoint = load_with_lora

    load_hf_weights = hf_to_megatron.load_hf_weights

    def load_hf_base(args, model, path):
        """Load HF weights into the base parameters only."""
        from slime.backends.megatron_utils.update_weight import common

        named = common.named_params_and_buffers

        def base_params(a, m):
            """Yield slime's named parameters without the adapters, under their unwrapped names."""
            return base_names(named(a, m))

        common.named_params_and_buffers = base_params
        try:
            return load_hf_weights(args, model, path)
        finally:
            common.named_params_and_buffers = named

    hf_to_megatron.load_hf_weights = load_hf_base


def model_provider(pre_process=True, post_process=True, vp_stage=None):
    """Build slime's GPT model and wrap the pt_lora target modules with Bridge's LoRA."""
    from megatron.training.global_vars import get_args
    from slime.backends.megatron_utils import model_provider as provider

    args = get_args()
    cfg = getattr(args, "pt_lora", None) or {}
    if not cfg.get("targets"):
        raise RuntimeError("pt_lora in plugins.yaml names no target modules")
    if getattr(args, "num_experts", None):
        raise RuntimeError("this LoRA plugin does not support MoE models yet")
    base_args = argparse.Namespace(**vars(args))
    base_args.custom_model_provider_path = None
    model = provider._get_model_provider_func(base_args)(pre_process=pre_process, post_process=post_process,
                                                          vp_stage=vp_stage)
    lora_cls, adapter_wrapper = import_bridge()
    install(adapter_wrapper)
    lora = lora_cls(target_modules=list(cfg["targets"]), dim=int(cfg["rank"]), alpha=cfg["alpha"],
                    dropout=float(cfg.get("dropout") or 0.0))
    model = lora(model, training=True)
    for module in model.modules():
        if isinstance(module, adapter_wrapper):
            module._register_load_state_dict_pre_hook(remap_hook, with_module=True)
    return model
