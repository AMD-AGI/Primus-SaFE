/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
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
	CICDProxySecretInvalid     = "env.PROXY_CREDENTIAL_SECRET: expected a non-deleting, workspace-bound general Secret with nonempty username and password keys containing no control characters"
	CICDProxySecretForbidden   = "env.PROXY_CREDENTIAL_SECRET: access to the referenced Secret is forbidden"
	CICDProxySecretUnavailable = "env.PROXY_CREDENTIAL_SECRET: unable to verify the referenced Secret; retry the request"
	invalidProxyURL            = "env.PROXY_URL: expected an absolute http URL with a host, an explicit port 1-65535, and no credentials, query, fragment, or non-root path"
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

// reservedCICDProxyNoProxy lists what a runner reaches without leaving the
// cluster: the in-cluster service domains, and the SaFE control plane the
// runner proxy calls back to create its EphemeralRunner. Routing either through
// an external proxy strands the runner with no runnable job.
func reservedCICDProxyNoProxy(workload *v1.Workload) []string {
	entries := []string{"localhost", "127.0.0.1", "::1", ".svc", ".cluster.local"}
	if host := v1.GetAdminControlPlane(workload); host != "" {
		entries = append(entries, host)
	}
	if host := os.Getenv("KUBERNETES_SERVICE_HOST"); host != "" {
		entries = append(entries, host)
	}
	return entries
}

// CICDProxyNoProxy combines the reserved entries, the cluster-wide default and
// the workload's own list, keeping first-seen order and dropping repeats. The
// workload cannot opt out of the reserved entries.
func CICDProxyNoProxy(workload *v1.Workload, clusterDefault string, userEntries []string) []string {
	merged := make([]string, 0, len(userEntries)+8)
	seen := make(map[string]struct{})
	add := func(entry string) {
		if entry = strings.TrimSpace(entry); entry == "" {
			return
		}
		if _, exists := seen[entry]; exists {
			return
		}
		seen[entry] = struct{}{}
		merged = append(merged, entry)
	}
	for _, entry := range reservedCICDProxyNoProxy(workload) {
		add(entry)
	}
	for _, entry := range strings.Split(clusterDefault, ",") {
		add(entry)
	}
	for _, entry := range userEntries {
		add(entry)
	}
	return merged
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
		parsed.Scheme != "http" || parsed.Hostname() == "" || parsed.Opaque != "" ||
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
	// The port is mandatory. This address is a forward proxy, not an origin server,
	// so there is no well-known port to fall back on: 80 and 3128 are both guesses,
	// and a wrong one strands the relay behind a peer that never answers. Reject it
	// here so the operator sees an admission error instead of a runner that fails to
	// reach anything.
	port := parsed.Port()
	if port == "" {
		return commonerrors.NewBadRequest(invalidProxyURL)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
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
	for _, key := range []string{"username", "password"} {
		if bytes.IndexFunc(secret.Data[key], unicode.IsControl) >= 0 {
			return commonerrors.NewBadRequest(fmt.Sprintf("%s: %s key contains a control character", CICDProxySecretInvalid, key))
		}
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

// IsCICDProxyRoot reports whether a workload is a CICD runner that carries the
// proxy configuration itself, as opposed to one that inherits it from an owner.
// A new runner kind joins the proxy by being named here; the relay and the
// credential handling below read this rather than any one kind.
func IsCICDProxyRoot(workload *v1.Workload) bool {
	return IsCICDScalingRunnerSet(workload) || IsCICDGithubRunner(workload)
}

// ExpectedCICDProxyOptIn reports whether the proxy marker belongs on a workload.
//
// Opt-in is one-way for the life of a Workload: once the marker is stamped this
// returns true even after every proxy env var is cleared. That is deliberate. The
// reconciler owns the ARC object's spec.proxy, its relay container and the proxy
// env on the proxied containers, and every teardown path in
// job-manager/pkg/dispatcher/cicd_proxy.go is gated on IsCICDProxyManaged(source).
// Letting the marker fall back to legacy the moment the env goes away would strand
// all of that on the ARC object with nothing left to remove it. Clearing the env
// instead drives a managed teardown; a genuinely unproxied scale set is expressed
// by creating a new Workload without proxy env, which takes the oldWorkload == nil
// branch below.
func ExpectedCICDProxyOptIn(workload, oldWorkload *v1.Workload) bool {
	if !IsCICDProxyRoot(workload) {
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
	if IsCICDProxyRoot(workload) {
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
