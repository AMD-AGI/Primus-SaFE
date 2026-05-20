/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package common

const (
	PrimusSafeName             = "primus-safe"
	PrimusSafeNamespace        = "primus-safe"
	PrimusFault                = "primus-safe-fault"
	PrimusPvmName              = "primus-safe-pv"
	PrimusFailover             = "primus-safe-failover"
	DefaultVersion             = "v1"
	PrimusRouterCustomRootPath = "api/" + DefaultVersion
	ImageImportSecretName      = "primus-safe-image-import-reg-cred"
	SecretPath                 = "/etc/secrets"

	AuthoringKind           = "Authoring"
	PytorchJobKind          = "PyTorchJob"
	JobKind                 = "Job"
	DeploymentKind          = "Deployment"
	StatefulSetKind         = "StatefulSet"
	CICDScaleRunnerSetKind  = "AutoscalingRunnerSet"
	CICDEphemeralRunnerKind = "EphemeralRunner"
	UnifiedJobKind          = "UnifiedJob"
	TorchFTKind             = "TorchFT"
	MonarchJob              = "MonarchJob"
	MonarchMesh             = "MonarchMesh"
	MonarchClient           = "MonarchClient"
	RayJobKind              = "RayJob"
	PodKind                 = "Pod"
	SandboxKind             = "Sandbox"
	DynamoDeploymentKind    = "DynamoDeployment"
	ConfigmapKind           = "ConfigMap"
	ClusterRoleKind         = "ClusterRole"
	ServiceAccountKind      = "ServiceAccount"

	GithubConfigUrl   = "GITHUB_CONFIG_URL"
	UnifiedJobEnable  = "UNIFIED_JOB_ENABLE"
	ScaleRunnerSetID  = "SCALE_RUNNER_SET_ID"
	ScaleRunnerID     = "SCALE_RUNNER_ID"
	ReplicaCount      = "REPLICA_COUNT"
	HostPerReplica    = "HOST_PER_REPLICA"
	MaxReplicaCount   = "MAX_REPLICA_COUNT"
	MinReplicaCount   = "MIN_REPLICA_COUNT"
	TorchFTLightHouse = "TORCHFT_LIGHTHOUSE"
	RayJobEntrypoint  = "RAY_JOB_ENTRYPOINT"
	// for preflight job
	GPU_PRODUCT = "GPU_PRODUCT"

	HigressClassname = "higress"
	HigressNamespace = "higress-system"
	HigressGateway   = "higress-gateway"
	HigressSSHPort   = 22

	NodeNameSelector       = "spec.nodeName="
	KubeSystemNamespace    = "kube-system"
	KubePublicNamespace    = "kube-public"
	PytorchJobPortName     = "pytorchjob-port"
	PfsSelectorKey         = "pfs-name"
	JsonContentType        = "application/json; charset=utf-8"
	KubernetesControlPlane = "node-role.kubernetes.io/control-plane"
	SpecifiedNodes         = "specified-nodes"
	ExcludedNodes          = "excluded-nodes"
	TaintAction            = "taint"
	CICDArcNamespace       = "arc-systems"

	RayJobSubmitterName    = "ray-job-submitter"
	RayJobSubmitterCpu     = "1"
	RayJobSubmitterMemory  = "1Gi"
	RayJobSubmitterStorage = "10Gi"
	RayJobGcsServerPort    = 6379
	RayJobDashboardPort    = 8265
	RayJobMetricsPort      = 18080

	MonarchMeshPortNum = 26600
	MonarchPort        = "MONARCH_PORT"
	MonarchMeshPrefix  = "MONARCH_MESH_PREFIX"

	DefaultBurst          = 1000
	DefaultQPS            = 1000
	DefaultMaxUnavailable = "25%"
	DefaultMaxMaxSurge    = "25%"

	HighPriority    = "high-priority"
	MedPriority     = "med-priority"
	LowPriority     = "low-priority"
	HighPriorityInt = 2
	MedPriorityInt  = 1
	LowPriorityInt  = 0

	NvidiaGpu            = "nvidia.com/gpu"
	NvidiaIdentification = "nvidia.com/gpu.present"
	AMDGpuIdentification = "feature.node.kubernetes.io/amd-gpu"
	AmdGpu               = "amd.com/gpu"

	UserName              = "userName"
	UserId                = "userId"
	UserType              = "userType"
	ApiKeyId              = "apiKeyId"
	UserSelf              = "self"
	UserSystem            = "primus-safe-system"
	UserWorkspaces        = "workspaces"
	UserManagedWorkspaces = "managedWorkspaces"
	Name                  = "name"
	PodId                 = "podId"
	AddonName             = "addonName"
)

const (
	ImageImportReadyStatus   = "Ready"
	ImageImportingStatus     = "Importing"
	ImageImportFailedStatus  = "Failed"
	ImageImportPendingStatus = "Pending"
)

const (
	NodesAffinityRequired  = "required"
	NodesAffinityPreferred = "preferred"
)

// Dynamo deployment constants. Used by the DynamoDeployment workload kind
// (see Phase 2 of the dynamo integration plan). Grouped in a dedicated block
// to keep the main constants block focused on cross-kind shared values.
const (
	// Service ports. Consumed by webhook (Service.TargetPort defaults) and
	// dispatcher when reserving host ports for hostNetwork pods.
	DynamoFrontendPort  = 8000
	DynamoNatsPort      = 4222
	DynamoFPMPort       = 20380
	DynamoBootstrapPort = 30001

	// NOTE: dynamo annotation keys (service-roles, kv-transfer-backend,
	// multinode.<role>, backend-framework) live in apis/pkg/apis/amd/v1/
	// well_known_constants.go alongside the rest of the primus-safe.* annotation
	// namespace. Reference them via v1.DynamoServiceRolesAnnotation etc.

	// Service role values for the service-roles annotation. The order in the
	// annotation must match the order of Workload.Spec.Resources.
	DynamoRoleFrontend = "frontend"
	DynamoRoleWorker   = "worker"
	DynamoRolePrefill  = "prefill"
	DynamoRoleDecode   = "decode"
	DynamoRolePlanner  = "planner"
	DynamoRoleEpp      = "epp"

	// KV transfer backend values. nixl is the default on MI300X + IB; mori
	// and mooncake are reserved for MI355X + ionic NIC scenarios.
	DynamoKVBackendNixl     = "nixl"
	DynamoKVBackendMori     = "mori"
	DynamoKVBackendMooncake = "mooncake"

	// DGD CRD kind on the K8s side. SaFE's Workload.Spec.Kind is
	// DynamoDeploymentKind (a SaFE-level abstraction); the dispatcher renders
	// it into a DynamoGraphDeployment object whose actual K8s kind matches
	// the constant below. The syncer must register this kind to feed DGD
	// status events back into the admin workload.
	DynamoGraphDeploymentKind = "DynamoGraphDeployment"

	// Defaults applied when the corresponding annotation is missing.
	DynamoDefaultKVBackend        = DynamoKVBackendNixl
	DynamoDefaultBackendFramework = "sglang"

	// Cluster-wide infrastructure addresses installed by the Phase 1 SaFE
	// addon. The dispatcher and webhook inject these into every dynamo pod
	// so user-supplied yaml does not need to hard-code them. The dispatcher
	// template ConfigMap (charts/primus-safe/templates/configmap/
	// dynamo_deployment_template.yaml) also references the same defaults to
	// keep the rendered DGD object self-contained when no Workload override
	// is provided.
	DynamoDefaultNatsURL          = "nats://nats.primus-safe.svc:4222"
	DynamoDefaultDiscoveryBackend = "kubernetes"
)
