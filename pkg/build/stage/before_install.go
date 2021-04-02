package stage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"

	"github.com/werf/logboek"
	"github.com/werf/logboek/pkg/level"

	"github.com/werf/werf/pkg/build/builder"
	"github.com/werf/werf/pkg/config"
	"github.com/werf/werf/pkg/container_runtime"
	"github.com/werf/werf/pkg/docker"
	"github.com/werf/werf/pkg/util"
)

func GenerateBeforeInstallStage(ctx context.Context, imageBaseConfig *config.StapelImageBase, baseStageOptions *NewBaseStageOptions) *BeforeInstallStage {
	b := getBuilder(imageBaseConfig, baseStageOptions)
	if b != nil && !b.IsBeforeInstallEmpty(ctx) {
		return newBeforeInstallStage(b, baseStageOptions)
	}

	return nil
}

func newBeforeInstallStage(builder builder.Builder, baseStageOptions *NewBaseStageOptions) *BeforeInstallStage {
	s := &BeforeInstallStage{}
	s.UserStage = newUserStage(builder, BeforeInstall, baseStageOptions)
	s.cacheRuntimeID = util.GenerateConsistentRandomString(5)
	return s
}

type BeforeInstallStage struct {
	*UserStage
	cacheRuntimeID string
}

func (s *BeforeInstallStage) GetDependencies(ctx context.Context, _ Conveyor, _, _ container_runtime.ImageInterface) (string, error) {
	return s.builder.BeforeInstallChecksum(ctx), nil
}

func (s *BeforeInstallStage) PrepareImage(ctx context.Context, c Conveyor, prevBuiltImage, image container_runtime.ImageInterface) error {
	if err := s.BaseStage.PrepareImage(ctx, c, prevBuiltImage, image); err != nil {
		return err
	}

	if err := s.builder.BeforeInstall(ctx, image.BuilderContainer()); err != nil {
		return err
	}

	return nil
}

func (s *BeforeInstallStage) PreRunHook(ctx context.Context, _ Conveyor, image container_runtime.ImageInterface) error {
	if len(s.cacheDirs()) == 0 {
		return nil
	}

	process := logboek.Context(ctx).Info().LogProcess("Running cache container")
	process.Start()

	process.StepEnd("Checking cache image existence ...")
	imageName := s.cacheImageName(ctx)
	exist, err := docker.ImageExist(ctx, imageName)
	if err != nil {
		process.Fail()
		return err
	}

	var cacheServerImageName string
	if !exist {
		process.StepEnd("Creating cache image ...")
		baseImage := "aigrychev/unfs3:3fa0568e6ef96e045286afe18444bc28fe93962b"

		var dockerfileLines []string
		dockerfileLines = append(dockerfileLines, fmt.Sprintf("FROM %s", baseImage))
		dockerfileLines = append(dockerfileLines, "RUN rm -rf /etc/exports")
		if err := s.forEachCacheDir(func(containerCacheDir, serverCacheDir string) error {
			dockerfileLines = append(
				dockerfileLines,
				fmt.Sprintf("RUN mkdir -p %s", serverCacheDir),
				fmt.Sprintf("RUN echo \"%s (rw,no_root_squash) # %s\" >> /etc/exports", serverCacheDir, containerCacheDir),
			)

			return nil
		}); err != nil {
			process.Fail()
			return err
		}

		cacheServerImageName = s.cacheTemporaryCacheImageName(ctx)
		buf := bytes.NewBufferString(strings.Join(dockerfileLines, "\n"))
		yaBuf := ioutil.NopCloser(buf)

		oldLvl := logboek.Context(ctx).AcceptedLevel() // TODO: fix logboek Mute/Unmute
		logboek.Context(ctx).SetAcceptedLevel(level.Error)
		if err := docker.CliBuild_LiveOutputWithCustomIn(ctx, yaBuf, "-", fmt.Sprintf("--tag=%s", cacheServerImageName)); err != nil {
			process.Fail()
			return err
		}
		logboek.Context(ctx).SetAcceptedLevel(oldLvl)
	} else {
		process.StepEnd("Using existing cache image ...")
		cacheServerImageName = imageName
	}

	process.StepEnd("Starting cache container ...")
	cacheServerContainerName := s.cacheContainerName(ctx)
	args := append([]string{}, "-d", "--name", cacheServerContainerName, cacheServerImageName)
	if err := docker.CliRun(ctx, args...); err != nil {
		process.Fail()
		return err
	}

	process.StepEnd("Waiting for NFS-server to start ...")
	for {
		r, err := docker.ContainerLogs(ctx, cacheServerContainerName)
		if err != nil {
			process.Fail()
			return err
		}

		data, err := io.ReadAll(r)
		if err != nil {
			process.Fail()
			return err
		}

		if bytes.Contains(data, []byte("ip 0.0.0.0 mask 0.0.0.0")) {
			break
		}

		logboek.Context(ctx).Info().Log(".")
		time.Sleep(time.Second * 1)

		// TODO: set time limit
	}
	logboek.Context(ctx).Info().LogOptionalLn()
	process.End()

	inspect, err := docker.ContainerInspect(ctx, cacheServerContainerName)
	if err != nil {
		return err
	}

	ipAddress := inspect.NetworkSettings.IPAddress
	if err := s.forEachCacheDir(func(containerCacheDir, serverCacheDir string) error {
		volumeName := s.cacheVolumeName(ctx, containerCacheDir)
		_, err = docker.VolumeCreate(ctx, volume.VolumeCreateBody{
			Driver: "local",
			DriverOpts: map[string]string{
				"type":   "nfs",
				"o":      fmt.Sprintf("nfsvers=3,addr=%s,nolock,rw", ipAddress),
				"device": fmt.Sprintf(":%s", serverCacheDir),
			},
			Labels: map[string]string{
				"werf":                     "true",
				"werf-container-cache-dir": containerCacheDir,
				"werf-server-cache-dir":    serverCacheDir,
				// TODO: add project
			},
			Name: volumeName,
		})

		image.Container().RunOptions().AddVolume(fmt.Sprintf("%s:%s", volumeName, containerCacheDir))

		return err
	}); err != nil {
		return err
	}

	return nil
}

func (s *BeforeInstallStage) AfterRunHook(ctx context.Context, _ Conveyor) error {
	if err := logboek.Context(ctx).Info().LogProcess("Saving cache image").DoError(func() error {
		_, err := docker.ContainerCommit(ctx, s.cacheContainerName(ctx), types.ContainerCommitOptions{
			Reference: s.cacheImageName(ctx),
		})

		return err
	}); err != nil {
		return err
	}

	if err := logboek.Context(ctx).Info().LogProcess("Deleting cache temporary data").DoError(func() error {
		if err := docker.ContainerRemove(ctx, s.cacheContainerName(ctx), types.ContainerRemoveOptions{Force: true}); err != nil {
			return err
		}

		{ // A temporary cache image is created when there is no existed cache image from previous build
			exist, err := docker.ContainerExist(ctx, s.cacheTemporaryCacheImageName(ctx))
			if err != nil {
				return err
			}

			if exist {
				if _, err = docker.ImageRemove(ctx, s.cacheTemporaryCacheImageName(ctx)); err != nil {
					return err
				}
			}
		}

		if err := s.forEachCacheDir(func(containerCacheDir, serverCacheDir string) error {
			return docker.VolumeRm(ctx, s.cacheVolumeName(ctx, containerCacheDir), true)
		}); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *BeforeInstallStage) cacheImageName(ctx context.Context) string {
	checksum := s.cacheChecksum(ctx)
	return fmt.Sprintf("werf.cache.%s", checksum)
}

func (s *BeforeInstallStage) cacheTemporaryCacheImageName(ctx context.Context) string {
	checksum := s.cacheChecksum(ctx)
	return fmt.Sprintf("werf.cache.%s.%s", checksum, s.cacheRuntimeID)
}

func (s *BeforeInstallStage) cacheContainerName(ctx context.Context) string {
	checksum := s.cacheChecksum(ctx)
	return fmt.Sprintf("werf.cache.%s.%s", checksum, s.cacheRuntimeID)
}

func (s *BeforeInstallStage) cacheVolumeName(ctx context.Context, containerCacheDir string) string {
	checksum := s.cacheChecksum(ctx)
	return fmt.Sprintf("werf.cache.%s.%s.%s", checksum, util.MurmurHash(containerCacheDir), s.cacheRuntimeID)
}

func (s *BeforeInstallStage) cacheDirs() []string {
	return s.builder.Cache().BeforeInstall
}

func (s *BeforeInstallStage) forEachCacheDir(f func(containerCacheDir, serverCacheDir string) error) error {
	for _, containerCacheDir := range s.cacheDirs() {
		id := util.MurmurHash(containerCacheDir)
		serverCacheDir := path.Join("/werf", id)

		if err := f(containerCacheDir, serverCacheDir); err != nil {
			return err
		}
	}

	return nil
}

func (s *BeforeInstallStage) cacheChecksum(ctx context.Context) string {
	var checksumArgs []string
	checksumArgs = append(checksumArgs, s.builder.Cache().BeforeInstall...)
	checksumArgs = append(checksumArgs, s.builder.Cache().BeforeInstallCacheVersion)
	checksumArgs = append(checksumArgs, s.builder.Cache().CacheVersion)
	checksumArgs = append(checksumArgs, s.builder.BeforeInstallChecksum(ctx))
	return util.Sha256Hash(checksumArgs...)
}
