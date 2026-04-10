# SFT Frontend Design

## Status Note

This document contains historical SFT-specific design notes.

The current final product direction is:

- training is created from `Model Lab -> PostTrain`
- `Model Square` no longer owns the primary training entry

For the final unified training UX, use:

- `training-frontend-design.md`
- `posttrain-frontend-design.md`

## Overview

This document describes the frontend product and implementation design for SFT (Supervised Fine-Tuning) in `Web/apps/safe`.

It covers:

- user entry points
- page and dialog flow
- filtering and display rules
- model export behavior
- Model Square changes for fine-tuned models
- the relationship between `accessMode`, `origin`, and deployability

This is a UI / frontend design document. For API contracts, see:

- `SaFE/docs/apis/sft.md`

## Product Goal

The product goal is:

1. user selects an existing base model from Model Square
2. user selects an SFT dataset
3. user configures training settings and resource settings
4. user optionally enables "export model after training"
5. system creates a training workload
6. after training succeeds, system exports a new model to Model Square
7. exported model is visible as a custom fine-tuned model and can be managed like other deployable local models

## Core Product Decision

### Exported model should stay `local_path`, not be rewritten to `local`

Frontend should **not** assume that exported fine-tuned models must be converted to `accessMode = local`.

The correct product and data model is:

- HuggingFace-imported model:
  - `accessMode = local`
  - `origin = external`
- exported fine-tuned model:
  - `accessMode = local_path`
  - `origin = fine_tuned`

Why:

1. `local_path` means the model files already exist on PFS/NFS and should not trigger any download pipeline.
2. `local` currently means a model managed through the normal download/sync lifecycle.
3. the exported fine-tuned model is semantically different from a HuggingFace-imported base model even if both are deployable.

### Why not simply convert exported models to `local`

At first glance, converting exported fine-tuned models from `local_path` to `local` may seem attractive because the current Model Square "Start Service" entry is written for `local` models.

However, this is **not** the recommended product direction.

If exported models were rewritten to `local`, frontend would gain short-term simplicity but the system semantics would become confusing:

1. `local` currently implies "managed by import/download lifecycle"
2. exported fine-tuned models already exist on PFS and should not be treated as downloadable artifacts
3. later troubleshooting and lifecycle handling would become harder because two different storage meanings would be mixed into the same access mode

Therefore the recommended strategy is:

- keep exported SFT models as `local_path`
- update frontend and backend deploy/start-service logic so both `local` and `local_path` behave as deployable local models

### Current Code Reality

Current implementation is split:

1. **Product expectation**:
   - exported fine-tuned models should be deployable from Model Square
2. **Current frontend behavior**:
   - list page only shows Start/Stop for `accessMode === 'local'`
3. **Current backend behavior**:
   - `GET /api/v1/playground/models/:id/workload-config` currently only accepts `local`

So the gap is not in the exported model format itself.

The gap is:

- frontend deployability condition is too narrow
- backend workload-config generation is too narrow

This document assumes we will close that gap by broadening deployability support, **not** by changing the meaning of `accessMode`.

### Frontend should introduce a derived concept: "deployable local model"

For UI behavior, frontend should treat both of these as deployable local models:

- `accessMode = local`
- `accessMode = local_path`

Recommended frontend helper:

```ts
const isDeployableLocalModel =
  model.accessMode === 'local' || model.accessMode === 'local_path'
```

This separates:

- **storage / lifecycle semantics** -> `accessMode`
- **product display / deployability semantics** -> derived UI state

## User Flow

### Flow A: Create SFT Job

```text
Model Square
  -> choose base model
  -> click "Fine-tune"
  -> open SFT Create page/dialog
  -> choose dataset
  -> configure training + resource + export option
  -> create SFT job
  -> redirect to Training Detail
```

### Flow B: Track Training

```text
Training page / Training detail
  -> track workload status
  -> if exportModel=true and training succeeds
  -> new model appears in Model Square
```

### Flow C: Use Exported Model

```text
Model Square
  -> exported fine-tuned model appears with "Custom" / "Fine-tuned" identity
  -> user can inspect owner, base model, source SFT job
  -> user can start inference service like other deployable local models
```

## Entry Points

### Model Square List Page

Add a `Fine-tune` action on model cards.

Visibility rule:

- show only when:
  - `accessMode === 'local'`
  - `phase === 'Ready'`
  - model is supported by current SFT backend recipe mapping

Do **not** show the SFT entry on:

- `remote_api`
- `local_path`
- failed / pending models

Reason:

- `v1` SFT is designed for known base models imported from HuggingFace / local base-model pipeline
- exported fine-tuned models should first be treated as results, not as new base models for recursive fine-tuning

If multi-generation fine-tuning is needed later, add it as an explicit product extension rather than enabling it implicitly now.

### Model Detail Page

Add a `Fine-tune` button next to existing model actions, using the same visibility rule as the list page.

## SFT Create UI

### Suggested Placement

Preferred options:

1. full page: `Model Square -> Fine-tune`
2. large drawer/dialog from Model Square

Recommendation:

- use a full page or wide drawer
- avoid a tiny modal because SFT contains model selection context, dataset selection, training parameters, resources, and export configuration

### Sections

Suggested UI sections:

1. Base Model
2. Dataset
3. Training Configuration
4. Resource Configuration
5. Output / Export
6. Advanced Settings

### Section 1: Base Model

Read-only section, prefilled from selected model:

- `displayName`
- `modelId`
- `modelName`
- `accessMode`
- `workspace availability`

Optional extra info:

- model icon
- tags
- max tokens

### Section 2: Dataset

Use `GET /api/v1/datasets` and filter in frontend by:

- `datasetType = sft`
- `status = Ready`

Strong recommendation:

- frontend should only display datasets whose `datasetType === 'sft'`
- frontend should not allow choosing evaluation or other dataset types for SFT creation

Optional additional frontend filtering:

- prefer exact workspace match
- otherwise allow datasets that are available through shared storage path accessibility

### Section 3: Training Configuration

Show commonly used parameters:

- `peft`
- `datasetFormat`
- `trainIters`
- `globalBatchSize`
- `microBatchSize`
- `seqLength`
- `finetuneLr`

Advanced accordion:

- `minLr`
- `lrWarmupIters`
- `evalInterval`
- `saveInterval`
- `precisionConfig`
- parallelism fields

### Section 4: Resource Configuration

Show fields aligned with training creation habits:

- `image`
- `nodeCount`
- `gpuCount`
- `cpu`
- `memory`
- `ephemeralStorage`
- `priority`

Recommended default UX:

- display backend default values directly in form
- allow user edits
- label `priority = 1` as `Medium`

### Section 5: Output / Export

Add a dedicated switch:

- label: `Export model after training`
- field: `exportModel`
- default: `true`

Help text:

- when enabled, training output will be exported to PFS and registered in Model Square as a new fine-tuned model

### Section 6: Advanced Settings

Optional fields:

- `timeout`
- `env`
- `hostpath`
- `forceHostNetwork`

## Defaults Strategy

### Current v1

Backend already applies defaults:

- `exportModel = true`
- `priority = 1`
- `gpuCount = 8`
- `nodeCount = 1`
- preset-based training defaults

Frontend should not hardcode too many business defaults beyond display convenience.

### Required Backend Config API

Frontend should depend on a dedicated backend config endpoint:

```text
GET /api/v1/playground/models/:id/sft-config?workspace=<workspaceId>
```

This endpoint should return:

- default image
- default resource values
- default training values
- supported dataset format options
- supported PEFT options
- whether the selected model is supported for SFT
- if unsupported, the reason why

Suggested frontend behavior:

1. enter SFT page/dialog from a selected model
2. immediately call `GET /api/v1/playground/models/:id/sft-config`
3. if `supported=false`, disable submit and display `reason`
4. if `supported=true`, hydrate the form from `defaults` and `options`

Example response shape:

```json
{
  "supported": true,
  "reason": "",
  "model": {
    "id": "model-zspk2",
    "displayName": "Qwen/Qwen3-8B",
    "modelName": "Qwen/Qwen3-8B",
    "accessMode": "local",
    "phase": "Ready",
    "workspace": "",
    "maxTokens": 40960
  },
  "datasetFilter": {
    "datasetType": "sft",
    "workspace": "project1-dev",
    "status": "Ready"
  },
  "defaults": {
    "exportModel": true,
    "image": "rocm/primus:v26.1",
    "nodeCount": 1,
    "gpuCount": 8,
    "cpu": "128",
    "memory": "1024Gi",
    "ephemeralStorage": "300Gi",
    "priority": 1,
    "trainConfig": {
      "peft": "none",
      "datasetFormat": "alpaca",
      "trainIters": 1000,
      "globalBatchSize": 128,
      "microBatchSize": 1,
      "seqLength": 2048,
      "finetuneLr": 0.0001,
      "minLr": 0,
      "lrWarmupIters": 50,
      "evalInterval": 30,
      "saveInterval": 50,
      "precisionConfig": "bf16_mixed",
      "tensorModelParallelSize": 1,
      "pipelineModelParallelSize": 1,
      "contextParallelSize": 1,
      "sequenceParallel": false,
      "peftDim": 0,
      "peftAlpha": 0,
      "packedSequence": false
    }
  },
  "options": {
    "peftOptions": ["none", "lora"],
    "datasetFormatOptions": ["alpaca"],
    "priorityOptions": [
      { "label": "Low", "value": 0 },
      { "label": "Medium", "value": 1 },
      { "label": "High", "value": 2 }
    ]
  }
}
```

This endpoint should be treated as a required frontend dependency for the SFT form.

## Create Action

Frontend calls:

```text
POST /api/v1/sft/jobs
```

After success:

- read `workloadId`
- redirect to `Training Detail`
- optionally show success toast with the workload ID

## Training Page Behavior

SFT jobs should continue using the existing Training workload pages.

This is the intended behavior:

- no separate SFT-only workload table is required in `v1`
- "My Workload" continues to work via the existing `userId` filter

Optional enhancement:

- show an `SFT` tag in workload list when `safe/workload-type = sft`

## Model Square After Export

### Display Strategy

Model Square should distinguish two dimensions:

1. **deployability class**
2. **model origin**

#### Deployability class

- `remote_api`
- `deployable local`

`deployable local` includes:

- `local`
- `local_path`

#### Model origin

- `external`
- `fine_tuned`

### Recommended Visual Rules

#### For HuggingFace / base models

- show badge: `HuggingFace` or `Base`
- typical shape:
  - `accessMode = local`
  - `origin = external`

#### For exported fine-tuned models

- show badge: `Custom`
- show badge: `Fine-tuned`
- typical shape:
  - `accessMode = local_path`
  - `origin = fine_tuned`

### Metadata to Display

For fine-tuned models, frontend should display:

- `userName`
- `baseModel`
- `sftJobId`
- `workspace`

Suggested list-card secondary line:

```text
By Yang, Chenyi · Base: Qwen/Qwen3-8B
```

Suggested detail page fields:

- owner
- base model
- source SFT job
- origin
- workspace

## Inference / Start Service Behavior

### Product Rule

Exported fine-tuned models should be startable from Model Square just like other deployable local models.

### Required Frontend Change

Current frontend only shows Start/Stop for `accessMode === 'local'`.

This should be changed to:

```ts
const canStartService =
  model.phase === 'Ready' &&
  (model.accessMode === 'local' || model.accessMode === 'local_path')
```

The same rule should be used consistently in:

- Model Square list card actions
- Model detail page action buttons
- any reusable "can deploy / can start service" helper

### Required Backend Dependency

Backend `GET /api/v1/playground/models/:id/workload-config` must support:

- `local`
- `local_path`

So the frontend and backend should be updated together.

### Current Gap Analysis

The current codebase behaves as follows:

- exported SFT models are correctly created as `local_path`
- Model Square can display them
- metadata for owner / origin / base model / source job is available
- but one-click inference from Model Square is still gated by `accessMode === 'local'`

So when reviewing current UI behavior, it may **look like** exported models are almost fully supported, but the final deploy/start-service path is still incomplete until the above frontend + backend changes are made.

### Important Design Decision

Do **not** change exported models from `local_path` to `local` just to make Start Service appear.

Instead:

- keep `local_path` as the correct storage semantics
- update deploy logic to support both `local` and `local_path`

### Alternative Option Considered But Rejected

Rejected option:

- export SFT result as `accessMode = local`

Reason for rejection:

- it hides an important storage/lifecycle distinction
- it makes future cleanup, troubleshooting, and data lineage harder
- it couples UI convenience to an incorrect backend semantic

Accepted option:

- export as `local_path`
- treat `local_path` as deployable in the UI
- extend backend workload-config generation accordingly

## Model Square Filtering

### Existing Filter

Current type filter is too coarse if it only shows:

- `local`
- `remote_api`

### Recommended Filters

Add or replace with:

1. **Source**
   - All
   - HuggingFace / Base
   - Custom / Fine-tuned
   - Remote API

2. **Owner**
   - All
   - Mine

3. **Access Mode**
   - optional advanced filter:
   - `local`
   - `local_path`
   - `remote_api`

Recommended product-facing grouping should use `origin`, not raw `accessMode`.

## Frontend Type Changes

Update `Web/apps/safe/src/services/playground/index.ts`.

### `PlaygroundModel`

Current type is incomplete for SFT fine-tuned models.

It should include:

```ts
accessMode?: 'remote_api' | 'local' | 'local_path' | 'cloud'
origin?: 'external' | 'fine_tuned'
sftJobId?: string
baseModel?: string
userId?: string
userName?: string
workspace?: string
```

## Component Change List

### Model Square

Files likely needing updates:

- `Web/apps/safe/src/pages/ModelSquare/index.vue`
- `Web/apps/safe/src/pages/ModelSquare/ModelSquareDetail.vue`
- `Web/apps/safe/src/pages/ModelSquare/Components/AddModelDialog.vue`
- `Web/apps/safe/src/services/playground/index.ts`

### New SFT UI

Suggested new files:

- `Web/apps/safe/src/pages/ModelSquare/Components/CreateSftDialog.vue`
- or `Web/apps/safe/src/pages/SFT/Create.vue`
- `Web/apps/safe/src/services/sft/index.ts`
- `Web/apps/safe/src/services/sft/type.ts`

### Dataset Selector Reuse

Can reuse existing dataset API service and dataset type concepts from:

- `Web/apps/safe/src/pages/Dataset`
- `Web/apps/safe/src/services/dataset`

## Backward Compatibility

Historical fine-tuned models created before ownership metadata was fully wired may have incomplete:

- `userId`
- `userName`

Frontend should tolerate empty ownership fields and fall back to:

- `-`
- `Unknown`
- or hide that row if missing

## Non-Goals for v1

The following should not be treated as required for the first UI release:

- recursive fine-tuning from exported `local_path` fine-tuned models
- dedicated SFT workload page separate from Training
- rich quality validation of exported models
- advanced model lineage graph

## Final Product Positioning

### Recommended v1 positioning

- base models:
  - imported from HuggingFace
  - selectable for fine-tuning
- fine-tuned models:
  - exported as `local_path`
  - displayed as `Custom / Fine-tuned`
  - show owner and lineage
  - should be deployable for inference

This gives the cleanest separation:

- **Base model selection** -> use HuggingFace-imported local models
- **Result model management** -> use exported fine-tuned models
- **Deployment behavior** -> both are deployable local models
