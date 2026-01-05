/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

// Using skopeo for data synchronization
// https://github.com/containers/skopeo/blob/main/cmd/skopeo/sync.go

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/containers/common/pkg/retry"
	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/cli"
	"github.com/containers/image/v5/pkg/cli/sigstore"
	"github.com/containers/image/v5/signature/signer"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
	"github.com/sirupsen/logrus"
)

const (
	// image env
	SrcImageEnv  = "SRC_IMAGE"
	DestImageEnv = "DEST_IMAGE"

	// Version is the version of the build.
	Version = "v0.1.0"
)

var (
	SrcImage  = os.Getenv(SrcImageEnv)  //  e.g: ollama/ollama:latest
	DestImage = os.Getenv(DestImageEnv) //  e.g: harbor.my.domain/my-repo/test/

	defaultUserAgent = "primussafe/" + Version
)

func main() {

	SrcImage = os.Getenv(SrcImageEnv) // e.g: ollama/ollama:latest
	DestImage = os.Getenv(DestImageEnv)

	config, err := parseTemplateConfig()
	if err != nil {
		fmt.Printf("error parse template config, %v", err)
		return
	}

	if config.Global.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	logrus.Debugf("Parsed Configuration: %+v\n", config)

	logrus.Debugf("Global Options: %+v\n", config.Global)
	logrus.Debugf("Source Image Options: %+v\n", config.SrcImage.DockerImageOptions)
	logrus.Debugf("Destination Image Options: %+v\n", config.DestImage.ImageOptions)
	logrus.Debugf("Retry Options: %+v\n", config.RetryOpts)

	if err := config.run([]string{SrcImage, DestImage}, os.Stdout); err != nil {
		logrus.Errorf("error exec sync %s to %s, %v ", SrcImage, DestImage, err)
		return
	}
	logrus.Infof("sync %s to %s successfully", SrcImage, DestImage)
}

func (opts *syncOptions) run(args []string, stdout io.Writer) (retErr error) {
	// args[0]：registry.example.com/busybox
	// args[1]：/media/usb
	if len(args) != 2 {
		return errors.New("exactly two arguments expected")
	}
	opts.DeprecatedTLSVerify.warnIfUsed([]string{"--src-tls-verify", "--dest-tls-verify"})

	// remove \n \t
	args = parseStr(args)

	logrus.Infof("sync image from %s to %s", args[0], args[1])

	policyContext, err := opts.Global.getPolicyContext()
	if err != nil {
		return fmt.Errorf("error loading trust policy: %s", err)
	}
	defer func() {
		if err := policyContext.Destroy(); err != nil {
			retErr = noteCloseFailure(retErr, "tearing down policy context", err)
		}
	}()

	// validate source and destination options
	if err := reqValid(opts.Source, opts.Destination); err != nil {
		return err
	}
	logrus.Infof("validating source and destination options success")

	opts.DestImage.warnAboutIneffectiveOptions(transports.Get(opts.Destination))

	imageListSelection := copy.CopySystemImage
	if opts.All {
		imageListSelection = copy.CopyAllImages
	}

	opts.SrcImage.Global = opts.Global
	opts.DestImage.Global = opts.Global

	sourceCtx, err := opts.SrcImage.newSystemContext()
	if err != nil {
		return err
	}

	var manifestType string
	if len(opts.Format) > 0 {
		manifestType, err = parseManifestFormat(opts.Format)
		if err != nil {
			return err
		}
	}

	ctx, cancel := opts.Global.commandTimeoutContext()
	defer cancel()

	sourceArg := args[0]
	var srcRepoList []repoDescriptor
	if err = retry.IfNecessary(ctx, func() error {
		srcRepoList, err = imagesToCopy(sourceArg, opts.Source, sourceCtx)
		return err
	}, &retry.Options{
		MaxRetry:         opts.RetryOpts.MaxRetry,
		Delay:            opts.RetryOpts.Delay,
		IsErrorRetryable: retry.IsErrorRetryable,
	}); err != nil {
		return err
	}

	destination := args[1]
	destinationCtx, err := opts.DestImage.newSystemContext()
	if err != nil {
		return err
	}

	// c/image/copy.Image does allow creating both simple signing and sigstore signatures simultaneously,
	// with independent passphrases, but that would make the CLI probably too confusing.
	// For now, use the passphrase with either, but only one of them.
	if opts.SignPassphraseFile != "" && opts.SignByFingerprint != "" && opts.SignBySigstorePrivateKey != "" {
		return fmt.Errorf("Only one of --sign-by and sign-by-sigstore-private-key can be used with sign-passphrase-file")
	}
	var passphrase string
	if opts.SignPassphraseFile != "" {
		p, err := cli.ReadPassphraseFile(opts.SignPassphraseFile)
		if err != nil {
			return err
		}
		passphrase = p
	} else if opts.SignBySigstorePrivateKey != "" {
		p, err := promptForPassphrase(opts.SignBySigstorePrivateKey, os.Stdin, os.Stdout)
		if err != nil {
			return err
		}
		passphrase = p
	}

	var signers []*signer.Signer
	if opts.SignBySigstoreParamFile != "" {
		inst, err := sigstore.NewSignerFromParameterFile(opts.SignBySigstoreParamFile, &sigstore.Options{
			PrivateKeyPassphrasePrompt: func(keyFile string) (string, error) {
				return promptForPassphrase(keyFile, os.Stdin, os.Stdout)
			},
			Stdin:  os.Stdin,
			Stdout: stdout,
		})
		if err != nil {
			return fmt.Errorf("error using --sign-by-sigstore: %s", err)
		}
		defer inst.Close()
		signers = append(signers, inst)
	}

	options := copy.Options{
		RemoveSignatures:                      opts.RemoveSignatures,
		Signers:                               signers,
		SignBy:                                opts.SignByFingerprint,
		SignPassphrase:                        passphrase,
		SignBySigstorePrivateKeyFile:          opts.SignBySigstorePrivateKey,
		SignSigstorePrivateKeyPassphrase:      []byte(passphrase),
		ReportWriter:                          stdout,
		ProgressInterval:                      15,
		Progress:                              make(chan types.ProgressProperties),
		DestinationCtx:                        destinationCtx,
		ImageListSelection:                    imageListSelection,
		PreserveDigests:                       opts.PreserveDigests,
		OptimizeDestinationImageAlreadyExists: true,
		ForceManifestMIMEType:                 manifestType,
	}
	errorsPresent := false
	imagesNumber := 0
	if opts.DryRun {
		logrus.Warn("Running in dry-run mode")
	}

	var digestFile *os.File
	if opts.DigestFile != "" && !opts.DryRun {
		digestFile, err = os.OpenFile(opts.DigestFile, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("error creating digest file: %w", err)
		}
		defer func() {
			if err := digestFile.Close(); err != nil {
				retErr = noteCloseFailure(retErr, "closing digest file", err)
			}
		}()
	}
	go func() {
		data := &UpstreamEvent{
			Data: make(map[string]types.ProgressProperties),
		}
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := upstreamData(opts.UpstreamDomain, destination, data); err != nil {
					logrus.WithError(err).Error("Error sending upsteam data")
				}
			case p, ok := <-options.Progress:
				if !ok {
					return
				}
				data.Data[p.Artifact.Digest.String()] = p
				switch p.Event {
				case types.ProgressEventSkipped, types.ProgressEventDone:
					if p.Event == types.ProgressEventSkipped {
						data.SkipLayerCount++
						data.SyncLayerCount++
					} else {
						data.ComplexLayerCount++
						data.SyncLayerCount++
					}
					if err := upstreamData(opts.UpstreamDomain, destination, data); err != nil {
						logrus.WithError(err).Error("Error sending upsteam data")
					}
				}
			}
		}
	}()
	defer func() {
		close(options.Progress)
	}()

	for _, srcRepo := range srcRepoList {
		options.SourceCtx = srcRepo.Context
		for counter, ref := range srcRepo.ImageRefs {
			var destSuffix string
			var manifestBytes []byte
			switch ref.Transport() {
			case docker.Transport:
				// docker -> dir or docker -> docker
				destSuffix = ref.DockerReference().String()
			case directory.Transport:
				// dir -> docker (we don't allow `dir` -> `dir` sync operations)
				destSuffix = strings.TrimPrefix(ref.StringWithinTransport(), srcRepo.DirBasePath)
				if destSuffix == "" {
					// if source is a full path to an image, have destPath scoped to repo:tag
					destSuffix = path.Base(srcRepo.DirBasePath)
				}
			}

			if !opts.Scoped {
				destSuffix = path.Base(destSuffix)
			}

			var destRef types.ImageReference
			if strings.HasSuffix(destination, destSuffix) { // e.g: harbor.my.domain/my-repo/test/ollama/ollama:latest
				destRef, err = destinationReference(destination, opts.Destination)
				if err != nil {
					return
				}
			} else { //  e.g: harbor.my.domain/my-repo/test/ollama -> harbor.my.domain/my-repo/test/ollama/ollama:latest
				destRef, err = destinationReference(path.Join(destination, destSuffix)+opts.AppendSuffix, opts.Destination)
				if err != nil {
					return err
				}
			}

			fromToFields := logrus.Fields{
				"from": transports.ImageName(ref),
				"to":   transports.ImageName(destRef),
			}
			if opts.DryRun {
				logrus.WithFields(fromToFields).Infof("Would have copied image ref %d/%d", counter+1, len(srcRepo.ImageRefs))
			} else {
				logrus.WithFields(fromToFields).Infof("Copying image ref %d/%d", counter+1, len(srcRepo.ImageRefs))
				if err = retry.IfNecessary(ctx, func() error {
					manifestBytes, err = copy.Image(ctx, policyContext, destRef, ref, &options)
					return err
				}, &retry.Options{
					MaxRetry:         opts.RetryOpts.MaxRetry,
					Delay:            opts.RetryOpts.Delay,
					IsErrorRetryable: retry.IsErrorRetryable,
				}); err != nil {
					if !opts.KeepGoing {
						return fmt.Errorf("error copying ref %q: %w", transports.ImageName(ref), err)
					}
					// log the error, keep a note that there was a failure and move on to the next
					// image ref
					errorsPresent = true
					logrus.WithError(err).Errorf("Error copying ref %q", transports.ImageName(ref))
					continue
				}
				// Ensure that we log the manifest digest to a file only if the copy operation was successful
				if opts.DigestFile != "" {
					manifestDigest, err := manifest.Digest(manifestBytes)
					if err != nil {
						return err
					}
					outputStr := fmt.Sprintf("%s %s", manifestDigest.String(), transports.ImageName(destRef))
					if _, err = digestFile.WriteString(outputStr + "\n"); err != nil {
						return fmt.Errorf("failed to write digest to file %q: %w", opts.DigestFile, err)
					}
				}
			}
			imagesNumber++
		}
	}

	if opts.DryRun {
		logrus.Infof("Would have synced %d images from %d sources", imagesNumber, len(srcRepoList))
	} else {
		logrus.Infof("Synced %d images from %d sources", imagesNumber, len(srcRepoList))
	}
	if !errorsPresent {
		return nil
	}
	return errors.New("sync failed due to previous reported error(s) for one or more images")
}

// syncOptions contains information retrieved from the skopeo sync command line.
type syncOptions struct {
	Global                   *GlobalOptions             `yaml:"global"` // Global (not command dependent) skopeo options
	DeprecatedTLSVerify      *DeprecatedTLSVerifyOption `yaml:"deprecated_tls_verify"`
	SrcImage                 *ImageOptions              `yaml:"src_image"`  // Source image options
	DestImage                *ImageDestOptions          `yaml:"dest_image"` // Destination image options
	RetryOpts                *reTryOptions              `yaml:"retry_opts"`
	RemoveSignatures         bool                       `yaml:"remove_signatures"`            // Do not copy signatures from the source image
	SignByFingerprint        string                     `yaml:"sign_by_fingerprint"`          // Sign the image using a GPG key with the specified fingerprint
	SignBySigstoreParamFile  string                     `yaml:"sign_by_sigstore_param_file"`  // Sign the image using a sigstore signature per configuration in a param file
	SignBySigstorePrivateKey string                     `yaml:"sign_by_sigstore_private_key"` // Sign the image using a sigstore private key
	SignPassphraseFile       string                     `yaml:"sign_passphrase_file"`         // Path pointing to a passphrase file when signing
	Format                   string                     `yaml:"format"`                       // Force conversion of the image to a specified format
	Source                   string                     `yaml:"source"`                       // Source repository name
	Destination              string                     `yaml:"destination"`                  // Destination registry name
	DigestFile               string                     `yaml:"digest_file"`                  // Write digest to this file
	Scoped                   bool                       `yaml:"scoped"`                       // When true, namespace copied images at destination using the source repository name
	All                      bool                       `yaml:"all"`                          // Copy all of the images if an image in the source is a list
	DryRun                   bool                       `yaml:"dry_run"`                      // Don't actually copy anything, just output what it would have done
	PreserveDigests          bool                       `yaml:"preserve_digests"`             // Preserve digests during sync
	KeepGoing                bool                       `yaml:"keep_going"`                   // Whether or not to abort the sync if there are any errors during syncing the images
	AppendSuffix             string                     `yaml:"append_suffix"`                // Suffix to append to destination image tag

	UpstreamDomain string `yaml:"upstream_domain"` // Domain to send upstream data to
}
