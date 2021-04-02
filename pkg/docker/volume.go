package docker

import (
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
	"golang.org/x/net/context"
)

func VolumeCreate(ctx context.Context, opts volume.VolumeCreateBody) (types.Volume, error) {
	return apiCli(ctx).VolumeCreate(ctx, opts)
}

func VolumeRm(ctx context.Context, volumeName string, force bool) error {
	return apiCli(ctx).VolumeRemove(ctx, volumeName, force)
}
