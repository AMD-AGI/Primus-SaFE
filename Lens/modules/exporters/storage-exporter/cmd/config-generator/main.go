// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// config-generator scans StorageClasses and generates deployment configuration
// for storage-exporter based on discovered WekaFS filesystems.
//
// Usage:
//   go run main.go --kubeconfig ~/.kube/config --output-dir ./generated
//
// This will generate:
//   - config.yaml: Configuration for storage-exporter
//   - deployment.yaml: Kubernetes Deployment with all necessary PVCs and mounts

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	wekafsProvisioner = "csi.weka.io"
)

// FilesystemInfo contains discovered filesystem information
type FilesystemInfo struct {
	Name             string
	StorageClassName string
	FilesystemName   string
	VolumeType       string
}

func main() {
	var kubeconfig string
	var outputDir string
	var namespace string
	var clusterName string
	var imageTag string

	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "path to kubeconfig file")
	} else {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	}
	flag.StringVar(&outputDir, "output-dir", "./generated", "output directory for generated files")
	flag.StringVar(&namespace, "namespace", "primus-lens", "namespace for deployment")
	flag.StringVar(&clusterName, "cluster-name", "default", "cluster name for metrics label")
	flag.StringVar(&imageTag, "image-tag", "latest", "image tag for storage-exporter")
	flag.Parse()

	// Create Kubernetes client
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Printf("Error building kubeconfig: %v\n", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error creating Kubernetes client: %v\n", err)
		os.Exit(1)
	}

	// Scan StorageClasses
	filesystems, err := scanStorageClasses(clientset)
	if err != nil {
		fmt.Printf("Error scanning StorageClasses: %v\n", err)
		os.Exit(1)
	}

	if len(filesystems) == 0 {
		fmt.Println("No WekaFS StorageClasses found")
		os.Exit(0)
	}

	fmt.Printf("Found %d WekaFS filesystem(s):\n", len(filesystems))
	for _, fs := range filesystems {
		fmt.Printf("  - %s (StorageClass: %s)\n", fs.FilesystemName, fs.StorageClassName)
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Generate config.yaml
	configContent := generateConfig(filesystems, clusterName)
	configPath := filepath.Join(outputDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		fmt.Printf("Error writing config.yaml: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Generated: %s\n", configPath)

	// Generate deployment.yaml
	deploymentContent := generateDeployment(filesystems, namespace, imageTag)
	deploymentPath := filepath.Join(outputDir, "deployment.yaml")
	if err := os.WriteFile(deploymentPath, []byte(deploymentContent), 0644); err != nil {
		fmt.Printf("Error writing deployment.yaml: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Generated: %s\n", deploymentPath)

	fmt.Println("\nTo deploy, run:")
	fmt.Printf("  kubectl apply -f %s\n", outputDir)
}

func scanStorageClasses(clientset *kubernetes.Clientset) ([]FilesystemInfo, error) {
	ctx := context.Background()

	scList, err := clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Group by filesystemName to avoid duplicates
	filesystemMap := make(map[string]FilesystemInfo)

	for _, sc := range scList.Items {
		if sc.Provisioner != wekafsProvisioner {
			continue
		}

		fsName := getParameterValue(sc, "filesystemName")
		if fsName == "" {
			continue
		}

		// Only keep one StorageClass per filesystem
		if _, exists := filesystemMap[fsName]; !exists {
			filesystemMap[fsName] = FilesystemInfo{
				Name:             sanitizeName(fsName),
				StorageClassName: sc.Name,
				FilesystemName:   fsName,
				VolumeType:       getParameterValue(sc, "volumeType"),
			}
		}
	}

	// Convert map to slice
	result := make([]FilesystemInfo, 0, len(filesystemMap))
	for _, fs := range filesystemMap {
		result = append(result, fs)
	}

	return result, nil
}

func getParameterValue(sc storagev1.StorageClass, key string) string {
	if sc.Parameters == nil {
		return ""
	}
	return sc.Parameters[key]
}

func sanitizeName(name string) string {
	// Replace special characters with dashes
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, ".", "-")
	return strings.ToLower(name)
}

func generateConfig(filesystems []FilesystemInfo, clusterName string) string {
	var mounts strings.Builder
	for _, fs := range filesystems {
		mounts.WriteString(fmt.Sprintf(`    - name: wekafs-%s
      mountPath: /mnt/wekafs-%s
      storageType: wekafs
      filesystemName: %s
      pvcName: storage-exporter-wekafs-%s
`, fs.Name, fs.Name, fs.FilesystemName, fs.Name))
	}

	return fmt.Sprintf(`# Storage Exporter Configuration
# Auto-generated by config-generator

httpPort: 8992
loadK8SClient: false

storage:
  scrapeInterval: 60s
  mounts:
%s
metrics:
  staticLabels:
    primus_lens_cluster: %s

middleware:
  enableLogging: true
  enableTracing: false
`, mounts.String(), clusterName)
}

func generateDeployment(filesystems []FilesystemInfo, namespace string, imageTag string) string {
	// Generate PVCs
	var pvcs strings.Builder
	for _, fs := range filesystems {
		pvcs.WriteString(fmt.Sprintf(`---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: storage-exporter-wekafs-%s
  namespace: %s
  labels:
    app: storage-exporter
    storage-type: wekafs
    filesystem-name: %s
spec:
  accessModes:
    - ReadOnlyMany
  storageClassName: %s
  resources:
    requests:
      storage: 1Gi
`, fs.Name, namespace, fs.FilesystemName, fs.StorageClassName))
	}

	// Generate volumeMounts
	var volumeMounts strings.Builder
	for _, fs := range filesystems {
		volumeMounts.WriteString(fmt.Sprintf(`            - name: wekafs-%s
              mountPath: /mnt/wekafs-%s
              readOnly: true
`, fs.Name, fs.Name))
	}

	// Generate volumes
	var volumes strings.Builder
	for _, fs := range filesystems {
		volumes.WriteString(fmt.Sprintf(`        - name: wekafs-%s
          persistentVolumeClaim:
            claimName: storage-exporter-wekafs-%s
`, fs.Name, fs.Name))
	}

	// Generate configmap mounts config
	var mountsConfig strings.Builder
	for _, fs := range filesystems {
		mountsConfig.WriteString(fmt.Sprintf(`        - name: wekafs-%s
          mountPath: /mnt/wekafs-%s
          storageType: wekafs
          filesystemName: %s
          pvcName: storage-exporter-wekafs-%s
`, fs.Name, fs.Name, fs.FilesystemName, fs.Name))
	}

	return fmt.Sprintf(`# Storage Exporter Deployment
# Auto-generated by config-generator
---
apiVersion: v1
kind: Namespace
metadata:
  name: %s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: storage-exporter
  namespace: %s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: storage-exporter-config
  namespace: %s
data:
  config.yaml: |
    httpPort: 8992
    loadK8SClient: false
    storage:
      scrapeInterval: 60s
      mounts:
%s
    metrics:
      staticLabels:
        primus_lens_cluster: %s
    middleware:
      enableLogging: true
      enableTracing: false
%s---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: storage-exporter
  namespace: %s
  labels:
    app: storage-exporter
spec:
  replicas: 1
  selector:
    matchLabels:
      app: storage-exporter
  template:
    metadata:
      labels:
        app: storage-exporter
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8992"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: storage-exporter
      containers:
        - name: storage-exporter
          image: docker.io/primussafe/storage-exporter:%s
          imagePullPolicy: Always
          ports:
            - name: http
              containerPort: 8992
          env:
            - name: CONFIG_PATH
              value: /etc/storage-exporter/config.yaml
          volumeMounts:
            - name: config
              mountPath: /etc/storage-exporter
              readOnly: true
%s          livenessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 256Mi
      volumes:
        - name: config
          configMap:
            name: storage-exporter-config
%s---
apiVersion: v1
kind: Service
metadata:
  name: storage-exporter
  namespace: %s
  labels:
    app: storage-exporter
spec:
  type: ClusterIP
  ports:
    - name: http
      port: 8992
      targetPort: http
  selector:
    app: storage-exporter
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: storage-exporter
  namespace: %s
  labels:
    app: storage-exporter
spec:
  selector:
    matchLabels:
      app: storage-exporter
  endpoints:
    - port: http
      path: /metrics
      interval: 60s
  namespaceSelector:
    matchNames:
      - %s
`, namespace, namespace, namespace, mountsConfig.String(), namespace, pvcs.String(), namespace, imageTag, volumeMounts.String(), volumes.String(), namespace, namespace, namespace)
}
