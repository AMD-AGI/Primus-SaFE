/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	WebhookPathPrefix = "amd-primus-safe-v1-"
	DisplayNameRule   = "^[a-z][-a-z0-9\\.]{0,%d}[a-z0-9]$"
	DNSNameRule       = "^[a-z][-a-z0-9]{0,%d}[a-z0-9]$"
	DisplayNamePrompt = "the name must consist of 1 to %d lower case alphanumeric characters or '-' or '.'" +
		", start with an alphabetic character, and end with an alphanumeric character"
	DNSNamePrompt = "the name must consist of 1 to %d lower case alphanumeric characters or '-', " +
		"start with an alphabetic character, and end with an alphanumeric character"

	MinPort = 1
	MaxPort = 65535
)

var (
	DisplayNameRegRule = fmt.Sprintf(DisplayNameRule, commonutils.MaxDisplayNameLen-2)
	DNSNameRegRule     = fmt.Sprintf(DNSNameRule, commonutils.MaxDisplayNameLen-2)

	DisplayNameRegexp = regexp.MustCompile(DisplayNameRegRule)
	DNSNameRegexp     = regexp.MustCompile(DNSNameRegRule)
)

func generateMutatePath(kind string) string {
	return "/mutate-" + WebhookPathPrefix + strings.ToLower(kind)
}

func generateValidatePath(kind string) string {
	return "/validate-" + WebhookPathPrefix + strings.ToLower(kind)
}

func handleError(name string, err error) admission.Response {
	if err == nil {
		return admission.Allowed("")
	}
	klog.ErrorS(err, fmt.Sprintf("failed to handle %s webhook", name))
	var apiStatus *apierrors.StatusError
	if !errors.As(err, &apiStatus) {
		apiStatus = commonerrors.NewBadRequest(err.Error())
	}
	return admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			Allowed: false,
			Result:  &apiStatus.ErrStatus,
		},
	}
}

func validateDisplayName(name string) error {
	if name == "" {
		return nil
	}
	if !DisplayNameRegexp.MatchString(name) {
		return commonerrors.NewBadRequest(fmt.Sprintf(DisplayNamePrompt, commonutils.MaxDisplayNameLen))
	}
	return nil
}

func validateDNSName(name string) error {
	if name == "" {
		return nil
	}
	if !DNSNameRegexp.MatchString(name) {
		return commonerrors.NewBadRequest(fmt.Sprintf(DNSNamePrompt, commonutils.MaxDisplayNameLen))
	}
	return nil
}

func validatePort(name string, port int) error {
	if port < MinPort || port > MaxPort {
		return commonerrors.NewBadRequest(
			fmt.Sprintf("The %s port(%d) is invalid and needs to be between %d and %d",
				name, port, MinPort, MaxPort))
	}
	return nil
}

func hasOwnerReferences(obj metav1.Object, name string) bool {
	for _, r := range obj.GetOwnerReferences() {
		if r.Name == name {
			return true
		}
	}
	return false
}
