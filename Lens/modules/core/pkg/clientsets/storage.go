package clientsets

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/sql"
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
}

var (
	currentClusterStorageClientSet     *StorageClientSet
	multiClusterStorageClientSet       = map[string]*StorageClientSet{}
	multiClusterStorageConfigJsonBytes []byte
)

func GetCurrentClusterStorageClientSet() *StorageClientSet {
	if currentClusterStorageClientSet == nil {
		panic("please init currentClusterStorageClientSet first")
	}
	return currentClusterStorageClientSet
}

func GetStorageClientSetByClusterName(clusterName string) (*StorageClientSet, error) {
	storageClientSet, exists := multiClusterStorageClientSet[clusterName]
	if !exists {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessagef("Storage client set for cluster %s not found", clusterName)
	}
	return storageClientSet, nil
}

func initStorageClientSets(ctx context.Context, multiCluster bool) error {
	var err error
	if !multiCluster {
		err = loadCurrentClusterStorageClients(ctx)
	} else {
		err = loadMultiClusterStorageClients(ctx)
	}
	if err != nil {
		return err
	}
	if multiCluster {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					err = loadMultiClusterStorageClients(ctx)
					if err != nil {
						log.Error("Failed to reload multi-cluster storage clients: %v", err)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	return nil
}

func loadCurrentClusterStorageClients(ctx context.Context) error {
	cfg, err := loadSingleClusterStorageConfig(ctx)
	if err != nil {
		return err
	}
	client, err := initStorageClients(ctx, *cfg)
	if err != nil {
		return err
	}
	currentClusterStorageClientSet = client
	log.Info("Initialized single-cluster storage clients successfully")
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
		storageClientSet, err := initStorageClients(ctx, singleCLusterConfig)
		if err != nil {
			return err
		}
		newMultiClusterStorageClientSet[clusterName] = storageClientSet
	}
	multiClusterStorageClientSet = newMultiClusterStorageClientSet
	log.Info("Initialized single-cluster storage clients successfully")
	return nil
}

func loadSingleClusterStorageConfig(ctx context.Context) (*PrimusLensClientConfig, error) {
	secret, err := GetCurrentClusterK8SClientSet().Clientsets.CoreV1().Secrets(StorageConfigSecretNamespace).Get(ctx, StorageConfigSecretName, metav1.GetOptions{})
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

func loadMultiClusterStorageConfig(ctx context.Context) (PrimusLensMultiClusterClientConfig, error) {
	secret, err := GetCurrentClusterK8SClientSet().Clientsets.CoreV1().Secrets(StorageConfigSecretNamespace).Get(ctx, MultiStorageConfigSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to get storage config secret").
			WithError(err)
	}
	cfg := PrimusLensMultiClusterClientConfig{}
	err = cfg.LoadFromSecret(secret.Data)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to load storage config from secret").
			WithError(err)
	}
	return cfg, nil
}

func initStorageClients(ctx context.Context, cfg PrimusLensClientConfig) (*StorageClientSet, error) {
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
	gormDb, err := sql.InitGormDB("default", sqlConfig)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to initialize default db").
			WithError(err)
	}
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
	return clientSet, nil
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
