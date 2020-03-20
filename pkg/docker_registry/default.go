package docker_registry

import (
	"strings"

	"github.com/flant/werf/pkg/image"
)

const DefaultImplementationName = "default"

type defaultImplementation struct {
	*api
}

type defaultImplementationOptions struct {
	apiOptions
}

func newDefaultImplementation(options defaultImplementationOptions) (*defaultImplementation, error) {
	d := &defaultImplementation{}
	d.api = newAPI(options.apiOptions)
	return d, nil
}

func (r *defaultImplementation) GetRepoImageList(reference string) ([]*image.Info, error) {
	return r.SelectRepoImageList(reference, func(_ *image.Info) bool { return true })
}

func (r *defaultImplementation) SelectRepoImageList(reference string, f func(*image.Info) bool) ([]*image.Info, error) {
	tags, err := r.api.Tags(reference)
	if err != nil {
		return nil, err
	}

	var repoImageList []*image.Info
	for _, tag := range tags {
		ref := strings.Join([]string{reference, tag}, ":")
		repoImage, err := r.GetRepoImage(ref)
		if err != nil {
			return nil, err
		}

		if f(repoImage) {
			repoImageList = append(repoImageList, repoImage)
		}
	}

	return repoImageList, nil
}

func (r *defaultImplementation) DeleteRepoImage(repoImageList ...*image.Info) error {
	for _, repoImage := range repoImageList {
		if err := r.deleteRepoImage(repoImage); err != nil {
			return err
		}
	}

	return nil
}

func (r *defaultImplementation) deleteRepoImage(repoImage *image.Info) error {
	reference := strings.Join([]string{repoImage.Repository, repoImage.RepoDigest}, "@")
	return r.api.deleteImageByReference(reference)
}

func (r *defaultImplementation) Validate() error {
	return nil
}

func (r *defaultImplementation) String() string {
	return DefaultImplementationName
}