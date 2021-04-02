package config

type Cache struct {
	BeforeInstall             []string
	Install                   []string
	BeforeSetup               []string
	Setup                     []string
	CacheVersion              string
	BeforeInstallCacheVersion string
	InstallCacheVersion       string
	BeforeSetupCacheVersion   string
	SetupCacheVersion         string

	raw *rawCache
}

func (c *Cache) GetDumpConfigSection() string {
	return dumpConfigDoc(c.raw.rawStapelImage.doc)
}

func (c *Cache) validate() error {
	if !allAbsolutePaths(c.BeforeInstall) {
		return newDetailedConfigError("`beforeInstall: [PATH, ...]|PATH` should be absolute paths!", nil, c.raw.rawStapelImage.doc)
	} else if !allAbsolutePaths(c.Install) {
		return newDetailedConfigError("`install: [PATH, ...]|PATH` should be absolute paths!", nil, c.raw.rawStapelImage.doc)
	} else if !allAbsolutePaths(c.BeforeSetup) {
		return newDetailedConfigError("`beforeSetup: [PATH, ...]|PATH` should be absolute paths!", nil, c.raw.rawStapelImage.doc)
	} else if !allAbsolutePaths(c.Setup) {
		return newDetailedConfigError("`setup: [PATH, ...]|PATH` should be absolute paths!", nil, c.raw.rawStapelImage.doc)
	}

	return nil
}
