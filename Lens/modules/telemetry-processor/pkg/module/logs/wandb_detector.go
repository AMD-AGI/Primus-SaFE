package logs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
)

// WandBDetectionRequest wandb-exporter 上报的请求数据
type WandBDetectionRequest struct {
	Source      string                 `json:"source"`       // "wandb"
	Type        string                 `json:"type"`         // "framework_detection_raw"
	Version     string                 `json:"version"`      // "1.0"
	WorkloadUID string                 `json:"workload_uid"` // 必需
	PodUID      string                 `json:"pod_uid"`
	PodName     string                 `json:"pod_name"`
	Namespace   string                 `json:"namespace"`
	Evidence    WandBEvidence          `json:"evidence"`     // 原始证据
	Hints       WandBHints             `json:"hints"`        // 轻量级 hints
	Timestamp   float64                `json:"timestamp"`
}

// WandBEvidence 原始证据数据
type WandBEvidence struct {
	WandB       WandBInfo              `json:"wandb"`
	Environment map[string]string      `json:"environment"`
	PyTorch     *PyTorchInfo           `json:"pytorch,omitempty"`
	System      map[string]interface{} `json:"system"`
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
	Available       bool              `json:"available"`
	Version         string            `json:"version"`
	CudaAvailable   bool              `json:"cuda_available"`
	DetectedModules map[string]bool   `json:"detected_modules"`
}

// WandBHints 预判断线索
type WandBHints struct {
	PossibleFrameworks []string `json:"possible_frameworks"`
	Confidence         string   `json:"confidence"` // low/medium/high
	PrimaryIndicators  []string `json:"primary_indicators"`
}

// DetectionResult 检测结果
type DetectionResult struct {
	Framework      string
	Confidence     float64
	Method         string
	MatchedEnvVars []string
	MatchedModules []string
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
	logrus.Infof("Processing WandB detection for workload %s", req.WorkloadUID)

	// 1. 验证必需字段
	if req.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}

	// 2. 记录 hints（用于监控和调优）
	if len(req.Hints.PossibleFrameworks) > 0 {
		logrus.Debugf("WandB hints: frameworks=%v, confidence=%s, indicators=%v",
			req.Hints.PossibleFrameworks,
			req.Hints.Confidence,
			req.Hints.PrimaryIndicators)
	}

	// 3. 执行框架检测规则
	result := d.detectFramework(req)
	if result == nil || result.Framework == "" {
		logrus.Debug("No framework detected from WandB data")
		return nil
	}

	logrus.Infof("✓ Detected framework from WandB: %s (confidence: %.2f, method: %s)",
		result.Framework, result.Confidence, result.Method)

	// 4. 构造证据
	evidence := map[string]interface{}{
		"method":           result.Method,
		"wandb_project":    req.Evidence.WandB.Project,
		"wandb_name":       req.Evidence.WandB.Name,
		"environment_vars": result.MatchedEnvVars,
		"pytorch_modules":  result.MatchedModules,
		"hints":            req.Hints,
		"detected_at":      time.Now().Format(time.RFC3339),
	}

	// 5. 上报到 FrameworkDetectionManager
	err := d.detectionManager.ReportDetection(
		ctx,
		req.WorkloadUID,
		"wandb",
		result.Framework,
		"training",
		result.Confidence,
		evidence,
	)

	if err != nil {
		logrus.Errorf("Failed to report WandB detection: %v", err)
		return err
	}

	logrus.Infof("✓ Successfully reported WandB detection for workload %s", req.WorkloadUID)

	return nil
}

// detectFramework 基于 WandB 数据检测框架
func (d *WandBFrameworkDetector) detectFramework(
	req *WandBDetectionRequest,
) *DetectionResult {

	// 按优先级应用检测规则

	// 1. 环境变量检测（最高优先级，confidence: 0.80）
	if result := d.detectFromEnvVars(req.Evidence.Environment); result != nil {
		return result
	}

	// 2. WandB Config 检测（confidence: 0.70）
	if result := d.detectFromWandBConfig(req.Evidence.WandB.Config); result != nil {
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

	return nil
}

// detectFromEnvVars 从环境变量检测
func (d *WandBFrameworkDetector) detectFromEnvVars(env map[string]string) *DetectionResult {

	// Primus
	primusVars := []string{"PRIMUS_CONFIG", "PRIMUS_VERSION", "PRIMUS_BACKEND"}
	if matched := hasAnyKey(env, primusVars); len(matched) > 0 {
		return &DetectionResult{
			Framework:      "primus",
			Confidence:     0.80,
			Method:         "env_vars",
			MatchedEnvVars: matched,
		}
	}

	// DeepSpeed
	deepspeedVars := []string{"DEEPSPEED_CONFIG", "DS_CONFIG", "DEEPSPEED_VERSION"}
	if matched := hasAnyKey(env, deepspeedVars); len(matched) > 0 {
		return &DetectionResult{
			Framework:      "deepspeed",
			Confidence:     0.80,
			Method:         "env_vars",
			MatchedEnvVars: matched,
		}
	}

	// Megatron
	megatronVars := []string{"MEGATRON_CONFIG", "MEGATRON_LM_PATH"}
	if matched := hasAnyKey(env, megatronVars); len(matched) > 0 {
		return &DetectionResult{
			Framework:      "megatron",
			Confidence:     0.80,
			Method:         "env_vars",
			MatchedEnvVars: matched,
		}
	}

	// 通用 FRAMEWORK 环境变量
	if fw := env["FRAMEWORK"]; fw != "" {
		return &DetectionResult{
			Framework:      strings.ToLower(fw),
			Confidence:     0.75,
			Method:         "env_framework",
			MatchedEnvVars: []string{"FRAMEWORK"},
		}
	}

	return nil
}

// detectFromWandBConfig 从 WandB Config 检测
func (d *WandBFrameworkDetector) detectFromWandBConfig(config map[string]interface{}) *DetectionResult {

	// 检查 config.framework 字段
	if fw, ok := config["framework"]; ok {
		framework := strings.ToLower(fmt.Sprintf("%v", fw))
		return &DetectionResult{
			Framework:  framework,
			Confidence: 0.70,
			Method:     "wandb_config_framework",
		}
	}

	// 检查 config.trainer 字段（可能包含框架信息）
	if trainer, ok := config["trainer"]; ok {
		trainerStr := strings.ToLower(fmt.Sprintf("%v", trainer))
		if strings.Contains(trainerStr, "deepspeed") {
			return &DetectionResult{
				Framework:  "deepspeed",
				Confidence: 0.65,
				Method:     "wandb_config_trainer",
			}
		}
	}

	// 检查特定框架配置键
	configKeys := map[string]string{
		"primus_config":    "primus",
		"deepspeed_config": "deepspeed",
		"megatron_config":  "megatron",
	}

	for key, frameworkName := range configKeys {
		if _, exists := config[key]; exists {
			return &DetectionResult{
				Framework:  frameworkName,
				Confidence: 0.65,
				Method:     "wandb_config_key",
			}
		}
	}

	return nil
}

// detectFromPyTorchModules 从 PyTorch 模块检测
func (d *WandBFrameworkDetector) detectFromPyTorchModules(pytorch *PyTorchInfo) *DetectionResult {

	modules := pytorch.DetectedModules
	if modules == nil {
		return nil
	}

	// 按优先级检查
	if modules["deepspeed"] {
		return &DetectionResult{
			Framework:      "deepspeed",
			Confidence:     0.60,
			Method:         "pytorch_modules",
			MatchedModules: []string{"deepspeed"},
		}
	}

	if modules["megatron"] {
		return &DetectionResult{
			Framework:      "megatron",
			Confidence:     0.60,
			Method:         "pytorch_modules",
			MatchedModules: []string{"megatron"},
		}
	}

	return nil
}

// detectFromWandBProject 从 WandB 项目名检测
func (d *WandBFrameworkDetector) detectFromWandBProject(project string) *DetectionResult {
	if project == "" {
		return nil
	}

	projectLower := strings.ToLower(project)

	frameworks := map[string][]string{
		"primus":    {"primus", "primus-training", "primus-exp"},
		"deepspeed": {"deepspeed", "ds-training", "deepspeed-exp"},
		"megatron":  {"megatron", "megatron-lm", "megatron-training"},
	}

	for frameworkName, patterns := range frameworks {
		for _, pattern := range patterns {
			if strings.Contains(projectLower, pattern) {
				return &DetectionResult{
					Framework:  frameworkName,
					Confidence: 0.50,
					Method:     "wandb_project_name",
				}
			}
		}
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

