package image_handlers

const (
	OSArchFormat = "%s/%s"
	DefaultOS    = "linux"
	DefaultArch  = "amd64"
	OsArchAll    = "ALL"
)

const (
	DefaultQueryLimit     = 50
	DefaultNamespace      = "primus-safe"
	ImageImportSecretName = "primus-safe-image-import-reg-cred"
	ImagePullSecretName   = "primus-safe-image"
)

const (
	// Global env
	DEBUG           = "DEBUG"
	GlobalTLSVerify = "GLOBAL_TLS_VERIFY"
	OverrideArch    = "OVERRIDE_ARCH"
	OverrideOS      = "OVERRIDE_OS"
	CommandTimeout  = "COMMAND_TIMEOUT"
	SourceType      = "SOURCE_TYPE"
	DestinationType = "DESTINATION_TYPE"
	All             = "SYNC_ALL_ARCH"

	// src env
	SrcUserName = "SOURCE_USERNAME"
	SrcPassword = "SOURCE_PASSWORD"
	TLSVerify   = "SOURCE_TLS_VERIFY"

	// dest env
	DestUserName  = "DESTINATION_USERNAME"
	DestPassword  = "DESTINATION_PASSWORD"
	DestTLSVerify = "DESTINATION_TLS_VERIFY"

	// image env
	SrcImageEnv  = "SRC_IMAGE"
	DestImageEnv = "DEST_IMAGE"

	// upstream domain
	UpstreamDomain = "UPSTREAM_DOMAIN"

	ApiServiceName = "primus-safe-apiserver.primus-safe.svc.cluster.local:8088"
)

const (
	StringValueTrue = "true"

	ImportImageJobLabelKey = "image-import"
	// importimg image job labels
	ImportImageSourceLabelKey = "image-import-source"
	ImportImageTargetLabelKey = "image-import-target"
	ImportImageOSArchLabelKey = "image-import-os-arch"

	ImageStatusKey      = "status"
	ImportImageStateKey = "state"
	ImportImageLogKey   = "log"
	ImageRelationDigest = "relation_digest"
)

const (
	SyncImageProject = "sync"
)

const (
	SyncerImage = "docker.io/primussafe/image-sync:202510121649"
)
