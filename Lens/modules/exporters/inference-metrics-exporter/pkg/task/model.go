package task

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// ScrapeTaskExt represents the ext field structure for inference scrape tasks
type ScrapeTaskExt struct {
	// Framework identification
	Framework string `json:"framework"`

	// Pod information
	Namespace string `json:"namespace"`
	PodName   string `json:"pod_name"`
	PodIP     string `json:"pod_ip"`

	// Metrics endpoint configuration
	MetricsPort int    `json:"metrics_port"`
	MetricsPath string `json:"metrics_path"`

	// Scrape configuration
	ScrapeInterval int `json:"scrape_interval"` // seconds
	ScrapeTimeout  int `json:"scrape_timeout"`  // seconds

	// Workload labels for metric enrichment
	Labels map[string]string `json:"labels"`

	// Scrape statistics
	LastScrapeAt    *time.Time `json:"last_scrape_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	ScrapeCount     int64      `json:"scrape_count"`
	ErrorCount      int64      `json:"error_count"`
	ConsecutiveErrs int        `json:"consecutive_errors"`
}

// ScrapeTask represents a complete scrape task with parsed ext field
type ScrapeTask struct {
	WorkloadUID string
	TaskType    string
	Status      string
	LockOwner   string
	LockVersion int64

	// Parsed from ext field
	Ext ScrapeTaskExt

	// Timestamps
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FromModel converts a WorkloadTaskState model to ScrapeTask
func FromModel(m *model.WorkloadTaskState) (*ScrapeTask, error) {
	if m == nil {
		return nil, nil
	}

	task := &ScrapeTask{
		WorkloadUID: m.WorkloadUID,
		TaskType:    m.TaskType,
		Status:      m.Status,
		LockOwner:   m.LockOwner,
		LockVersion: m.LockVersion,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}

	// Parse ext field
	if m.Ext != nil {
		extBytes, err := json.Marshal(m.Ext)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(extBytes, &task.Ext); err != nil {
			return nil, err
		}
	}

	// Set defaults
	if task.Ext.MetricsPath == "" {
		task.Ext.MetricsPath = "/metrics"
	}
	if task.Ext.ScrapeInterval == 0 {
		task.Ext.ScrapeInterval = 15
	}
	if task.Ext.ScrapeTimeout == 0 {
		task.Ext.ScrapeTimeout = 10
	}

	return task, nil
}

// ToExtMap converts ScrapeTaskExt to a map for database storage
func (e *ScrapeTaskExt) ToExtMap() model.ExtType {
	extBytes, _ := json.Marshal(e)
	var extMap model.ExtType
	json.Unmarshal(extBytes, &extMap)
	return extMap
}

// GetMetricsURL returns the full metrics URL for scraping
func (t *ScrapeTask) GetMetricsURL() string {
	if t.Ext.PodIP == "" || t.Ext.MetricsPort == 0 {
		return ""
	}
	return "http://" + t.Ext.PodIP + ":" + strconv.Itoa(t.Ext.MetricsPort) + t.Ext.MetricsPath
}

// GetScrapeInterval returns the scrape interval as duration
func (t *ScrapeTask) GetScrapeInterval() time.Duration {
	if t.Ext.ScrapeInterval <= 0 {
		return 15 * time.Second
	}
	return time.Duration(t.Ext.ScrapeInterval) * time.Second
}

// GetScrapeTimeout returns the scrape timeout as duration
func (t *ScrapeTask) GetScrapeTimeout() time.Duration {
	if t.Ext.ScrapeTimeout <= 0 {
		return 10 * time.Second
	}
	return time.Duration(t.Ext.ScrapeTimeout) * time.Second
}
