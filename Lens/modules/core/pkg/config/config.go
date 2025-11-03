package config

import (
	"fmt"
	"os"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	"gopkg.in/yaml.v2"
)

type Config struct {
	MultiCluster    bool                `json:"multiCluster" yaml:"multiCluster"`
	LoadK8SClient   bool                `json:"loadK8SClient" yaml:"loadK8SClient"`
	LoadStorageClient bool              `json:"loadStorageClient" yaml:"loadStorageClient"`
	Controller      ControllerConfig    `yaml:"controller"`
	HttpPort        int                 `json:"httpPort" yaml:"httpPort"`
	NodeExporter    *NodeExporterConfig `json:"nodeExporter" yaml:"nodeExporter"`
	Jobs            *JobsConfig         `json:"jobs" yaml:"jobs"`
	Netflow         *NetFlow            `json:"netflow" yaml:"netflow"`
}

type ControllerConfig struct {
	Namespace        string `json:"namespace" yaml:"namespace"`
	LeaderElectionId string `json:"leader_election_id" yaml:"leaderElectionId"`
	MetricsPort      int    `json:"metricsPort" yaml:"metricsPort"`
	HealthzPort      int    `json:"healthzPort" yaml:"healthzPort"`
	PprofPort        int    `json:"pprofPort" yaml:"pprofPort"`
}

func (cfg ControllerConfig) GetMetricsBindAddress() string {
	port := cfg.MetricsPort
	if port == 0 {
		port = 19191
	}
	return fmt.Sprintf(":%d", port)
}

func (cfg ControllerConfig) GetHealthzBindAddress() string {
	port := cfg.HealthzPort
	if port == 0 {
		port = 19192
	}
	return fmt.Sprintf(":%d", port)
}

func (cfg ControllerConfig) GetPprofBindAddress() string {
	port := cfg.PprofPort
	if port == 0 {
		port = 19193
	}
	return fmt.Sprintf(":%d", port)
}

var config *Config

func LoadConfig() (*Config, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}
	configFile, err := os.Open(configPath)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("failed to open config file").
			WithError(err)
	}
	defer configFile.Close()
	decoder := yaml.NewDecoder(configFile)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("failed to parse config file").
			WithError(err)
	}
	return config, nil
}

type NodeExporterConfig struct {
	ContainerdSocketPath string `yaml:"containerd_socket_path" json:"containerd_socket_path"`
	GrpcServer           string `yaml:"grpc_server" json:"grpc_server"`
}

type JobsConfig struct {
	GrpcPort int `yaml:"grpc_port" json:"grpc_port"`
}

type NetFlow struct {
	ScanPortListenIntervalSeconds int    `json:"scan_port_listen_interval_seconds" yaml:"scan_port_listen_interval_seconds"`
	PolicyConfigPath              string `json:"policy_config_path" yaml:"policy_config_path"`
}

func (n NetFlow) GetScanPortListenInterval() time.Duration {
	if n.ScanPortListenIntervalSeconds <= 0 {
		return 2 * time.Second
	}
	return time.Duration(n.ScanPortListenIntervalSeconds) * time.Second
}
