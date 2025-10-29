package main

import (
	"context"
	"fmt"

	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/types"
	"github.com/distribution/reference"
	"github.com/sirupsen/logrus"
)

var (
	TransportDocker = docker.Transport.Name()
	TransportDir    = directory.Transport.Name()
	//TransportWithYaml = "yaml"
)

var TransportList = []string{TransportDocker, TransportDir}

// imagesToCopyFromRepo builds a list of image references from the tags
// found in a source repository.
// It returns an image reference slice with as many elements as the tags found
// and any error encountered.
func imagesToCopyFromRepo(sys *types.SystemContext, repoRef reference.Named) ([]types.ImageReference, error) {
	tags, err := getImageTags(context.Background(), sys, repoRef)
	if err != nil {
		return nil, err
	}

	var sourceReferences []types.ImageReference
	for _, tag := range tags {
		taggedRef, err := reference.WithTag(repoRef, tag)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"repo": repoRef.Name(),
				"tag":  tag,
			}).Errorf("Error creating a tagged reference from registry tag list: %v", err)
			continue
		}
		ref, err := docker.NewReference(taggedRef)
		if err != nil {
			return nil, fmt.Errorf("Cannot obtain a valid image reference for transport %q and reference %s: %w", docker.Transport.Name(), taggedRef.String(), err)
		}
		sourceReferences = append(sourceReferences, ref)
	}
	return sourceReferences, nil
}

// imagesToCopy retrieves all the images to copy from a specified sync source
// and transport.
// It returns a slice of repository descriptors, where each descriptor is a
// list of tagged image references to be used as sync source, and any error
// encountered.
func imagesToCopy(source string, transport string, sourceCtx *types.SystemContext) ([]repoDescriptor, error) {
	var descriptors []repoDescriptor
	switch transport {
	case docker.Transport.Name():
		desc := repoDescriptor{
			Context: sourceCtx,
		}

		named, err := reference.ParseNormalizedNamed(source) // May be a repository or an image.
		if err != nil {
			return nil, fmt.Errorf("Cannot obtain a valid image reference for transport %q and reference %q: %w", docker.Transport.Name(), source, err)
		}
		imageTagged := !reference.IsNameOnly(named)
		logrus.WithFields(logrus.Fields{
			"imagename": source,
			"tagged":    imageTagged,
		}).Info("Tag presence check")
		if imageTagged {
			srcRef, err := docker.NewReference(named)
			if err != nil {
				return nil, fmt.Errorf("Cannot obtain a valid image reference for transport %q and reference %q: %w", docker.Transport.Name(), named.String(), err)
			}
			desc.ImageRefs = []types.ImageReference{srcRef}
		} else {
			desc.ImageRefs, err = imagesToCopyFromRepo(sourceCtx, named)
			if err != nil {
				return descriptors, err
			}
			if len(desc.ImageRefs) == 0 {
				return descriptors, fmt.Errorf("No images to sync found in %q", source)
			}
	}
	descriptors = append(descriptors, desc)

	// dir and yaml-based sync are not supported yet

	// case directory.Transport.Name():
		// 	desc := repoDescriptor{
		// 		Context: sourceCtx,
		// 	}

		// 	if _, err := os.Stat(source); err != nil {
		// 		return descriptors, fmt.Errorf("Invalid source directory specified: %w", err)
		// 	}
		// 	desc.DirBasePath = source
		// 	var err error
		// 	desc.ImageRefs, err = imagesToCopyFromDir(source)
		// 	if err != nil {
		// 		return descriptors, err
		// 	}
		// 	if len(desc.ImageRefs) == 0 {
		// 		return descriptors, fmt.Errorf("No images to sync found in %q", source)
		// 	}
		// 	descriptors = append(descriptors, desc)

		// case "yaml":
		// 	cfg, err := newSourceConfig(source)
		// 	if err != nil {
		// 		return descriptors, err
		// 	}
		// 	for registryName, registryConfig := range cfg {
		// 		descs, err := imagesToCopyFromRegistry(registryName, registryConfig, *sourceCtx)
		// 		if err != nil {
		// 			return descriptors, fmt.Errorf("Failed to retrieve list of images from registry %q: %w", registryName, err)
		// 		}
		// 		descriptors = append(descriptors, descs...)
		// 	}
	}

	return descriptors, nil
}
