/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import "strings"

// Waiting and rejection reasons job-manager writes for workloads in an external workspace.
// A reason may be followed by " - " and details.
const (
	ExternalCapacityReason    = "In queue - waiting for external capacity"
	ExternalProfileReason     = "In queue - execution profile is not validated"
	ExternalConstraintReason  = "Rejected - constraints cannot be satisfied by external capacity"
	ExternalUnsupportedReason = "Rejected - workload shape is not supported by external capacity"
	ExternalInvalidReason     = "Rejected - request refused by external capacity"
	// ExternalBudgetMissingReason is terminal when the request names a resource the
	// external-budget quota does not declare (for example rdma/hca or ephemeral-storage).
	ExternalBudgetMissingReason = "Rejected - external budget does not include required resource"
	ExternalUnavailableReason   = "In queue - external capacity service is unavailable"
	ExternalRateLimitedReason   = "In queue - external capacity controller rate limited"
	ExternalAuthReason          = "In queue - not authorized by external capacity"

	// ExternalWaitingScaleUpReason is shown while a single Pod waits for B to provision.
	ExternalWaitingScaleUpReason = "In queue - waiting for scale-up"
	// ExternalWaitingPRAcceptReason is shown when the PR has no conditions yet.
	ExternalWaitingPRAcceptReason = "In queue - waiting for capacity service to accept"
	// ExternalWaitingPRReason is shown while a gang waits for Provisioned=True.
	ExternalWaitingPRReason = "In queue - waiting for capacity service"
	// ExternalCapacityUnavailableReason is shown for Provisioned=False/CapacityUnavailable.
	ExternalCapacityUnavailableReason = "In queue - external capacity unavailable"
	// ExternalPRFailedReason prefixes a terminal PR Failed condition.
	ExternalPRFailedReason = "Rejected - provisioning request failed"
	// ExternalPRExpiredReason is shown briefly while a booking is rebuilt.
	ExternalPRExpiredReason = "In queue - capacity reservation expired, retrying"
)

// externalWaitReasons are the external reasons of a workload that still waits for capacity
// and gets it without anyone acting. Authorization, profile validation and rejections are not.
var externalWaitReasons = []string{
	ExternalWaitingScaleUpReason,
	ExternalCapacityReason,
	ExternalWaitingPRAcceptReason,
	ExternalWaitingPRReason,
	ExternalCapacityUnavailableReason,
	ExternalUnavailableReason,
	ExternalRateLimitedReason,
	ExternalPRExpiredReason,
}

// IsExternalWaitReason reports whether message starts with an external reason under which the
// workload waits for external capacity.
func IsExternalWaitReason(message string) bool {
	for _, reason := range externalWaitReasons {
		if strings.HasPrefix(message, reason) {
			return true
		}
	}
	return false
}
