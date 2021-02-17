package common

import (
	"context"
	"fmt"

	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/werf/pkg/deploy/helm/v3/maintenance_helper"
	"helm.sh/helm/v3/pkg/action"
)

func Helm3ReleaseExistanceGuard(ctx context.Context, releaseName, namespace string, maintenanceHelper *maintenance_helper.MaintenanceHelper) error {
	list, err := maintenanceHelper.GetHelm3ReleasesList(ctx)
	if err != nil {
		return fmt.Errorf("error getting helm 3 releases list: %s", err)
	}

	for _, existingReleaseName := range list {
		if existingReleaseName == releaseName {
			return fmt.Errorf(`found existing helm 3 release %q in the namespace %q: cannot continue deploy process

Please use werf v1.2 to converge your application.`, releaseName, namespace)
		}
	}
	return nil
}

func CreateMaintenanceHelper(ctx context.Context, cmdData *CmdData, actionConfig *action.Configuration) (*maintenance_helper.MaintenanceHelper, error) {
	maintenanceOpts := maintenance_helper.MaintenanceHelperOptions{
		KubeConfigOptions: kube.KubeConfigOptions{
			Context: *cmdData.KubeContext,
		},
	}

	if helmReleaseStorageType, err := GetHelmReleaseStorageType(*cmdData.HelmReleaseStorageType); err != nil {
		return nil, err
	} else {
		maintenanceOpts.Helm2ReleaseStorageType = helmReleaseStorageType
	}
	maintenanceOpts.Helm2ReleaseStorageNamespace = *cmdData.HelmReleaseStorageNamespace

	return maintenance_helper.NewMaintenanceHelper(actionConfig, maintenanceOpts), nil
}
