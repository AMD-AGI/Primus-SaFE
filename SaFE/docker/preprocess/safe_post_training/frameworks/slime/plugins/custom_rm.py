#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Custom reward: loads the user's reward function from the Python file PT_CUSTOM_RM names.

slime imports reward functions by module path, so it can't load a file in the workspace directly. PT_CUSTOM_RM is
"<file>.py" or "<file>.py:<function>"; the function defaults to custom_rm and may be async, as slime expects, or
plain.
"""

import importlib.util
import inspect
import os

SPEC_ENV = "PT_CUSTOM_RM"
DEFAULT_FUNCTION = "custom_rm"

_function = None


def load(spec):
    """Load and return the function a "<file>.py[:<function>]" spec names."""
    path, _, name = spec.partition(":")
    if not os.path.isfile(path):
        raise FileNotFoundError("the custom reward file %s does not exist" % path)
    module_spec = importlib.util.spec_from_file_location("safe_post_training_custom_reward", path)
    module = importlib.util.module_from_spec(module_spec)
    module_spec.loader.exec_module(module)
    function = getattr(module, name or DEFAULT_FUNCTION, None)
    if not callable(function):
        raise AttributeError("%s defines no function %s" % (path, name or DEFAULT_FUNCTION))
    return function


async def custom_rm(args, sample, **kwargs):
    """Score one sample with the user's reward function, loaded on first use."""
    global _function
    if _function is None:
        _function = load(os.environ.get(SPEC_ENV, ""))
    result = _function(args, sample, **kwargs)
    if inspect.isawaitable(result):
        result = await result
    return result
