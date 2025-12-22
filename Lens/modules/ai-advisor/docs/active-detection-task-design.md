# Active Framework Detection Task Design

## Overview

This document describes the design for adding an **Active Detection Task** to the ai-advisor module. The goal is to complement the existing passive detection mechanism with proactive framework detection that runs when workloads are created.

## Current Architecture (Passive Only)

```
┌─────────────────────────────────────────────────────────────────┐
│                    Current Detection Flow                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   External Sources                 Detection System              │
│   ┌──────────────┐                                              │
│   │ WandB        │──────┐                                       │
│   │ Exporter     │      │        ┌──────────────────────┐      │
│   └──────────────┘      ├───────▶│  FrameworkDetection  │      │
│   ┌──────────────┐      │        │  Manager             │      │
│   │ Log Stream   │──────┤        │                      │      │
│   └──────────────┘      │        │  - ReportDetection() │      │
│   ┌──────────────┐      │        │  - MergeDetections() │      │
│   │ API Request  │──────┘        └──────────┬───────────┘      │
│   └──────────────┘                          │                   │
│                                             ▼                   │
│                                 ┌──────────────────────┐       │
│                                 │    TaskCreator       │       │
│                                 │ (creates downstream  │       │
│                                 │  tasks on detection) │       │
│                                 └──────────────────────┘       │
│                                                                 │
│   Problem: No detection happens unless external sources report  │
└─────────────────────────────────────────────────────────────────┘
```

## Proposed Architecture (Passive + Active)

```
┌─────────────────────────────────────────────────────────────────┐
│                    New Detection Flow                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   ┌─────────────────── PASSIVE LAYER ─────────────────────────┐ │
│   │                                                            │ │
│   │  WandB Exporter ──┐                                       │ │
│   │  Log Stream ──────┼──▶ ReportDetection() ──┐              │ │
│   │  API Request ─────┘                        │              │ │
│   │                                            │              │ │
│   └────────────────────────────────────────────┼──────────────┘ │
│                                                │                 │
│   ┌─────────────────── ACTIVE LAYER ──────────┼───────────────┐ │
│   │                                           │                │ │
│   │  ┌────────────────────────────────────────▼─────────────┐ │ │
│   │  │            FrameworkDetectionManager                  │ │ │
│   │  │                                                       │ │ │
│   │  │  - ReportDetection() (passive input)                  │ │ │
│   │  │  - MergeDetections() (multi-source fusion)            │ │ │
│   │  │  - Event dispatch to listeners                        │ │ │
│   │  └───────────────────────────────────────────────────────┘ │ │
│   │                              │                             │ │
│   │  ┌───────────────────────────▼─────────────────────────┐  │ │
│   │  │              TaskCreator                             │  │ │
│   │  │                                                      │  │ │
│   │  │  On Workload Created:                                │  │ │
│   │  │    → Create ActiveDetectionTask (NEW!)               │  │ │
│   │  │                                                      │  │ │
│   │  │  On Detection Confirmed:                             │  │ │
│   │  │    → Create MetadataCollectionTask                   │  │ │
│   │  │    → Create ProfilerCollectionTask                   │  │ │
│   │  └───────────────────────────────────────────────────────┘ │ │
│   │                              │                             │ │
│   │  ┌───────────────────────────▼─────────────────────────┐  │ │
│   │  │         ActiveDetectionExecutor (NEW!)               │  │ │
│   │  │                                                      │  │ │
│   │  │  1. Collect evidence from multiple sources:          │  │ │
│   │  │     - Process cmdline, env vars                      │  │ │
│   │  │     - Container image name                           │  │ │
│   │  │     - Pod labels/annotations                         │  │ │
│   │  │     - Existing evidence in DetectionManager          │  │ │
│   │  │                                                      │  │ │
│   │  │  2. Run pattern matching                             │  │ │
│   │  │                                                      │  │ │
│   │  │  3. Report detection result                          │  │ │
│   │  │                                                      │  │ │
│   │  │  4. Reschedule if not confirmed                      │  │ │
│   │  └──────────────────────────────────────────────────────┘  │ │
│   │                                                            │ │
│   └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Implementation Plan

### 1. Add New Task Type

**File: `core/pkg/constant/task.go`**

```go
const (
    // ... existing task types
    TaskTypeActiveDetection = "active_detection"  // NEW
)
```

### 2. Create ActiveDetectionTask on Workload Creation

**File: `ai-advisor/pkg/detection/task_creator.go`**

Add a new method to create active detection task when workload is first seen:

```go
// CreateActiveDetectionTask creates an active detection task for a new workload
// This is triggered when a workload is first discovered (before any detection)
func (tc *TaskCreator) CreateActiveDetectionTask(
    ctx context.Context,
    workloadUID string,
) error {
    // Check if task already exists
    existingTask, err := tc.taskFacade.GetTask(ctx, workloadUID, constant.TaskTypeActiveDetection)
    if err == nil && existingTask != nil {
        // Task already exists, skip
        return nil
    }

    task := &model.WorkloadTaskState{
        WorkloadUID: workloadUID,
        TaskType:    constant.TaskTypeActiveDetection,
        Status:      constant.TaskStatusPending,
        Ext: model.ExtType{
            // Detection configuration
            "max_attempts":      5,     // Maximum detection attempts
            "attempt_count":     0,
            "retry_interval":    30,    // Seconds between retries
            "timeout":           60,    // Per-attempt timeout
            
            // Evidence sources to probe
            "probe_process":     true,  // Probe process info
            "probe_env":         true,  // Probe environment variables
            "probe_image":       true,  // Check container image
            "probe_labels":      true,  // Check pod labels/annotations
            
            // Task metadata
            "created_by":   "workload_discovery",
            "created_at":   time.Now().Format(time.RFC3339),
            "triggered_by": "active_detection",
        },
    }

    if err := tc.taskFacade.UpsertTask(ctx, task); err != nil {
        return fmt.Errorf("failed to create active detection task: %w", err)
    }

    log.Infof("Active detection task created for workload %s", workloadUID)
    return nil
}
```

### 3. Hook into Workload Discovery

The active detection task should be created when a new GPU workload is discovered. This can be done by:

**Option A: Listen to workload creation events**

```go
// In bootstrap or initialization code
func RegisterWorkloadListener(workloadFacade database.WorkloadFacadeInterface, taskCreator *TaskCreator) {
    workloadFacade.RegisterListener(func(event WorkloadEvent) {
        if event.Type == WorkloadEventCreated {
            ctx := context.Background()
            if err := taskCreator.CreateActiveDetectionTask(ctx, event.WorkloadUID); err != nil {
                log.Warnf("Failed to create active detection task: %v", err)
            }
        }
    })
}
```

**Option B: Periodic scan for workloads without detection**

```go
// Run periodically to find workloads that need active detection
func (tc *TaskCreator) ScanForUndetectedWorkloads(ctx context.Context) error {
    // Query workloads that:
    // 1. Have no detection result yet
    // 2. Don't have a pending/running active_detection task
    
    workloads, err := tc.findUndetectedWorkloads(ctx)
    if err != nil {
        return err
    }
    
    for _, workload := range workloads {
        if err := tc.CreateActiveDetectionTask(ctx, workload.UID); err != nil {
            log.Warnf("Failed to create active detection task for %s: %v", workload.UID, err)
        }
    }
    
    return nil
}
```

### 4. Implement ActiveDetectionExecutor

**File: `ai-advisor/pkg/task/active_detection_executor.go`**

```go
package task

import (
    "context"
    "fmt"
    "strings"
    "time"

    "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/detection"
    "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
    coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
    coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// ActiveDetectionExecutor proactively detects framework for workloads
type ActiveDetectionExecutor struct {
    coreTask.BaseExecutor

    collector        *metadata.Collector
    detectionManager *framework.FrameworkDetectionManager
    podFacade        database.PodFacadeInterface
    workloadFacade   database.WorkloadFacadeInterface
    taskFacade       database.WorkloadTaskFacadeInterface
}

// NewActiveDetectionExecutor creates new executor
func NewActiveDetectionExecutor(
    collector *metadata.Collector,
    detectionManager *framework.FrameworkDetectionManager,
) *ActiveDetectionExecutor {
    return &ActiveDetectionExecutor{
        collector:        collector,
        detectionManager: detectionManager,
        podFacade:        database.NewPodFacade(),
        workloadFacade:   database.GetFacade().GetWorkload(),
        taskFacade:       database.NewWorkloadTaskFacade(),
    }
}

// GetTaskType returns task type
func (e *ActiveDetectionExecutor) GetTaskType() string {
    return constant.TaskTypeActiveDetection
}

// Validate validates task parameters
func (e *ActiveDetectionExecutor) Validate(task *model.WorkloadTaskState) error {
    if task.WorkloadUID == "" {
        return fmt.Errorf("workload_uid is required")
    }
    return nil
}

// Execute performs active framework detection
func (e *ActiveDetectionExecutor) Execute(
    ctx context.Context,
    execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
    task := execCtx.Task
    workloadUID := task.WorkloadUID

    log.Infof("Starting active detection for workload %s", workloadUID)

    // Get attempt count
    attemptCount := e.GetExtInt(task, "attempt_count")
    maxAttempts := e.GetExtInt(task, "max_attempts")
    if maxAttempts == 0 {
        maxAttempts = 5
    }

    attemptCount++
    updates := map[string]interface{}{
        "attempt_count": attemptCount,
        "last_attempt":  time.Now().Format(time.RFC3339),
    }

    // Step 1: Check if detection already confirmed (from passive sources)
    existingDetection, err := e.detectionManager.GetDetection(ctx, workloadUID)
    if err == nil && existingDetection != nil {
        if existingDetection.Status == coreModel.DetectionStatusConfirmed ||
            existingDetection.Status == coreModel.DetectionStatusVerified {
            log.Infof("Workload %s already has confirmed detection, completing task", workloadUID)
            updates["result"] = "already_detected"
            updates["existing_frameworks"] = existingDetection.Frameworks
            return coreTask.SuccessResult(updates), nil
        }
    }

    // Step 2: Get pod info for this workload
    pod, err := e.selectTargetPod(ctx, workloadUID)
    if err != nil || pod == nil {
        updates["error"] = fmt.Sprintf("failed to get pod info: %v", err)
        return e.handleRetryOrFail(ctx, task, updates, attemptCount, maxAttempts)
    }

    updates["pod_name"] = pod.Name
    updates["pod_namespace"] = pod.Namespace
    updates["node_name"] = pod.NodeName

    // Step 3: Collect evidence from multiple sources
    evidence := e.collectEvidence(ctx, task, pod)
    updates["evidence_collected"] = evidence.Sources

    // Step 4: Run detection based on collected evidence
    detectionResult := e.runDetection(ctx, workloadUID, evidence)
    
    if detectionResult == nil || detectionResult.Framework == "" {
        log.Infof("No framework detected for workload %s (attempt %d/%d)", 
            workloadUID, attemptCount, maxAttempts)
        updates["result"] = "not_detected"
        return e.handleRetryOrFail(ctx, task, updates, attemptCount, maxAttempts)
    }

    // Step 5: Report detection to manager
    err = e.detectionManager.ReportDetectionWithLayers(
        ctx,
        workloadUID,
        "active_detection",           // source
        detectionResult.Framework,    // framework
        detectionResult.Type,         // taskType (training/inference)
        detectionResult.Confidence,   // confidence
        detectionResult.Evidence,     // evidence
        detectionResult.Layer,        // frameworkLayer
        detectionResult.Wrapper,      // wrapperFramework
        detectionResult.Base,         // baseFramework
    )

    if err != nil {
        log.Errorf("Failed to report detection for workload %s: %v", workloadUID, err)
        updates["error"] = err.Error()
        return e.handleRetryOrFail(ctx, task, updates, attemptCount, maxAttempts)
    }

    log.Infof("Active detection completed for workload %s: framework=%s, confidence=%.2f",
        workloadUID, detectionResult.Framework, detectionResult.Confidence)

    updates["result"] = "detected"
    updates["framework"] = detectionResult.Framework
    updates["confidence"] = detectionResult.Confidence
    updates["detection_method"] = "active"

    return coreTask.SuccessResult(updates), nil
}

// EvidenceCollection holds collected evidence
type EvidenceCollection struct {
    Sources     []string               // List of evidence sources used
    ProcessInfo *ProcessEvidence       // From process probing
    ImageInfo   *ImageEvidence         // From container image
    LabelInfo   *LabelEvidence         // From pod labels
    EnvInfo     *EnvEvidence           // From environment variables
    Existing    *ExistingEvidence      // From existing detection data
}

type ProcessEvidence struct {
    Cmdlines     []string
    ProcessNames []string
}

type ImageEvidence struct {
    ImageName string
    ImageTag  string
}

type LabelEvidence struct {
    Labels      map[string]string
    Annotations map[string]string
}

type EnvEvidence struct {
    EnvVars map[string]string
}

type ExistingEvidence struct {
    Sources []coreModel.DetectionSource
}

// collectEvidence collects evidence from configured sources
func (e *ActiveDetectionExecutor) collectEvidence(
    ctx context.Context,
    task *model.WorkloadTaskState,
    pod *model.GpuPods,
) *EvidenceCollection {
    evidence := &EvidenceCollection{
        Sources: []string{},
    }

    // 1. Probe process info
    if e.GetExtBool(task, "probe_process") {
        if procEvidence := e.probeProcessInfo(ctx, pod); procEvidence != nil {
            evidence.ProcessInfo = procEvidence
            evidence.Sources = append(evidence.Sources, "process")
        }
    }

    // 2. Check container image
    if e.GetExtBool(task, "probe_image") {
        if imageEvidence := e.probeImageInfo(pod); imageEvidence != nil {
            evidence.ImageInfo = imageEvidence
            evidence.Sources = append(evidence.Sources, "image")
        }
    }

    // 3. Check pod labels/annotations
    if e.GetExtBool(task, "probe_labels") {
        if labelEvidence := e.probeLabelInfo(ctx, pod); labelEvidence != nil {
            evidence.LabelInfo = labelEvidence
            evidence.Sources = append(evidence.Sources, "labels")
        }
    }

    // 4. Probe environment variables
    if e.GetExtBool(task, "probe_env") {
        if envEvidence := e.probeEnvInfo(ctx, pod); envEvidence != nil {
            evidence.EnvInfo = envEvidence
            evidence.Sources = append(evidence.Sources, "env")
        }
    }

    // 5. Get existing detection evidence (from passive sources)
    if existing := e.getExistingEvidence(ctx, task.WorkloadUID); existing != nil {
        evidence.Existing = existing
        evidence.Sources = append(evidence.Sources, "existing")
    }

    return evidence
}

// probeProcessInfo gets process info from node-exporter
func (e *ActiveDetectionExecutor) probeProcessInfo(
    ctx context.Context,
    pod *model.GpuPods,
) *ProcessEvidence {
    client, err := e.collector.GetNodeExporterClientForPod(ctx, pod.NodeName)
    if err != nil {
        log.Debugf("Failed to get node-exporter client: %v", err)
        return nil
    }

    processTree, err := client.GetPodProcessTree(ctx, &types.ProcessTreeRequest{
        PodName:        pod.Name,
        PodNamespace:   pod.Namespace,
        PodUID:         pod.UID,
        IncludeCmdline: true,
        IncludeEnv:     false,
    })

    if err != nil {
        log.Debugf("Failed to get process tree: %v", err)
        return nil
    }

    evidence := &ProcessEvidence{
        Cmdlines:     []string{},
        ProcessNames: []string{},
    }

    // Extract cmdlines from Python processes
    e.extractProcessInfo(processTree, evidence)

    return evidence
}

// probeImageInfo extracts image information
func (e *ActiveDetectionExecutor) probeImageInfo(pod *model.GpuPods) *ImageEvidence {
    if pod.Image == "" {
        return nil
    }

    evidence := &ImageEvidence{
        ImageName: pod.Image,
    }

    // Extract tag
    parts := strings.Split(pod.Image, ":")
    if len(parts) > 1 {
        evidence.ImageTag = parts[len(parts)-1]
    }

    return evidence
}

// probeLabelInfo gets pod labels and annotations
func (e *ActiveDetectionExecutor) probeLabelInfo(
    ctx context.Context,
    pod *model.GpuPods,
) *LabelEvidence {
    // Get detailed pod info from K8s API or database
    // This is simplified - actual implementation needs K8s client
    
    evidence := &LabelEvidence{
        Labels:      make(map[string]string),
        Annotations: make(map[string]string),
    }

    // Parse labels from pod metadata if stored
    // In real implementation, query K8s API or parse stored metadata

    return evidence
}

// probeEnvInfo gets environment variables from process
func (e *ActiveDetectionExecutor) probeEnvInfo(
    ctx context.Context,
    pod *model.GpuPods,
) *EnvEvidence {
    client, err := e.collector.GetNodeExporterClientForPod(ctx, pod.NodeName)
    if err != nil {
        return nil
    }

    processTree, err := client.GetPodProcessTree(ctx, &types.ProcessTreeRequest{
        PodName:      pod.Name,
        PodNamespace: pod.Namespace,
        PodUID:       pod.UID,
        IncludeEnv:   true,
    })

    if err != nil {
        return nil
    }

    evidence := &EnvEvidence{
        EnvVars: make(map[string]string),
    }

    // Extract environment variables from first Python process
    if proc := e.findFirstPythonProcess(processTree); proc != nil {
        for _, env := range proc.Env {
            parts := strings.SplitN(env, "=", 2)
            if len(parts) == 2 {
                evidence.EnvVars[parts[0]] = parts[1]
            }
        }
    }

    return evidence
}

// getExistingEvidence retrieves existing detection evidence
func (e *ActiveDetectionExecutor) getExistingEvidence(
    ctx context.Context,
    workloadUID string,
) *ExistingEvidence {
    existing, err := e.detectionManager.GetDetection(ctx, workloadUID)
    if err != nil || existing == nil {
        return nil
    }

    return &ExistingEvidence{
        Sources: existing.Sources,
    }
}

// DetectionResult holds the result of active detection
type DetectionResult struct {
    Framework  string
    Type       string  // training/inference
    Confidence float64
    Layer      string  // wrapper/base
    Wrapper    string
    Base       string
    Evidence   map[string]interface{}
}

// runDetection runs detection logic on collected evidence
func (e *ActiveDetectionExecutor) runDetection(
    ctx context.Context,
    workloadUID string,
    evidence *EvidenceCollection,
) *DetectionResult {
    result := &DetectionResult{
        Evidence: make(map[string]interface{}),
    }

    var matchedFrameworks []string
    var totalConfidence float64
    var matchCount int

    // 1. Check process patterns
    if evidence.ProcessInfo != nil {
        for fwName, matcher := range detection.GetInferencePatternMatchers() {
            // Build match context
            matchCtx := &detection.InferenceMatchContext{
                ProcessNames:    evidence.ProcessInfo.ProcessNames,
                ProcessCmdlines: evidence.ProcessInfo.Cmdlines,
            }
            if evidence.ImageInfo != nil {
                matchCtx.ImageName = evidence.ImageInfo.ImageName
            }
            if evidence.EnvInfo != nil {
                matchCtx.EnvVars = evidence.EnvInfo.EnvVars
            }

            matchResult := matcher.MatchInference(matchCtx)
            if matchResult.Matched {
                matchedFrameworks = append(matchedFrameworks, fwName)
                totalConfidence += matchResult.Confidence
                matchCount++
                result.Type = detection.FrameworkTypeInference
            }
        }

        // Check training frameworks via cmdline patterns
        for _, cmdline := range evidence.ProcessInfo.Cmdlines {
            if fw := e.detectTrainingFrameworkFromCmdline(cmdline); fw != "" {
                matchedFrameworks = append(matchedFrameworks, fw)
                totalConfidence += 0.7
                matchCount++
                result.Type = detection.FrameworkTypeTraining
            }
        }
    }

    // 2. Check environment variables
    if evidence.EnvInfo != nil {
        if fw := e.detectFrameworkFromEnv(evidence.EnvInfo.EnvVars); fw != nil {
            matchedFrameworks = append(matchedFrameworks, fw.Framework)
            totalConfidence += fw.Confidence
            matchCount++
            result.Wrapper = fw.Wrapper
            result.Base = fw.Base
            if result.Type == "" {
                result.Type = detection.FrameworkTypeTraining
            }
        }
    }

    // 3. Check container image
    if evidence.ImageInfo != nil {
        if fw := e.detectFrameworkFromImage(evidence.ImageInfo.ImageName); fw != "" {
            matchedFrameworks = append(matchedFrameworks, fw)
            totalConfidence += 0.6
            matchCount++
        }
    }

    // 4. Merge with existing evidence
    if evidence.Existing != nil && len(evidence.Existing.Sources) > 0 {
        for _, src := range evidence.Existing.Sources {
            if len(src.Frameworks) > 0 {
                matchedFrameworks = append(matchedFrameworks, src.Frameworks...)
                totalConfidence += src.Confidence
                matchCount++
            }
        }
    }

    if matchCount == 0 {
        return nil
    }

    // Calculate average confidence
    result.Confidence = totalConfidence / float64(matchCount)

    // Select primary framework (prioritize wrapper frameworks)
    result.Framework = e.selectPrimaryFramework(matchedFrameworks)
    result.Layer = e.determineFrameworkLayer(result.Framework)

    // Build evidence map
    result.Evidence["sources"] = evidence.Sources
    result.Evidence["matched_frameworks"] = matchedFrameworks
    result.Evidence["detection_method"] = "active_probing"

    return result
}

// handleRetryOrFail decides whether to retry or fail the task
func (e *ActiveDetectionExecutor) handleRetryOrFail(
    ctx context.Context,
    task *model.WorkloadTaskState,
    updates map[string]interface{},
    attemptCount, maxAttempts int,
) (*coreTask.ExecutionResult, error) {
    if attemptCount >= maxAttempts {
        log.Infof("Active detection for workload %s reached max attempts (%d), marking as completed",
            task.WorkloadUID, maxAttempts)
        updates["final_result"] = "detection_not_confirmed"
        return coreTask.SuccessResult(updates), nil
    }

    // Reschedule task
    retryInterval := e.GetExtInt(task, "retry_interval")
    if retryInterval == 0 {
        retryInterval = 30
    }

    updates["next_attempt_at"] = time.Now().Add(time.Duration(retryInterval) * time.Second).Format(time.RFC3339)
    
    // Update task and reset to pending
    e.taskFacade.UpdateTaskExt(ctx, task.WorkloadUID, task.TaskType, updates)
    
    // Task will be re-picked by scheduler after retry interval
    return coreTask.ProgressResult(updates), nil
}

// Helper methods (simplified implementations)

func (e *ActiveDetectionExecutor) selectTargetPod(ctx context.Context, workloadUID string) (*model.GpuPods, error) {
    // Similar to MetadataCollectionExecutor.selectTargetPod
    // Implementation omitted for brevity
    return nil, nil
}

func (e *ActiveDetectionExecutor) detectTrainingFrameworkFromCmdline(cmdline string) string {
    cmdlineLower := strings.ToLower(cmdline)
    
    patterns := map[string][]string{
        "primus":    {"primus", "primus-train"},
        "megatron":  {"megatron", "pretrain_gpt"},
        "deepspeed": {"deepspeed", "ds_config"},
        "pytorch":   {"torch.distributed", "torchrun"},
    }

    for fw, keywords := range patterns {
        for _, kw := range keywords {
            if strings.Contains(cmdlineLower, kw) {
                return fw
            }
        }
    }
    return ""
}

type FrameworkFromEnv struct {
    Framework  string
    Confidence float64
    Wrapper    string
    Base       string
}

func (e *ActiveDetectionExecutor) detectFrameworkFromEnv(envVars map[string]string) *FrameworkFromEnv {
    // Check for specific framework environment variables
    if _, ok := envVars["PRIMUS_CONFIG"]; ok {
        return &FrameworkFromEnv{Framework: "primus", Confidence: 0.8, Wrapper: "primus"}
    }
    if _, ok := envVars["DEEPSPEED_CONFIG"]; ok {
        return &FrameworkFromEnv{Framework: "deepspeed", Confidence: 0.8, Base: "deepspeed"}
    }
    if backend, ok := envVars["PRIMUS_BACKEND"]; ok {
        return &FrameworkFromEnv{Framework: "primus", Confidence: 0.8, Wrapper: "primus", Base: backend}
    }
    return nil
}

func (e *ActiveDetectionExecutor) detectFrameworkFromImage(imageName string) string {
    imageLower := strings.ToLower(imageName)
    
    patterns := map[string][]string{
        "vllm":   {"vllm"},
        "triton": {"triton", "tritonserver"},
        "tgi":    {"text-generation-inference", "tgi"},
    }

    for fw, keywords := range patterns {
        for _, kw := range keywords {
            if strings.Contains(imageLower, kw) {
                return fw
            }
        }
    }
    return ""
}

func (e *ActiveDetectionExecutor) selectPrimaryFramework(frameworks []string) string {
    // Priority: wrapper > base
    priority := []string{"primus", "lightning", "megatron", "deepspeed", "vllm", "triton", "pytorch"}
    
    for _, p := range priority {
        for _, fw := range frameworks {
            if strings.ToLower(fw) == p {
                return fw
            }
        }
    }
    
    if len(frameworks) > 0 {
        return frameworks[0]
    }
    return ""
}

func (e *ActiveDetectionExecutor) determineFrameworkLayer(framework string) string {
    wrappers := map[string]bool{"primus": true, "lightning": true}
    if wrappers[strings.ToLower(framework)] {
        return "wrapper"
    }
    return "base"
}

// Cancel cancels the task
func (e *ActiveDetectionExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
    log.Infof("Active detection task cancelled for workload %s", task.WorkloadUID)
    return nil
}
```

### 5. Register Executor

**File: `ai-advisor/pkg/bootstrap/bootstrap.go`**

```go
func InitializeTaskExecutors(
    scheduler *coreTask.TaskScheduler,
    collector *metadata.Collector,
    detectionManager *framework.FrameworkDetectionManager,
) error {
    // Register existing executors
    scheduler.RegisterExecutor(task.NewMetadataCollectionExecutor(collector))
    scheduler.RegisterExecutor(task.NewTensorBoardStreamExecutor())
    scheduler.RegisterExecutor(task.NewProfilerCollectionExecutor())

    // Register new active detection executor
    scheduler.RegisterExecutor(task.NewActiveDetectionExecutor(collector, detectionManager))

    return nil
}
```

## Detection Flow Summary

```
┌──────────────────────────────────────────────────────────────────────────┐
│                        Active Detection Flow                              │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  1. Workload Created                                                      │
│         │                                                                 │
│         ▼                                                                 │
│  2. TaskCreator.CreateActiveDetectionTask()                               │
│         │                                                                 │
│         ▼                                                                 │
│  3. Task Scheduler picks up task                                          │
│         │                                                                 │
│         ▼                                                                 │
│  4. ActiveDetectionExecutor.Execute()                                     │
│         │                                                                 │
│         ├─── 4a. Check existing detection (from passive sources)         │
│         │          └─── If confirmed → Complete task                      │
│         │                                                                 │
│         ├─── 4b. Collect evidence                                         │
│         │          ├── Process info (cmdline, process names)              │
│         │          ├── Container image                                    │
│         │          ├── Pod labels/annotations                             │
│         │          ├── Environment variables                              │
│         │          └── Existing detection evidence                        │
│         │                                                                 │
│         ├─── 4c. Run pattern matching                                     │
│         │          ├── Inference framework patterns                       │
│         │          ├── Training framework patterns                        │
│         │          └── Environment variable patterns                      │
│         │                                                                 │
│         └─── 4d. Report detection result                                  │
│                    │                                                      │
│                    ├── If detected → Report to DetectionManager           │
│                    │                 → Complete task                      │
│                    │                                                      │
│                    └── If not detected                                    │
│                         ├── If attempts < max → Reschedule                │
│                         └── If attempts >= max → Complete (not detected)  │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

## Periodic Evidence Aggregation

When the active detection task is running but the framework is not yet confirmed, the system will **periodically aggregate all evidence** from `workload_detection_evidence` table to compute the final detection result. This ensures that evidence from passive sources (WandB, logs, etc.) accumulated during the detection window is properly utilized.

### Aggregation Flow

```
┌──────────────────────────────────────────────────────────────────────────┐
│                   Periodic Evidence Aggregation Flow                      │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│   Active Detection Task Running (detection_state = in_progress)           │
│                    │                                                      │
│                    ▼                                                      │
│   ┌────────────────────────────────────────────────────────────────────┐ │
│   │                    On Each Attempt                                  │ │
│   │                                                                     │ │
│   │   1. Query ALL unprocessed evidence from evidence table             │ │
│   │      SELECT * FROM workload_detection_evidence                      │ │
│   │      WHERE workload_uid = ? AND processed = false                   │ │
│   │                                                                     │ │
│   │   2. Aggregate evidence from multiple sources:                      │ │
│   │      ┌─────────────────────────────────────────────────────────┐   │ │
│   │      │ Evidence Sources (accumulated over time)                 │   │ │
│   │      │                                                          │   │ │
│   │      │  [t=0]  process probe    → primus (0.7)                  │   │ │
│   │      │  [t=5]  wandb report     → primus + megatron (0.9)       │   │ │
│   │      │  [t=10] env probe        → primus (0.8)                  │   │ │
│   │      │  [t=15] log pattern      → megatron (0.6)                │   │ │
│   │      │  [t=20] image detection  → pytorch (0.5)                 │   │ │
│   │      └─────────────────────────────────────────────────────────┘   │ │
│   │                                                                     │ │
│   │   3. Run multi-source fusion algorithm:                             │ │
│   │      - Weight by source priority (wandb > process > env > image)   │ │
│   │      - Weight by confidence                                         │ │
│   │      - Detect conflicts between sources                             │ │
│   │      - Calculate aggregated confidence                              │ │
│   │                                                                     │ │
│   │   4. Check if threshold reached:                                    │ │
│   │      ┌──────────────────────────────────────────────────────────┐  │ │
│   │      │ Confidence >= 0.8 → status = verified, COMPLETE task     │  │ │
│   │      │ Confidence >= 0.6 → status = confirmed, COMPLETE task    │  │ │
│   │      │ Confidence >= 0.4 → status = suspected, continue retry   │  │ │
│   │      │ Confidence <  0.4 → status = unknown, continue retry     │  │ │
│   │      └──────────────────────────────────────────────────────────┘  │ │
│   │                                                                     │ │
│   │   5. Mark processed evidence                                        │ │
│   │      UPDATE workload_detection_evidence SET processed = true        │ │
│   │      WHERE id IN (...)                                              │ │
│   │                                                                     │ │
│   └────────────────────────────────────────────────────────────────────┘ │
│                    │                                                      │
│                    ├── If confirmed/verified → Complete task              │
│                    │                                                      │
│                    └── If not confirmed                                   │
│                         │                                                 │
│                         ▼                                                 │
│                    Wait retry_interval, then repeat                       │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

### Evidence Aggregator Implementation

```go
// EvidenceAggregator aggregates evidence from multiple sources
type EvidenceAggregator struct {
    evidenceFacade WorkloadDetectionEvidenceFacadeInterface
    detectionFacade WorkloadDetectionFacadeInterface
}

// AggregationResult holds the result of evidence aggregation
type AggregationResult struct {
    Framework        string
    Frameworks       []string
    WorkloadType     string
    Confidence       float64
    Status           DetectionStatus
    FrameworkLayer   string
    WrapperFramework string
    BaseFramework    string
    EvidenceCount    int
    Sources          []string
    Conflicts        []DetectionConflict
}

// AggregateEvidence aggregates all evidence for a workload
func (a *EvidenceAggregator) AggregateEvidence(
    ctx context.Context,
    workloadUID string,
) (*AggregationResult, error) {
    // 1. Query all unprocessed evidence
    evidences, err := a.evidenceFacade.ListUnprocessedEvidence(ctx, workloadUID)
    if err != nil {
        return nil, fmt.Errorf("failed to list evidence: %w", err)
    }

    if len(evidences) == 0 {
        // No new evidence, return current state
        return a.getCurrentState(ctx, workloadUID)
    }

    // 2. Group evidence by framework
    frameworkVotes := make(map[string]*FrameworkVote)
    var sources []string
    var conflicts []DetectionConflict

    for _, ev := range evidences {
        sources = append(sources, ev.Source)
        
        weight := a.getSourceWeight(ev.Source)
        score := ev.Confidence * weight

        if _, exists := frameworkVotes[ev.Framework]; !exists {
            frameworkVotes[ev.Framework] = &FrameworkVote{
                Framework:        ev.Framework,
                TotalScore:       0,
                VoteCount:        0,
                HighestConfidence: 0,
                Sources:          []string{},
            }
        }

        vote := frameworkVotes[ev.Framework]
        vote.TotalScore += score
        vote.VoteCount++
        vote.Sources = append(vote.Sources, ev.Source)
        if ev.Confidence > vote.HighestConfidence {
            vote.HighestConfidence = ev.Confidence
            vote.WrapperFramework = ev.WrapperFramework
            vote.BaseFramework = ev.BaseFramework
            vote.FrameworkLayer = ev.FrameworkLayer
            vote.WorkloadType = ev.WorkloadType
        }
    }

    // 3. Detect conflicts (multiple frameworks with high confidence)
    conflicts = a.detectConflicts(frameworkVotes)

    // 4. Select winning framework
    var winner *FrameworkVote
    for _, vote := range frameworkVotes {
        if winner == nil || vote.TotalScore > winner.TotalScore {
            winner = vote
        }
    }

    if winner == nil {
        return nil, nil
    }

    // 5. Calculate aggregated confidence
    aggregatedConfidence := a.calculateConfidence(winner, len(evidences))

    // 6. Determine status based on confidence
    status := a.determineStatus(aggregatedConfidence, len(sources), conflicts)

    result := &AggregationResult{
        Framework:        winner.Framework,
        Frameworks:       a.buildFrameworkList(winner),
        WorkloadType:     winner.WorkloadType,
        Confidence:       aggregatedConfidence,
        Status:           status,
        FrameworkLayer:   winner.FrameworkLayer,
        WrapperFramework: winner.WrapperFramework,
        BaseFramework:    winner.BaseFramework,
        EvidenceCount:    len(evidences),
        Sources:          a.uniqueSources(sources),
        Conflicts:        conflicts,
    }

    // 7. Mark evidence as processed
    evidenceIDs := make([]int64, len(evidences))
    for i, ev := range evidences {
        evidenceIDs[i] = ev.ID
    }
    a.evidenceFacade.MarkEvidenceProcessed(ctx, evidenceIDs)

    // 8. Update detection state
    a.updateDetectionState(ctx, workloadUID, result)

    return result, nil
}

// Source weight configuration
var sourceWeights = map[string]float64{
    "wandb":            1.0,   // Highest priority - explicit framework info
    "import_detection": 0.95,  // Import detection from wandb
    "process":          0.85,  // Process cmdline analysis
    "env":              0.80,  // Environment variables
    "log":              0.75,  // Log pattern matching
    "active_detection": 0.70,  // Active probing results
    "image":            0.60,  // Container image name
    "label":            0.50,  // Pod labels
    "default":          0.30,  // Default/fallback
}

func (a *EvidenceAggregator) getSourceWeight(source string) float64 {
    if weight, ok := sourceWeights[source]; ok {
        return weight
    }
    return sourceWeights["default"]
}

// determineStatus determines detection status based on aggregated results
func (a *EvidenceAggregator) determineStatus(
    confidence float64,
    sourceCount int,
    conflicts []DetectionConflict,
) DetectionStatus {
    // If there are unresolved conflicts
    if len(conflicts) > 0 {
        return DetectionStatusConflict
    }

    // Based on confidence thresholds
    switch {
    case confidence >= 0.8:
        return DetectionStatusVerified
    case confidence >= 0.6:
        return DetectionStatusConfirmed
    case confidence >= 0.4:
        return DetectionStatusSuspected
    default:
        return DetectionStatusUnknown
    }
}

// calculateConfidence calculates aggregated confidence
func (a *EvidenceAggregator) calculateConfidence(
    winner *FrameworkVote,
    totalEvidenceCount int,
) float64 {
    if winner == nil || winner.VoteCount == 0 {
        return 0.0
    }

    // Base confidence from highest single source
    baseConfidence := winner.HighestConfidence

    // Boost for multiple sources agreeing (multi-source bonus)
    sourceBonus := math.Min(0.15, float64(winner.VoteCount-1)*0.05)

    // Calculate final confidence (capped at 1.0)
    finalConfidence := math.Min(1.0, baseConfidence+sourceBonus)

    return finalConfidence
}
```

### Updated ActiveDetectionExecutor with Aggregation

The executor now uses the aggregator in each attempt:

```go
// Execute performs active framework detection with evidence aggregation
func (e *ActiveDetectionExecutor) Execute(
    ctx context.Context,
    execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
    task := execCtx.Task
    workloadUID := task.WorkloadUID

    log.Infof("Starting active detection attempt for workload %s", workloadUID)

    attemptCount := e.GetExtInt(task, "attempt_count") + 1
    maxAttempts := e.GetExtInt(task, "max_attempts")
    if maxAttempts == 0 {
        maxAttempts = 5
    }

    updates := map[string]interface{}{
        "attempt_count": attemptCount,
        "last_attempt":  time.Now().Format(time.RFC3339),
    }

    // Step 1: Aggregate ALL existing evidence (from passive + previous active attempts)
    aggregationResult, err := e.aggregator.AggregateEvidence(ctx, workloadUID)
    if err != nil {
        log.Warnf("Evidence aggregation failed: %v", err)
    }

    // Step 2: Check if aggregated result meets threshold
    if aggregationResult != nil {
        updates["aggregation"] = map[string]interface{}{
            "evidence_count": aggregationResult.EvidenceCount,
            "sources":        aggregationResult.Sources,
            "confidence":     aggregationResult.Confidence,
            "status":         string(aggregationResult.Status),
        }

        if aggregationResult.Status == DetectionStatusConfirmed ||
            aggregationResult.Status == DetectionStatusVerified {
            log.Infof("Detection confirmed via evidence aggregation: framework=%s, confidence=%.2f, sources=%v",
                aggregationResult.Framework, aggregationResult.Confidence, aggregationResult.Sources)
            
            updates["result"] = "confirmed_via_aggregation"
            updates["framework"] = aggregationResult.Framework
            updates["confidence"] = aggregationResult.Confidence
            return coreTask.SuccessResult(updates), nil
        }
    }

    // Step 3: Perform active probing to collect new evidence
    pod, err := e.selectTargetPod(ctx, workloadUID)
    if err != nil || pod == nil {
        updates["probe_error"] = fmt.Sprintf("failed to get pod: %v", err)
        return e.handleRetryOrFail(ctx, task, updates, attemptCount, maxAttempts)
    }

    evidence := e.collectEvidence(ctx, task, pod)
    
    // Step 4: Store collected evidence to evidence table
    if err := e.storeCollectedEvidence(ctx, workloadUID, evidence); err != nil {
        log.Warnf("Failed to store evidence: %v", err)
    }

    // Step 5: Re-aggregate with new evidence
    aggregationResult, err = e.aggregator.AggregateEvidence(ctx, workloadUID)
    if err != nil {
        log.Warnf("Re-aggregation failed: %v", err)
    }

    if aggregationResult != nil &&
        (aggregationResult.Status == DetectionStatusConfirmed ||
            aggregationResult.Status == DetectionStatusVerified) {
        log.Infof("Detection confirmed after new evidence: framework=%s, confidence=%.2f",
            aggregationResult.Framework, aggregationResult.Confidence)
        
        updates["result"] = "confirmed_after_probe"
        updates["framework"] = aggregationResult.Framework
        updates["confidence"] = aggregationResult.Confidence
        return coreTask.SuccessResult(updates), nil
    }

    // Step 6: Not confirmed yet, schedule retry or complete
    log.Infof("Framework not confirmed for workload %s (attempt %d/%d, confidence=%.2f)",
        workloadUID, attemptCount, maxAttempts,
        func() float64 {
            if aggregationResult != nil {
                return aggregationResult.Confidence
            }
            return 0
        }())

    return e.handleRetryOrFail(ctx, task, updates, attemptCount, maxAttempts)
}

// storeCollectedEvidence stores evidence from active probing
func (e *ActiveDetectionExecutor) storeCollectedEvidence(
    ctx context.Context,
    workloadUID string,
    evidence *EvidenceCollection,
) error {
    var evidenceRecords []*model.WorkloadDetectionEvidence

    // Store process evidence
    if evidence.ProcessInfo != nil {
        for _, cmdline := range evidence.ProcessInfo.Cmdlines {
            if fw := e.detectTrainingFrameworkFromCmdline(cmdline); fw != "" {
                evidenceRecords = append(evidenceRecords, &model.WorkloadDetectionEvidence{
                    WorkloadUID:  workloadUID,
                    Source:       "process",
                    SourceType:   "active",
                    Framework:    fw,
                    WorkloadType: "training",
                    Confidence:   0.7,
                    Evidence: model.ExtType{
                        "cmdline": cmdline,
                        "method":  "cmdline_pattern",
                    },
                    DetectedAt: time.Now(),
                })
            }
        }
    }

    // Store env evidence
    if evidence.EnvInfo != nil {
        if fw := e.detectFrameworkFromEnv(evidence.EnvInfo.EnvVars); fw != nil {
            evidenceRecords = append(evidenceRecords, &model.WorkloadDetectionEvidence{
                WorkloadUID:      workloadUID,
                Source:           "env",
                SourceType:       "active",
                Framework:        fw.Framework,
                WrapperFramework: fw.Wrapper,
                BaseFramework:    fw.Base,
                WorkloadType:     "training",
                Confidence:       fw.Confidence,
                Evidence: model.ExtType{
                    "matched_vars": e.getMatchedEnvVars(evidence.EnvInfo.EnvVars),
                    "method":       "env_pattern",
                },
                DetectedAt: time.Now(),
            })
        }
    }

    // Store image evidence
    if evidence.ImageInfo != nil {
        if fw := e.detectFrameworkFromImage(evidence.ImageInfo.ImageName); fw != "" {
            evidenceRecords = append(evidenceRecords, &model.WorkloadDetectionEvidence{
                WorkloadUID:  workloadUID,
                Source:       "image",
                SourceType:   "active",
                Framework:    fw,
                WorkloadType: "inference",
                Confidence:   0.6,
                Evidence: model.ExtType{
                    "image_name": evidence.ImageInfo.ImageName,
                    "method":     "image_pattern",
                },
                DetectedAt: time.Now(),
            })
        }
    }

    // Batch insert evidence
    for _, record := range evidenceRecords {
        if err := e.evidenceFacade.CreateEvidence(ctx, record); err != nil {
            log.Warnf("Failed to store evidence: %v", err)
        }
    }

    return nil
}
```

### Aggregation Timing

| Scenario | Aggregation Trigger |
|----------|---------------------|
| Active detection attempt | Before and after each probe |
| Passive evidence arrives | Evidence stored, aggregated on next active attempt |
| Detection task retry | Full re-aggregation of all evidence |
| Manual trigger | API call to force re-aggregation |

### Confidence Calculation with Multi-Source Bonus

```
Final Confidence = Base Confidence + Multi-Source Bonus

Where:
  Base Confidence = Highest confidence from any single source
  Multi-Source Bonus = min(0.15, (source_count - 1) * 0.05)

Examples:
  - Single source (wandb, 0.85) → 0.85 + 0 = 0.85
  - Two sources (wandb 0.85, process 0.70) → 0.85 + 0.05 = 0.90
  - Three sources (wandb 0.85, process 0.70, env 0.80) → 0.85 + 0.10 = 0.95
  - Four+ sources → 0.85 + 0.15 = 1.00 (capped)
```

## Configuration

### Task Configuration Options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `attempt_count` | int | 0 | Current attempt count |
| `retry_interval` | int | 10 | Base interval in seconds (exponential backoff applied) |
| `timeout` | int | 60 | Per-attempt timeout in seconds |
| `probe_process` | bool | true | Whether to probe process info |
| `probe_env` | bool | true | Whether to probe env vars |
| `probe_image` | bool | true | Whether to check container image |
| `probe_labels` | bool | true | Whether to check pod labels |

**Note:** No `max_attempts` limit - the task runs continuously until:
1. Detection is confirmed (confidence threshold reached)
2. Workload terminates (no running pods)

**Exponential Backoff:**
- Base interval: 10 seconds
- Formula: `baseInterval * 2^(attemptCount-1)`
- Maximum interval: 60 seconds (1 minute)

## Benefits

1. **No detection blind spots**: Every workload gets active detection attempt
2. **Complementary to passive detection**: Merges with evidence from WandB, logs, etc.
3. **Configurable retry**: Workloads that aren't immediately detectable can be retried
4. **Evidence accumulation**: Multiple evidence sources increase confidence
5. **Task-based scheduling**: Leverages existing task scheduler infrastructure

## Migration Path

1. **Phase 1**: Deploy with passive detection + active detection running in parallel
2. **Phase 2**: Monitor detection coverage and confidence improvements
3. **Phase 3**: Tune retry intervals and max attempts based on production data

---

## Database Schema Refactoring

As part of this design, we propose refactoring the database schema to better separate concerns and support the active detection workflow.

### Current Schema Problems

The current `ai_workload_metadata` table mixes multiple concerns:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Current: ai_workload_metadata                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  • Detection info (framework, type, confidence)                  │  ← Detection result
│  • Detection sources (in metadata JSONB)                        │  ← Evidence
│  • Workload metadata (cmdline, env, config)                     │  ← Raw metadata
│  • Framework config (tensorboard, profiler paths)               │  ← Collected data
│                                                                  │
│  Problem: Mixed responsibilities, hard to query, hard to extend │
└─────────────────────────────────────────────────────────────────┘
```

### Proposed Schema: Three Tables with Clear Responsibilities

```
┌─────────────────────────────────────────────────────────────────┐
│                        New Table Design                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │  1. workload_detection_evidence (Evidence Store)             ││
│  │                                                              ││
│  │  Purpose: Store ALL detection evidence from ALL sources      ││
│  │  Each evidence is a separate row, enabling:                  ││
│  │    - Query by source, confidence, time                       ││
│  │    - Track evidence accumulation over time                   ││
│  │    - Audit trail for detection decisions                     ││
│  └─────────────────────────────────────────────────────────────┘│
│                              │                                   │
│                              ▼                                   │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │  2. workload_detection (Detection State)                     ││
│  │                                                              ││
│  │  Purpose: Track detection state for active detection         ││
│  │    - Current detection status                                ││
│  │    - Aggregated result from evidence                         ││
│  │    - Detection task context                                  ││
│  └─────────────────────────────────────────────────────────────┘│
│                              │                                   │
│                              ▼                                   │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │  3. ai_workload_metadata (Pure Metadata)                     ││
│  │                                                              ││
│  │  Purpose: Store raw workload metadata (no detection logic)   ││
│  │    - Process cmdline, env vars                               ││
│  │    - WandB config, hyperparameters                           ││
│  │    - Framework config paths                                  ││
│  │    - TensorBoard info                                        ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Table 1: `workload_detection_evidence`

Stores all detection evidence from all sources (passive and active).

```sql
CREATE TABLE workload_detection_evidence (
    id              SERIAL PRIMARY KEY,
    workload_uid    VARCHAR(255) NOT NULL,
    
    -- Evidence source
    source          VARCHAR(100) NOT NULL,  -- 'wandb', 'process', 'env', 'image', 'log', 'active_detection', etc.
    source_type     VARCHAR(50),            -- 'passive' or 'active'
    
    -- Detection result from this evidence
    framework       VARCHAR(100),           -- Primary detected framework
    frameworks      JSONB,                  -- All detected frameworks ["primus", "megatron"]
    workload_type   VARCHAR(50),            -- 'training' or 'inference'
    confidence      FLOAT NOT NULL,         -- Confidence score [0.0-1.0]
    
    -- Dual-layer framework info
    framework_layer   VARCHAR(20),          -- 'wrapper' or 'base'
    wrapper_framework VARCHAR(100),
    base_framework    VARCHAR(100),
    
    -- Raw evidence data
    evidence        JSONB NOT NULL,         -- Source-specific evidence details
    
    -- Metadata
    detected_at     TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMP,              -- Optional: evidence expiration
    processed       BOOLEAN DEFAULT FALSE,  -- Whether this evidence has been processed
    
    -- Indexes
    INDEX idx_evidence_workload (workload_uid),
    INDEX idx_evidence_source (source),
    INDEX idx_evidence_framework (framework),
    INDEX idx_evidence_confidence (confidence DESC),
    INDEX idx_evidence_detected_at (detected_at DESC)
);

-- Comments
COMMENT ON TABLE workload_detection_evidence IS 'Stores all detection evidence from passive and active sources';
COMMENT ON COLUMN workload_detection_evidence.source IS 'Evidence source: wandb, process, env, image, log, label, active_detection';
COMMENT ON COLUMN workload_detection_evidence.evidence IS 'Raw evidence data specific to the source type';
```

**Go Model:**

```go
package model

import "time"

const TableNameWorkloadDetectionEvidence = "workload_detection_evidence"

// WorkloadDetectionEvidence stores detection evidence from various sources
type WorkloadDetectionEvidence struct {
    ID              int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
    WorkloadUID     string    `gorm:"column:workload_uid;not null;index" json:"workload_uid"`
    
    // Evidence source
    Source          string    `gorm:"column:source;not null" json:"source"`
    SourceType      string    `gorm:"column:source_type" json:"source_type"` // passive, active
    
    // Detection result
    Framework       string    `gorm:"column:framework" json:"framework"`
    Frameworks      ExtType   `gorm:"column:frameworks;type:jsonb" json:"frameworks"`
    WorkloadType    string    `gorm:"column:workload_type" json:"workload_type"`
    Confidence      float64   `gorm:"column:confidence;not null" json:"confidence"`
    
    // Dual-layer
    FrameworkLayer   string   `gorm:"column:framework_layer" json:"framework_layer,omitempty"`
    WrapperFramework string   `gorm:"column:wrapper_framework" json:"wrapper_framework,omitempty"`
    BaseFramework    string   `gorm:"column:base_framework" json:"base_framework,omitempty"`
    
    // Raw evidence
    Evidence        ExtType   `gorm:"column:evidence;type:jsonb;not null" json:"evidence"`
    
    // Metadata
    DetectedAt      time.Time `gorm:"column:detected_at;not null;default:now()" json:"detected_at"`
    ExpiresAt       *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
    Processed       bool      `gorm:"column:processed;default:false" json:"processed"`
}

func (*WorkloadDetectionEvidence) TableName() string {
    return TableNameWorkloadDetectionEvidence
}
```

### Table 2: `workload_detection`

Tracks detection state and aggregated results for active detection.

```sql
CREATE TABLE workload_detection (
    id              SERIAL PRIMARY KEY,
    workload_uid    VARCHAR(255) NOT NULL UNIQUE,
    
    -- Aggregated detection result
    status          VARCHAR(50) NOT NULL DEFAULT 'unknown',  -- unknown, suspected, confirmed, verified, conflict
    framework       VARCHAR(100),                            -- Primary framework
    frameworks      JSONB,                                   -- All frameworks ["primus", "megatron"]
    workload_type   VARCHAR(50),                             -- training, inference
    confidence      FLOAT DEFAULT 0.0,                       -- Aggregated confidence
    
    -- Dual-layer
    framework_layer   VARCHAR(20),
    wrapper_framework VARCHAR(100),
    base_framework    VARCHAR(100),
    
    -- Detection task state
    detection_state   VARCHAR(50) DEFAULT 'pending',  -- pending, in_progress, completed, failed
    attempt_count     INT DEFAULT 0,
    max_attempts      INT DEFAULT 5,
    last_attempt_at   TIMESTAMP,
    next_attempt_at   TIMESTAMP,
    
    -- Context and configuration
    context         JSONB,                             -- Detection context (retry config, probe settings, etc.)
    
    -- Evidence summary
    evidence_count    INT DEFAULT 0,                   -- Number of evidence records
    evidence_sources  JSONB,                           -- List of sources that contributed
    conflicts         JSONB,                           -- Conflict records if any
    
    -- Timestamps
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    confirmed_at    TIMESTAMP,                         -- When detection was confirmed
    
    -- Indexes
    INDEX idx_detection_status (status),
    INDEX idx_detection_state (detection_state),
    INDEX idx_detection_framework (framework),
    INDEX idx_detection_next_attempt (next_attempt_at)
);

COMMENT ON TABLE workload_detection IS 'Tracks detection state and aggregated results for each workload';
COMMENT ON COLUMN workload_detection.status IS 'Detection status: unknown, suspected, confirmed, verified, conflict';
COMMENT ON COLUMN workload_detection.detection_state IS 'Active detection task state: pending, in_progress, completed, failed';
```

**Go Model:**

```go
package model

import "time"

const TableNameWorkloadDetection = "workload_detection"

// WorkloadDetection tracks detection state and aggregated results
type WorkloadDetection struct {
    ID              int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
    WorkloadUID     string    `gorm:"column:workload_uid;not null;uniqueIndex" json:"workload_uid"`
    
    // Aggregated detection result
    Status          string    `gorm:"column:status;not null;default:unknown" json:"status"`
    Framework       string    `gorm:"column:framework" json:"framework"`
    Frameworks      ExtType   `gorm:"column:frameworks;type:jsonb" json:"frameworks"`
    WorkloadType    string    `gorm:"column:workload_type" json:"workload_type"`
    Confidence      float64   `gorm:"column:confidence;default:0.0" json:"confidence"`
    
    // Dual-layer
    FrameworkLayer   string   `gorm:"column:framework_layer" json:"framework_layer,omitempty"`
    WrapperFramework string   `gorm:"column:wrapper_framework" json:"wrapper_framework,omitempty"`
    BaseFramework    string   `gorm:"column:base_framework" json:"base_framework,omitempty"`
    
    // Detection task state
    DetectionState  string     `gorm:"column:detection_state;default:pending" json:"detection_state"`
    AttemptCount    int        `gorm:"column:attempt_count;default:0" json:"attempt_count"`
    MaxAttempts     int        `gorm:"column:max_attempts;default:5" json:"max_attempts"`
    LastAttemptAt   *time.Time `gorm:"column:last_attempt_at" json:"last_attempt_at,omitempty"`
    NextAttemptAt   *time.Time `gorm:"column:next_attempt_at" json:"next_attempt_at,omitempty"`
    
    // Context
    Context         ExtType   `gorm:"column:context;type:jsonb" json:"context"`
    
    // Evidence summary
    EvidenceCount   int       `gorm:"column:evidence_count;default:0" json:"evidence_count"`
    EvidenceSources ExtType   `gorm:"column:evidence_sources;type:jsonb" json:"evidence_sources"`
    Conflicts       ExtType   `gorm:"column:conflicts;type:jsonb" json:"conflicts"`
    
    // Timestamps
    CreatedAt       time.Time  `gorm:"column:created_at;not null;default:now()" json:"created_at"`
    UpdatedAt       time.Time  `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
    ConfirmedAt     *time.Time `gorm:"column:confirmed_at" json:"confirmed_at,omitempty"`
}

func (*WorkloadDetection) TableName() string {
    return TableNameWorkloadDetection
}
```

### Table 3: `ai_workload_metadata` (Refactored)

Pure metadata storage, no detection logic.

```sql
-- Simplified ai_workload_metadata (pure metadata, no detection)
CREATE TABLE ai_workload_metadata (
    id              SERIAL PRIMARY KEY,
    workload_uid    VARCHAR(255) NOT NULL UNIQUE,
    
    -- Basic info
    pod_name        VARCHAR(255),
    pod_namespace   VARCHAR(255),
    node_name       VARCHAR(255),
    image           VARCHAR(500),
    image_prefix    VARCHAR(500),
    
    -- Raw collected metadata (JSONB for flexibility)
    metadata        JSONB NOT NULL DEFAULT '{}',
    
    -- Timestamps
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    
    INDEX idx_metadata_workload (workload_uid),
    INDEX idx_metadata_image_prefix (image_prefix)
);

COMMENT ON TABLE ai_workload_metadata IS 'Pure workload metadata storage (no detection logic)';
```

**Metadata JSONB Structure:**

```json
{
  "process": {
    "cmdline": "python -m primus.train --config exp.yaml",
    "process_name": "python",
    "cwd": "/workspace",
    "pid": 12345
  },
  "environment": {
    "PRIMUS_CONFIG": "/config/primus.yaml",
    "CUDA_VISIBLE_DEVICES": "0,1,2,3",
    "MASTER_ADDR": "10.0.0.1"
  },
  "wandb": {
    "project": "llm-training",
    "run_id": "abc123",
    "config": {
      "learning_rate": 0.001,
      "batch_size": 32
    }
  },
  "hyperparameters": {
    "learning_rate": 0.001,
    "batch_size": 32,
    "num_layers": 24
  },
  "framework_config": {
    "tensorboard_dir": "/logs/tensorboard",
    "profiler_dir": "/logs/profiler",
    "checkpoint_dir": "/checkpoints"
  },
  "tensorboard": {
    "enabled": true,
    "log_dir": "/logs/tensorboard",
    "event_files": ["/logs/tensorboard/events.out.tfevents.123"]
  },
  "collected_at": "2024-01-01T12:00:00Z",
  "collection_source": "node-exporter"
}
```

### Data Flow with New Schema

```
┌──────────────────────────────────────────────────────────────────────────┐
│                    Data Flow with New Schema                              │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │                      Evidence Collection                            │  │
│  │                                                                     │  │
│  │   Passive Sources:              Active Sources:                     │  │
│  │   ┌─────────────┐              ┌─────────────┐                     │  │
│  │   │ WandB       │              │ Process     │                     │  │
│  │   │ Log Stream  │              │ Probe       │                     │  │
│  │   │ API Report  │              │ Env Probe   │                     │  │
│  │   └──────┬──────┘              └──────┬──────┘                     │  │
│  │          │                            │                             │  │
│  │          └────────────┬───────────────┘                             │  │
│  │                       ▼                                             │  │
│  │         ┌─────────────────────────────┐                             │  │
│  │         │ workload_detection_evidence │  ← All evidence stored here │  │
│  │         └─────────────┬───────────────┘                             │  │
│  └───────────────────────┼────────────────────────────────────────────┘  │
│                          │                                                │
│  ┌───────────────────────┼────────────────────────────────────────────┐  │
│  │                       ▼            Detection Engine                 │  │
│  │         ┌─────────────────────────────┐                             │  │
│  │         │   Evidence Aggregator       │                             │  │
│  │         │   - Query unprocessed       │                             │  │
│  │         │   - Merge multi-source      │                             │  │
│  │         │   - Calculate confidence    │                             │  │
│  │         │   - Detect conflicts        │                             │  │
│  │         └─────────────┬───────────────┘                             │  │
│  │                       ▼                                             │  │
│  │         ┌─────────────────────────────┐                             │  │
│  │         │    workload_detection       │  ← Aggregated state         │  │
│  │         │    - status: confirmed      │                             │  │
│  │         │    - confidence: 0.85       │                             │  │
│  │         │    - detection_state: done  │                             │  │
│  │         └─────────────┬───────────────┘                             │  │
│  └───────────────────────┼────────────────────────────────────────────┘  │
│                          │                                                │
│  ┌───────────────────────┼────────────────────────────────────────────┐  │
│  │                       ▼            Downstream Tasks                 │  │
│  │         ┌─────────────────────────────┐                             │  │
│  │         │  On detection confirmed:    │                             │  │
│  │         │  → Create metadata task     │                             │  │
│  │         │  → Create profiler task     │                             │  │
│  │         └─────────────┬───────────────┘                             │  │
│  │                       ▼                                             │  │
│  │         ┌─────────────────────────────┐                             │  │
│  │         │   ai_workload_metadata      │  ← Pure metadata             │  │
│  │         │   - cmdline, env            │                             │  │
│  │         │   - wandb config            │                             │  │
│  │         │   - hyperparameters         │                             │  │
│  │         │   - tensorboard paths       │                             │  │
│  │         └─────────────────────────────┘                             │  │
│  └────────────────────────────────────────────────────────────────────┘  │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

### Facade Interface Updates

```go
// WorkloadDetectionEvidenceFacadeInterface for evidence operations
type WorkloadDetectionEvidenceFacadeInterface interface {
    // Store evidence
    CreateEvidence(ctx context.Context, evidence *model.WorkloadDetectionEvidence) error
    
    // Query evidence
    ListEvidenceByWorkload(ctx context.Context, workloadUID string) ([]*model.WorkloadDetectionEvidence, error)
    ListUnprocessedEvidence(ctx context.Context, workloadUID string) ([]*model.WorkloadDetectionEvidence, error)
    ListEvidenceBySource(ctx context.Context, workloadUID string, source string) ([]*model.WorkloadDetectionEvidence, error)
    
    // Mark as processed
    MarkEvidenceProcessed(ctx context.Context, evidenceIDs []int64) error
    
    // Cleanup
    DeleteExpiredEvidence(ctx context.Context) error
}

// WorkloadDetectionFacadeInterface for detection state operations
type WorkloadDetectionFacadeInterface interface {
    // Get/Create detection
    GetDetection(ctx context.Context, workloadUID string) (*model.WorkloadDetection, error)
    CreateDetection(ctx context.Context, detection *model.WorkloadDetection) error
    UpdateDetection(ctx context.Context, detection *model.WorkloadDetection) error
    UpsertDetection(ctx context.Context, detection *model.WorkloadDetection) error
    
    // Query by state
    ListPendingDetections(ctx context.Context) ([]*model.WorkloadDetection, error)
    ListDetectionsByStatus(ctx context.Context, status string) ([]*model.WorkloadDetection, error)
    ListDetectionsNeedingRetry(ctx context.Context, before time.Time) ([]*model.WorkloadDetection, error)
    
    // Update detection state
    UpdateDetectionState(ctx context.Context, workloadUID string, state string) error
    UpdateDetectionResult(ctx context.Context, workloadUID string, result *DetectionResult) error
    IncrementAttemptCount(ctx context.Context, workloadUID string) error
}
```

### Benefits of New Schema

| Aspect | Before | After |
|--------|--------|-------|
| **Evidence Storage** | Mixed in metadata JSONB | Separate table, queryable |
| **Evidence Traceability** | Hard to track sources | Each evidence is a row with source |
| **Detection State** | Implicit in task/metadata | Explicit state machine |
| **Metadata Purity** | Mixed with detection | Pure metadata only |
| **Query Performance** | Full JSONB scan | Indexed columns |
| **Extensibility** | Hard to add sources | Just add new evidence rows |
| **Conflict Detection** | Complex JSONB parsing | Simple row comparison |

### Migration Strategy

1. **Phase 1: Create New Tables**
   - Create `workload_detection_evidence` and `workload_detection` tables
   - Keep existing `ai_workload_metadata` unchanged

2. **Phase 2: Dual-Write**
   - Write to both old and new schema
   - Migrate existing data in background

3. **Phase 3: Switch Read Path**
   - Read from new tables
   - Verify data consistency

4. **Phase 4: Cleanup**
   - Remove detection-related fields from `ai_workload_metadata`
   - Drop deprecated columns

### Data Migration Scripts

#### Option 1: SQL Migration (Recommended for Production)

**File:** `core/pkg/database/migrations/patch038-migrate_detection_data.sql`

```bash
# Run migrations in order
psql -h <host> -U <user> -d <database> -f patch037-workload_detection_tables.sql
psql -h <host> -U <user> -d <database> -f patch038-migrate_detection_data.sql
```

The SQL migration script:
- Creates `workload_detection` records from existing `ai_workload_metadata`
- Creates `workload_detection_evidence` records for each detection source
- Preserves original data (does NOT delete from `ai_workload_metadata`)
- Handles WandB-specific evidence separately
- Updates evidence counts in detection records

#### Option 2: Go Migration Tool (For Complex Migrations)

**File:** `ai-advisor/scripts/migrate_detection_data.go`

```bash
# Build the migration tool
cd ai-advisor/scripts
go build -o migrate_detection_data migrate_detection_data.go

# Dry run first
./migrate_detection_data \
  -dsn "host=localhost user=postgres password=xxx dbname=primus_lens sslmode=disable" \
  -dry-run

# Execute migration
./migrate_detection_data \
  -dsn "host=localhost user=postgres password=xxx dbname=primus_lens sslmode=disable" \
  -batch-size 100
```

Features:
- Batch processing for large datasets
- Progress tracking and statistics
- Dry-run mode for testing
- Skips already-migrated records
- Detailed error reporting

#### Verification Queries

After migration, run these queries to verify:

```sql
-- Check record counts
SELECT 'ai_workload_metadata' as table_name, COUNT(*) as count FROM ai_workload_metadata
UNION ALL
SELECT 'workload_detection', COUNT(*) FROM workload_detection
UNION ALL
SELECT 'workload_detection_evidence', COUNT(*) FROM workload_detection_evidence;

-- Check detection status distribution
SELECT status, COUNT(*) as count 
FROM workload_detection 
GROUP BY status 
ORDER BY count DESC;

-- Check evidence source distribution
SELECT source, COUNT(*) as count 
FROM workload_detection_evidence 
GROUP BY source 
ORDER BY count DESC;

-- Verify all workloads with frameworks are migrated
SELECT COUNT(*) as missing_count
FROM ai_workload_metadata m
WHERE m.framework IS NOT NULL AND m.framework != ''
  AND NOT EXISTS (
    SELECT 1 FROM workload_detection wd 
    WHERE wd.workload_uid = m.workload_uid
  );
```

---

## Implementation Plan & Progress Tracking

### Overview

This section tracks the implementation progress of the Active Detection system. The implementation is divided into 4 milestones with clear deliverables and dependencies.

### Milestone Summary

| Milestone | Description | Status | Target Date |
|-----------|-------------|--------|-------------|
| M1 | Database Schema Refactoring | ✅ Completed | - |
| M2 | Evidence Storage & Aggregation | ✅ Completed | - |
| M3 | Active Detection Task | ✅ Completed | - |
| M4 | Integration & Testing | 🔲 Not Started | TBD |

**Status Legend:** 🔲 Not Started | 🔄 In Progress | ✅ Completed | ⏸️ Blocked

---

### Milestone 1: Database Schema Refactoring

**Goal:** Create new table structure to support evidence-based detection

| Task ID | Task | Owner | Status | Notes |
|---------|------|-------|--------|-------|
| M1-1 | Design `workload_detection_evidence` table schema | | ✅ | Completed |
| M1-2 | Design `workload_detection` table schema | | ✅ | Completed |
| M1-3 | Create database migration scripts | | ✅ | `patch037-workload_detection_tables.sql` |
| M1-4 | Generate GORM models using gen tool | | ✅ | Models generated |
| M1-5 | Implement `WorkloadDetectionEvidenceFacade` | | ✅ | `workload_detection_evidence_facade.go` |
| M1-6 | Implement `WorkloadDetectionFacade` | | ✅ | `workload_detection_facade.go` |
| M1-7 | Add database indexes for query optimization | | ✅ | Included in migration |
| M1-8 | Write unit tests for new facades | | 🔲 | Deferred to M4 |

**Deliverables:**
- [x] SQL migration file: `migrations/patch037-workload_detection_tables.sql`
- [x] Model files: `model/workload_detection_evidence.gen.go`, `model/workload_detection.gen.go`
- [x] Facade files: `database/workload_detection_evidence_facade.go`, `database/workload_detection_facade.go`

---

### Milestone 2: Evidence Storage & Aggregation

**Goal:** Implement evidence storage and multi-source aggregation logic

| Task ID | Task | Owner | Status | Notes |
|---------|------|-------|--------|-------|
| M2-1 | Implement `EvidenceAggregator` core logic | | ✅ | `evidence_aggregator.go` |
| M2-2 | Implement source weight configuration | | ✅ | DefaultSourceWeights in aggregator |
| M2-3 | Implement multi-source fusion algorithm | | ✅ | calculateVotes, selectWinner |
| M2-4 | Implement conflict detection logic | | ✅ | detectConflicts |
| M2-5 | Implement confidence calculation with multi-source bonus | | ✅ | calculateConfidence |
| M2-6 | Add evidence expiration cleanup job | | ✅ | `evidence_cleanup.go` |
| M2-7 | Modify existing detection sources to write to new evidence table | | 🔄 | In progress |
| M2-7a | - WandB detector writes to evidence table | | ✅ | Updated `wandb_detector.go` |
| M2-7b | - Log pattern matcher writes to evidence table | | 🔲 | Planned |
| M2-7c | - Inference detector writes to evidence table | | 🔲 | Planned |
| M2-8 | Write unit tests for aggregator | | 🔲 | Deferred to M4 |
| M2-9 | Write integration tests for multi-source fusion | | 🔲 | Deferred to M4 |

**Deliverables:**
- [x] File: `ai-advisor/pkg/detection/evidence_aggregator.go`
- [x] File: `ai-advisor/pkg/detection/evidence_store.go`
- [x] File: `ai-advisor/pkg/detection/evidence_cleanup.go`
- [ ] File: `ai-advisor/pkg/detection/evidence_aggregator_test.go`
- [x] Updated: `ai-advisor/pkg/detection/wandb_detector.go`
- [ ] Updated: `ai-advisor/pkg/detection/pattern_matcher.go`

---

### Milestone 3: Active Detection Task

**Goal:** Implement the active detection executor and task lifecycle

| Task ID | Task | Owner | Status | Notes |
|---------|------|-------|--------|-------|
| M3-1 | Add `TaskTypeActiveDetection` constant | | ✅ | Added to `task.go` |
| M3-2 | Implement `ActiveDetectionExecutor` | | ✅ | `active_detection_executor.go` |
| M3-2a | - Implement evidence collection (process probe) | | ✅ | `probeProcessInfo` |
| M3-2b | - Implement evidence collection (env probe) | | ✅ | `probeEnvInfo` |
| M3-2c | - Implement evidence collection (image check) | | ✅ | `probeImageInfo` |
| M3-2d | - Implement evidence collection (label check) | | ✅ | `probeLabelInfo` (stub) |
| M3-3 | Integrate `EvidenceAggregator` into executor | | ✅ | Integrated |
| M3-4 | Implement retry logic with exponential backoff | | ✅ | `handleRetryOrComplete` |
| M3-5 | Implement task completion criteria | | ✅ | Confidence threshold based |
| M3-6 | Add `CreateActiveDetectionTask` to TaskCreator | | ✅ | Added method |
| M3-7 | Hook into workload creation events | | 🔲 | Planned |
| M3-8 | Implement periodic scan for undetected workloads | | ✅ | `ScanForUndetectedWorkloads` |
| M3-9 | Register executor with TaskScheduler | | ✅ | Updated `bootstrap.go` |
| M3-10 | Write unit tests for executor | | 🔲 | Deferred to M4 |
| M3-11 | Write integration tests | | 🔲 | Deferred to M4 |

**Deliverables:**
- [x] File: `ai-advisor/pkg/task/active_detection_executor.go`
- [ ] File: `ai-advisor/pkg/task/active_detection_executor_test.go`
- [x] Updated: `core/pkg/constant/task.go`
- [x] Updated: `ai-advisor/pkg/detection/task_creator.go`
- [x] Updated: `ai-advisor/pkg/bootstrap/bootstrap.go`

---

### Milestone 4: Integration & Testing

**Goal:** End-to-end integration, testing, and production readiness

| Task ID | Task | Owner | Status | Notes |
|---------|------|-------|--------|-------|
| M4-1 | Dual-write migration: write to both old and new tables | | 🔲 | |
| M4-2 | Data migration script for existing workloads | | 🔲 | |
| M4-3 | Add metrics for active detection | | 🔲 | |
| M4-3a | - Detection attempts counter | | 🔲 | |
| M4-3b | - Detection success/failure rate | | 🔲 | |
| M4-3c | - Evidence aggregation latency | | 🔲 | |
| M4-3d | - Evidence count per workload | | 🔲 | |
| M4-4 | Add alerting rules for detection anomalies | | 🔲 | |
| M4-5 | Performance testing with high workload count | | 🔲 | |
| M4-6 | E2E tests: workload creation → detection → confirmation | | 🔲 | |
| M4-7 | Documentation update | | 🔲 | |
| M4-8 | Code review | | 🔲 | |
| M4-9 | Production deployment plan | | 🔲 | |
| M4-10 | Rollback plan | | 🔲 | |

**Deliverables:**
- [ ] Migration script: `scripts/migrate_detection_data.go`
- [ ] Metrics in Grafana dashboard
- [ ] E2E test suite
- [ ] Production deployment runbook

---

### Dependency Graph

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Dependency Graph                                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│   M1: Database Schema                                                    │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │  M1-1 ─┬─▶ M1-3 ─▶ M1-4 ─┬─▶ M1-5 ─┬─▶ M1-8                    │   │
│   │  M1-2 ─┘                 └─▶ M1-6 ─┘                            │   │
│   └─────────────────────────────┬───────────────────────────────────┘   │
│                                 │                                        │
│                                 ▼                                        │
│   M2: Evidence & Aggregation                                             │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │  M2-1 ─┬─▶ M2-3 ─▶ M2-5 ─▶ M2-8                                │   │
│   │        └─▶ M2-4 ─┘                                              │   │
│   │  M2-2                                                           │   │
│   │  M1-5 ─▶ M2-7a/b/c ─▶ M2-9                                     │   │
│   └─────────────────────────────┬───────────────────────────────────┘   │
│                                 │                                        │
│                                 ▼                                        │
│   M3: Active Detection Task                                              │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │  M3-1                                                           │   │
│   │  M3-2a/b/c/d ─▶ M3-2 ─┬─▶ M3-3 (requires M2-1)                 │   │
│   │                       └─▶ M3-4 ─▶ M3-5 ─▶ M3-10                │   │
│   │  M3-6 ─▶ M3-7                                                   │   │
│   │  M3-8                                                           │   │
│   │  M3-2 ─▶ M3-9 ─▶ M3-11                                         │   │
│   └─────────────────────────────┬───────────────────────────────────┘   │
│                                 │                                        │
│                                 ▼                                        │
│   M4: Integration & Testing                                              │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │  M4-1 ─▶ M4-2                                                   │   │
│   │  M4-3a/b/c/d ─▶ M4-4                                           │   │
│   │  M4-5, M4-6 ─▶ M4-7 ─▶ M4-8 ─▶ M4-9, M4-10                    │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

### Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Database migration causes downtime | Low | High | Use online migration, dual-write pattern |
| Evidence table grows too large | Medium | Medium | Implement expiration policy, partition by time |
| Performance degradation with many evidence rows | Low | Medium | Proper indexing, query optimization |
| Backward compatibility issues | Medium | High | Dual-write phase, gradual migration |
| Task scheduler overload | Low | Medium | Rate limiting, priority-based scheduling |

---

### Change Log

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| YYYY-MM-DD | 0.1 | | Initial design document |
| | | | |

---

### Notes & Decisions

**Open Questions:**
- [ ] Should evidence expire after a certain time?
- [ ] What's the maximum number of retry attempts?
- [ ] Should we support manual trigger of active detection via API?

**Decisions Made:**
- Decision 1: Use separate evidence table instead of embedding in detection result
  - Rationale: Better queryability, traceability, and extensibility
- Decision 2: Periodic aggregation instead of real-time
  - Rationale: Lower system load, batch processing efficiency

---

### Quick Links

- [FrameworkDetectionManager](../../../core/pkg/framework/detection_manager.go)
- [TaskCreator](../detection/task_creator.go)
- [PatternMatcher](../detection/pattern_matcher.go)
- [TaskScheduler](../../../core/pkg/task/scheduler.go)
