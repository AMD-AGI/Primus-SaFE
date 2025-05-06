/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package v1

const (
	SafePrefix = "safe."

	// common
	UncontrolledAnnotation = SafePrefix + "uncontrolled"
	// 各个cr用于对外展示的名字
	DisplayNameLabel = SafePrefix + "display.name"
	SecretMd5Label   = SafePrefix + "secret.md5"
	// 总失败次数，内部重试的场景使用
	FailedCountAnnotation      = SafePrefix + "failed.count"
	DescriptionAnnotation      = "description"
	ImagePullSecretsAnnotation = "imagePullSecrets"
	KindLabel                  = "kind"
	// 是否禁用挂载分布式文件系统(xpfs/gpfs)
	DisableDFSAnnotation = SafePrefix + "disable.dfs"
	PPCountAnnotation    = SafePrefix + "pp.count"
	TPCountAnnotation    = SafePrefix + "tp.count"
	// 通过safe创建的 object
	SafeCreated = "safe.01ai.io/created"
	// safe公共概念，任何人只要有权限就可以访问，不受租户隔离影响
	SafePublicLabel = "safe.public"
	// master节点标记
	KubernetesControlPlane = "node-role.kubernetes.io/control-plane"
	// 受限label
	RestrictedLabel = "restricted"
	// 备份label
	BackupLabel = SafePrefix + "backup"
	// 芯片类型
	ChipTypeAnnotation = SafePrefix + "chip.type"
	// 节点npu芯片名
	NpuChipNameLabel   = "node.kubernetes.io/npu.chip.name"
	VirtualMachineKind = "VirtualMachine"

	// 是否同步给数据面，value是数据cluster名，如果配置 '*'就是对应所有数据面，配置具体的cluster则只同步该cluster, 如果配置多个用逗号分割
	SyncDataPlanes = "syncDataPlanes"
	// 数据面对应的namespace，这个目前给同步configmap用，如果不指定，则用default
	DataPlaneNamespace = "dataPlaneNamespace"
	// 数据面对应的name，这个目前给同步configmap用，如果不指定，则用管理面cm的name
	DataPlaneName = "dataPlaneName"
	// 是否禁止自动更新configmap到数据面，设置true是不自动更新（也就是不做update操作，但会执行create)
	DisableAutoUpdate = "disableAutoUpdate"

	// user
	UserPrefix              = SafePrefix + "user."
	UserIdLabel             = UserPrefix + "id"
	UserLoginNameLabel      = UserPrefix + "login.name"
	UserNameMd5Label        = UserPrefix + "name.md5"
	UserNameAnnotation      = UserPrefix + "name"
	UserEmailMd5Label       = UserPrefix + "email.md5"
	UserEmailAnnotation     = UserPrefix + "email"
	UserLastLoginAnnotation = UserPrefix + "last.login"
	UserRoleExpirationLabel = UserPrefix + "role.expiration" // 角色过期时间，搭配支持角色过期的角色白名单使用
	UserAvatarUrlAnnotation = UserPrefix + "avatar.url"
	// 用户雇佣类型：比如实习生
	UserEmployeeTypeLabel = UserPrefix + "employee.type"
	// 是否cicd审批人
	UserCicdApproverAnnotation = UserPrefix + "cicd.approver"
	CreatorUserIdLabel         = UserPrefix + "creator.id"

	// tenant
	TenantPrefix         = SafePrefix + "tenant."
	TenantIdLabel        = TenantPrefix + "id"
	TenantNameMd5Label   = TenantPrefix + "name.md5"
	TenantNameAnnotation = TenantPrefix + "name"
	CreatorTenantIdLabel = TenantPrefix + "creator.id"
	TenantProtectLabel   = TenantPrefix + "protect"
	TenantFinalizer      = TenantPrefix + "finalizer"

	// cluster
	ClusterPrefix       = SafePrefix + "cluster."
	ClusterNameLabel    = ClusterPrefix + "name"
	ClusterOldNameLabel = ClusterPrefix + "old.name"
	ClusterFinalizer    = ClusterPrefix + "finalizer"
	ClusterProtectLabel = ClusterPrefix + "protect"
	ClusterTypeLabel    = ClusterPrefix + "type"

	// node
	NodePrefix = SafePrefix + "node."
	// 节点绑定的专属队列（默认）
	NodeBindWorkspaceLabel = SafePrefix + "bind.workspace"
	// 节点绑定的弹性队列
	NodeBindElasticWorkspaceLabel = SafePrefix + "bind.elastic.workspace"
	NodeIdLabel                   = NodePrefix + "id"
	// 节点期望的gpu count，必须放label，这样才会自动同步到machine node
	NodeGpuCountLabel = NodePrefix + "gpu.count"
	// 节点数据盘信息
	NodeDataDiskAnnotation = NodePrefix + "data.disk"
	NodeRegionPodLabel     = NodePrefix + "region.pod"
	// 节点上次启动的时间
	NodeStartupTimeLabel         = NodePrefix + "startup.time"
	NodeFinalizer                = NodePrefix + "finalizer"
	NodeInspectionTimeAnnotation = NodePrefix + "inspection.time"
	NodePrivateIpLabel           = NodePrefix + "private.ip"
	NodeTemplateAnnotation       = NodePrefix + "node.template"
	// 给节点的任务输入
	NodeJobInputAnnotation = NodePrefix + "job.input"
	// 标记当前节点是pytorchjob训练时按照rank排序后的最后一个节点
	LastNodeLabel = SafePrefix + "last.node"

	// 节点操作类型，目前支持add和remove
	NodesWorkspaceAction  = SafePrefix + "nodes.workspace.action"
	NodesLabelAction      = SafePrefix + "nodes.label.action"
	NodesAnnotationAction = SafePrefix + "nodes.annotation.action"
	NodeActionAdd         = "add"
	NodeActionRemove      = "remove"

	// nodeflavor
	NodeFlavorPrefix = SafePrefix + "nodeflavor."
	NodeFlavorLabel  = NodeFlavorPrefix + "name"

	// addOn template
	AddOnTemplatePrefix         = SafePrefix + "addon.template."
	AddOnTemplateComponentLabel = AddOnTemplatePrefix + "component"
	AddOnTemplateVersionLabel   = AddOnTemplatePrefix + "version"

	// fault
	FaultPrefix        = SafePrefix + "fault."
	FaultFinalizer     = FaultPrefix + "finalizer"
	FaultIDLabel       = FaultPrefix + "id"
	FaultIDsAnnotation = FaultPrefix + "ids"
	// safe污点固定前缀
	SafeTaintPrefix = "safe.01ai/"

	// workload
	WorkloadPrefix    = SafePrefix + "workload."
	WorkloadIdLabel   = WorkloadPrefix + "id"
	WorkloadFinalizer = WorkloadPrefix + "finalizer"
	// 当前下发次数, 备注：必须是label，用于log查询
	WorkloadDispatchCntLabel = WorkloadPrefix + "dispatch.count"
	// 标记任务是否成功下发
	WorkloadDispatchedAnnotation = WorkloadPrefix + "dispatched"
	// 标记任务是否经过scheduler调度
	WorkloadScheduledAnnotation = WorkloadPrefix + "scheduled"
	// WorkloadScheduledAdvanceAnnotation 标记任务是否有优先调度
	WorkloadScheduledAdvanceAnnotation = WorkloadPrefix + "scheduled.advance"
	// 标记任务是否关闭failover，默认开启
	WorkloadDisableFailoverAnnotation = WorkloadPrefix + "disable.failover"
	// 标记任务是否开启抢占，默认关闭
	WorkloadEnablePreemptAnnotation = WorkloadPrefix + "enable.preempt"
	// 任务主容器名
	WorkloadMainContainer = WorkloadPrefix + "main.container"
	// 标记任务是否被抢占
	WorkloadPreemptedAnnotation = WorkloadPrefix + "preempted"
	// 标记任务是否强制做fo
	WorkloadForcedFoAnnotation = WorkloadPrefix + "forced.failover"
	// 标记任务类型
	DevelopMachineLabel = WorkloadPrefix + "develop.machine"
	// 标记任务是否开启hostnetwork，默认开启
	EnableHostNetworkAnnotation = WorkloadPrefix + "enable.host.network"
	// 任务执行定时扩缩容，value是扩缩之前的初始值：保存的是map序列化后的string, map key是resource_name（比如Master), value是replica
	CronScaleInitialAnnotation = WorkloadPrefix + "cron.scale.initial"
	// workload最大运行时长，单位小时，annotation字段
	WorkloadMaxRuntimeHour = WorkloadPrefix + "max.runtime.hour"
	// workload运行时长，annotation字段
	WorkloadRunTimeAnnotation = WorkloadPrefix + "runtime"

	// exporter
	ExporterPrefix    = SafePrefix + "exporter."
	ExporterFinalizer = ExporterPrefix + "finalizer"

	// workspace
	WorkspacePrefix    = SafePrefix + "workspace."
	WorkspaceFinalizer = WorkspacePrefix + "finalizer"
	// 管理面workspace id
	WorkspaceIdLabel    = WorkspacePrefix + "id"
	WorkspaceOldIdLabel = WorkspacePrefix + "old.id"
	// 标记一个workspace的调度策略，value值是具体策略目前支持dp优先和pp优先（默认)
	SchedulerPolicyAnnotation = SafePrefix + "scheduler.policy"
	// 使用平衡策略排队时，队首元素等待超时时间(单位秒)，超时后会做遍历，查看下一个任务
	QueueBalanceTimeoutAnnotation = "queue.balance.timeout"
	// 工作空间类型, 目前只给workspace和workload做了设置
	WorkspaceTypeLabel = WorkspacePrefix + "type"
	// 是否开启抢占，有label就认为开启，不考虑value
	WorkspaceEnablePreemptLabel = WorkspacePrefix + "preempt"

	// job
	JobPrefix = SafePrefix + "job."
	// job下发时间
	JobDispatchTimeAnnotation = JobPrefix + "dispatch.time"
	JobIdLabel                = JobPrefix + "id"
	JobTypeLabel              = JobPrefix + "type"
	JobUserAnnotation         = JobPrefix + "user"
	// 插件安全升级
	JobSecurityUpgradeAnnotation = JobPrefix + "security.upgrade"
	JobBatchCountAnnotation      = JobPrefix + "batch.count"
	JobFinalizer                 = JobPrefix + "finalizer"

	// dataset
	DatasetPrefix  = SafePrefix + "dataset."
	DataSetIdLabel = DatasetPrefix + "id"

	// llm model
	LlmModelPrefix            = SafePrefix + "llm.model."
	LlmModelIdLabel           = LlmModelPrefix + "id"             // 模型 unique_key
	LlmModelGroupIdLabel      = LlmModelPrefix + "group.id"       // 模型组 unique_key
	LlmModelTaskConfigIdLabel = LlmModelPrefix + "task.config.id" // 模型任务配置 unique_key

	// kubernetes
	KubernetesPrefix                  = SafePrefix + "kubernetes."
	KubernetesManageActionLabel       = KubernetesPrefix + "managed.action"
	KubernetesManageNodeLabel         = KubernetesPrefix + "managed.node"
	KubernetesManageClusterLabel      = KubernetesPrefix + "managed.cluster"
	KubernetesManageClusterHostsLabel = KubernetesPrefix + "managed.hosts"
	KubernetesManageNodeClusterLabel  = KubernetesPrefix + "managed.node.cluster"
	KubernetesManageScaleDownLabel    = KubernetesPrefix + "managed.scale.down"
	KubernetesServiceName             = KubernetesPrefix + "service.name"

	// storage
	StoragePrefix              = SafePrefix + "storage."
	StorageDefaultClusterLabel = StoragePrefix + "default.cluster"
	StorageTypeLabel           = StoragePrefix + "type"
	StorageClusterNameLabel    = StoragePrefix + "cluster.name"

	// secret
	SecretPrefix    = SafePrefix + "secret."
	SecretTypeLabel = SecretPrefix + "type"

	// 报警组
	AlertGroupLabel = SafePrefix + "alert.group"

	DomainLabel             = SafePrefix + "resource.domain"
	DomainTypeLabel         = SafePrefix + "resource.domain.type"
	DomainBindLabel         = SafePrefix + "resource.domain.bind"
	VirtualService          = SafePrefix + "virtual.service"
	VirtualServiceNamespace = SafePrefix + "virtual.service.namespace"
	CertificateLabel        = SafePrefix + "resource.certificate"

	// certificate
	CertificatePrefix           = SafePrefix + "certificate."
	CertificateNameLabel        = CertificatePrefix + "name"
	CertificateClusterNameLabel = CertificatePrefix + "cluster.name"

	// domain
	DomainPrefix               = SafePrefix + "domain."
	DomainGatewayLabel         = DomainPrefix + "gateway"
	DomainVirtualLabel         = DomainPrefix + "virtual"
	DomainNameLabel            = DomainPrefix + "name"
	DomainClusterNameLabel     = DomainPrefix + "cluster.name"
	DomainCertificateNameLabel = DomainPrefix + "certificate.name"
	DomainProtocolLabel        = DomainPrefix + "protocol"
	DomainPortLabel            = DomainPrefix + "port"
	DomainTaskTypeLabel        = DomainPrefix + "task.type"

	// evalTask
	EvalTaskPrefix      = SafePrefix + "eval.task."
	EvalTaskFinalizer   = EvalTaskPrefix + "finalizer"
	EvalTaskNameLabel   = EvalTaskPrefix + "name"
	EvalTaskUserIDLabel = EvalTaskPrefix + "user.id"
	EvalTaskIDLabel     = EvalTaskPrefix + "id"

	// inference
	InferencePrefix      = SafePrefix + "inference."
	InferenceFinalizer   = InferencePrefix + "finalizer"
	InferenceNameLabel   = InferencePrefix + "name"
	InferenceUserIDLabel = InferencePrefix + "user.id"

	// trainTask
	TrainTaskPrefix        = SafePrefix + "train.task."
	TrainTaskFinalizer     = TrainTaskPrefix + "finalizer"
	TrainTaskNameLabel     = TrainTaskPrefix + "name"
	TrainTaskUserNameLabel = TrainTaskPrefix + "user.name"
	TrainTaskUserIDLabel   = TrainTaskPrefix + "user.id"
	TrainTaskIDLabel       = TrainTaskPrefix + "id"
)

const (
	// XOS

	// Postgres
	ClusterStatusUnknown      = ""
	ClusterStatusCreating     = "Creating"
	ClusterStatusCreateFailed = "CreateFailed"
	ClusterStatusUpdating     = "Updating"
	ClusterStatusUpdateFailed = "UpdateFailed"
	ClusterStatusSyncFailed   = "SyncFailed"
	ClusterStatusAddFailed    = "CreateFailed"
	ClusterStatusRunning      = "running"
	ClusterStatusInvalid      = "Invalid"
	ClusterStatusReconciling  = "Reconciling"
	serviceNameMaxLength      = 63
	clusterNameMaxLength      = serviceNameMaxLength - len("-repl")
	serviceNameRegexString    = `^[a-z]([-a-z0-9]*[a-z0-9])?$`
)
