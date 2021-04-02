package config

type rawCache struct {
	BeforeInstall             interface{} `yaml:"beforeInstall,omitempty"`
	Install                   interface{} `yaml:"install,omitempty"`
	BeforeSetup               interface{} `yaml:"beforeSetup,omitempty"`
	Setup                     interface{} `yaml:"setup,omitempty"`
	CacheVersion              string      `yaml:"cacheVersion,omitempty"`
	BeforeInstallCacheVersion string      `yaml:"beforeInstallCacheVersion,omitempty"`
	InstallCacheVersion       string      `yaml:"installCacheVersion,omitempty"`
	BeforeSetupCacheVersion   string      `yaml:"beforeSetupCacheVersion,omitempty"`
	SetupCacheVersion         string      `yaml:"setupCacheVersion,omitempty"`

	rawStapelImage *rawStapelImage `yaml:"-"` // parent

	UnsupportedAttributes map[string]interface{} `yaml:",inline"`
}

func (c *rawCache) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if parent, ok := parentStack.Peek().(*rawStapelImage); ok {
		c.rawStapelImage = parent
	}

	type plain rawCache
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if err := checkOverflow(c.UnsupportedAttributes, c, c.rawStapelImage.doc); err != nil {
		return err
	}

	return nil
}

func (c *rawCache) toDirective() (cache *Cache, err error) {
	cache = &Cache{}
	cache.CacheVersion = c.CacheVersion
	cache.BeforeInstallCacheVersion = c.BeforeInstallCacheVersion
	cache.InstallCacheVersion = c.InstallCacheVersion
	cache.BeforeSetupCacheVersion = c.BeforeSetupCacheVersion
	cache.SetupCacheVersion = c.SetupCacheVersion

	if beforeInstall, err := InterfaceToStringArray(c.BeforeInstall, c, c.rawStapelImage.doc); err != nil {
		return nil, err
	} else {
		cache.BeforeInstall = beforeInstall
	}

	if install, err := InterfaceToStringArray(c.Install, c, c.rawStapelImage.doc); err != nil {
		return nil, err
	} else {
		cache.Install = install
	}

	if beforeSetup, err := InterfaceToStringArray(c.BeforeSetup, c, c.rawStapelImage.doc); err != nil {
		return nil, err
	} else {
		cache.BeforeSetup = beforeSetup
	}

	if setup, err := InterfaceToStringArray(c.Setup, c, c.rawStapelImage.doc); err != nil {
		return nil, err
	} else {
		cache.Setup = setup
	}

	cache.raw = c

	if err := c.validateDirective(cache); err != nil {
		return nil, err
	}

	return cache, nil
}

func (c *rawCache) validateDirective(cache *Cache) error {
	if err := cache.validate(); err != nil {
		return err
	}

	return nil
}
