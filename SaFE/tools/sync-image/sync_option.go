package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
	"github.com/sirupsen/logrus"
)

// repoDescriptor contains information of a single repository used as a sync source.
type repoDescriptor struct {
	DirBasePath string                 // base path when source is 'dir'
	ImageRefs   []types.ImageReference // List of tagged image found for the repository
	Context     *types.SystemContext   // SystemContext for the sync command
}

type GlobalOptions struct {
	Debug              bool          `yaml:"debug"`                // Enable debug output
	TLSVerify          bool          `yaml:"tls_verify"`           // Require HTTPS and verify certificates (for docker: and docker-daemon:)
	PolicyPath         string        `yaml:"policy_path"`          // Path to a signature verification policy file
	InsecurePolicy     bool          `yaml:"insecure_policy"`      // Use an "allow everything" signature verification policy
	RegistriesDirPath  string        `yaml:"registries_dir_path"`  // Path to a "registries.d" registry
	OverrideArch       string        `yaml:"override_arch"`        // Architecture to use for choosing images, instead of the runtime one
	OverrideOS         string        `yaml:"override_os"`          // OS to use for choosing images, instead of the runtime one
	OverrideVariant    string        `yaml:"override_variant"`     // Architecture variant to use for choosing images, instead of the runtime one
	CommandTimeout     time.Duration `yaml:"command_timeout"`      // Timeout for the command execution
	RegistriesConfPath string        `yaml:"registries_conf_path"` // Path to the "registries.conf" file
	TmpDir             string        `yaml:"tmp_dir"`              // Path to use for big temporary files
}

// newSystemContext returns a *types.SystemContext corresponding to opts.
// It is guaranteed to return a fresh instance, so it is safe to make additional updates to it.
func (opts *GlobalOptions) newSystemContext() *types.SystemContext {
	ctx := &types.SystemContext{
		RegistriesDirPath:        opts.RegistriesDirPath,
		ArchitectureChoice:       opts.OverrideArch,
		OSChoice:                 opts.OverrideOS,
		VariantChoice:            opts.OverrideVariant,
		SystemRegistriesConfPath: opts.RegistriesConfPath,
		BigFilesTemporaryDir:     opts.TmpDir,
		DockerRegistryUserAgent:  defaultUserAgent,
	}
	// DEPRECATED: We support this for backward compatibility, but override it if a per-image flag is provided.
	if opts.TLSVerify {
		ctx.DockerInsecureSkipTLSVerify = types.NewOptionalBool(!opts.TLSVerify)
	}
	return ctx
}

// getPolicyContext returns a *signature.PolicyContext based on opts.
func (opts *GlobalOptions) getPolicyContext() (*signature.PolicyContext, error) {
	var policy *signature.Policy // This could be cached across calls in opts.
	var err error
	if opts.InsecurePolicy {
		policy = &signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}}
	} else if opts.PolicyPath == "" {
		policy, err = signature.DefaultPolicy(nil)
	} else {
		policy, err = signature.NewPolicyFromFile(opts.PolicyPath)
	}
	if err != nil {
		return nil, err
	}
	return signature.NewPolicyContext(policy)
}

// commandTimeoutContext returns a context.Context and a cancellation callback based on opts.
// The caller should usually "defer cancel()" immediately after calling this.
func (opts *GlobalOptions) commandTimeoutContext() (context.Context, context.CancelFunc) {
	ctx := context.Background()
	var cancel context.CancelFunc = func() {}
	if opts.CommandTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, opts.CommandTimeout)
	}
	return ctx, cancel
}

// deprecatedTLSVerifyOption represents a deprecated --tls-verify option,
// which was accepted for all subcommands, for a time.
// Every user should call deprecatedTLSVerifyOption.warnIfUsed() as part of handling the CLI,
// whether or not the value actually ends up being used.
// DO NOT ADD ANY NEW USES OF THIS; just call dockerImageFlags with an appropriate, possibly empty, flagPrefix.
type DeprecatedTLSVerifyOption struct {
	TLSVerify bool `yaml:"tls_verify"` // FIXME FIXME: Warn if this is used, or even if it is ignored.
}

// warnIfUsed warns if tlsVerify was set by the user, and suggests alternatives (which should
// start with "--").
// Every user should call this as part of handling the CLI, whether or not the value actually
// ends up being used.
func (opts *DeprecatedTLSVerifyOption) warnIfUsed(alternatives []string) {
	if opts.TLSVerify {
		logrus.Warnf("'--tls-verify' is deprecated, instead use: %s", strings.Join(alternatives, ", "))
	}
}

// imageOptions collects CLI flags which are the same across subcommands, but may be different for each image
// (e.g. may differ between the source and destination of a copy)
type ImageOptions struct {
	DockerImageOptions `yaml:"docker_image_options"` // May be shared across several imageOptions instances.
	SharedBlobDir      string                        `yaml:"shared_blob_dir"`    // A directory to use for OCI blobs, shared across repositories
	DockerDaemonHost   string                        `yaml:"docker_daemon_host"` // docker-daemon: host to connect to
}

// newSystemContext returns a *types.SystemContext corresponding to opts.
// It is guaranteed to return a fresh instance, so it is safe to make additional updates to it.
func (opts *ImageOptions) newSystemContext() (*types.SystemContext, error) {
	// *types.SystemContext instance from globalOptions
	//  imageOptions option overrides the instance if both are present.
	ctx := opts.Global.newSystemContext()
	ctx.DockerCertPath = opts.DockerCertPath
	ctx.OCISharedBlobDirPath = opts.SharedBlobDir
	ctx.AuthFilePath = opts.Shared.AuthFilePath
	ctx.DockerDaemonHost = opts.DockerDaemonHost
	ctx.DockerDaemonCertPath = opts.DockerCertPath
	if len(opts.DockerImageOptions.AuthFilePath) > 0 {
		ctx.AuthFilePath = opts.DockerImageOptions.AuthFilePath
	}
	if opts.DeprecatedTLSVerify != nil && opts.DeprecatedTLSVerify.TLSVerify {
		// If both this deprecated option and a non-deprecated option is present, we use the latter value.
		ctx.DockerInsecureSkipTLSVerify = types.NewOptionalBool(!opts.DeprecatedTLSVerify.TLSVerify)
	}
	if opts.TlsVerify {
		ctx.DockerDaemonInsecureSkipTLSVerify = !opts.TlsVerify
	}
	if opts.TlsVerify {
		ctx.DockerInsecureSkipTLSVerify = types.NewOptionalBool(opts.TlsVerify)
	}
	if opts.CredsOption != "" && opts.NoCreds {
		return nil, errors.New("creds and no-creds cannot be specified at the same time")
	}
	if opts.UserName != "" && opts.NoCreds {
		return nil, errors.New("username and no-creds cannot be specified at the same time")
	}
	if opts.CredsOption != "" && opts.UserName != "" {
		return nil, errors.New("creds and username cannot be specified at the same time")
	}
	// if any of username or password is present, then both are expected to be present
	if (opts.UserName != "") != (opts.Password != "") {
		if opts.UserName != "" {
			return nil, errors.New("password must be specified when username is specified")
		}
		return nil, errors.New("username must be specified when password is specified")
	}
	if opts.CredsOption != "" {
		var err error
		ctx.DockerAuthConfig, err = getDockerAuth(opts.CredsOption)
		if err != nil {
			return nil, err
		}
	} else if opts.UserName != "" {
		ctx.DockerAuthConfig = &types.DockerAuthConfig{
			Username: opts.UserName,
			Password: opts.Password,
		}
	}
	if opts.RegistryToken != "" {
		ctx.DockerBearerRegistryToken = opts.RegistryToken
	}
	if opts.NoCreds {
		ctx.DockerAuthConfig = &types.DockerAuthConfig{}
	}

	return ctx, nil
}

// dockerImageOptions collects CLI flags specific to the "docker" transport, which are
// the same across subcommands, but may be different for each image
// (e.g. may differ between the source and destination of a copy)
type DockerImageOptions struct {
	Global              *GlobalOptions             `yaml:"global"`                // May be shared across several imageOptions instances.
	Shared              *SharedImageOptions        `yaml:"shared"`                // May be shared across several imageOptions instances.
	DeprecatedTLSVerify *DeprecatedTLSVerifyOption `yaml:"deprecated_tls_verify"` // May be shared across several imageOptions instances, or nil.
	AuthFilePath        string                     `yaml:"auth_file_path"`        // Path to a */containers/auth.json (prefixed version to override shared image option).
	CredsOption         string                     `yaml:"creds_option"`          // username[:password] for accessing a registry
	UserName            string                     `yaml:"username"`              // username for accessing a registry
	Password            string                     `yaml:"password"`              // password for accessing a registry
	RegistryToken       string                     `yaml:"registry_token"`        // token to be used directly as a Bearer token when accessing the registry
	DockerCertPath      string                     `yaml:"docker_cert_path"`      // A directory using Docker-like *.{crt,cert,key} files for connecting to a registry or a daemon
	TlsVerify           bool                       `yaml:"tls_verify"`            // Require HTTPS and verify certificates (for docker: and docker-daemon:)
	NoCreds             bool                       `yaml:"no_creds"`              // Access the registry anonymously
}

// sharedImageOptions collects CLI flags which are image-related, but do not change across images.
// This really should be a part of globalOptions, but that would break existing users of (skopeo copy --authfile=).
type SharedImageOptions struct {
	AuthFilePath string `yaml:"auth_file_path"` // Path to a */containers/auth.json
}

// imageDestOptions is a superset of imageOptions specialized for image destinations.
// Every user should call imageDestOptions.warnAboutIneffectiveOptions() as part of handling the CLI
type ImageDestOptions struct {
	*ImageOptions               `yaml:"image_options"` // May be shared across several imageOptions instances.
	DirForceCompression         bool                   `yaml:"dir_force_compression"`          // Compress layers when saving to the dir: transport
	DirForceDecompression       bool                   `yaml:"dir_force_decompression"`        // Decompress layers when saving to the dir: transport
	OciAcceptUncompressedLayers bool                   `yaml:"oci_accept_uncompressed_layers"` // Whether to accept uncompressed layers in the oci: transport
	CompressionFormat           string                 `yaml:"compression_format"`             // Format to use for the compression
	CompressionLevel            int                    `yaml:"compression_level"`              // Level to use for the compression
	PrecomputeDigests           bool                   `yaml:"precompute_digests"`             // Precompute digests to dedup layers when saving to the docker: transport
	ImageDestFlagPrefix         string                 `yaml:"image_dest_flag_prefix"`
}

// warnAboutIneffectiveOptions warns if any ineffective option was set by the user
// Every user should call this as part of handling the CLI
func (opts *ImageDestOptions) warnAboutIneffectiveOptions(destTransport types.ImageTransport) {
	if destTransport.Name() != directory.Transport.Name() {
		if opts.DirForceCompression {
			logrus.Warnf("--%s can only be used if the destination transport is 'dir'", opts.ImageDestFlagPrefix+"compress")
		}
		if opts.DirForceDecompression {
			logrus.Warnf("--%s can only be used if the destination transport is 'dir'", opts.ImageDestFlagPrefix+"decompress")
		}
	}
}

// errorShouldDisplayUsage is a subtype of error used by command handlers to indicate that cli.ShowSubcommandHelp should be called.
type errorShouldDisplayUsage struct {
	error
}

type reTryOptions struct {
	MaxRetry         int              `yaml:"max_retry"`
	Delay            time.Duration    `yaml:"delay"`
	IsErrorRetryable func(error) bool `yaml:"-"`
}
