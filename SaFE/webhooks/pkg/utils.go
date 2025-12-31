/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	WebhookPathPrefix = "amd-primus-safe-v1-"
	DisplayNameRule   = "^[a-z][-a-z0-9\\.]{0,%d}[a-z0-9]$"
	DNSNameRule       = "^[a-z][-a-z0-9]{0,%d}[a-z0-9]$"
	LabelKeyRule      = "^[a-z0-9]([a-zA-Z0-9._-]{0,%d}[a-z0-9])?$"
	DisplayNamePrompt = "the name(%s) must consist of 1 to %d lower case alphanumeric characters or '-' or '.', " +
		"start with an alphabetic character, and end with an alphanumeric character"
	DNSNamePrompt = "the name(%s) must consist of 1 to %d lower case alphanumeric characters or '-', " +
		"start with an alphabetic character, and end with an alphanumeric character"
	LabelKeyPrompt = "the name(%s) must consist of 1 to %d alphanumeric characters or '-' or '.' or '_', " +
		"start and end with an alphanumeric character."
	MinPort = 1
	MaxPort = 65535
)

var (
	DisplayNameRegRule = fmt.Sprintf(DisplayNameRule, commonutils.MaxDisplayNameLen)
	DNSNameRegRule     = fmt.Sprintf(DNSNameRule, commonutils.MaxDisplayNameLen)
	LabelKeyRegRule    = fmt.Sprintf(LabelKeyRule, commonutils.MaxNameLength-1)

	DisplayNameRegexp = regexp.MustCompile(DisplayNameRegRule)
	DNSNameRegexp     = regexp.MustCompile(DNSNameRegRule)
	LabelKeyRegexp    = regexp.MustCompile(LabelKeyRegRule)
)

// generateMutatePath generates the mutation webhook path for a given resource kind.
func generateMutatePath(kind string) string {
	return "/mutate-" + WebhookPathPrefix + strings.ToLower(kind)
}

// generateValidatePath generates the validation webhook path for a given resource kind.
func generateValidatePath(kind string) string {
	return "/validate-" + WebhookPathPrefix + strings.ToLower(kind)
}

// handleError processes and logs errors, returning an appropriate response.
func handleError(name string, err error) admission.Response {
	if err == nil {
		return admission.Allowed("")
	}
	klog.ErrorS(err, fmt.Sprintf("failed to handle %s webhook", name))
	var apiError *apierrors.StatusError
	if !errors.As(err, &apiError) {
		apiError = commonerrors.NewBadRequest(err.Error())
	}
	return admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			Allowed: false,
			Result:  &apiError.ErrStatus,
		},
	}
}

// validateDisplayName validates a display name against the naming rules.
func validateDisplayName(name string) error {
	if name == "" {
		return nil
	}
	if !DisplayNameRegexp.MatchString(name) {
		return commonerrors.NewBadRequest(fmt.Sprintf(DisplayNamePrompt, name, commonutils.MaxDisplayNameLen))
	}
	return nil
}

// validateDNSName validates a DNS name against the naming rules.
func validateDNSName(name string) error {
	if name == "" {
		return nil
	}
	if !DNSNameRegexp.MatchString(name) {
		return commonerrors.NewBadRequest(fmt.Sprintf(DNSNamePrompt, name, commonutils.MaxDisplayNameLen))
	}
	return nil
}

// validateLabelKey validates a label key against the naming rules.
func validateLabelKey(name string) error {
	if name == "" {
		return nil
	}
	if !LabelKeyRegexp.MatchString(name) {
		return commonerrors.NewBadRequest(fmt.Sprintf(LabelKeyPrompt, name, commonutils.MaxNameLength))
	}
	return nil
}

// validateLabels ensures labels are valid.
// For keys containing '/', splits by '/' and validates each part separately.
func validateLabels(labels map[string]string) error {
	for key := range labels {
		// If key contains '/', split it and validate each part
		parts := strings.Split(key, "/")
		// Only allow one '/' - exactly 2 parts
		if len(parts) > 2 {
			return commonerrors.NewBadRequest(fmt.Sprintf(LabelKeyPrompt, key, commonutils.MaxNameLength))
		}
		for _, part := range parts {
			if validateLabelKey(part) != nil {
				return commonerrors.NewBadRequest(fmt.Sprintf(LabelKeyPrompt, key, commonutils.MaxNameLength))
			}
		}
	}
	return nil
}

// validatePort validates that a port number is within the valid range (1-65535).
func validatePort(name string, port int) error {
	if port < MinPort || port > MaxPort {
		return commonerrors.NewBadRequest(
			fmt.Sprintf("The %s port(%d) is invalid and needs to be between %d and %d",
				name, port, MinPort, MaxPort))
	}
	return nil
}
