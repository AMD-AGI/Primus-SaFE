#!/usr/bin/env python3
"""
Batch optimization demo: submit 8 models to SaFE optimization API.
Already running: Qwen3-8B (opt-ddc9ff04), Qwen3-32B (opt-af61fa33)
Total: 10 models across sizes and architectures.

TP selection rationale (MI300X = 192GB/GPU, FP8 ~= 1 byte/param):
  Model       Params  FP8 size  TP  GPUs  per-GPU
  Qwen3-30B-A3B  30B    30GB   4   4    7.5GB   (MoE: only active 3B params hot)
  Qwen3-235B-A22B 235B  235GB  8   8   29GB
  DeepSeek-R1-0528 671B 671GB  8   8   84GB
  DeepSeek-V3.2   671B  671GB  8   8   84GB
  GLM-4.7         358B  358GB  8   8   45GB    (MoE: 358B total, ~36B active)
  GLM-5           754B  754GB  8   8   94GB    (MoE)
  Kimi-K2.5      1059B 1059GB  8   8  132GB    (MoE: tight but fits with FP8)
  DeepSeek-V4-Flash 158B 158GB 8   8   20GB
"""

import json
import ssl
import sys
import urllib.error
import urllib.request

API_URL = "https://core42.primus-safe.amd.com"
API_KEY = "REMOVED_LEAKED_SECRET"

# 8 new models (Qwen3-8B TP=1 and Qwen3-32B TP=8 already submitted)
MODELS = [
    # displayName, modelId, tp, concurrency
    ("Qwen3-30B-A3B-fp8-mi300x",      "qwen3-30b-a3b-mxlb9",      4,  64),
    ("Qwen3-235B-A22B-fp8-mi300x",    "qwen3-235b-a22b-wmznf",     8,  32),
    ("DeepSeek-R1-0528-fp8-mi300x",   "deepseek-r1-0528-qgfr4",    8,  16),
    ("DeepSeek-V3.2-fp8-mi300x",      "deepseek-v3-2-2cvgl",       8,  16),
    ("GLM-4.7-fp8-mi300x",            "glm-4-7-8pt8h",             8,  32),
    ("GLM-5-fp8-mi300x",              "glm-5-spfg9",               8,  16),
    ("Kimi-K2.5-fp8-mi300x",          "kimi-k2-5-6ffpj",           8,  16),
    ("DeepSeek-V4-Flash-fp8-mi300x",  "deepseek-v4-flash-7j259",   8,  32),
]

items = [
    {
        "displayName": name,
        "modelId": model_id,
        "workspace": "core42-sandbox",
        "mode": "claw",
        "framework": "sglang",
        "precision": "FP8",
        "tp": tp,
        "ep": 1,
        "concurrency": conc,
        "kernelBackends": ["Claude Code"],
    }
    for name, model_id, tp, conc in MODELS
]

payload = json.dumps({"items": items}).encode()

ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

req = urllib.request.Request(
    f"{API_URL}/api/v1/optimization/tasks/batch",
    data=payload,
    headers={
        "Authorization": f"Bearer {API_KEY}",
        "Content-Type": "application/json",
    },
    method="POST",
)

try:
    with urllib.request.urlopen(req, context=ctx) as resp:
        result = json.load(resp)
        tasks = result.get("items", result) if isinstance(result, dict) else result
        print(f"Submitted {len(tasks)} tasks:\n")
        for t in tasks:
            print(f"  {t.get('id','?')}  {t.get('displayName', t.get('clawSessionId','?'))}")
except urllib.error.HTTPError as e:
    body = e.read().decode()
    print(f"HTTP {e.code}: {body}", file=sys.stderr)
    sys.exit(1)
