# Training Validation Status / 训练状态总览

## Scope

This document records the latest known validation status for OCI productized SFT and RL training flows.

It is intended to answer four questions:

1. Which scenarios are already confirmed to work end-to-end
2. Which scenarios had historical failures or false-success incidents
3. Which code fixes have already landed
4. Which scenarios still require rerun validation before being treated as stable

This document complements, rather than replaces:

- `oci-sft-validation-summary.md` for SFT-focused OCI validation
- `rl-training-design.md` for RL API and architecture design

## Status Definitions

- `Confirmed`: already revalidated successfully in a later fix round
- `Code Fixed, Pending Rerun`: the identified code-level blocker has been fixed, but the exact scenario still needs a fresh rerun
- `Mitigated, Pending Rerun`: timeout / networking / diagnostics were improved, but success still depends on cluster stability
- `In Queue`: a live verification workload exists but has not started yet

## Current Snapshot

- SFT on OCI has a later 4/4 successful validation round for `full1n`, `lora1n`, `full2n`, and `lora2n`
- RL currently has one confirmed productized success: `fsdp2 1n`
- `RL 1n` resource topology has been fixed so API-created single-node jobs now request only one training node
- `RL megatron 1n/2n` no longer use the earlier known-bad API-generated schema, but still need rerun confirmation
- Multi-node timeout issues are not treated as fully solved; they are mitigated and still need environment-level reruns

## Validation Matrix

| Scenario | Representative Workload | Latest Known State | Historical Issue | Current Judgement |
|----------|--------------------------|--------------------|------------------|-------------------|
| SFT full1n | `sft-test-1-v261-single-full-vw6w9` | Confirmed | None | Ready baseline |
| SFT lora1n | `sft-test-2-v261-single-lora-b5tzc` | Confirmed | None | Ready baseline |
| SFT full2n | `sft-test-3-v261-multi-full-qdlx2` | Confirmed | Earlier API run `sft-qwen3-8b-sft-full2n-v261proxy-0409-00090-55335` was a false success | Ready after export guard and later revalidation |
| SFT lora2n | `sft-test-4-v261-multi-lora-zfl5x` | Confirmed | Earlier API run `sft-qwen3-8b-sft-lora2n-v261proxy-0409-00090-58469` failed with NCCL timeout | Ready in latest SFT validation, but still cluster-sensitive |
| RL fsdp2 1n | `rl-qwen3-8b-rl-fsdp2-1n-0409-005335-18000` | Confirmed | None | Current RL productized baseline |
| RL fsdp2 2n | `rl-qwen3-8b-rl-fsdp2-2n-0409-005335-21499` | Failed historically | Ray submitter timeout was the surface symptom; underlying issue was multi-node NCCL collective timeout | Mitigated, pending rerun |
| RL megatron 1n | `rl-qwen3-8b-rl-megatron-1n-0409-005335-26071` | Failed historically | API-generated megatron Hydra overrides were incompatible with the current `verl` image schema | Code fixed, pending rerun |
| RL megatron 2n | `rl-qwen3-8b-rl-megatron-2n-0409-005335-30352` | Failed historically | Same schema mismatch as 1n, plus multi-node execution risk | Code fixed for schema, pending rerun |

## Historical Incident Summary

### 1. SFT full2n false success

Historical workload:

- `sft-qwen3-8b-sft-full2n-v261proxy-0409-00090-55335`

Observed behavior:

- workload showed `Succeeded`
- `Model` CRD was created in `Ready`
- export directory did not contain a complete HuggingFace model

Root cause:

- export stage could not find a valid checkpoint
- script fell back to copying raw output
- model registration still continued without validating exported artifacts

Current handling:

- export now requires `config.json`
- export now requires tokenizer files
- export now requires `*.safetensors` or equivalent HF weight artifacts
- model registration is blocked if the export is incomplete

### 2. SFT lora2n failure

Historical workload:

- `sft-qwen3-8b-sft-lora2n-v261proxy-0409-00090-58469`

Historical root cause:

- multi-node NCCL collective timeout during training
- watchdog terminated the distributed group after timeout

Current judgement:

- this exact failure mode is not considered a pure code bug
- later OCI SFT validation did pass for multi-node LoRA
- still treat multi-node runs as cluster-sensitive and monitor closely

### 3. RL fsdp2 2n failure

Historical workload:

- `rl-qwen3-8b-rl-fsdp2-2n-0409-005335-21499`

Historical root cause:

- platform reported `rayJob submitter has timed out`
- archived logs showed the real failure was NCCL collective timeout during multi-node RL execution

Current handling:

- longer RL multi-node NCCL timeout
- longer Ray registration timeout
- runtime network detection instead of static workspace inference

Current judgement:

- mitigated, not fully guaranteed
- still requires a fresh rerun on a healthy OCI multi-node environment

### 4. RL megatron 1n/2n failure

Historical workloads:

- `rl-qwen3-8b-rl-megatron-1n-0409-005335-26071`
- `rl-qwen3-8b-rl-megatron-2n-0409-005335-30352`

Historical root cause:

- API-generated megatron overrides did not match the Hydra schema accepted by the current `verl` megatron image
- tasks failed at startup with `unrecognized arguments`

Current handling:

- megatron entrypoint generation now switches to `ppo_megatron_trainer.yaml`
- parallelism defaults were aligned to historical successful scripts
- megatron-specific override paths were updated to the currently accepted schema

Current judgement:

- the earlier known-bad schema mismatch is treated as fixed
- `1n` has higher confidence than `2n`
- both still need a clean rerun before being called release-ready

## Current Live Verification

There is an active queueing workload that exists specifically to validate that productized `RL 1n` is now a true single-node RayJob:

- `rl-chenyi-ant-rl-1n-0409-161416-59124`

Current meaning of this workload:

- it was created through the frontend-style API flow
- it requests only one training resource block instead of `head + worker` for `nodeCount=1`
- it is currently pending in `control-plane-anthropic` due to lack of free GPUs

This workload is useful as a live proof that the `RL 1n` topology bug has been fixed, even before training starts.

## Code Fix Mapping

### `SaFE/apiserver/pkg/handlers/model-handlers/rl.go`

- fixed `RAY_JOB_ENTRYPOINT` base64 encoding
- fixed `RL 1n` resource generation so `nodeCount=1` creates only a single training resource block

### `SaFE/apiserver/pkg/handlers/model-handlers/rl_entrypoint_builder.go`

- aligned RL megatron overrides with the current `verl` image schema
- aligned megatron default parallelism with historical successful scripts
- added runtime AINIC/Broadcom network detection
- increased multi-node RL timeout-related settings
- added RL HuggingFace export completeness validation before model registration

### `SaFE/apiserver/pkg/handlers/model-handlers/entrypoint_builder.go`

- added HuggingFace export completeness validation for SFT before model registration
- prevents export-empty workloads from being recorded as successful model registrations

## Recommended Next Validation Order

1. Rerun `RL megatron 1n`
2. Rerun `RL fsdp2 2n`
3. Rerun `RL megatron 2n`
4. Keep monitoring live `RL 1n` queue/start behavior in `anthropic`

## Notes For Leadership / Productization Reviews

- `Confirmed` means there is already at least one later successful validation with the relevant fix set
- `Code Fixed, Pending Rerun` means the earlier deterministic code bug has been addressed, but the scenario still needs proof by execution
- `Mitigated, Pending Rerun` means the main work done so far is runtime hardening and timeout/network stabilization, not a guaranteed deterministic fix
- `loss` and richer training metrics are not yet exposed through a stable structured API and should be treated as a separate productization track
