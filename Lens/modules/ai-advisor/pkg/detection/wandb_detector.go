package detection

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// WandBDetectionRequest wandb-exporter 上报的请求数据
type WandBDetectionRequest struct {
	Source      string        `json:"source"`                 // "wandb"
	Type        string        `json:"type"`                   // "framework_detection_raw"
	Version     string        `json:"version"`                // "1.0"
	WorkloadUID string        `json:"workload_uid,omitempty"` // 可选（兼容性）
	PodUID      string        `json:"pod_uid,omitempty"`
	PodName     string        `json:"pod_name"` // 必需：客户端从环境变量获取
	Namespace   string        `json:"namespace"`
	Evidence    WandBEvidence `json:"evidence"` // 原始证据
	Hints       WandBHints    `json:"hints"`    // 轻量级 hints
	Timestamp   float64       `json:"timestamp"`
}

// WandBEvidence 原始证据数据
type WandBEvidence struct {
	WandB             WandBInfo                         `json:"wandb"`
	Environment       map[string]string                 `json:"environment"`
	PyTorch           *PyTorchInfo                      `json:"pytorch,omitempty"`
	WrapperFrameworks map[string]map[string]interface{} `json:"wrapper_frameworks,omitempty"` // 外层包装框架检测结果
	BaseFrameworks    map[string]map[string]interface{} `json:"base_frameworks,omitempty"`    // 底层基础框架检测结果
	System            map[string]interface{}            `json:"system"`
}

// WandBInfo WandB 项目信息
type WandBInfo struct {
	Project string                 `json:"project"`
	Name    string                 `json:"name"`
	ID      string                 `json:"id"`
	Config  map[string]interface{} `json:"config"`
	Tags    []string               `json:"tags"`
}

// PyTorchInfo PyTorch 环境信息
type PyTorchInfo struct {
	Available       bool            `json:"available"`
	Version         string          `json:"version"`
	CudaAvailable   bool            `json:"cuda_available"`
	DetectedModules map[string]bool `json:"detected_modules"`
}

// WandBHints 预判断线索（支持双层框架检测）
type WandBHints struct {
	WrapperFrameworks  []string                          `json:"wrapper_frameworks"`         // 外层包装框架（如 primus, lightning）
	BaseFrameworks     []string                          `json:"base_frameworks"`            // 底层基础框架（如 megatron, deepspeed, jax）
	PossibleFrameworks []string                          `json:"possible_frameworks"`        // 所有框架（保持向后兼容）
	Confidence         string                            `json:"confidence"`                 // low/medium/high
	PrimaryIndicators  []string                          `json:"primary_indicators"`         // 检测指标来源
	FrameworkLayers    map[string]map[string]interface{} `json:"framework_layers,omitempty"` // 框架层级关系映射
}

// DetectionResult 检测结果（支持双层框架）
type DetectionResult struct {
	Framework        string // 主要框架（wrapper 或 base）
	FrameworkLayer   string // 框架层级：wrapper 或 base
	WrapperFramework string // 外层包装框架（如果有）
	BaseFramework    string // 底层基础框架（如果有）
	Confidence       float64
	Method           string
	MatchedEnvVars   []string
	MatchedModules   []string
}

// WandBFrameworkDetector WandB 框架检测器
type WandBFrameworkDetector struct {
	detectionManager *framework.FrameworkDetectionManager
}

// NewWandBFrameworkDetector 创建检测器
func NewWandBFrameworkDetector(
	detectMgr *framework.FrameworkDetectionManager,
) *WandBFrameworkDetector {
	return &WandBFrameworkDetector{
		detectionManager: detectMgr,
	}
}

// ProcessWandBDetection 处理 WandB 检测请求
func (d *WandBFrameworkDetector) ProcessWandBDetection(
	ctx context.Context,
	req *WandBDetectionRequest,
) error {
	// Record metrics: request count and duration
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		log.Debugf("WandB detection processed in %.3fs", duration)
	}()

	// 1. 从 PodName 解析 WorkloadUID
	workloadUID, err := resolveWorkloadUID(req.WorkloadUID, req.PodName)
	if err != nil {
		return err
	}

	log.Infof("Processing WandB detection for pod %s -> workload %s", req.PodName, workloadUID)

	// 2. 记录 hints（用于监控和调优，支持双层框架）
	if len(req.Hints.WrapperFrameworks) > 0 || len(req.Hints.BaseFrameworks) > 0 {
		log.Infof("WandB hints (双层框架): wrapper=%v, base=%v, confidence=%s",
			req.Hints.WrapperFrameworks,
			req.Hints.BaseFrameworks,
			req.Hints.Confidence)
		log.Debugf("WandB hints indicators: %v", req.Hints.PrimaryIndicators)
	} else if len(req.Hints.PossibleFrameworks) > 0 {
		// 向后兼容：旧格式的 hints
		log.Debugf("WandB hints (legacy): frameworks=%v, confidence=%s, indicators=%v",
			req.Hints.PossibleFrameworks,
			req.Hints.Confidence,
			req.Hints.PrimaryIndicators)
	}

	// 3. 执行框架检测规则
	result := d.detectFramework(req)
	if result == nil || result.Framework == "" {
		log.Debug("No framework detected from WandB data")
		return nil
	}

	// 根据框架层级输出不同的日志
	if result.FrameworkLayer == "wrapper" && result.BaseFramework != "" {
		log.Infof("✓ Detected framework from WandB: %s/%s (wrapper/base, confidence: %.2f, method: %s)",
			result.Framework, result.BaseFramework, result.Confidence, result.Method)
	} else if result.FrameworkLayer != "" {
		log.Infof("✓ Detected framework from WandB: %s (layer: %s, confidence: %.2f, method: %s)",
			result.Framework, result.FrameworkLayer, result.Confidence, result.Method)
	} else {
		log.Infof("✓ Detected framework from WandB: %s (confidence: %.2f, method: %s)",
			result.Framework, result.Confidence, result.Method)
	}

	// 4. 构造证据（包含双层框架信息）
	evidence := map[string]interface{}{
		"method":            result.Method,
		"framework_layer":   result.FrameworkLayer,
		"wrapper_framework": result.WrapperFramework,
		"base_framework":    result.BaseFramework,
		"wandb_project":     req.Evidence.WandB.Project,
		"wandb_name":        req.Evidence.WandB.Name,
		"environment_vars":  result.MatchedEnvVars,
		"pytorch_modules":   result.MatchedModules,
		"hints":             req.Hints,
		"pod_name":          req.PodName,
		"detected_at":       time.Now().Format(time.RFC3339),
	}

	// 5. 上报到 FrameworkDetectionManager
	err = d.detectionManager.ReportDetection(
		ctx,
		workloadUID,
		"wandb",
		result.Framework,
		"training",
		result.Confidence,
		evidence,
	)

	if err != nil {
		log.Errorf("Failed to report WandB detection: %v", err)
		return err
	}

	log.Infof("✓ Successfully reported WandB detection for workload %s", workloadUID)

	return nil
}

// detectFramework 基于 WandB 数据检测框架（支持双层框架）
func (d *WandBFrameworkDetector) detectFramework(
	req *WandBDetectionRequest,
) *DetectionResult {

	// 优先使用 Import 检测结果（最强指标）
	if result := d.detectFromImportEvidence(req.Evidence); result != nil {
		return result
	}

	// 按优先级应用检测规则

	// 1. 环境变量检测（最高优先级，confidence: 0.80）
	if result := d.detectFromEnvVars(req.Evidence.Environment); result != nil {
		return result
	}

	// 2. WandB Config 检测（confidence: 0.70）
	if result := d.detectFromWandBConfig(req.Evidence.WandB.Config); result != nil {
		// 尝试从 hints 中补充 wrapper 框架信息
		if result.WrapperFramework == "" && len(req.Hints.WrapperFrameworks) > 0 {
			// 从 hints 中选择第一个 wrapper 框架
			result.WrapperFramework = req.Hints.WrapperFrameworks[0]
			// 如果当前检测到的是 base 框架，更新主框架为 wrapper
			if result.FrameworkLayer == "base" {
				result.Framework = result.WrapperFramework
				result.FrameworkLayer = "wrapper"
			}
		}
		return result
	}

	// 3. PyTorch 模块检测（confidence: 0.60）
	if req.Evidence.PyTorch != nil && req.Evidence.PyTorch.Available {
		if result := d.detectFromPyTorchModules(req.Evidence.PyTorch); result != nil {
			return result
		}
	}

	// 4. WandB Project 名称检测（confidence: 0.50）
	if result := d.detectFromWandBProject(req.Evidence.WandB.Project); result != nil {
		return result
	}

	// 5. Fallback: 如果所有检测方法都失败，尝试使用 hints（confidence: 0.40）
	if result := d.detectFromHints(req.Hints); result != nil {
		return result
	}

	return nil
}

// detectFromImportEvidence 从 Import 检测证据中提取框架信息（最强指标）
func (d *WandBFrameworkDetector) detectFromImportEvidence(evidence WandBEvidence) *DetectionResult {
	var wrapperFramework string
	var baseFramework string

	// 检查 wrapper_frameworks
	if len(evidence.WrapperFrameworks) > 0 {
		// 优先选择 Primus（如果存在）
		if primusInfo, ok := evidence.WrapperFrameworks["primus"]; ok {
			if detected, _ := primusInfo["detected"].(bool); detected {
				wrapperFramework = "primus"
				// 尝试获取 Primus 的 base_framework
				if baseFrameworkVal, ok := primusInfo["base_framework"]; ok && baseFrameworkVal != nil {
					if baseStr, ok := baseFrameworkVal.(string); ok && baseStr != "" {
						baseFramework = strings.ToLower(baseStr)
					}
				}
			}
		}

		// 其他 wrapper frameworks
		if wrapperFramework == "" {
			for frameworkName, frameworkInfo := range evidence.WrapperFrameworks {
				if detected, ok := frameworkInfo["detected"].(bool); ok && detected {
					wrapperFramework = frameworkName
					break
				}
			}
		}
	}

	// 检查 base_frameworks
	if len(evidence.BaseFrameworks) > 0 && baseFramework == "" {
		// 优先级：megatron > deepspeed > jax > transformers
		priority := []string{"megatron", "deepspeed", "jax", "transformers"}
		for _, frameworkName := range priority {
			if frameworkInfo, ok := evidence.BaseFrameworks[frameworkName]; ok {
				if detected, ok := frameworkInfo["detected"].(bool); ok && detected {
					baseFramework = frameworkName
					break
				}
			}
		}

		// 如果优先级中没有找到，检查其他框架
		if baseFramework == "" {
			for frameworkName, frameworkInfo := range evidence.BaseFrameworks {
				if detected, ok := frameworkInfo["detected"].(bool); ok && detected {
					baseFramework = frameworkName
					break
				}
			}
		}
	}

	// 构造检测结果
	if wrapperFramework != "" || baseFramework != "" {
		result := &DetectionResult{
			Confidence: 0.90, // Import 检测是最强指标
			Method:     "import_detection",
		}

		// 优先报告 wrapper 框架
		if wrapperFramework != "" {
			result.Framework = wrapperFramework
			result.FrameworkLayer = "wrapper"
			result.WrapperFramework = wrapperFramework
			result.BaseFramework = baseFramework
		} else {
			result.Framework = baseFramework
			result.FrameworkLayer = "base"
			result.BaseFramework = baseFramework
		}

		return result
	}

	return nil
}

// detectFromEnvVars 从环境变量检测（支持双层框架）
func (d *WandBFrameworkDetector) detectFromEnvVars(env map[string]string) *DetectionResult {

	// Wrapper Frameworks

	// Primus (wrapper)
	primusVars := []string{"PRIMUS_CONFIG", "PRIMUS_VERSION", "PRIMUS_BACKEND"}
	if matched := hasAnyKey(env, primusVars); len(matched) > 0 {
		result := &DetectionResult{
			Framework:        "primus",
			FrameworkLayer:   "wrapper",
			WrapperFramework: "primus",
			Confidence:       0.80,
			Method:           "env_vars",
			MatchedEnvVars:   matched,
		}
		// 检查 PRIMUS_BACKEND 以确定底层框架
		if backend := env["PRIMUS_BACKEND"]; backend != "" {
			result.BaseFramework = strings.ToLower(backend)
		}
		return result
	}

	// Base Frameworks

	// DeepSpeed (base)
	deepspeedVars := []string{"DEEPSPEED_CONFIG", "DS_CONFIG", "DEEPSPEED_VERSION"}
	if matched := hasAnyKey(env, deepspeedVars); len(matched) > 0 {
		return &DetectionResult{
			Framework:      "deepspeed",
			FrameworkLayer: "base",
			BaseFramework:  "deepspeed",
			Confidence:     0.80,
			Method:         "env_vars",
			MatchedEnvVars: matched,
		}
	}

	// Megatron (base)
	megatronVars := []string{"MEGATRON_CONFIG", "MEGATRON_LM_PATH"}
	if matched := hasAnyKey(env, megatronVars); len(matched) > 0 {
		return &DetectionResult{
			Framework:      "megatron",
			FrameworkLayer: "base",
			BaseFramework:  "megatron",
			Confidence:     0.80,
			Method:         "env_vars",
			MatchedEnvVars: matched,
		}
	}

	// JAX (base)
	jaxVars := []string{"JAX_BACKEND", "JAX_PLATFORMS"}
	if matched := hasAnyKey(env, jaxVars); len(matched) > 0 {
		return &DetectionResult{
			Framework:      "jax",
			FrameworkLayer: "base",
			BaseFramework:  "jax",
			Confidence:     0.80,
			Method:         "env_vars",
			MatchedEnvVars: matched,
		}
	}

	// 通用 FRAMEWORK 环境变量（根据框架名称判断层级）
	if fw := env["FRAMEWORK"]; fw != "" {
		fwLower := strings.ToLower(fw)
		result := &DetectionResult{
			Framework:      fwLower,
			Confidence:     0.75,
			Method:         "env_framework",
			MatchedEnvVars: []string{"FRAMEWORK"},
		}

		// 判断是 wrapper 还是 base
		if isWrapperFramework(fwLower) {
			result.FrameworkLayer = "wrapper"
			result.WrapperFramework = fwLower
		} else {
			result.FrameworkLayer = "base"
			result.BaseFramework = fwLower
		}

		return result
	}

	return nil
}

// detectFromWandBConfig 从 WandB Config 检测（支持双层框架）
func (d *WandBFrameworkDetector) detectFromWandBConfig(config map[string]interface{}) *DetectionResult {

	// 检查 config.framework 字段
	if fw, ok := config["framework"]; ok {
		framework := strings.ToLower(fmt.Sprintf("%v", fw))
		result := &DetectionResult{
			Framework:  framework,
			Confidence: 0.70,
			Method:     "wandb_config_framework",
		}

		// 判断框架层级
		if isWrapperFramework(framework) {
			result.FrameworkLayer = "wrapper"
			result.WrapperFramework = framework
		} else {
			result.FrameworkLayer = "base"
			result.BaseFramework = framework
		}

		return result
	}

	// 检查 config.base_framework 字段（Primus 特定）
	if baseFw, ok := config["base_framework"]; ok {
		baseFramework := strings.ToLower(fmt.Sprintf("%v", baseFw))
		return &DetectionResult{
			Framework:      baseFramework,
			FrameworkLayer: "base",
			BaseFramework:  baseFramework,
			Confidence:     0.70,
			Method:         "wandb_config_base_framework",
		}
	}

	// 检查 config.trainer 字段（可能包含框架信息）
	if trainer, ok := config["trainer"]; ok {
		trainerStr := strings.ToLower(fmt.Sprintf("%v", trainer))
		if strings.Contains(trainerStr, "deepspeed") {
			return &DetectionResult{
				Framework:      "deepspeed",
				FrameworkLayer: "base",
				BaseFramework:  "deepspeed",
				Confidence:     0.65,
				Method:         "wandb_config_trainer",
			}
		}
		if strings.Contains(trainerStr, "megatron") {
			return &DetectionResult{
				Framework:      "megatron",
				FrameworkLayer: "base",
				BaseFramework:  "megatron",
				Confidence:     0.65,
				Method:         "wandb_config_trainer",
			}
		}
	}

	// 检查特定框架配置键
	configKeys := map[string]struct {
		framework string
		layer     string
	}{
		"primus_config":    {"primus", "wrapper"},
		"deepspeed_config": {"deepspeed", "base"},
		"megatron_config":  {"megatron", "base"},
	}

	for key, info := range configKeys {
		if _, exists := config[key]; exists {
			result := &DetectionResult{
				Framework:      info.framework,
				FrameworkLayer: info.layer,
				Confidence:     0.65,
				Method:         "wandb_config_key",
			}

			if info.layer == "wrapper" {
				result.WrapperFramework = info.framework
			} else {
				result.BaseFramework = info.framework
			}

			return result
		}
	}

	return nil
}

// detectFromPyTorchModules 从 PyTorch 模块检测（支持双层框架）
func (d *WandBFrameworkDetector) detectFromPyTorchModules(pytorch *PyTorchInfo) *DetectionResult {

	modules := pytorch.DetectedModules
	if modules == nil {
		return nil
	}

	// Wrapper frameworks
	if modules["lightning"] {
		return &DetectionResult{
			Framework:        "lightning",
			FrameworkLayer:   "wrapper",
			WrapperFramework: "lightning",
			Confidence:       0.60,
			Method:           "pytorch_modules",
			MatchedModules:   []string{"lightning"},
		}
	}

	// Base frameworks (按优先级检查)
	if modules["deepspeed"] {
		return &DetectionResult{
			Framework:      "deepspeed",
			FrameworkLayer: "base",
			BaseFramework:  "deepspeed",
			Confidence:     0.60,
			Method:         "pytorch_modules",
			MatchedModules: []string{"deepspeed"},
		}
	}

	if modules["megatron"] {
		return &DetectionResult{
			Framework:      "megatron",
			FrameworkLayer: "base",
			BaseFramework:  "megatron",
			Confidence:     0.60,
			Method:         "pytorch_modules",
			MatchedModules: []string{"megatron"},
		}
	}

	if modules["transformers"] {
		return &DetectionResult{
			Framework:      "transformers",
			FrameworkLayer: "base",
			BaseFramework:  "transformers",
			Confidence:     0.55,
			Method:         "pytorch_modules",
			MatchedModules: []string{"transformers"},
		}
	}

	return nil
}

// detectFromWandBProject 从 WandB 项目名检测（支持双层框架）
func (d *WandBFrameworkDetector) detectFromWandBProject(project string) *DetectionResult {
	if project == "" {
		return nil
	}

	projectLower := strings.ToLower(project)

	// Wrapper frameworks
	wrapperFrameworks := map[string][]string{
		"primus":    {"primus", "primus-training", "primus-exp"},
		"lightning": {"lightning", "pl-training", "pytorch-lightning"},
	}

	for frameworkName, patterns := range wrapperFrameworks {
		for _, pattern := range patterns {
			if strings.Contains(projectLower, pattern) {
				return &DetectionResult{
					Framework:        frameworkName,
					FrameworkLayer:   "wrapper",
					WrapperFramework: frameworkName,
					Confidence:       0.50,
					Method:           "wandb_project_name",
				}
			}
		}
	}

	// Base frameworks
	baseFrameworks := map[string][]string{
		"deepspeed":    {"deepspeed", "ds-training", "deepspeed-exp"},
		"megatron":     {"megatron", "megatron-lm", "megatron-training"},
		"jax":          {"jax", "jax-training"},
		"transformers": {"transformers", "hf-transformers"},
	}

	for frameworkName, patterns := range baseFrameworks {
		for _, pattern := range patterns {
			if strings.Contains(projectLower, pattern) {
				return &DetectionResult{
					Framework:      frameworkName,
					FrameworkLayer: "base",
					BaseFramework:  frameworkName,
					Confidence:     0.50,
					Method:         "wandb_project_name",
				}
			}
		}
	}

	return nil
}

// detectFromHints 从 hints 提取框架信息（最低优先级 fallback）
func (d *WandBFrameworkDetector) detectFromHints(hints WandBHints) *DetectionResult {
	var wrapperFramework string
	var baseFramework string

	// 优先选择 wrapper 框架
	if len(hints.WrapperFrameworks) > 0 {
		// 优先选择 primus
		for _, fw := range hints.WrapperFrameworks {
			if fw == "primus" {
				wrapperFramework = fw
				break
			}
		}
		// 如果没有 primus，选择第一个
		if wrapperFramework == "" {
			wrapperFramework = hints.WrapperFrameworks[0]
		}
	}

	// 选择 base 框架
	if len(hints.BaseFrameworks) > 0 {
		// 按优先级选择：megatron > deepspeed > jax > transformers
		priority := []string{"megatron", "deepspeed", "jax", "transformers"}
		for _, priorityFw := range priority {
			for _, fw := range hints.BaseFrameworks {
				if fw == priorityFw {
					baseFramework = fw
					break
				}
			}
			if baseFramework != "" {
				break
			}
		}
		// 如果优先级中没有匹配，选择第一个
		if baseFramework == "" {
			baseFramework = hints.BaseFrameworks[0]
		}
	}

	// 构造检测结果
	if wrapperFramework != "" || baseFramework != "" {
		result := &DetectionResult{
			Confidence: 0.40, // Hints 是最低优先级的 fallback
			Method:     "hints_fallback",
		}

		// 优先报告 wrapper 框架
		if wrapperFramework != "" {
			result.Framework = wrapperFramework
			result.FrameworkLayer = "wrapper"
			result.WrapperFramework = wrapperFramework
			result.BaseFramework = baseFramework
		} else {
			result.Framework = baseFramework
			result.FrameworkLayer = "base"
			result.BaseFramework = baseFramework
		}

		return result
	}

	return nil
}

// hasAnyKey 检查 map 中是否有任意一个 key
func hasAnyKey(m map[string]string, keys []string) []string {
	matched := []string{}
	for _, key := range keys {
		if _, ok := m[key]; ok {
			matched = append(matched, key)
		}
	}
	return matched
}

// isWrapperFramework 判断框架是否为 wrapper 框架
func isWrapperFramework(framework string) bool {
	wrapperFrameworks := map[string]bool{
		"primus":               true,
		"lightning":            true,
		"pytorch_lightning":    true,
		"transformers_trainer": true,
	}
	return wrapperFrameworks[framework]
}

// resolveWorkloadUID 从 PodName 或 WorkloadUID 解析出 workload UID
func resolveWorkloadUID(workloadUID, podName string) (string, error) {
	// 如果直接提供了 WorkloadUID，使用它
	if workloadUID != "" {
		return workloadUID, nil
	}

	// 从 PodName 解析（假设格式: workload-name-replica-index）
	if podName == "" {
		return "", fmt.Errorf("both workload_uid and pod_name are empty")
	}

	// TODO: 实现从 PodName 到 WorkloadUID 的映射
	// 这里暂时使用 PodName 作为 WorkloadUID
	// 在实际实现中，可能需要查询数据库或调用其他服务
	return podName, nil
}
