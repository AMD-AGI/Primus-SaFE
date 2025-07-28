/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
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
	ctrl "sigs.k8s.io/controller-runtime"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const (
	userNameFormat = "system:serviceaccount:%s:%s"
	helmDriver     = "secrets"
	installedMsg   = "cannot re-use a name that is still in use"
	Timeout        = time.Second * 30
	MaxHistory     = 20
)

type Options struct {
	QPS float32

	Burst int
}

// Option is a function that configures an RESTClientGetter.
type Option func(*RESTClientGetter)

func WithNamespace(namespace string) Option {
	return func(c *RESTClientGetter) {
		c.namespace = namespace
	}
}

func WithImpersonate(serviceAccount, namespace string) Option {
	return func(c *RESTClientGetter) {
		if username := SetImpersonationConfig(c.cfg, namespace, serviceAccount); username != "" {
			c.impersonate = username
		}
	}
}

func WithClientOptions(opts Options) Option {
	return func(c *RESTClientGetter) {
		c.cfg.Burst = opts.Burst
		c.cfg.QPS = opts.QPS
	}
}

func WithPersistent(persist bool) Option {
	return func(c *RESTClientGetter) {
		c.persistent = persist
	}
}

// RESTClientGetter is a resource.RESTClientGetter that uses an
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
	for _, opts := range opts {
		opts(g)
	}
	g.setDefaults()
	return g
}

func NewInClusterRESTClientGetter(opts ...Option) (*RESTClientGetter, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get config for in-cluster REST client: %w", err)
	}
	return NewRESTClientGetter(cfg, opts...), nil
}

// ToRESTConfig returns the in-memory REST config.
func (c *RESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	if c.cfg == nil {
		return nil, fmt.Errorf("RESTClientGetter has no REST config")
	}
	return c.cfg, nil
}

// ToDiscoveryClient returns a memory cached discovery client. Calling it
// multiple times will return the same instance.
func (c *RESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	if c.persistent {
		return c.toPersistentDiscoveryClient()
	}
	return c.toDiscoveryClient()
}

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

// ToRESTMapper returns a meta.RESTMapper using the discovery client. Calling
// it multiple times will return the same instance.
func (c *RESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	if c.persistent {
		return c.toPersistentRESTMapper()
	}
	return c.toRESTMapper()
}

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

func (c *RESTClientGetter) toRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := c.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	return restmapper.NewShortcutExpander(mapper, discoveryClient, nil), nil
}

func (c *RESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	if c.persistent {
		return c.toPersistentRawKubeConfigLoader()
	}
	return c.toRawKubeConfigLoader()
}

func (c *RESTClientGetter) toPersistentRawKubeConfigLoader() clientcmd.ClientConfig {
	c.clientCfgMu.Lock()
	defer c.clientCfgMu.Unlock()

	if c.clientCfg == nil {
		c.clientCfg = c.toRawKubeConfigLoader()
	}
	return c.clientCfg
}

func (c *RESTClientGetter) toRawKubeConfigLoader() clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig

	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	overrides.Context.Namespace = c.namespace
	overrides.AuthInfo.Impersonate = c.impersonate

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
}

var DefaultServiceAccountName string

func SetImpersonationConfig(cfg *rest.Config, namespace, serviceAccount string) string {
	name := DefaultServiceAccountName
	if serviceAccount != "" {
		name = serviceAccount
	}
	if name != "" && namespace != "" {
		username := fmt.Sprintf(userNameFormat, namespace, name)
		cfg.Impersonate = rest.ImpersonationConfig{UserName: username}
		return username
	}
	return ""
}

type ClustersGetter struct {
	sync.Mutex
	getter map[string]*RESTClientGetter
}

type getCluster func(ctx context.Context, cluster *corev1.ObjectReference) (*rest.Config, error)

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

	// Create a new registry client
	registryClient, err := registry.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return registryClient, nil
}

func ignoreUpgrade(addon *v1.Addon) bool {
	if addon.Status.AddonSourceStatus.HelmRepositoryStatus == nil {
		return true
	}
	if addon.Spec.AddonSource.HelmRepository.Values != "" {
		var diff func(values, values2 map[string]interface{}) bool
		diff = func(values, values2 map[string]interface{}) bool {
			for k, v := range values {
				if val, ok := values2[k]; ok {
					vall, ok := v.(map[string]interface{})
					valll, okk := val.(map[string]interface{})
					if ok && okk {
						if !diff(valll, vall) {
							return false
						}
					} else {
						str, ok := v.(string)
						str2, ok2 := v.(string)
						klog.Infof("compare %s %s", str, str2)
						if ok && ok2 && str != str2 {
							return false
						}
						if reflect.DeepEqual(v, val) {
							return false
						}
					}
				} else {
					return false
				}

			}
			return true
		}
		values := make(map[string]interface{})
		_ = yaml.Unmarshal([]byte(addon.Spec.AddonSource.HelmRepository.Values), &values)
		values2 := make(map[string]interface{})
		_ = yaml.Unmarshal([]byte(addon.Status.AddonSourceStatus.HelmRepositoryStatus.Values), &values2)
		if !reflect.DeepEqual(values, values2) {
			return false
		}
	}
	if addon.Spec.AddonSource.HelmRepository.Template != nil && addon.Status.AddonSourceStatus.HelmRepositoryStatus.Template != nil &&
		addon.Spec.AddonSource.HelmRepository.Template.Name != addon.Status.AddonSourceStatus.HelmRepositoryStatus.Template.Name {
		return false
	}
	if addon.Spec.AddonSource.HelmRepository.Template == nil && addon.Spec.AddonSource.HelmRepository.ChartVersion != addon.Status.AddonSourceStatus.HelmRepositoryStatus.ChartVersion {
		return false
	}
	return true
}

func replaceValues(values, base map[string]interface{}) map[string]interface{} {
	for k, v := range base {
		if val, ok := values[k]; ok {
			vall, ok := v.(map[string]interface{})
			if ok {
				valll, okk := val.(map[string]interface{})
				if okk {
					values[k] = replaceValues(valll, vall)
					continue
				}
			}
		} else {
			values[k] = v
		}

	}
	return values
}

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
				vall, ok := v.(map[string]interface{})
				valll, okk := val.(map[string]interface{})
				if ok && okk {
					rollback(valll, vall)
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
