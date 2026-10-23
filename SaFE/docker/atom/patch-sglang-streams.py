#!/usr/bin/env python3
"""Realize scheduler HIP streams before Mooncake dma-buf registration."""

import os
from pathlib import Path

path = Path(
    os.environ.get(
        "SGLANG_SCHEDULER",
        "/sgl-workspace/sglang/python/sglang/srt/managers/scheduler.py",
    )
)
marker = "Realize HIP streams before Mooncake dma-buf export"
text = path.read_text()
old_loop = """        for realized in (self.schedule_stream, self.forward_stream):
            if realized is not None and hasattr(realized, "cuda_stream"):
                _ = realized.cuda_stream
"""
new_loop = """        for name in ("schedule_stream", "forward_stream"):
            realized = getattr(self, name, None)
            if realized is not None and hasattr(realized, "cuda_stream"):
                _ = realized.cuda_stream
"""
if marker in text:
    if new_loop in text:
        print("scheduler streams already realized")
        raise SystemExit(0)
    if old_loop not in text:
        raise SystemExit("scheduler stream loop anchor missing")
    path.write_text(text.replace(old_loop, new_loop, 1))
    print("scheduler streams upgraded")
    raise SystemExit(0)

anchor = """        ) = self.tp_worker.get_worker_info()
        # DFlash auto-enables the legacy formula; other workloads opt in via
"""
replacement = """        ) = self.tp_worker.get_worker_info()
        # Realize HIP streams before Mooncake dma-buf export. Export leaves
        # a later hipStreamCreate on this thread segfaulting.
        for name in ("schedule_stream", "forward_stream"):
            realized = getattr(self, name, None)
            if realized is not None and hasattr(realized, "cuda_stream"):
                _ = realized.cuda_stream
        # DFlash auto-enables the legacy formula; other workloads opt in via
"""
if anchor not in text:
    raise SystemExit("scheduler stream anchor missing")
path.write_text(text.replace(anchor, replacement, 1))
print("scheduler streams patched")
