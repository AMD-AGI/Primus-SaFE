#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Patch PyTorch inductor duplicate registration assertions.
# Idempotent: safe to run multiple times; no-op if already patched.

python3 -c "
import glob
for pat, repls in {
    '/opt/venv/lib/python*/site-packages/torch/_inductor/select_algorithm.py': [
        ('assert name not in self.all_templates, \"duplicate template name\"', 'pass'),
        ('assert not hasattr(extern_kernels, name), f\"duplicate extern kernel: {name}\"', 'pass'),
    ],
    '/opt/venv/lib/python*/site-packages/torch/_inductor/lowering.py': [
        ('assert name not in', 'if name in'),
    ],
}.items():
    for fp in glob.glob(pat):
        try:
            t = open(fp).read()
            changed = False
            for o, n in repls:
                if o in t:
                    t = t.replace(o, n)
                    changed = True
            if changed:
                open(fp, 'w').write(t)
                print(f'Patched: {fp}')
        except Exception as e:
            print(f'Skip {fp}: {e}')
" 2>&1
