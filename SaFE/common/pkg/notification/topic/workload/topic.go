/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	sliceutil "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// ConditionLowUtilization announces that a running workload will be stopped
// for low GPU utilization.
const ConditionLowUtilization = "LowUtilization"

type Topic struct {
}

// Name returns the name of the client factory.
func (t *Topic) Name() string {
	return model.TopicWorkload
}

// Filter determines if the notification should be sent based on the workload phase.
// Only meaningful phase transitions produce notifications; intermediate phases are dropped.
func (t *Topic) Filter(data map[string]interface{}) bool {
	phase, ok := data["condition"].(string)
	if !ok || phase == "" {
		return false
	}
	notifyPhases := []string{
		string(v1.WorkloadRunning),
		string(v1.WorkloadSucceeded),
		string(v1.WorkloadFailed),
		string(v1.WorkloadStopped),
		ConditionLowUtilization,
	}
	return sliceutil.Contains(notifyPhases, phase)
}

// BuildMessage constructs a notification message from the provided data.
func (t *Topic) BuildMessage(ctx context.Context, data map[string]interface{}) ([]*model.Message, error) {
	topicData := &TopicData{}
	err := commonutils.TransMapToStruct(data, topicData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert data to TopicData: %w", err)
	}
	emailData := EmailData{
		JobName:      topicData.Workload.Name,
		Status:       topicData.Condition,
		StatusColor:  getStatusColor(topicData.Condition),
		ScheduleTime: topicData.Workload.CreationTimestamp.Time.Format(time.DateTime),
		JobURL:       getWorkloadUrl(topicData.Workload.Name),
	}
	switch topicData.Condition {
	case string(v1.WorkloadFailed):
		emailData.ErrorMessage = topicData.Message
	case string(v1.WorkloadStopped):
		emailData.StopReason = topicData.Message
	case ConditionLowUtilization:
		if notice := topicData.LowUtilization; notice != nil {
			emailData.LowUtilThreshold = fmt.Sprintf("%d%%", notice.Threshold)
			emailData.LowUtilWindow = notice.Window
			emailData.LowUtilRules = notice.Rules
			emailData.PlannedStopTime = notice.StopTime
		}
	}
	emailContent, err := renderEmailTemplate(emailData)
	if err != nil {
		return nil, fmt.Errorf("failed to render email template: %w", err)
	}
	message := &model.Message{
		Email: &model.EmailMessage{
			Title:   fmt.Sprintf("【Primus Safe】Workload %s ( %s )", topicData.Condition, topicData.Workload.Name),
			Content: emailContent,
			To:      extractUserEmails(topicData.Users),
		},
	}
	if len(message.Email.To) == 0 {
		klog.Warningf("No email recipients found for workload %s", topicData.Workload.Name)
		return nil, nil
	}
	return []*model.Message{message}, nil
}

type TopicData struct {
	Workload  *v1.Workload `json:"workload,omitempty" yaml:"workload,omitempty"`
	Condition string       `json:"condition,omitempty" yaml:"condition,omitempty"`
	Message   string       `json:"message,omitempty" yaml:"message,omitempty"`
	Users     []*v1.User   `json:"users,omitempty" yaml:"users,omitempty"`
	// Set with ConditionLowUtilization.
	LowUtilization *LowUtilizationNotice `json:"lowUtilization,omitempty" yaml:"lowUtilization,omitempty"`
}

// LowUtilizationNotice describes a planned stop for low GPU utilization.
type LowUtilizationNotice struct {
	// GPU utilization threshold in percent.
	Threshold int    `json:"threshold" yaml:"threshold"`
	Window    string `json:"window" yaml:"window"`
	StopTime  string `json:"stopTime" yaml:"stopTime"`
	// One description per idle rule; when set it is shown instead of Threshold and Window.
	Rules []string `json:"rules,omitempty" yaml:"rules,omitempty"`
}

type EmailData struct {
	JobName      string
	JobID        string
	Status       string
	StatusColor  string
	ScheduleTime string
	ErrorMessage string
	StopReason   string
	JobURL       string

	LowUtilThreshold string
	LowUtilWindow    string
	LowUtilRules     []string
	PlannedStopTime  string
}

// renderEmailTemplate renders the HTML email template using Go's html/template.
func renderEmailTemplate(data EmailData) (string, error) {
	tmpl, err := template.New("email_template").Parse(emailTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse email template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render email template: %w", err)
	}

	return buf.String(), nil
}

func getStatusColor(phase string) string {
	switch v1.WorkloadPhase(phase) {
	case v1.WorkloadFailed:
		return "#c53030" // red
	case v1.WorkloadSucceeded:
		return "#2f855a" // green
	case v1.WorkloadRunning:
		return "#3182ce" // blue
	case v1.WorkloadStopped:
		return "#d69e2e" // yellow
	case ConditionLowUtilization:
		return "#dd6b20" // orange
	default:
		return "#4a5568" // gray
	}
}

func extractUserEmails(users []*v1.User) []string {
	emails := []string{}
	for _, user := range users {
		email := v1.GetUserEmail(user)
		if email != "" {
			emails = append(emails, email)
		}
	}
	return emails
}

func getWorkloadUrl(workloadId string) string {
	return fmt.Sprintf("%s/training/detail?id=%s", commonconfig.GetSystemHost(), workloadId)
}
