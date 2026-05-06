#!/usr/bin/env python3
"""
optimize_submit.py — Submit SaFE inference optimization tasks.

Modes:
  Manual : all parameters specified explicitly
  Auto   : only HuggingFace repo ID required; framework/TP/precision auto-detected

Usage examples:
  # Auto mode — single model
  python3 optimize_submit.py --model Qwen/Qwen3-8B

  # Auto mode — multiple models
  python3 optimize_submit.py --model Qwen/Qwen3-8B meta-llama/Llama-3.1-70B-Instruct

  # Auto mode — top-N from HuggingFace
  python3 optimize_submit.py --hf-top 10

  # Manual mode — override everything
  python3 optimize_submit.py --model Qwen/Qwen3-8B \\
      --framework sglang --precision FP8 --tp 1 --concurrency 64 \\
      --image harbor.xxx/proxy/lmsysorg/sglang:v0.5.10-rocm720-mi30x

  # Dry run — print what would be submitted without actually submitting
  python3 optimize_submit.py --model Qwen/Qwen3-8B --dry-run
"""

import argparse
import json
import os
import ssl
import sys
import time
import urllib.error
import urllib.request
from typing import Optional

# ── Configuration ──────────────────────────────────────────────────────────────

API_URL  = os.environ.get("SAFE_API_URL", "https://core42.primus-safe.amd.com")
SAFE_TOKEN  = os.environ.get("SAFE_API_KEY", "")
PROXY    = "harbor.core42.primus-safe.amd.com/proxy"
WORKSPACE      = "core42-hyperloom"
WEKAFS_VOLUME  = "/wekafs"

DEFAULT_SGLANG_IMAGE = f"{PROXY}/lmsysorg/sglang:v0.5.10-rocm720-mi30x"
DEFAULT_VLLM_IMAGE   = f"{PROXY}/vllm/vllm-openai-rocm:v0.18.0"

# ── Architecture → Framework mapping ───────────────────────────────────────────

# Architectures well-supported by SGLang (as of v0.5.x on ROCm)
SGLANG_ARCHS = {
    "LlamaForCausalLM", "LlamaForCausalLMWithVisualEncoder",
    "Qwen2ForCausalLM", "Qwen3ForCausalLM",
    "Qwen2MoeForCausalLM", "Qwen3MoeForCausalLM",
    "MistralForCausalLM", "MixtralForCausalLM",
    "DeepseekV2ForCausalLM", "DeepseekV3ForCausalLM", "DeepseekV32ForCausalLM",
    "GemmaForCausalLM", "Gemma2ForCausalLM", "Gemma3ForCausalLM",
    "InternLM2ForCausalLM", "InternLM3ForCausalLM",
    "Phi3ForCausalLM", "PhiForCausalLM",
    "GPTBigCodeForCausalLM",
    "FalconForCausalLM",
    "ChatGLMModel",
}

# Architectures that require vLLM (Lightning Attention, sparse, or special quant)
VLLM_REQUIRED_ARCHS = {
    "MiniMaxText01ForCausalLM",
    "KimiForConditionalGeneration",
    "KimiK25ForConditionalGeneration",
}

# Quantization types that require vLLM
VLLM_QUANT_TYPES = {"mxfp4", "nvfp4", "int4", "gptq", "awq"}

# ── HuggingFace helpers ─────────────────────────────────────────────────────────

_ssl_ctx = ssl.create_default_context()
_ssl_ctx.check_hostname = False
_ssl_ctx.verify_mode = ssl.CERT_NONE


def hf_get(url: str, hf_token: str = "") -> dict:
    headers = {"User-Agent": "safe-optimize-submit/1.0"}
    if hf_token:
        headers["Authorization"] = f"Bearer {hf_token}"
    req = urllib.request.Request(url, headers=headers)
    try:
        with urllib.request.urlopen(req, context=_ssl_ctx, timeout=15) as r:
            return json.load(r)
    except Exception as e:
        raise RuntimeError(f"HF request failed: {url} — {e}")


def fetch_hf_model_info(repo_id: str, hf_token: str = "") -> dict:
    """Fetch model metadata from HuggingFace API."""
    return hf_get(f"https://huggingface.co/api/models/{repo_id}", hf_token)


def fetch_hf_config(repo_id: str, hf_token: str = "") -> dict:
    """Fetch config.json from HuggingFace."""
    url = f"https://huggingface.co/{repo_id}/resolve/main/config.json"
    return hf_get(url, hf_token)


def fetch_hf_top_models(limit: int = 10, hf_token: str = "", min_params_b: float = 0.0) -> list[str]:
    """Fetch top text-generation models from HuggingFace by downloads.

    Fetches a larger candidate pool, then for each repo calls the individual
    model API (which includes safetensors.total) to filter by min_params_b.
    """
    # Fetch a generous pool; individual API calls below will prune it
    pool_size = max(limit * 10, 100)
    url = (f"https://huggingface.co/api/models?sort=downloads&direction=-1"
           f"&limit={pool_size}&filter=text-generation")
    data = hf_get(url, hf_token)

    repos = []
    for m in data:
        if len(repos) >= limit:
            break
        repo = m.get("modelId") or m.get("id", "")
        if not repo or "/" not in repo:
            continue
        if min_params_b > 0:
            try:
                info = fetch_hf_model_info(repo, hf_token)
                st = (info.get("safetensors") or {}).get("total", 0)
                params_b = st / 1e9 if st else 0
                if params_b < min_params_b:
                    continue
            except Exception:
                continue  # skip gated/errored models
        repos.append(repo)
    return repos


# ── Auto-detection logic ────────────────────────────────────────────────────────

def detect_framework(config: dict, hf_info: dict = {}) -> str:
    arch = (config.get("architectures") or [""])[0]
    quant = config.get("quantization_config") or {}
    quant_type = (quant.get("quant_type") or quant.get("quantization_type") or "").lower()

    if arch in VLLM_REQUIRED_ARCHS:
        return "vllm"
    if any(q in quant_type for q in VLLM_QUANT_TYPES):
        return "vllm"
    if arch in SGLANG_ARCHS:
        return "sglang"
    # Unknown architecture → safer to use vLLM (broader support)
    print(f"  [warn] Unknown architecture '{arch}', defaulting to vllm", file=sys.stderr)
    return "vllm"


def detect_precision(config: dict) -> str:
    quant = config.get("quantization_config") or {}
    quant_type = (quant.get("quant_type") or quant.get("quantization_type") or "").lower()
    torch_dtype = (config.get("torch_dtype") or "").lower()

    if "fp8" in quant_type:      return "FP8"
    if "mxfp4" in quant_type:    return "FP4"
    if "nvfp4" in quant_type:    return "FP4"
    if "int4" in quant_type:     return "INT4"
    if "gptq" in quant_type:     return "INT4"
    if "awq" in quant_type:      return "INT4"
    # For unquantized models, default to FP8 (best perf on MI300X)
    return "FP8"


def detect_param_count(hf_info: dict, config: dict) -> float:
    """Returns parameter count in billions."""
    # Try safetensors total first
    st = hf_info.get("safetensors") or {}
    total = st.get("total", 0)
    if total:
        return total / 1e9

    # Estimate from config
    h = config.get("hidden_size", 0)
    n = config.get("num_hidden_layers", 0)
    vocab = config.get("vocab_size", 0)
    if h and n:
        # Rough: 12 * h^2 * n params (transformer rule of thumb)
        return (12 * h * h * n + vocab * h) / 1e9
    return 0.0


def detect_tp(params_b: float) -> int:
    if params_b <= 0:    return 1
    if params_b < 15:    return 1
    if params_b < 40:    return 4
    return 8


def detect_concurrency(tp: int, framework: str) -> int:
    if framework == "vllm":
        return 64 if tp <= 4 else 16
    # SGLang
    return 64 if tp == 1 else 32 if tp <= 4 else 64


def detect_image(framework: str, precision: str) -> str:
    if framework == "vllm":
        return DEFAULT_VLLM_IMAGE
    return DEFAULT_SGLANG_IMAGE


def auto_detect(repo_id: str, hf_token: str = "") -> dict:
    """Return a dict of detected optimization parameters for a HF repo."""
    print(f"  Fetching HF metadata for {repo_id}...")
    try:
        hf_info = fetch_hf_model_info(repo_id, hf_token)
        config  = fetch_hf_config(repo_id, hf_token)
    except RuntimeError as e:
        print(f"  [error] {e}", file=sys.stderr)
        return {}

    params_b  = detect_param_count(hf_info, config)
    framework = detect_framework(config, hf_info)
    precision = detect_precision(config)
    tp        = detect_tp(params_b)
    conc      = detect_concurrency(tp, framework)
    image     = detect_image(framework, precision)
    arch      = (config.get("architectures") or ["unknown"])[0]

    print(f"  arch={arch}  params={params_b:.1f}B  framework={framework}  "
          f"precision={precision}  tp={tp}  conc={conc}")

    return {
        "framework": framework,
        "precision": precision,
        "tp": tp,
        "concurrency": conc,
        "image": image,
        "params_b": params_b,
        "arch": arch,
    }


# ── SaFE API helpers ────────────────────────────────────────────────────────────

def safe_request(method: str, path: str, body: dict = None) -> dict:
    if not SAFE_TOKEN:
        raise RuntimeError("SAFE_API_KEY not set")
    url = f"{API_URL}/{path.lstrip('/')}"
    data = json.dumps(body).encode() if body else None
    req = urllib.request.Request(
        url, data=data,
        headers={"Authorization": f"Bearer {SAFE_TOKEN}",
                 "Content-Type": "application/json"},
        method=method,
    )
    try:
        with urllib.request.urlopen(req, context=_ssl_ctx, timeout=30) as r:
            return json.load(r)
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"HTTP {e.code}: {e.read().decode()}")


def find_safe_model(repo_id: str) -> Optional[dict]:
    """Find a model in SaFE registry by HuggingFace URL."""
    hf_url = f"https://huggingface.co/{repo_id}"
    try:
        data = safe_request("GET", "api/v1/playground/models?limit=200")
        for m in data.get("items", []):
            src = m.get("sourceURL", "")
            if src.rstrip("/") == hf_url.rstrip("/"):
                return m
    except Exception:
        pass
    return None


def register_model(repo_id: str, hf_token: str = "") -> str:
    """Register a HuggingFace model in SaFE and return its model ID."""
    print(f"  Registering {repo_id} in SaFE (workspace={WORKSPACE}, volume={WEKAFS_VOLUME})...")
    result = safe_request("POST", "api/v1/playground/models", {
        "source": {
            "url": repo_id,
            "accessMode": "local",
            **({"token": hf_token} if hf_token else {}),
        },
        "workspace": WORKSPACE,
        "target": {"volume": WEKAFS_VOLUME},
    })
    return result.get("id", "")


def wait_for_model_ready(model_id: str, timeout_min: int = 120) -> bool:
    """Poll until model phase == Ready or timeout."""
    print(f"  Waiting for model {model_id} to be Ready", end="", flush=True)
    deadline = time.time() + timeout_min * 60
    while time.time() < deadline:
        try:
            m = safe_request("GET", f"api/v1/playground/models/{model_id}")
            phase = m.get("phase", "")
            if phase == "Ready":
                print(" OK")
                return True
            if phase == "Failed":
                print(f" FAILED: {m.get('message','')}")
                return False
        except Exception:
            pass
        print(".", end="", flush=True)
        time.sleep(30)
    print(" timeout")
    return False


def submit_task(model_id: str, display_name: str, params: dict) -> dict:
    body = {
        "displayName": display_name,
        "modelId": model_id,
        "workspace": WORKSPACE,
        "mode": "claw",
        "framework": params["framework"],
        "precision": params["precision"],
        "tp": params["tp"],
        "ep": 1,
        "isl": params.get("isl", 1024),
        "osl": params.get("osl", 1024),
        "concurrency": params["concurrency"],
        "kernelBackends": ["Claude Code"],
    }
    if params.get("image"):
        body["image"] = params["image"]
    return safe_request("POST", "api/v1/optimization/tasks", body)


# ── Main ────────────────────────────────────────────────────────────────────────

def process_model(repo_id: str, args) -> bool:
    print(f"\n{'='*60}")
    print(f"Model: {repo_id}")

    # Build params: start from auto-detection, then apply manual overrides
    params = {}
    if not args.manual:
        params = auto_detect(repo_id, args.hf_token)
        if not params:
            print("  [skip] Failed to auto-detect parameters")
            return False
    else:
        # Manual mode requires explicit framework
        if not args.framework:
            print("  [skip] --framework required in manual mode")
            return False

    # Apply manual overrides
    if args.framework:   params["framework"]   = args.framework
    if args.precision:   params["precision"]   = args.precision
    if args.tp:          params["tp"]          = args.tp
    if args.concurrency: params["concurrency"] = args.concurrency
    if args.image:       params["image"]       = args.image
    if args.isl:         params["isl"]         = args.isl
    if args.osl:         params["osl"]         = args.osl

    # Set default image if not specified
    if not params.get("image"):
        params["image"] = detect_image(params["framework"], params["precision"])

    print(f"  >> framework={params['framework']}  precision={params['precision']}  "
          f"tp={params['tp']}  conc={params['concurrency']}")
    print(f"  >> image={params.get('image','(default)')}")

    if args.dry_run:
        print("  [dry-run] Would submit task — skipping")
        return True

    # Find or register model in SaFE
    safe_model = find_safe_model(repo_id)
    if safe_model:
        model_id = safe_model["id"]
        phase    = safe_model.get("phase", "")
        print(f"  Found in SaFE: {model_id}  phase={phase}")
        if phase != "Ready":
            if not wait_for_model_ready(model_id):
                return False
    else:
        model_id = register_model(repo_id, args.hf_token)
        if not model_id:
            print("  [error] Failed to register model")
            return False
        if not wait_for_model_ready(model_id):
            return False

    # Build display name
    model_name = repo_id.split("/")[-1]
    fw_tag = params["framework"]
    pr_tag = params["precision"].lower()
    display_name = f"{model_name}-{pr_tag}-{fw_tag}-mi300x"

    # Submit optimization task
    result = submit_task(model_id, display_name, params)
    task_id = result.get("id", "?")
    print(f"  [OK] Submitted: {task_id}  ({display_name})")
    return True


def main():
    parser = argparse.ArgumentParser(
        description="Submit SaFE inference optimization tasks",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    # Model selection
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--model", nargs="+", metavar="HF_REPO",
                       help="HuggingFace repo ID(s), e.g. Qwen/Qwen3-8B")
    group.add_argument("--hf-top", type=int, metavar="N",
                       help="Auto-select top-N models from HuggingFace by downloads")
    parser.add_argument("--min-params", type=float, default=0.0, metavar="B",
                        help="Only include models with >= B billion parameters (e.g. 7 for 7B+)")

    # Mode
    parser.add_argument("--manual", action="store_true",
                        help="Manual mode: all parameters must be specified explicitly")

    # Manual overrides (also usable in auto mode to override detected values)
    parser.add_argument("--framework", choices=["sglang", "vllm"])
    parser.add_argument("--precision", choices=["FP8", "FP4", "BF16", "INT4"])
    parser.add_argument("--tp", type=int, choices=[1, 2, 4, 8])
    parser.add_argument("--concurrency", type=int)
    parser.add_argument("--isl", type=int, default=1024)
    parser.add_argument("--osl", type=int, default=1024)
    parser.add_argument("--image", help="Custom container image")

    # Auth
    parser.add_argument("--hf-token", default=os.environ.get("HF_TOKEN", ""),
                        help="HuggingFace token for gated models (or set HF_TOKEN env)")
    parser.add_argument("--api-key", default="",
                        help="SaFE API key (or set SAFE_API_KEY env)")

    # Misc
    parser.add_argument("--dry-run", action="store_true",
                        help="Print what would be submitted without actually submitting")
    parser.add_argument("--workspace", default=WORKSPACE)
    parser.add_argument("--volume", default=WEKAFS_VOLUME,
                        help="Workspace volume mountPath for model download (default: /wekafs)")

    args = parser.parse_args()

    # Apply overrides
    global SAFE_TOKEN, WORKSPACE, WEKAFS_VOLUME
    if args.api_key:
        SAFE_TOKEN = args.api_key
    WORKSPACE     = args.workspace
    WEKAFS_VOLUME = args.volume
    if not SAFE_TOKEN:
        print("Error: SAFE_API_KEY not set. Use --api-key or set SAFE_API_KEY env var.", file=sys.stderr)
        sys.exit(1)

    # Collect model list
    if args.hf_top:
        min_p = args.min_params
        label = f"top-{args.hf_top}" + (f" (>={min_p}B)" if min_p else "")
        print(f"Fetching {label} text-generation models from HuggingFace...")
        repos = fetch_hf_top_models(args.hf_top, args.hf_token, min_params_b=min_p)
        print(f"Selected {len(repos)} models: {repos}")
    else:
        repos = args.model

    # Process each model
    results = {"ok": [], "failed": []}
    for repo in repos:
        ok = process_model(repo, args)
        (results["ok"] if ok else results["failed"]).append(repo)

    # Summary
    print(f"\n{'='*60}")
    print(f"Done: {len(results['ok'])} submitted, {len(results['failed'])} failed")
    if results["failed"]:
        print(f"Failed: {results['failed']}")


if __name__ == "__main__":
    main()
