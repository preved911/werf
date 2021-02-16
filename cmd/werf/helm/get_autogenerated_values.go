package helm

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/logboek"
	"github.com/werf/logboek/pkg/level"

	"github.com/werf/werf/cmd/werf/common"
	"github.com/werf/werf/pkg/build"
	"github.com/werf/werf/pkg/container_runtime"
	"github.com/werf/werf/pkg/deploy/helm/v3/chart_extender"
	"github.com/werf/werf/pkg/docker"
	"github.com/werf/werf/pkg/git_repo"
	"github.com/werf/werf/pkg/image"
	"github.com/werf/werf/pkg/ssh_agent"
	"github.com/werf/werf/pkg/storage/manager"
	"github.com/werf/werf/pkg/tmp_manager"
	"github.com/werf/werf/pkg/true_git"
	"github.com/werf/werf/pkg/util"
	"github.com/werf/werf/pkg/werf"
)

var getAutogeneratedValuedCmdData common.CmdData

func NewGetAutogeneratedValuesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-autogenerated-values",
		Short: "Get service values yaml generated by werf for helm chart during deploy",
		Long: common.GetLongCommandDescription(`Get service values generated by werf for helm chart during deploy.

These values includes project name, docker images ids and other`),
		DisableFlagsInUseLine: true,
		Annotations: map[string]string{
			common.CmdEnvAnno: common.EnvsDescription(common.WerfSecretKey),
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := common.ProcessLogOptions(&getAutogeneratedValuedCmdData); err != nil {
				common.PrintHelp(cmd)
				return err
			}

			return runGetServiceValues()
		},
	}

	common.SetupDir(&getAutogeneratedValuedCmdData, cmd)
	common.SetupGitWorkTree(&getAutogeneratedValuedCmdData, cmd)
	common.SetupConfigTemplatesDir(&getAutogeneratedValuedCmdData, cmd)
	common.SetupConfigPath(&getAutogeneratedValuedCmdData, cmd)
	common.SetupEnvironment(&getAutogeneratedValuedCmdData, cmd)

	common.SetupGiterminismInspectorOptions(&getAutogeneratedValuedCmdData, cmd)

	common.SetupTmpDir(&getAutogeneratedValuedCmdData, cmd)
	common.SetupHomeDir(&getAutogeneratedValuedCmdData, cmd)
	common.SetupSSHKey(&getAutogeneratedValuedCmdData, cmd)

	common.SetupSecondaryStagesStorageOptions(&getAutogeneratedValuedCmdData, cmd)
	common.SetupStagesStorageOptions(&getAutogeneratedValuedCmdData, cmd)
	common.SetupSynchronization(&getAutogeneratedValuedCmdData, cmd)
	common.SetupKubeConfig(&getAutogeneratedValuedCmdData, cmd)
	common.SetupKubeConfigBase64(&getAutogeneratedValuedCmdData, cmd)
	common.SetupKubeContext(&getAutogeneratedValuedCmdData, cmd)

	common.SetupVirtualMerge(&getAutogeneratedValuedCmdData, cmd)
	common.SetupVirtualMergeFromCommit(&getAutogeneratedValuedCmdData, cmd)
	common.SetupVirtualMergeIntoCommit(&getAutogeneratedValuedCmdData, cmd)

	common.SetupNamespace(&getAutogeneratedValuedCmdData, cmd)

	common.SetupDockerConfig(&getAutogeneratedValuedCmdData, cmd, "Command needs granted permissions to read and pull images from the specified repo")
	common.SetupInsecureRegistry(&getAutogeneratedValuedCmdData, cmd)
	common.SetupSkipTlsVerifyRegistry(&getAutogeneratedValuedCmdData, cmd)

	common.SetupStubTags(&getAutogeneratedValuedCmdData, cmd)

	common.SetupLogOptions(&getAutogeneratedValuedCmdData, cmd)

	return cmd
}

func runGetServiceValues() error {
	logboek.SetAcceptedLevel(level.Error)

	ctx := common.BackgroundContext()

	if err := werf.Init(*getAutogeneratedValuedCmdData.TmpDir, *getAutogeneratedValuedCmdData.HomeDir); err != nil {
		return fmt.Errorf("initialization error: %s", err)
	}

	if err := git_repo.Init(); err != nil {
		return err
	}

	if err := image.Init(); err != nil {
		return err
	}

	if err := true_git.Init(true_git.Options{LiveGitOutput: *getAutogeneratedValuedCmdData.LogVerbose || *getAutogeneratedValuedCmdData.LogDebug}); err != nil {
		return err
	}

	if err := common.DockerRegistryInit(&getAutogeneratedValuedCmdData); err != nil {
		return err
	}

	if err := docker.Init(ctx, *getAutogeneratedValuedCmdData.DockerConfig, *getAutogeneratedValuedCmdData.LogVerbose, *getAutogeneratedValuedCmdData.LogDebug); err != nil {
		return err
	}

	ctxWithDockerCli, err := docker.NewContext(ctx)
	if err != nil {
		return err
	}
	ctx = ctxWithDockerCli

	giterminismManager, err := common.GetGiterminismManager(&getAutogeneratedValuedCmdData)
	if err != nil {
		return err
	}

	werfConfig, err := common.GetRequiredWerfConfig(ctx, &getAutogeneratedValuedCmdData, giterminismManager, common.GetWerfConfigOptions(&getAutogeneratedValuedCmdData, false))
	if err != nil {
		return fmt.Errorf("unable to load werf config: %s", err)
	}

	projectName := werfConfig.Meta.Project
	environment := *getAutogeneratedValuedCmdData.Environment

	namespace, err := common.GetKubernetesNamespace(*getAutogeneratedValuedCmdData.Namespace, environment, werfConfig)
	if err != nil {
		return err
	}

	if err := ssh_agent.Init(ctx, common.GetSSHKey(&getAutogeneratedValuedCmdData)); err != nil {
		return fmt.Errorf("cannot initialize ssh agent: %s", err)
	}
	defer func() {
		err := ssh_agent.Terminate()
		if err != nil {
			logboek.Error().LogF("WARNING: ssh agent termination failed: %s\n", err)
		}
	}()

	var imagesRepository string
	var imagesInfoGetters []*image.InfoGetter
	if *getAutogeneratedValuedCmdData.StubTags {
		imagesInfoGetters = common.StubImageInfoGetters(werfConfig)
		imagesRepository = common.StubRepoAddress
	} else {
		projectTmpDir, err := tmp_manager.CreateProjectDir(ctx)
		if err != nil {
			return fmt.Errorf("getting project tmp dir failed: %s", err)
		}
		defer tmp_manager.ReleaseProjectDir(projectTmpDir)

		stagesStorageAddress, err := common.GetStagesStorageAddress(&getAutogeneratedValuedCmdData)
		if err != nil {
			return fmt.Errorf("%s (use --stub-tags option to get service values without real tags)", err)
		}
		containerRuntime := &container_runtime.LocalDockerServerRuntime{} // TODO
		stagesStorage, err := common.GetStagesStorage(stagesStorageAddress, containerRuntime, &getAutogeneratedValuedCmdData)
		if err != nil {
			return err
		}
		synchronization, err := common.GetSynchronization(ctx, &getAutogeneratedValuedCmdData, projectName, stagesStorage)
		if err != nil {
			return err
		}
		stagesStorageCache, err := common.GetStagesStorageCache(synchronization)
		if err != nil {
			return err
		}
		storageLockManager, err := common.GetStorageLockManager(ctx, synchronization)
		if err != nil {
			return err
		}
		secondaryStagesStorageList, err := common.GetSecondaryStagesStorageList(stagesStorage, containerRuntime, &getAutogeneratedValuedCmdData)
		if err != nil {
			return err
		}

		storageManager := manager.NewStorageManager(projectName, stagesStorage, secondaryStagesStorageList, storageLockManager, stagesStorageCache)

		conveyorWithRetry := build.NewConveyorWithRetryWrapper(werfConfig, giterminismManager, []string{}, giterminismManager.ProjectDir(), projectTmpDir, ssh_agent.SSHAuthSock, containerRuntime, storageManager, storageLockManager, common.GetConveyorOptions(&getAutogeneratedValuedCmdData))
		defer conveyorWithRetry.Terminate()

		if err := conveyorWithRetry.WithRetryBlock(ctx, func(c *build.Conveyor) error {
			if err := c.ShouldBeBuilt(ctx); err != nil {
				return err
			}

			imagesRepository = storageManager.StagesStorage.String()
			imagesInfoGetters = c.GetImageInfoGetters()

			return nil
		}); err != nil {
			return err
		}
	}

	serviceValues, err := chart_extender.GetServiceValues(ctx, projectName, imagesRepository, imagesInfoGetters, chart_extender.ServiceValuesOptions{Namespace: namespace, Env: environment})
	if err != nil {
		return fmt.Errorf("error creating service values: %s", err)
	}

	fmt.Printf("%s", util.DumpYaml(serviceValues))

	return nil
}
