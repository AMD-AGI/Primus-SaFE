#!/usr/bin/env python3

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

"""
Monarch pre-flight patches. Run once before any training code.
All patches are idempotent and no-op when the target is already fixed.

1. PyTorch inductor duplicate registration assertions (fixed upstream Sep 2025)
2. socket.gethostname -> POD_IP so ManagerServer advertises a routable address
"""

import glob
import os
import sys


# ---------------------------------------------------------------------------
# 1. Patch PyTorch inductor duplicate registration assertions
# ---------------------------------------------------------------------------
_INDUCTOR_PATCHES = {
    "/opt/venv/lib/python*/site-packages/torch/_inductor/select_algorithm.py": [
        (
            'assert name not in self.all_templates, "duplicate template name"',
            "pass",
        ),
        (
            'assert not hasattr(extern_kernels, name), f"duplicate extern kernel: {name}"',
            "pass",
        ),
    ],
    "/opt/venv/lib/python*/site-packages/torch/_inductor/lowering.py": [
        ("assert name not in", "if name in"),
    ],
}

for pattern, replacements in _INDUCTOR_PATCHES.items():
    for fpath in glob.glob(pattern):
        try:
            text = open(fpath).read()
            changed = False
            for old, new in replacements:
                if old in text:
                    text = text.replace(old, new)
                    changed = True
            if changed:
                open(fpath, "w").write(text)
                print(f"[patch_monarch] inductor patched: {fpath}")
        except Exception as e:
            print(f"[patch_monarch] inductor skip {fpath}: {e}")


# ---------------------------------------------------------------------------
# 2. Patch socket.gethostname via sitecustomize.py
# ---------------------------------------------------------------------------
_MARKER = "# monarch-hostname-patch"
_PATCH_TEMPLATE = """
{marker}
import socket as _socket
_orig_gethostname = _socket.gethostname
def _patched_gethostname():
    import os as _os
    return _os.environ.get("POD_IP", _orig_gethostname())
_socket.gethostname = _patched_gethostname
"""

pod_ip = os.environ.get("POD_IP", "")
if pod_ip:
    site_dirs = glob.glob("/opt/venv/lib/python*/site-packages")
    if not site_dirs:
        site_dirs = [d for d in sys.path if "site-packages" in d]
    for site_dir in site_dirs:
        sc_path = os.path.join(site_dir, "sitecustomize.py")
        try:
            existing = open(sc_path).read() if os.path.exists(sc_path) else ""
            if _MARKER in existing:
                continue
            with open(sc_path, "a") as f:
                f.write(_PATCH_TEMPLATE.format(marker=_MARKER))
            print(f"[patch_monarch] hostname patched: {sc_path}")
        except Exception as e:
            print(f"[patch_monarch] hostname skip {sc_path}: {e}")
else:
    print("[patch_monarch] POD_IP not set, skipping hostname patch")
