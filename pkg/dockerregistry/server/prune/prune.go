package prune

import (
	"context"
	"fmt"

	"github.com/docker/distribution"
	dcontext "github.com/docker/distribution/context"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/opencontainers/go-digest"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	dockerapiv10 "github.com/openshift/api/image/docker10"
	imageapiv1 "github.com/openshift/api/image/v1"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	regstorage "github.com/openshift/image-registry/pkg/dockerregistry/server/storage"
	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
	util "github.com/openshift/image-registry/pkg/origin-common/util"
)

// Pruner defines a common set of operations for pruning
type Pruner interface {
	DeleteRepository(ctx context.Context, reponame string) error
	DeleteManifestLink(ctx context.Context, svc distribution.ManifestService, reponame string, dgst digest.Digest) error
	DeleteBlob(ctx context.Context, dgst digest.Digest) error
}

// DryRunPruner prints information about each object that going to remove.
type DryRunPruner struct{}

var _ Pruner = &DryRunPruner{}

func (p *DryRunPruner) DeleteRepository(ctx context.Context, reponame string) error {
	logger := dcontext.GetLogger(ctx)
	logger.Printf("Would delete repository: %s", reponame)
	return nil
}

func (p *DryRunPruner) DeleteManifestLink(ctx context.Context, svc distribution.ManifestService, reponame string, dgst digest.Digest) error {
	logger := dcontext.GetLogger(ctx)
	logger.Printf("Would delete manifest link: %s@%s", reponame, dgst)
	return nil
}

func (p *DryRunPruner) DeleteBlob(ctx context.Context, dgst digest.Digest) error {
	logger := dcontext.GetLogger(ctx)
	logger.Printf("Would delete blob: %s", dgst)
	return nil
}

// RegistryPruner deletes objects.
type RegistryPruner struct {
	StorageDriver driver.StorageDriver
}

var _ Pruner = &RegistryPruner{}

// DeleteRepository removes a repository directory from the storage
func (p *RegistryPruner) DeleteRepository(ctx context.Context, reponame string) error {
	vacuum := storage.NewVacuum(ctx, p.StorageDriver)

	// Log message will be generated by RemoveRepository with loglevel=info.
	if err := vacuum.RemoveRepository(reponame); err != nil {
		return fmt.Errorf("unable to remove the repository %s: %v", reponame, err)
	}

	return nil
}

// DeleteManifestLink removes a manifest link from the storage
func (p *RegistryPruner) DeleteManifestLink(ctx context.Context, svc distribution.ManifestService, reponame string, dgst digest.Digest) error {
	logger := dcontext.GetLogger(ctx)

	logger.Printf("Deleting manifest link: %s@%s", reponame, dgst)
	if err := svc.Delete(ctx, dgst); err != nil {
		return fmt.Errorf("failed to delete the manifest link %s@%s: %v", reponame, dgst, err)
	}

	return nil
}

// DeleteBlob removes a blob from the storage
func (p *RegistryPruner) DeleteBlob(ctx context.Context, dgst digest.Digest) error {
	vacuum := storage.NewVacuum(ctx, p.StorageDriver)

	// Log message will be generated by RemoveBlob with loglevel=info.
	if err := vacuum.RemoveBlob(string(dgst)); err != nil {
		return fmt.Errorf("failed to delete the blob %s: %v", dgst, err)
	}

	return nil
}

// garbageCollector holds objects for later deletion. If the object is replaced,
// then the previous one will be deleted.
type garbageCollector struct {
	Pruner Pruner
	Ctx    context.Context

	repoName string

	manifestService distribution.ManifestService
	manifestRepo    string
	manifestLink    digest.Digest
}

func (gc *garbageCollector) AddRepository(repoName string) error {
	// If the place is occupied, then it is necessary to clean it.
	if err := gc.Collect(); err != nil {
		return err
	}

	gc.repoName = repoName

	return nil
}

func (gc *garbageCollector) AddManifestLink(svc distribution.ManifestService, repoName string, dgst digest.Digest) error {
	// If the place is occupied, then it is necessary to clean it.
	if err := gc.Collect(); err != nil {
		return err
	}

	gc.manifestService = svc
	gc.manifestRepo = repoName
	gc.manifestLink = dgst

	return nil
}

func (gc *garbageCollector) Collect() error {
	if len(gc.manifestLink) > 0 {
		if err := gc.Pruner.DeleteManifestLink(gc.Ctx, gc.manifestService, gc.manifestRepo, gc.manifestLink); err != nil {
			return err
		}
		gc.manifestLink = ""
	}
	if len(gc.repoName) > 0 {
		if err := gc.Pruner.DeleteRepository(gc.Ctx, gc.repoName); err != nil {
			return err
		}
		gc.repoName = ""
	}
	return nil
}

func imageStreamHasManifestDigest(is *imageapiv1.ImageStream, dgst digest.Digest) bool {
	for _, tagEventList := range is.Status.Tags {
		for _, tagEvent := range tagEventList.Items {
			if tagEvent.Image == string(dgst) {
				return true
			}
		}
	}
	return false
}

// Summary is cumulative information about what was pruned.
type Summary struct {
	Blobs     int
	DiskSpace int64
}

// Prune removes blobs which are not used by Images in OpenShift.
//
// On error, the Summary will contain what was deleted so far.
//
// TODO(dmage): remove layer links to a blob if the blob is removed or it doesn't belong to the ImageStream.
// TODO(dmage): keep young blobs (docker/distribution#2297).
func Prune(ctx context.Context, registry distribution.Namespace, registryClient client.RegistryClient, pruner Pruner) (Summary, error) {
	logger := dcontext.GetLogger(ctx)

	enumStorage := regstorage.Enumerator{Registry: registry}

	oc, err := registryClient.Client()
	if err != nil {
		return Summary{}, fmt.Errorf("error getting clients: %v", err)
	}

	imageList, err := oc.Images().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Summary{}, fmt.Errorf("error listing images: %v", err)
	}

	inuse := make(map[string]string)
	for _, image := range imageList.Items {
		// Keep the manifest.
		inuse[image.Name] = image.DockerImageReference

		if err := util.ImageWithMetadata(&image); err != nil {
			return Summary{}, fmt.Errorf("error getting image metadata: %v", err)
		}
		// Keep the config for a schema 2 manifest.
		if image.DockerImageManifestMediaType == schema2.MediaTypeManifest {
			meta, ok := image.DockerImageMetadata.Object.(*dockerapiv10.DockerImage)
			if ok {
				inuse[meta.ID] = image.DockerImageReference
			}
		}

		// Keep image layers.
		for _, layer := range image.DockerImageLayers {
			inuse[layer.Name] = image.DockerImageReference
		}
	}

	var stats Summary

	// The Enumerate calls a Stat() on each file or directory in the tree before call our handler.
	// Therefore, we can not delete subdirectories from the handler. On some types of storage (S3),
	// this can lead to an error in the Enumerate.
	// We are waiting for the completion of our handler and perform deferred deletion of objects.
	gc := &garbageCollector{
		Ctx:    ctx,
		Pruner: pruner,
	}

	err = enumStorage.Repositories(ctx, func(repoName string) error {
		logger.Debugln("Processing repository", repoName)

		named, err := reference.WithName(repoName)
		if err != nil {
			return fmt.Errorf("failed to parse the repo name %s: %v", repoName, err)
		}

		ref, err := imageapi.ParseDockerImageReference(repoName)
		if err != nil {
			return fmt.Errorf("failed to parse the image reference %s: %v", repoName, err)
		}

		// XXX Due to an old bug we may have some images stored  with
		// with invalid names. If we try to GET these images through
		// API an error will be thrown back. Here we pre-check if the
		// image contains an invalid name, if true then we add the repo
		// to be pruned.
		if ers := rest.IsValidPathSegmentName(ref.Name); len(ers) > 0 {
			logger.Printf("Invalid image name %s, removing whole repository", repoName)
			return gc.AddRepository(repoName)
		}

		is, err := oc.ImageStreams(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if kerrors.IsNotFound(err) {
			logger.Printf("The image stream %s/%s is not found, will remove the whole repository", ref.Namespace, ref.Name)
			return gc.AddRepository(repoName)
		} else if err != nil {
			return fmt.Errorf("failed to get the image stream %s: %v", repoName, err)
		}

		repository, err := registry.Repository(ctx, named)
		if err != nil {
			return err
		}

		manifestService, err := repository.Manifests(ctx)
		if err != nil {
			return err
		}

		err = enumStorage.Manifests(ctx, repoName, func(dgst digest.Digest) error {
			if _, ok := inuse[string(dgst)]; ok && imageStreamHasManifestDigest(is, dgst) {
				logger.Debugf("Keeping the manifest link %s@%s", repoName, dgst)
				return nil
			}

			return gc.AddManifestLink(manifestService, repoName, dgst)
		})
		if e, ok := err.(driver.PathNotFoundError); ok {
			logger.Printf("Skipped manifest link pruning for the repository %s: %v", repoName, e)
		} else if err != nil {
			return fmt.Errorf("failed to prune manifest links in the repository %s: %v", repoName, err)
		}

		return nil
	})
	if e, ok := err.(driver.PathNotFoundError); ok {
		logger.Warnf("No repositories found: %v", e)
		return stats, nil
	} else if err != nil {
		return stats, err
	}

	if err := gc.Collect(); err != nil {
		return stats, err
	}

	logger.Debugln("Processing blobs")
	blobStatter := registry.BlobStatter()
	err = enumStorage.Blobs(ctx, func(dgst digest.Digest) error {
		if imageReference, ok := inuse[string(dgst)]; ok {
			logger.Debugf("Keeping the blob %s (it belongs to the image %s)", dgst, imageReference)
			return nil
		}

		desc, err := blobStatter.Stat(ctx, dgst)
		if err != nil {
			return err
		}

		stats.Blobs++
		stats.DiskSpace += desc.Size

		return pruner.DeleteBlob(ctx, dgst)
	})
	return stats, err
}
