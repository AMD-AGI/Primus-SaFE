/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	CICDProxySecretMissing     = "env.PROXY_CREDENTIAL_SECRET: referenced Secret does not exist; create it with the Secret API and bind it to this workspace"
	CICDProxySecretInvalid     = "env.PROXY_CREDENTIAL_SECRET: expected a non-deleting, workspace-bound general Secret with nonempty username and password keys"
	CICDProxySecretForbidden   = "env.PROXY_CREDENTIAL_SECRET: access to the referenced Secret is forbidden"
	CICDProxySecretUnavailable = "env.PROXY_CREDENTIAL_SECRET: unable to verify the referenced Secret; retry the request"
	invalidProxyURL            = "env.PROXY_URL: expected an absolute http or https URL with a host, optional port 1-65535, and no credentials, query, fragment, or non-root path"
)

type CICDProxyConfig struct {
	URL              string
	CredentialSecret string
	NoProxy          []string
}

func CICDProxyEnvKeys() []string {
	return []string{common.ProxyUrl, common.ProxyCredentialSecret, common.NoProxy}
}

func CICDProxyEnv(env map[string]string) (map[string]string, error) {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	result := make(map[string]string)
	for _, key := range keys {
		normalized := strings.TrimSpace(key)
		if !slices.Contains(CICDProxyEnvKeys(), normalized) {
			continue
		}
		if _, exists := result[normalized]; exists {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("env.%s: multiple keys normalize to the same reserved key", normalized))
		}
		result[normalized] = env[key]
	}
	return result, nil
}

func ParseCICDProxy(env map[string]string) (*CICDProxyConfig, error) {
	reserved, err := CICDProxyEnv(env)
	if err != nil {
		return nil, err
	}
	endpoint, secret := reserved[common.ProxyUrl], reserved[common.ProxyCredentialSecret]
	if endpoint == "" {
		if secret != "" {
			return nil, commonerrors.NewBadRequest("env.PROXY_CREDENTIAL_SECRET: requires a nonempty PROXY_URL")
		}
		return nil, nil
	}
	if err = validateCICDProxyURL(endpoint); err != nil {
		return nil, err
	}
	if secret != "" && len(validation.IsDNS1123Subdomain(secret)) != 0 {
		return nil, commonerrors.NewBadRequest("env.PROXY_CREDENTIAL_SECRET: expected a Kubernetes Secret name (DNS subdomain, at most 253 characters)")
	}
	config := &CICDProxyConfig{URL: endpoint, CredentialSecret: secret}
	for _, entry := range strings.Split(reserved[common.NoProxy], ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			config.NoProxy = append(config.NoProxy, entry)
		}
	}
	return config, nil
}

func validateCICDProxyURL(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return commonerrors.NewBadRequest(invalidProxyURL)
	}
	if parsed.User != nil {
		return commonerrors.NewBadRequest("env.PROXY_URL: userinfo is not allowed; use PROXY_CREDENTIAL_SECRET with a pre-existing Secret")
	}
	if strings.IndexFunc(endpoint, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.Opaque != "" ||
		(parsed.Path != "" && parsed.Path != "/") || strings.ContainsAny(endpoint, "?#") {
		return commonerrors.NewBadRequest(invalidProxyURL)
	}
	host := parsed.Hostname()
	if net.ParseIP(host) == nil && len(validation.IsDNS1123Subdomain(strings.ToLower(strings.TrimSuffix(host, ".")))) != 0 {
		return commonerrors.NewBadRequest(invalidProxyURL)
	}
	if (strings.Contains(host, ":") && !strings.HasPrefix(parsed.Host, "[")) ||
		(strings.HasPrefix(parsed.Host, "[") && net.ParseIP(host) == nil) {
		return commonerrors.NewBadRequest(invalidProxyURL)
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return commonerrors.NewBadRequest(invalidProxyURL)
		}
	} else if strings.HasSuffix(parsed.Host, ":") {
		return commonerrors.NewBadRequest(invalidProxyURL)
	}
	return nil
}

func ValidateCICDProxySecret(secret *corev1.Secret, workspace string) error {
	if secret == nil {
		return commonerrors.NewBadRequest(CICDProxySecretMissing)
	}
	var workspaces []string
	if !secret.DeletionTimestamp.IsZero() || secret.Type != corev1.SecretTypeOpaque ||
		v1.GetSecretType(secret) != string(v1.SecretGeneral) || len(secret.Data["username"]) == 0 || len(secret.Data["password"]) == 0 ||
		json.Unmarshal([]byte(v1.GetAnnotation(secret, v1.WorkspaceIdsAnnotation)), &workspaces) != nil ||
		workspace == "" || !slices.Contains(workspaces, workspace) {
		return commonerrors.NewBadRequest(CICDProxySecretInvalid)
	}
	return nil
}

func CICDProxySecretLookupError(err error) error {
	if apierrors.IsNotFound(err) || commonerrors.IsNotFound(err) {
		return commonerrors.NewBadRequest(CICDProxySecretMissing)
	}
	if commonerrors.IsInternal(err) {
		return commonerrors.NewInternalError(CICDProxySecretUnavailable)
	}
	return apierrors.NewServiceUnavailable(CICDProxySecretUnavailable)
}

func ReadCICDProxySecret(ctx context.Context, reader client.Reader, workload *v1.Workload, config *CICDProxyConfig) error {
	if config == nil || config.CredentialSecret == "" {
		return nil
	}
	if reader == nil {
		return commonerrors.NewInternalError(CICDProxySecretUnavailable)
	}
	secret := &corev1.Secret{}
	if err := reader.Get(ctx, client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: config.CredentialSecret}, secret); err != nil {
		return CICDProxySecretLookupError(err)
	}
	return ValidateCICDProxySecret(secret, workload.Spec.Workspace)
}

func IsCICDProxyManaged(workload *v1.Workload) bool {
	return workload != nil && v1.GetAnnotation(workload, v1.CICDProxyManagedAnnotation) == v1.TrueStr
}

func CICDProxyEnvChanged(oldWorkload, newWorkload *v1.Workload) bool {
	oldEnv, oldErr := CICDProxyEnv(oldWorkload.Spec.Env)
	newEnv, newErr := CICDProxyEnv(newWorkload.Spec.Env)
	return oldErr != nil || newErr != nil || !reflect.DeepEqual(oldEnv, newEnv)
}

func ExpectedCICDProxyOptIn(workload, oldWorkload *v1.Workload) bool {
	if !IsCICDScalingRunnerSet(workload) {
		return false
	}
	if oldWorkload != nil {
		return IsCICDProxyManaged(oldWorkload) ||
			(!reflect.DeepEqual(workload.Spec, oldWorkload.Spec) && CICDProxyEnvChanged(oldWorkload, workload))
	}
	env, _ := CICDProxyEnv(workload.Spec.Env)
	return len(env) > 0
}

func ResolveCICDProxySource(ctx context.Context, reader client.Reader, workload *v1.Workload) (*v1.Workload, error) {
	if IsCICDScalingRunnerSet(workload) {
		return workload, nil
	}
	if !IsCICDEphemeralRunner(workload) {
		return nil, nil
	}
	owner := metav1.GetControllerOf(workload)
	if owner == nil || owner.Kind != v1.WorkloadKind || owner.APIVersion != v1.SchemeGroupVersion.String() ||
		owner.UID == "" || owner.Name != workload.GetEnv(common.ScaleRunnerSetID) {
		return nil, commonerrors.NewBadRequest("env.SCALE_RUNNER_SET_ID: expected the owning scale set Workload controller reference")
	}
	parent := &v1.Workload{}
	if err := reader.Get(ctx, client.ObjectKey{Name: owner.Name}, parent); err != nil {
		return nil, fmt.Errorf("env.SCALE_RUNNER_SET_ID: unable to resolve the owning scale set Workload")
	}
	if !IsCICDScalingRunnerSet(parent) || parent.UID != owner.UID || parent.Spec.Workspace != workload.Spec.Workspace ||
		v1.GetClusterId(parent) != v1.GetClusterId(workload) {
		return nil, commonerrors.NewBadRequest("env.SCALE_RUNNER_SET_ID: owner kind, UID, workspace, and cluster must match the scale set Workload")
	}
	return parent, nil
}
