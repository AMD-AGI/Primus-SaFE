/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"

	sliceutil "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

type Topic struct {
}

// Name returns the name of the client factory.
func (t *Topic) Name() string {
	return model.TopicWorkload
}

// Filter determines if the notification should be sent based on the data.
func (t *Topic) Filter(data map[string]interface{}) bool {
	if condition, ok := data["condition"].(string); ok {
		targetConditions := []string{
			string(v1.AdminScheduled),
			string(v1.K8sPending),
			string(v1.K8sUpdating),
			string(v1.K8sDeleted),
		}
		if !sliceutil.Contains(targetConditions, condition) {
			return true
		}
	}
	return false
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
	targetConditions := []string{string(v1.K8sFailed), string(v1.AdminFailed), string(v1.AdminFailover)}
	if sliceutil.Contains(targetConditions, topicData.Condition) {
		emailData.ErrorMessage = topicData.Message
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
}

type EmailData struct {
	JobName      string
	JobID        string
	Status       string
	StatusColor  string
	ScheduleTime string
	ErrorMessage string
	JobURL       string
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

func getStatusColor(status string) string {
	switch strings.ToLower(status) {
	case string(v1.K8sFailed), string(v1.AdminFailed), string(v1.AdminFailover):
		return "#c53030" // red
	case string(v1.K8sSucceeded):
		return "#2f855a" // green
	case string(v1.AdminDispatched):
		return "#3182ce" // blue
	case string(v1.K8sPending):
		return "#d69e2e" // yellow
	default:
		return "#4a5568" // gray (unknown)
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
