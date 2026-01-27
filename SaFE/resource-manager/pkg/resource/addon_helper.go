/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const (
	// helmDriver specifies the Helm storage driver
	helmDriver = "secrets"
	// installedMsg is the error message when trying to reuse an existing Helm release name
	installedMsg = "cannot re-use a name that is still in use"
	// Timeout specifies the timeout for Helm operations
	Timeout = time.Minute * 5
	// MaxHistory specifies the maximum number of Helm release versions to keep
	MaxHistory       = 20
	DefaultNamespace = "primus-safe"
)

// Options represents configuration options for a REST client.
type Options struct {
	// QPS is the queries per second rate limit
	QPS float32
	// Burst is the maximum burst rate
	Burst int
}

// Option is a function that configures a RESTClientGetter
type Option func(*RESTClientGetter)

// RESTClientGetter is a resource.RESTClientGetter that uses an in-memory REST config
type RESTClientGetter struct {
	namespace   string
	impersonate string
	persistent  bool

	cfg *rest.Config

	restMapper   meta.RESTMapper
	restMapperMu sync.Mutex

	discoveryClient discovery.CachedDiscoveryInterface
	discoveryMu     sync.Mutex

	clientCfg   clientcmd.ClientConfig
	clientCfgMu sync.Mutex
}

// setDefaults sets default values for the RESTClientGetter.
func (c *RESTClientGetter) setDefaults() {
	if c.namespace == "" {
		c.namespace = "default"
	}
}

// NewRESTClientGetter returns a new RESTClientGetter.
func NewRESTClientGetter(cfg *rest.Config, opts ...Option) *RESTClientGetter {
	g := &RESTClientGetter{
		cfg: cfg,
	}
	for _, opt := range opts {
		opt(g)
	}
	g.setDefaults()
	return g
}

// ToRESTConfig returns the in-memory REST config.
func (c *RESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	if c.cfg == nil {
		return nil, fmt.Errorf("RESTClientGetter has no REST config")
	}
	return c.cfg, nil
}

// ToDiscoveryClient returns a memory cached discovery client.
// Calling it multiple times will return the same instance.
func (c *RESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	if c.persistent {
		return c.toPersistentDiscoveryClient()
	}
	return c.toDiscoveryClient()
}

// toPersistentDiscoveryClient returns a cached discovery client instance with lazy initialization.
// It uses a mutex to ensure thread-safe access and caches the client for subsequent calls.
// Returns an error if the initial discovery client creation fails.
func (c *RESTClientGetter) toPersistentDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	c.discoveryMu.Lock()
	defer c.discoveryMu.Unlock()

	if c.discoveryClient == nil {
		discoveryClient, err := c.toDiscoveryClient()
		if err != nil {
			return nil, err
		}
		c.discoveryClient = discoveryClient
	}
	return c.discoveryClient, nil
}

// toDiscoveryClient returns a memory cached discovery client.
func (c *RESTClientGetter) toDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := c.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(discoveryClient), nil
}

// ToRESTMapper returns a meta.RESTMapper using the discovery client.
// Calling it multiple times will return the same instance.
func (c *RESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	if c.persistent {
		return c.toPersistentRESTMapper()
	}
	return c.toRESTMapper()
}

// toPersistentRESTMapper returns a cached RESTMapper instance with lazy initialization.
// It uses a mutex to ensure thread-safe access and caches the mapper for subsequent calls.
// Returns an error if the initial RESTMapper creation fails.
func (c *RESTClientGetter) toPersistentRESTMapper() (meta.RESTMapper, error) {
	c.restMapperMu.Lock()
	defer c.restMapperMu.Unlock()

	if c.restMapper == nil {
		restMapper, err := c.toRESTMapper()
		if err != nil {
			return nil, err
		}
		c.restMapper = restMapper
	}
	return c.restMapper, nil
}

// toRESTMapper returns a meta.RESTMapper using the discovery client.
func (c *RESTClientGetter) toRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := c.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	return restmapper.NewShortcutExpander(mapper, discoveryClient, nil), nil
}

// ToRawKubeConfigLoader returns a clientcmd.ClientConfig.
func (c *RESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	if c.persistent {
		return c.toPersistentRawKubeConfigLoader()
	}
	return c.toRawKubeConfigLoader()
}

// toPersistentRawKubeConfigLoader returns a cached ClientConfig instance with lazy initialization.
// It uses a mutex to ensure thread-safe access and caches the config for subsequent calls.
func (c *RESTClientGetter) toPersistentRawKubeConfigLoader() clientcmd.ClientConfig {
	c.clientCfgMu.Lock()
	defer c.clientCfgMu.Unlock()

	if c.clientCfg == nil {
		c.clientCfg = c.toRawKubeConfigLoader()
	}
	return c.clientCfg
}

// ToRawKubeConfigLoader returns a clientcmd.ClientConfig.
func (c *RESTClientGetter) toRawKubeConfigLoader() clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig

	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	overrides.Context.Namespace = c.namespace
	overrides.AuthInfo.Impersonate = c.impersonate

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
}

// ClustersGetter manages RESTClientGetter instances for different clusters.
type ClustersGetter struct {
	sync.Mutex
	getter map[string]*RESTClientGetter
}

// getCluster defines a function type for retrieving cluster REST config.
type getCluster func(ctx context.Context, cluster *corev1.ObjectReference) (*rest.Config, error)

// get retrieves or creates a RESTClientGetter for the specified cluster.
// It uses a cache to return existing getters if the REST config hasn't changed.
// Creates a new getter if the cluster is not cached or if the config has been updated.
func (c *ClustersGetter) get(ctx context.Context, cluster *corev1.ObjectReference, get getCluster) (*RESTClientGetter, error) {
	c.Lock()
	defer c.Unlock()
	if c.getter == nil {
		c.getter = make(map[string]*RESTClientGetter)
	}

	config, err := get(ctx, cluster)
	if err != nil {
		return nil, err
	}

	if g, ok := c.getter[cluster.Name]; ok && reflect.DeepEqual(g.cfg, config) {
		return g, nil
	}

	getter := NewRESTClientGetter(config)
	c.getter[cluster.Name] = getter
	return getter, nil
}

// newDefaultRegistryClient creates a new Helm registry client.
func newDefaultRegistryClient(plainHTTP bool, settings *cli.EnvSettings) (*registry.Client, error) {
	opts := []registry.ClientOption{
		registry.ClientOptDebug(settings.Debug),
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(os.Stderr),
		registry.ClientOptCredentialsFile(settings.RegistryConfig),
	}
	if plainHTTP {
		opts = append(opts, registry.ClientOptPlainHTTP())
	}

	// Create HTTP client with TLS verification disabled for self-signed certificates
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	opts = append(opts, registry.ClientOptHTTPClient(httpClient))

	// Create a new registry client
	registryClient, err := registry.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return registryClient, nil
}

// shouldIgnoreUpgrade determines whether a Helm upgrade should be skipped.
func shouldIgnoreUpgrade(addon *v1.Addon) bool {
	if !isStatusReady(addon) {
		return false
	}

	if !areValuesEqual(addon) {
		return false
	}

	if !isTemplateVersionEqual(addon) {
		return false
	}

	return isChartVersionEqual(addon)
}

// isStatusReady checks if the addon status is ready for upgrade.
func isStatusReady(addon *v1.Addon) bool {
	if addon.Status.AddonSourceStatus.HelmRepositoryStatus == nil {
		return true
	}
	status := addon.Status.AddonSourceStatus.HelmRepositoryStatus.Status
	return status != v1.AddonFailed && status != v1.AddonError
}

// areValuesEqual compares the values between spec and status.
func areValuesEqual(addon *v1.Addon) bool {
	if addon.Spec.AddonSource.HelmRepository.Values == "" {
		return true
	}

	specValues := make(map[string]interface{})
	_ = yaml.Unmarshal([]byte(addon.Spec.AddonSource.HelmRepository.Values), &specValues)

	statusValues := make(map[string]interface{})
	_ = yaml.Unmarshal([]byte(addon.Status.AddonSourceStatus.HelmRepositoryStatus.Values), &statusValues)

	return reflect.DeepEqual(specValues, statusValues)
}

// isTemplateVersionEqual checks if template version matches.
func isTemplateVersionEqual(addon *v1.Addon) bool {
	if addon.Spec.AddonSource.HelmRepository.Template == nil || addon.Status.AddonSourceStatus.HelmRepositoryStatus == nil || addon.Status.AddonSourceStatus.HelmRepositoryStatus.Template == nil {
		return true
	}
	return addon.Spec.AddonSource.HelmRepository.Template.Name == addon.Status.AddonSourceStatus.HelmRepositoryStatus.Template.Name
}

// isChartVersionEqual checks if chart version matches.
func isChartVersionEqual(addon *v1.Addon) bool {
	if addon.Spec.AddonSource.HelmRepository.Template != nil || addon.Status.AddonSourceStatus.HelmRepositoryStatus == nil {
		return true
	}
	return addon.Spec.AddonSource.HelmRepository.ChartVersion == addon.Status.AddonSourceStatus.HelmRepositoryStatus.ChartVersion
}

// replaceValues merges values with base values, replacing existing values.
func replaceValues(values, base map[string]interface{}) map[string]interface{} {
	for k, v := range base {
		if val, ok := values[k]; ok {
			obj1, ok1 := v.(map[string]interface{})
			if ok1 {
				obj2, ok2 := val.(map[string]interface{})
				if ok2 {
					values[k] = replaceValues(obj2, obj1)
					continue
				}
			}
		} else {
			values[k] = v
		}
	}
	return values
}

// rollbackValues merges rollback values with base values.
func rollbackValues(str string, base map[string]interface{}) string {
	values := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(str), &values)
	if err != nil {
		klog.Errorf("rollback values failed %+v", err)
		return ""
	}

	var rollback func(values, base map[string]interface{})
	rollback = func(values, base map[string]interface{}) {
		for k, v := range values {
			if val, ok := base[k]; ok {
				obj1, ok1 := v.(map[string]interface{})
				obj2, ok2 := val.(map[string]interface{})
				if ok1 && ok2 {
					rollback(obj2, obj1)
					continue
				} else {
					values[k] = val
				}
			}
		}
	}
	rollback(values, base)
	data, err := yaml.Marshal(values)
	if err != nil {
		klog.Errorf("rollback values failed %+v", err)
		return ""
	}
	return string(data)
}

// GetReleaseNamespace returns the namespace for addon release.
func GetReleaseNamespace(addon *v1.Addon) string {
	if addon.Spec.AddonSource.HelmRepository.Namespace != "" {
		return addon.Spec.AddonSource.HelmRepository.Namespace
	}
	return DefaultNamespace
}
