package builder

import (
	"fmt"
	"gopkg.in/oleiade/reflections.v1"
	"sort"

	"github.com/werf/werf/pkg/config"
	"github.com/werf/werf/pkg/util"
)

type Cache struct {
	*config.Cache
}

func (b *Cache) IsBeforeInstallCacheExist() bool {
	return b.isEmptyStageCache("BeforeInstall")
}
func (b *Cache) IsInstallCacheExist() bool { return b.isEmptyStageCache("Install") }
func (b *Cache) IsBeforeSetupCacheExist() bool {
	return b.isEmptyStageCache("BeforeSetup")
}
func (b *Cache) IsSetupCacheExist() bool { return b.isEmptyStageCache("Setup") }

//func (b *Cache) BeforeInstall(_ context.Context, container Container) error {
//	return b.stage("BeforeInstall", container)
//}
//func (b *Cache) Install(_ context.Context, container Container) error {
//	return b.stage("Install", container)
//}
//func (b *Cache) BeforeSetup(_ context.Context, container Container) error {
//	return b.stage("BeforeSetup", container)
//}
//func (b *Cache) Setup(_ context.Context, container Container) error {
//	return b.stage("Setup", container)
//}
//
func (b *Cache) BeforeInstallCacheChecksum() string {
	return b.stageCacheChecksum("BeforeInstall")
}
func (b *Cache) InstallCacheChecksum() string { return b.stageCacheChecksum("Install") }
func (b *Cache) BeforeSetupCacheChecksum() string {
	return b.stageCacheChecksum("BeforeSetup")
}
func (b *Cache) SetupCacheChecksum() string { return b.stageCacheChecksum("Setup") }

func (b *Cache) stageCacheChecksum(userStageName string) string {
	dirs := b.stageCacheDirs(userStageName)
	sort.Strings(dirs)
	return util.Sha256Hash(dirs...)
}

func (b *Cache) isEmptyStageCache(userStageName string) bool {
	return len(b.stageCacheDirs(userStageName)) == 0
}

func (b *Cache) stageCacheDirs(userStageName string) []string {
	dirs, err := util.InterfaceToStringArray(b.cacheFieldValue(userStageName))
	if err != nil {
		panic(fmt.Sprintf("runtime error: %s", err))
	}

	return dirs
}

func (b *Cache) cacheFieldValue(fieldName string) interface{} {
	value, err := reflections.GetField(b.Cache, fieldName)
	if err != nil {
		panic(fmt.Sprintf("runtime error: %s", err))
	}

	return value
}
