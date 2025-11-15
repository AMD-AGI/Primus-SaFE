package database

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
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
			Description: "Generic error log detection, matches ERROR, FATAL, Exception and other keywords",
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
					"summary":     "Error log detected",
					"description": "Pod {{.PodName}} generated error log: {{.LogMessage}}",
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
			Description: "GPU out of memory detection",
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
					"summary":     "GPU out of memory",
					"description": "Pod {{.PodName}} of workload {{.WorkloadName}} experienced GPU OOM",
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
			Description: "GPU frequent OOM detection (more than 3 times in 10 minutes)",
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
					"summary":     "GPU frequent out of memory",
					"description": "Workload {{.WorkloadName}} experienced multiple GPU OOM in the past 10 minutes",
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
			Description: "NCCL communication error detection",
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
					"summary":     "NCCL communication error",
					"description": "Pod {{.PodName}} of workload {{.WorkloadName}} encountered NCCL error: {{.LogMessage}}",
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
			Description: "InfiniBand network error detection",
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
					"summary":     "InfiniBand network error",
					"description": "Node {{.NodeName}} encountered InfiniBand network error",
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
			Description: "Training loss NaN detection",
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
					"summary":     "Training loss anomaly",
					"description": "Workload {{.WorkloadName}} has training loss value of NaN",
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
			Description: "Training checkpoint save failure detection",
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
					"summary":     "Checkpoint save failed",
					"description": "Checkpoint save failed for workload {{.WorkloadName}}",
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
			Description: "Training throughput degradation detection (TFLOPS < 100)",
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
					"summary":     "Training throughput degradation",
					"description": "TFLOPS of workload {{.WorkloadName}} consistently below expected",
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
			Description: "Pod restart detection",
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
					"summary":     "Pod restarted",
					"description": "Pod {{.PodName}} restarted on node {{.NodeName}}",
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
			Description: "Production environment critical error detection",
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
					"summary":     "Production critical error",
					"description": "Pod {{.PodName}} in production namespace {{.Namespace}} generated critical error",
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
			Description: "Disk space insufficient warning",
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
					"summary":     "Disk space insufficient",
					"description": "Node {{.NodeName}} has insufficient disk space",
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
			Description: "Connection timeout detection",
			TemplateConfig: buildTemplateConfig(map[string]interface{}{
				"label_selectors": []map[string]interface{}{},
				"match_type":      "pattern",
				"match_config": map[string]interface{}{
					"pattern":     "connection timeout|timeout.*connect|timed out",
					"ignore_case": true,
				},
				"severity": "warning",
				"alert_template": map[string]interface{}{
					"summary":     "Connection timeout",
					"description": "Pod {{.PodName}} experienced connection timeout",
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
