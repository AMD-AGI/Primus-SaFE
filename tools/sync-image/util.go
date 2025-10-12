package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	"github.com/distribution/reference"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"golang.org/x/term"
)

// parseManifestFormat parses format parameter for copy and sync command.
// It returns string value to use as manifest MIME type
func parseManifestFormat(manifestFormat string) (string, error) {
	switch manifestFormat {
	case "oci":
		return imgspecv1.MediaTypeImageManifest, nil
	case "v2s1":
		return manifest.DockerV2Schema1SignedMediaType, nil
	case "v2s2":
		return manifest.DockerV2Schema2MediaType, nil
	default:
		return "", fmt.Errorf("unknown format %q. Choose one of the supported formats: 'oci', 'v2s1', or 'v2s2'", manifestFormat)
	}
}

// destinationReference creates an image reference using the provided transport.
// It returns a image reference to be used as destination of an image copy and
// any error encountered.
func destinationReference(destination string, transport string) (types.ImageReference, error) {
	var imageTransport types.ImageTransport

	switch transport {
	case docker.Transport.Name():
		destination = fmt.Sprintf("//%s", destination)
		imageTransport = docker.Transport
	case directory.Transport.Name():
		_, err := os.Stat(destination)
		if err == nil {
			return nil, fmt.Errorf("Refusing to overwrite destination directory %q", destination)
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("Destination directory could not be used: %w", err)
		}
		// the directory holding the image must be created here
		if err = os.MkdirAll(destination, 0755); err != nil {
			return nil, fmt.Errorf("Error creating directory for image %s: %w", destination, err)
		}
		imageTransport = directory.Transport
	default:
		return nil, fmt.Errorf("%q is not a valid destination transport", transport)
	}
	logrus.Debugf("Destination for transport %q: %s", transport, destination)

	destRef, err := imageTransport.ParseReference(destination)
	if err != nil {
		return nil, fmt.Errorf("Cannot obtain a valid image reference for transport %q and reference %q: %w", imageTransport.Name(), destination, err)
	}

	return destRef, nil
}

// noteCloseFailure returns (possibly-nil) err modified to account for (non-nil) closeErr.
// The error for closeErr is annotated with description (which is not a format string)
// Typical usage:
//
//	defer func() {
//		if err := something.Close(); err != nil {
//			returnedErr = noteCloseFailure(returnedErr, "closing something", err)
//		}
//	}
func noteCloseFailure(err error, description string, closeErr error) error {
	// We don’t accept a Closer() and close it ourselves because signature.PolicyContext has .Destroy(), not .Close().
	// This also makes it harder for a caller to do
	//     defer noteCloseFailure(returnedErr, …)
	// which doesn’t use the right value of returnedErr, and doesn’t update it.
	if err == nil {
		return fmt.Errorf("%s: %w", description, closeErr)
	}
	// In this case we prioritize the primary error for use with %w; closeErr is usually less relevant, or might be a consequence of the primary error.
	return fmt.Errorf("%w (%s: %v)", err, description, closeErr)
}

// getImageTags lists all tags in a repository.
// It returns a string slice of tags and any error encountered.
func getImageTags(ctx context.Context, sysCtx *types.SystemContext, repoRef reference.Named) ([]string, error) {
	name := repoRef.Name()
	logrus.WithFields(logrus.Fields{
		"image": name,
	}).Info("Getting tags")
	// Ugly: NewReference rejects IsNameOnly references, and GetRepositoryTags ignores the tag/digest.
	// So, we use TagNameOnly here only to shut up NewReference
	dockerRef, err := docker.NewReference(reference.TagNameOnly(repoRef))
	if err != nil {
		return nil, err // Should never happen for a reference with tag and no digest
	}
	tags, err := docker.GetRepositoryTags(ctx, sysCtx, dockerRef)
	if err != nil {
		return nil, fmt.Errorf("Error determining repository tags for repo %s: %w", name, err)
	}

	return tags, nil
}

func reqValid(source, destination string) error {
	// validate source and destination options
	if len(source) == 0 {
		return errors.New("A source transport must be specified")
	}
	if !slices.Contains(TransportList, source) {
		return fmt.Errorf("%q is not a valid source transport", source)
	}

	if len(destination) == 0 {
		return errors.New("A destination transport must be specified")
	}
	if !slices.Contains(TransportList, destination) {
		return fmt.Errorf("%q is not a valid destination transport", destination)
	}

	if source == destination && source == TransportDir {
		return errors.New("sync from 'dir' to 'dir' not implemented, consider using rsync instead")
	}
	return nil
}

// promptForPassphrase interactively prompts for a passphrase related to privateKeyFile
func promptForPassphrase(privateKeyFile string, stdin, stdout *os.File) (string, error) {
	stdinFd := int(stdin.Fd())
	if !term.IsTerminal(stdinFd) {
		return "", fmt.Errorf("Cannot prompt for a passphrase for key %s, standard input is not a TTY", privateKeyFile)
	}

	fmt.Fprintf(stdout, "Passphrase for key %s: ", privateKeyFile)
	passphrase, err := term.ReadPassword(stdinFd)
	if err != nil {
		return "", fmt.Errorf("Error reading password: %w", err)
	}
	fmt.Fprintf(stdout, "\n")
	return string(passphrase), nil
}

func getDockerAuth(creds string) (*types.DockerAuthConfig, error) {
	username, password, err := parseCreds(creds)
	if err != nil {
		return nil, err
	}
	return &types.DockerAuthConfig{
		Username: username,
		Password: password,
	}, nil
}

func parseCreds(creds string) (string, string, error) {
	if creds == "" {
		return "", "", errors.New("credentials can't be empty")
	}
	username, password, _ := strings.Cut(creds, ":") // Sets password to "" if there is no ":"
	if username == "" {
		return "", "", errors.New("username can't be empty")
	}
	return username, password, nil
}
