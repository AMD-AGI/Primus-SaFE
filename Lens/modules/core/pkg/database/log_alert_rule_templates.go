package database

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
)

// InitBuiltinLogAlertRuleTemplates initializes built-in log alert rule templates
func InitBuiltinLogAlertRuleTemplates(ctx context.Context) error {
	facade := GetFacade().GetLogAlertRule()
	
	templates := getBuiltinTemplates()
	
	for _, template := range templates {
		// Check if template already exists
		existing, err := facade.GetLogAlertRuleTemplateByID(ctx, template.ID)
		if err != nil {
			log.Warnf("Failed to check existing template %s: %v", template.Name, err)
			continue
		}
		
		if existing != nil {
			// Skip if already exists
			log.Debugf("Template %s already exists, skipping", template.Name)
			continue
		}
		
		// Create template
		if err := facade.CreateLogAlertRuleTemplate(ctx, template); err != nil {
			log.Errorf("Failed to create builtin template %s: %v", template.Name, err)
			// Continue with other templates
			continue
		}
		
		log.Infof("Created builtin log alert rule template: %s", template.Name)
	}
	
	log.Info("Builtin log alert rule templates initialized")
	return nil
}

// getBuiltinTemplates returns all built-in template definitions
func getBuiltinTemplates() []*model.LogAlertRuleTemplates {
	return []*model.LogAlertRuleTemplates{
		// 1. Basic Error Detection
		{
			Name:        "Generic-Error-Detection",
			Category:    "basic",
			Description: "通用错误日志检测，匹配 ERROR、FATAL、Exception 等关键字",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "namespace",
						"key":      "namespace",
						"operator": "notexists",
					},
				},
				"match_type": "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "ERROR|FATAL|Exception|panic|failed",
					"ignore_case": true,
				},
				"severity": "warning",
				"alert_template": map[string]interface{}{
					"summary":     "检测到错误日志",
					"description": "Pod {{.PodName}} 产生错误日志: {{.LogMessage}}",
					"labels": map[string]string{
						"category": "error",
					},
				},
			}),
			Tags:      strings.Join([]string{"error", "basic", "generic"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
		
		// 2. GPU OOM Detection
		{
			Name:        "GPU-OOM-Detection",
			Category:    "gpu",
			Description: "GPU 内存溢出检测",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "workload",
						"key":      "training.kubeflow.org/job-name",
						"operator": "exists",
					},
				},
				"match_type": "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "CUDA out of memory|OutOfMemoryError|OOM|out of memory",
					"ignore_case": true,
				},
				"severity": "critical",
				"alert_template": map[string]interface{}{
					"summary":     "GPU 内存溢出",
					"description": "工作负载 {{.WorkloadName}} 的 Pod {{.PodName}} 发生 GPU OOM",
					"labels": map[string]string{
						"category":  "gpu",
						"component": "memory",
					},
				},
			}),
			Tags:      strings.Join([]string{"gpu", "oom", "memory", "critical"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
		
		// 3. GPU OOM Frequency Detection
		{
			Name:        "GPU-OOM-Frequency",
			Category:    "gpu",
			Description: "GPU 频繁 OOM 检测（10分钟内3次以上）",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "workload",
						"key":      "training.kubeflow.org/job-name",
						"operator": "exists",
					},
				},
				"match_type": "threshold",
				"match_config": map[string]interface{}{
					"pattern":     "CUDA out of memory|OutOfMemoryError",
					"ignore_case": true,
					"threshold": map[string]interface{}{
						"count_threshold": 3,
						"time_window":     600, // 10 minutes
						"aggregate_by":    []string{"workload_id", "pod_name"},
					},
				},
				"severity": "critical",
				"alert_template": map[string]interface{}{
					"summary":     "GPU 频繁内存溢出",
					"description": "工作负载 {{.WorkloadName}} 在过去10分钟内发生多次 GPU OOM",
					"labels": map[string]string{
						"category":  "gpu",
						"component": "memory",
						"frequency": "high",
					},
				},
			}),
			Tags:      strings.Join([]string{"gpu", "oom", "threshold", "critical"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
		
		// 4. NCCL Error Detection
		{
			Name:        "NCCL-Error-Detection",
			Category:    "network",
			Description: "NCCL 通信错误检测",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "workload",
						"key":      "training.kubeflow.org/job-name",
						"operator": "exists",
					},
				},
				"match_type": "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "NCCL (error|WARN|failed)|AllReduce.*timeout",
					"ignore_case": false,
				},
				"severity": "critical",
				"alert_template": map[string]interface{}{
					"summary":     "NCCL 通信错误",
					"description": "工作负载 {{.WorkloadName}} 的 Pod {{.PodName}} 出现 NCCL 错误: {{.LogMessage}}",
					"labels": map[string]string{
						"category":  "network",
						"component": "nccl",
					},
				},
			}),
			Tags:      strings.Join([]string{"network", "nccl", "communication", "critical"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
		
		// 5. InfiniBand Error Detection
		{
			Name:        "InfiniBand-Error-Detection",
			Category:    "network",
			Description: "InfiniBand 网络错误检测",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "node",
						"key":      "node_name",
						"operator": "exists",
					},
				},
				"match_type": "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "IB|InfiniBand.*(error|timeout|failed)|RDMA.*(error|failed)",
					"ignore_case": true,
				},
				"severity": "warning",
				"alert_template": map[string]interface{}{
					"summary":     "InfiniBand 网络错误",
					"description": "节点 {{.NodeName}} 的 InfiniBand 网络出现错误",
					"labels": map[string]string{
						"category":  "network",
						"component": "infiniband",
					},
				},
			}),
			Tags:      strings.Join([]string{"network", "infiniband", "rdma", "warning"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
		
		// 6. Training Loss NaN Detection
		{
			Name:        "Training-Loss-NaN",
			Category:    "training",
			Description: "训练损失值 NaN 检测",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "workload",
						"key":      "training.kubeflow.org/job-name",
						"operator": "exists",
					},
				},
				"match_type": "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "loss.*NaN|NaN.*loss|loss is nan",
					"ignore_case": true,
				},
				"severity": "critical",
				"alert_template": map[string]interface{}{
					"summary":     "训练损失值异常",
					"description": "工作负载 {{.WorkloadName}} 的训练损失值为 NaN",
					"labels": map[string]string{
						"category":  "training",
						"component": "loss",
					},
				},
			}),
			Tags:      strings.Join([]string{"training", "loss", "nan", "critical"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
		
		// 7. Checkpoint Save Failed
		{
			Name:        "Training-Checkpoint-Failed",
			Category:    "training",
			Description: "训练检查点保存失败检测",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "workload",
						"key":      "training.kubeflow.org/job-name",
						"operator": "exists",
					},
				},
				"match_type": "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "checkpoint.*(fail|error)|save.*checkpoint.*(fail|error)",
					"ignore_case": true,
				},
				"severity": "warning",
				"alert_template": map[string]interface{}{
					"summary":     "检查点保存失败",
					"description": "工作负载 {{.WorkloadName}} 的检查点保存失败",
					"labels": map[string]string{
						"category":  "training",
						"component": "checkpoint",
					},
				},
			}),
			Tags:      strings.Join([]string{"training", "checkpoint", "save", "warning"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
		
		// 8. Throughput Degradation
		{
			Name:        "Training-Throughput-Degradation",
			Category:    "performance",
			Description: "训练吞吐量下降检测（TFLOPS < 100）",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "workload",
						"key":      "training.kubeflow.org/job-name",
						"operator": "exists",
					},
				},
				"match_type": "threshold",
				"match_config": map[string]interface{}{
					"pattern":     "iteration.*TFLOPS",
					"ignore_case": false,
					"threshold": map[string]interface{}{
						"count_threshold": 5,
						"time_window":     300, // 5 minutes
						"aggregate_by":    []string{"workload_id"},
					},
				},
				"severity": "warning",
				"alert_template": map[string]interface{}{
					"summary":     "训练吞吐量下降",
					"description": "工作负载 {{.WorkloadName}} 的 TFLOPS 持续低于预期",
					"labels": map[string]string{
						"category":  "performance",
						"component": "throughput",
					},
				},
			}),
			Tags:      strings.Join([]string{"performance", "throughput", "tflops", "warning"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
		
		// 9. Pod Restart Detection
		{
			Name:        "Pod-Restart-Detection",
			Category:    "basic",
			Description: "Pod 重启检测",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "pod",
						"key":      "pod_name",
						"operator": "exists",
					},
				},
				"match_type": "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "Container.*restart|Pod.*restart|Restarting container",
					"ignore_case": true,
				},
				"severity": "warning",
				"alert_template": map[string]interface{}{
					"summary":     "Pod 重启",
					"description": "Pod {{.PodName}} 在节点 {{.NodeName}} 上重启",
					"labels": map[string]string{
						"category":  "kubernetes",
						"component": "pod",
					},
				},
			}),
			Tags:      strings.Join([]string{"kubernetes", "pod", "restart", "warning"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
		
		// 10. Production Environment Critical Error
		{
			Name:        "Production-Critical-Error",
			Category:    "basic",
			Description: "生产环境严重错误检测",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "namespace",
						"key":      "namespace",
						"operator": "regex",
						"values":   []string{"^prod-.*"},
					},
				},
				"match_type": "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "FATAL|CRITICAL|panic",
					"ignore_case": false,
				},
				"severity": "critical",
				"alert_template": map[string]interface{}{
					"summary":     "生产环境严重错误",
					"description": "生产环境命名空间 {{.Namespace}} 的 Pod {{.PodName}} 产生严重错误",
					"labels": map[string]string{
						"category":    "production",
						"environment": "prod",
					},
				},
			}),
			Tags:      strings.Join([]string{"production", "critical", "fatal", "priority"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
		
		// 11. Disk Space Warning
		{
			Name:        "Disk-Space-Warning",
			Category:    "basic",
			Description: "磁盘空间不足警告",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{
					{
						"type":     "node",
						"key":      "node_name",
						"operator": "exists",
					},
				},
				"match_type": "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "disk space|no space left|disk full|insufficient disk",
					"ignore_case": true,
				},
				"severity": "warning",
				"alert_template": map[string]interface{}{
					"summary":     "磁盘空间不足",
					"description": "节点 {{.NodeName}} 的磁盘空间不足",
					"labels": map[string]string{
						"category":  "storage",
						"component": "disk",
					},
				},
			}),
			Tags:      strings.Join([]string{"storage", "disk", "space", "warning"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
		
		// 12. Connection Timeout
		{
			Name:        "Connection-Timeout",
			Category:    "network",
			Description: "连接超时检测",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{},
				"match_type": "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "connection timeout|timeout.*connect|timed out",
					"ignore_case": true,
				},
				"severity": "warning",
				"alert_template": map[string]interface{}{
					"summary":     "连接超时",
					"description": "Pod {{.PodName}} 出现连接超时",
					"labels": map[string]string{
						"category":  "network",
						"component": "connection",
					},
				},
			}),
			Tags:      strings.Join([]string{"network", "timeout", "connection", "warning"}, ","),
			IsBuiltin: true,
			CreatedBy: "system",
		},
	}
}

// buildTemplateConfig builds an ExtType from a map
func buildTemplateConfig(config map[string]interface{}) model.ExtType {
	configBytes, _ := json.Marshal(config)
	var ext model.ExtType
	json.Unmarshal(configBytes, &ext)
	return ext
}

