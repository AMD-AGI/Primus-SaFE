// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/sql"
	"github.com/VictoriaMetrics/VictoriaMetrics/app/vmalert/remotewrite"
	"github.com/opensearch-project/opensearch-go"
	"github.com/prometheus/client_golang/api"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StorageClientSet struct {
	DB              *gorm.DB
	OpenSearch      *opensearch.Client // OpenSearch client
	PrometheusRead  api.Client         // Prometheus HTTP API client
	PrometheusWrite *remotewrite.Client
	Config          *PrimusLensClientConfig // Storage configuration for debugging
}

var (
	currentClusterStorageClientSet     *StorageClientSet
	multiClusterStorageClientSet       = map[string]*StorageClientSet{}
	multiClusterStorageConfigJsonBytes []byte
)

// getCurrentClusterStorageClientSet returns the storage client for current cluster
// This is internal function, external code should use ClusterManager.GetCurrentClusterClients()
func getCurrentClusterStorageClientSet() *StorageClientSet {
	if currentClusterStorageClientSet == nil {
		panic("please init currentClusterStorageClientSet first")
	}
	return currentClusterStorageClientSet
}

// getStorageClientSetByClusterName returns storage client for a specific cluster
// This is internal function, external code should use ClusterManager.GetClientSetByClusterName()
func getStorageClientSetByClusterName(clusterName string) (*StorageClientSet, error) {
	storageClientSet, exists := multiClusterStorageClientSet[clusterName]
	if !exists {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessagef("Storage client set for cluster %s not found", clusterName)
	}
	return storageClientSet, nil
}

// initStorageClientSets is now handled by ClusterManager
// This function is kept for backward compatibility but should not be called directly
// Use InitClusterManager instead

func loadCurrentClusterStorageClients(ctx context.Context) error {
	cfg, err := loadSingleClusterStorageConfig(ctx, getCurrentClusterK8SClientSet())
	if err != nil {
		return err
	}
	clusterName := getCurrentClusterName()
	client, err := initStorageClients(ctx, clusterName, *cfg)
	if err != nil {
		return err
	}
	currentClusterStorageClientSet = client
	log.Infof("Initialized single-cluster storage clients successfully for cluster: %s", clusterName)
	return nil
}

func loadMultiClusterStorageClients(ctx context.Context) error {
	cfg, err := loadMultiClusterStorageConfig(ctx)
	if err != nil {
		return err
	}
	cfgJsonBytes, err := json.Marshal(cfg)
	if err != nil {
		return errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to marshal storage config to json").
			WithError(err)
	}
	if multiClusterStorageConfigJsonBytes != nil {
		if string(cfgJsonBytes) == string(multiClusterStorageConfigJsonBytes) {
			return nil
		}
	}
	multiClusterStorageConfigJsonBytes = cfgJsonBytes
	newMultiClusterStorageClientSet := map[string]*StorageClientSet{}
	for clusterName, singleCLusterConfig := range cfg {
		storageClientSet, err := initStorageClients(ctx, clusterName, singleCLusterConfig)
		if err != nil {
			log.Errorf("Failed to initialize storage clients for cluster %s: %v", clusterName, err)
			continue
		}
		newMultiClusterStorageClientSet[clusterName] = storageClientSet
		log.Infof("Initialized storage clients for cluster %s successfully", clusterName)
	}
	multiClusterStorageClientSet = newMultiClusterStorageClientSet
	log.Info("Initialized multi-cluster storage clients successfully")
	return errors.NewError().WithCode(errors.CodeInitializeError).WithMessage("Failed to initialize storage clients for some clusters").WithError(err)
}

func loadSingleClusterStorageConfig(ctx context.Context, k8sClient *K8SClientSet) (*PrimusLensClientConfig, error) {
	secret, err := k8sClient.Clientsets.CoreV1().Secrets(StorageConfigSecretNamespace).Get(ctx, StorageConfigSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to get storage config secret").
			WithError(err)
	}
	cfg := &PrimusLensClientConfig{}
	err = cfg.LoadFromSecret(secret.Data)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to load storage config from secret").
			WithError(err)
	}
	return cfg, nil
}

// LoadSingleClusterStorageConfig exported method for loading storage config from a specified K8S client
func LoadSingleClusterStorageConfig(ctx context.Context, k8sClient *K8SClientSet) (*PrimusLensClientConfig, error) {
	return loadSingleClusterStorageConfig(ctx, k8sClient)
}

func loadMultiClusterStorageConfig(ctx context.Context) (PrimusLensMultiClusterClientConfig, error) {
	secret, err := getCurrentClusterK8SClientSet().Clientsets.CoreV1().Secrets(StorageConfigSecretNamespace).Get(ctx, MultiStorageConfigSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to get multi-cluster storage config secret").
			WithError(err)
	}
	cfg := PrimusLensMultiClusterClientConfig{}
	err = cfg.LoadFromSecret(secret.Data)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to load multi-cluster storage config from secret").
			WithError(err)
	}
	return cfg, nil
}

func initStorageClients(ctx context.Context, clusterName string, cfg PrimusLensClientConfig) (*StorageClientSet, error) {
	clientSet := &StorageClientSet{}
	sqlConfig := sql.DatabaseConfig{
		Host:        fmt.Sprintf("%s.%s.svc.cluster.local", cfg.Postgres.Service, cfg.Postgres.Namespace),
		Port:        int(cfg.Postgres.Port),
		UserName:    cfg.Postgres.Username,
		Password:    cfg.Postgres.Password,
		DBName:      cfg.Postgres.DBName,
		LogMode:     false,
		MaxIdleConn: 10,
		MaxOpenConn: 40,
		SSLMode:     cfg.Postgres.SSLMode,
		Driver:      sql.DriverNamePostgres,
	}
	log.Infof("Initializing storage clients for cluster '%s' ", clusterName)
	gormDb, err := sql.InitGormDB(clusterName, sqlConfig,
		sql.WithTracingCallback(),
		sql.WithErrorStackCallback(),
		sql.WithReconnectCallback(), // Automatically handle master-slave failover and reconnection
	)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessagef("Failed to initialize db for cluster '%s'", clusterName).
			WithError(err)
	}
	log.Infof("Successfully initialized DB for cluster '%s', DB pointer: %p", clusterName, gormDb)
	clientSet.DB = gormDb
	// Init Opensearch client
	opensearchClient, err := opensearch.NewClient(opensearch.Config{
		Addresses: []string{
			fmt.Sprintf("%s://%s.%s.svc.cluster.local:%d", cfg.Opensearch.Scheme, cfg.Opensearch.Service, cfg.Opensearch.Namespace, cfg.Opensearch.Port),
		},
		Username:      cfg.Opensearch.Username,
		Password:      cfg.Opensearch.Password,
		EnableMetrics: true,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	})
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to initialize opensearch client").
			WithError(err)
	}
	clientSet.OpenSearch = opensearchClient
	// Init Prometheus Client
	readClient, err := initPrometheusClient(fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/select/0/prometheus", cfg.Prometheus.ReadService, cfg.Prometheus.Namespace, cfg.Prometheus.ReadPort))
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to initialize prometheus read client").
			WithError(err)
	}
	clientSet.PrometheusRead = readClient
	cli, err := remotewrite.NewClient(context.Background(), remotewrite.Config{
		Addr: fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/insert/0/prometheus", cfg.Prometheus.WriteService, cfg.Prometheus.Namespace, cfg.Prometheus.WritePort),
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	})
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to initialize prometheus write client").
			WithError(err)
	}
	clientSet.PrometheusWrite = cli
	// Store configuration for debugging
	clientSet.Config = &cfg
	return clientSet, nil
}

// InitStorageClients exported method for initializing storage clients from config
func InitStorageClients(ctx context.Context, clusterName string, cfg PrimusLensClientConfig) (*StorageClientSet, error) {
	return initStorageClients(ctx, clusterName, cfg)
}

func initPrometheusClient(endpoints string) (api.Client, error) {
	promCfg := api.Config{
		Address: endpoints,
		Client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
	return api.NewClient(promCfg)
}
