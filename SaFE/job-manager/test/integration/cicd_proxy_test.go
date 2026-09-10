//go:build integration

/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type proxyDeployment struct {
	APIURL          string `json:"apiURL"`
	AuthFile        string `json:"authFile"`
	Workspace       string `json:"workspace"`
	AdminKubeconfig string `json:"adminKubeconfig"`
	DataKubeconfig  string `json:"dataKubeconfig"`
}

type proxyAcceptanceConfig struct {
	WithLogs            proxyDeployment `json:"withLogs"`
	WithoutLogs         proxyDeployment `json:"withoutLogs"`
	ScaleSetRequestFile string          `json:"scaleSetRequestFile"`
	ChildRequestFile    string          `json:"childRequestFile"`
	ReachableProxyURL   string          `json:"reachableProxyURL"`
	UnreachableProxyURL string          `json:"unreachableProxyURL"`
	CredentialSecret    string          `json:"credentialSecret"`
}

type proxyAcceptance struct {
	config     proxyAcceptanceConfig
	deployment proxyDeployment
	admin      client.Client
	data       dynamic.Interface
	token      string
	http       *http.Client
}

func readAcceptanceJSON(t *testing.T, path string, result interface{}) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("acceptance configuration file is missing or unreadable")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		t.Fatal("acceptance configuration is invalid JSON")
	}
}

func acceptanceKubeconfig(t *testing.T, path string) *rest.Config {
	t.Helper()
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path != "" {
		rules.ExplicitPath = path
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		t.Fatal("acceptance Kubernetes configuration is unavailable")
	}
	config.Timeout = 10 * time.Second
	return config
}

func newProxyAcceptance(t *testing.T, withLogs bool) *proxyAcceptance {
	t.Helper()
	path := os.Getenv("CICD_PROXY_ACCEPTANCE_CONFIG")
	if path == "" {
		t.Fatal("CICD_PROXY_ACCEPTANCE_CONFIG must reference an isolated acceptance deployment configuration")
	}
	result := &proxyAcceptance{http: &http.Client{Timeout: 15 * time.Second}}
	readAcceptanceJSON(t, path, &result.config)
	result.deployment = result.config.WithoutLogs
	if withLogs {
		result.deployment = result.config.WithLogs
	}
	if result.deployment.Workspace == "" || result.deployment.APIURL == "" || result.config.CredentialSecret == "" {
		t.Fatal("workspace, apiURL, and credentialSecret are required")
	}
	for _, endpoint := range []string{result.config.ReachableProxyURL, result.config.UnreachableProxyURL} {
		config, err := commonworkload.ParseCICDProxy(map[string]string{common.ProxyUrl: endpoint, common.ProxyCredentialSecret: result.config.CredentialSecret})
		if err != nil || config == nil {
			t.Fatal("acceptance proxy endpoints must be valid and contain no credentials")
		}
	}
	token, err := os.ReadFile(result.deployment.AuthFile)
	if err != nil || strings.TrimSpace(string(token)) == "" {
		t.Fatal("authFile must contain a test API bearer token")
	}
	result.token = strings.TrimSpace(string(token))
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	result.admin, err = client.New(acceptanceKubeconfig(t, result.deployment.AdminKubeconfig), client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal("admin client initialization failed")
	}
	result.data, err = dynamic.NewForConfig(acceptanceKubeconfig(t, result.deployment.DataKubeconfig))
	if err != nil {
		t.Fatal("data client initialization failed")
	}
	return result
}

func (a *proxyAcceptance) request(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("acceptance request encoding failed")
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(a.deployment.APIURL, "/")+path, bytes.NewReader(data))
	if err != nil {
		return nil, 0, fmt.Errorf("acceptance apiURL is invalid")
	}
	request.Header.Set("Authorization", "Bearer "+a.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := a.http.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("acceptance API request failed")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	return raw, response.StatusCode, err
}

func (a *proxyAcceptance) create(t *testing.T, kind, endpoint string, parent *v1.Workload) *v1.Workload {
	t.Helper()
	file := a.config.ScaleSetRequestFile
	if parent != nil {
		file = a.config.ChildRequestFile
	}
	request := map[string]interface{}{}
	readAcceptanceJSON(t, file, &request)
	env, ok := request["env"].(map[string]interface{})
	if !ok {
		t.Fatal("acceptance request fixture must contain env")
	}
	for _, key := range commonworkload.CICDProxyEnvKeys() {
		delete(env, key)
	}
	request["kind"], request["version"], request["workspace"], request["workspaceId"] = kind, "v1", a.deployment.Workspace, a.deployment.Workspace
	request["workloadId"] = fmt.Sprintf("proxy-acceptance-%d", time.Now().UnixNano())
	request["displayName"], request["maxRetry"], request["ttlSecondsAfterFinished"] = "Proxy acceptance", 0, 3600
	if parent == nil {
		env[common.ProxyUrl], env[common.ProxyCredentialSecret] = endpoint, a.config.CredentialSecret
	} else {
		env[common.ScaleRunnerSetID] = parent.Name
		env[common.GithubConfigUrl] = parent.Spec.Env[common.GithubConfigUrl]
		env["GITHUB_SECRET_ID"] = v1.GetGithubSecretId(parent)
	}
	raw, status, err := a.request(t.Context(), http.MethodPost, "/workloads", request)
	if err != nil || status != http.StatusOK {
		t.Fatalf("acceptance workload creation failed (HTTP %d)", status)
	}
	var created struct {
		WorkloadID string `json:"workloadId"`
	}
	if json.Unmarshal(raw, &created) != nil || created.WorkloadID == "" {
		t.Fatal("workload creation returned no workloadId")
	}
	workload := &v1.Workload{}
	if err := a.admin.Get(t.Context(), client.ObjectKey{Name: created.WorkloadID}, workload); err != nil {
		t.Fatal("created workload could not be read")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		current := &v1.Workload{}
		if err := a.admin.Get(ctx, client.ObjectKeyFromObject(workload), current); apierrors.IsNotFound(err) {
			return
		} else if err != nil {
			t.Error("acceptance cleanup could not read workload")
			return
		}
		if current.UID != workload.UID {
			t.Error("acceptance workload identity changed before cleanup")
			return
		}
		if err := a.admin.Delete(ctx, current); client.IgnoreNotFound(err) != nil {
			t.Error("acceptance workload cleanup failed")
		}
	})
	return workload
}

func waitForAcceptance(t *testing.T, limit time.Duration, condition func(context.Context) bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), limit)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if condition(ctx) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("acceptance condition did not converge before its deadline")
		case <-ticker.C:
		}
	}
}

func (a *proxyAcceptance) registeredPair(t *testing.T) (*v1.Workload, *v1.Workload) {
	t.Helper()
	parent := a.create(t, common.CICDScaleRunnerSetKind, a.config.ReachableProxyURL, nil)
	waitForAcceptance(t, 11*time.Minute, func(ctx context.Context) bool {
		return a.admin.Get(ctx, client.ObjectKeyFromObject(parent), parent) == nil && parent.Status.RunnerScaleSetId != ""
	})
	child := a.create(t, common.CICDEphemeralRunnerKind, "", parent)
	waitForAcceptance(t, 11*time.Minute, func(ctx context.Context) bool {
		obj, err := a.arcObject(ctx, child)
		if err != nil {
			return false
		}
		id, _, err := unstructured.NestedInt64(obj.Object, "status", "runnerId")
		return err == nil && id > 0
	})
	a.expectProxy(t, parent, a.config.ReachableProxyURL, a.config.CredentialSecret)
	a.expectProxy(t, child, a.config.ReachableProxyURL, a.config.CredentialSecret)
	return parent, child
}

func (a *proxyAcceptance) arcObject(ctx context.Context, w *v1.Workload) (*unstructured.Unstructured, error) {
	resource := "autoscalingrunnersets"
	if w.SpecKind() == common.CICDEphemeralRunnerKind {
		resource = "ephemeralrunners"
	}
	return a.data.Resource(schema.GroupVersionResource{Group: "actions.github.com", Version: "v1alpha1", Resource: resource}).Namespace(w.Spec.Workspace).Get(ctx, w.Name, metav1.GetOptions{})
}

func (a *proxyAcceptance) expectProxy(t *testing.T, w *v1.Workload, endpoint, secret string) {
	t.Helper()
	waitForAcceptance(t, time.Minute, func(ctx context.Context) bool {
		obj, err := a.arcObject(ctx, w)
		if err != nil {
			return false
		}
		proxy, exists, err := unstructured.NestedMap(obj.Object, "spec", "proxy")
		if err != nil {
			return false
		}
		if endpoint == "" {
			return !exists && !v1.HasAnnotation(obj, v1.CICDProxyManagedAnnotation)
		}
		expected := map[string]interface{}{}
		for _, protocol := range []string{"http", "https"} {
			entry := map[string]interface{}{"url": endpoint}
			if secret != "" {
				entry["credentialSecretRef"] = secret
			}
			expected[protocol] = entry
		}
		return reflect.DeepEqual(proxy, expected) && v1.GetAnnotation(obj, v1.CICDProxyManagedAnnotation) == v1.TrueStr
	})
}

func (a *proxyAcceptance) patchProxy(t *testing.T, w *v1.Workload, endpoint, secret string) {
	t.Helper()
	if err := a.admin.Get(t.Context(), client.ObjectKeyFromObject(w), w); err != nil {
		t.Fatal("parent workload lookup failed")
	}
	env := w.DeepCopy().Spec.Env
	for _, key := range commonworkload.CICDProxyEnvKeys() {
		delete(env, key)
	}
	if endpoint != "" {
		env[common.ProxyUrl] = endpoint
	}
	if secret != "" {
		env[common.ProxyCredentialSecret] = secret
	}
	_, status, err := a.request(t.Context(), http.MethodPatch, "/workloads/"+url.PathEscape(w.Name), map[string]interface{}{"env": env})
	if err != nil || status != http.StatusOK {
		t.Fatalf("parent proxy update failed (HTTP %d)", status)
	}
}

func (a *proxyAcceptance) failed(t *testing.T, w *v1.Workload) {
	t.Helper()
	waitForAcceptance(t, 11*time.Minute, func(ctx context.Context) bool {
		return a.admin.Get(ctx, client.ObjectKeyFromObject(w), w) == nil && w.Status.Phase == v1.WorkloadFailed
	})
	if strings.TrimSpace(w.Status.Message) == "" || commonworkload.GetWorkloadFailureMessage(w.Status.Conditions, v1.GetWorkloadDispatchCnt(w)) != w.Status.Message {
		t.Fatal("Failed workload lacks a consistent persisted diagnostic")
	}
	if !strings.Contains(w.Status.Message, "registration timed out after 10m0s") || !strings.Contains(w.Status.Message, "Proxy configuration is enabled") {
		t.Fatal("registration timeout lacks its timeout or proxy diagnostic")
	}
}

func (a *proxyAcceptance) expectAPIMessage(t *testing.T, w *v1.Workload) {
	t.Helper()
	waitForAcceptance(t, 2*time.Minute, func(ctx context.Context) bool {
		raw, status, err := a.request(ctx, http.MethodGet, "/workloads/"+url.PathEscape(w.Name), nil)
		if err != nil || status != http.StatusOK {
			return false
		}
		var detail struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &detail) != nil || detail.Message != w.Status.Message {
			return false
		}
		raw, status, err = a.request(ctx, http.MethodGet, "/workloads?workloadId="+url.QueryEscape(w.Name)+"&workspaceId="+url.QueryEscape(w.Spec.Workspace), nil)
		if err != nil || status != http.StatusOK {
			return false
		}
		var list struct {
			Items []struct {
				WorkloadID string `json:"workloadId"`
				Message    string `json:"message"`
			} `json:"items"`
		}
		if json.Unmarshal(raw, &list) != nil {
			return false
		}
		for _, item := range list.Items {
			if item.WorkloadID == w.Name {
				return item.Message == w.Status.Message
			}
		}
		return false
	})
}

func TestCICDProxy_ReachableRegistration(t *testing.T) {
	acceptance := newProxyAcceptance(t, true)
	acceptance.registeredPair(t)
}

func TestCICDProxy_UnreachableRegistration(t *testing.T) {
	acceptance := newProxyAcceptance(t, false)
	workload := acceptance.create(t, common.CICDScaleRunnerSetKind, acceptance.config.UnreachableProxyURL, nil)
	raw, status, err := acceptance.request(t.Context(), http.MethodPost, "/workloads/"+url.PathEscape(workload.Name)+"/arclogs", map[string]interface{}{})
	if err != nil || status != http.StatusInternalServerError || !bytes.Contains(raw, []byte("The logging function is not enabled")) {
		t.Fatal("withoutLogs deployment must have OpenSearch disabled")
	}
	acceptance.failed(t, workload)
	if strings.Contains(workload.Status.Message, " ARC controller: ") {
		t.Fatal("disabled log backend unexpectedly enriched the diagnostic")
	}
	acceptance.expectAPIMessage(t, workload)
}

func TestCICDProxy_LiveUpdateAndClear(t *testing.T) {
	acceptance := newProxyAcceptance(t, true)
	parent, child := acceptance.registeredPair(t)
	for _, change := range [][2]string{{acceptance.config.UnreachableProxyURL, acceptance.config.CredentialSecret}, {acceptance.config.UnreachableProxyURL, ""}, {"", ""}} {
		acceptance.patchProxy(t, parent, change[0], change[1])
		acceptance.expectProxy(t, parent, change[0], change[1])
		acceptance.expectProxy(t, child, change[0], change[1])
	}
	if err := acceptance.admin.Get(t.Context(), client.ObjectKeyFromObject(parent), parent); err != nil {
		t.Fatal("cleared parent lookup failed")
	}
	if !commonworkload.IsCICDProxyManaged(parent) {
		t.Fatal("proxy removal erased durable admission opt-in")
	}
}

func TestCICDProxy_ControllerLogEnrichment(t *testing.T) {
	acceptance := newProxyAcceptance(t, true)
	workload := acceptance.create(t, common.CICDScaleRunnerSetKind, acceptance.config.UnreachableProxyURL, nil)
	acceptance.failed(t, workload)
	waitForAcceptance(t, time.Minute, func(ctx context.Context) bool {
		return acceptance.admin.Get(ctx, client.ObjectKeyFromObject(workload), workload) == nil && strings.Contains(workload.Status.Message, " ARC controller: ")
	})
	acceptance.expectAPIMessage(t, workload)
}

func TestARCStatusContract_FailureMapping(t *testing.T) {
	data, err := dynamic.NewForConfig(acceptanceKubeconfig(t, os.Getenv("CICD_PROXY_SCHEMA_KUBECONFIG")))
	if err != nil {
		t.Fatal("schema client initialization failed")
	}
	for _, resource := range []string{"autoscalingrunnersets", "ephemeralrunners"} {
		crd, err := data.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}).Get(t.Context(), resource+".actions.github.com", metav1.GetOptions{})
		if err != nil {
			t.Fatal("installed ARC CRD could not be read")
		}
		versions, _, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
		if err != nil {
			t.Fatal("installed ARC CRD has unreadable versions")
		}
		found := false
		for _, entry := range versions {
			version, ok := entry.(map[string]interface{})
			if !ok || version["name"] != "v1alpha1" || version["served"] != true {
				continue
			}
			found = true
			properties, _, err := unstructured.NestedMap(version, "schema", "openAPIV3Schema", "properties")
			if err != nil {
				t.Fatal("ARC schema properties are unreadable")
			}
			for _, protocol := range []string{"http", "https"} {
				for _, field := range []string{"url", "credentialSecretRef"} {
					kind, exists, err := unstructured.NestedString(properties, "spec", "properties", "proxy", "properties", protocol, "properties", field, "type")
					if err != nil || !exists || kind != "string" {
						t.Fatal("installed ARC proxy schema differs from the accepted contract")
					}
				}
			}
			kind, exists, err := unstructured.NestedString(properties, "spec", "properties", "proxy", "properties", "noProxy", "type")
			if err != nil || !exists || kind != "array" {
				t.Fatal("installed ARC noProxy must be a string list")
			}
			itemKind, exists, err := unstructured.NestedString(properties, "spec", "properties", "proxy", "properties", "noProxy", "items", "type")
			if err != nil || !exists || itemKind != "string" {
				t.Fatal("installed ARC noProxy entries must be strings")
			}
			if resource == "autoscalingrunnersets" {
				_, exists, _ := unstructured.NestedFieldNoCopy(properties, "status", "properties", "conditions")
				if !exists {
					t.Log("AutoscalingRunnerSet status.conditions is unavailable; timeout and log enrichment provide failure diagnostics.")
				} else {
					t.Log("ARC exposes conditions; live terminal semantics require confirmation before enabling the optional mapping.")
				}
			}
		}
		if !found {
			t.Fatal("installed ARC CRD does not serve v1alpha1")
		}
	}
}
