package file_reader

import (
	"github.com/werf/werf/pkg/giterminism"
)

type configType string

const (
	giterminismConfigErrorConfigType configType = "giterminism config"
	configErrorConfigType            configType = "werf config"
	configTemplateErrorConfigType    configType = "werf config template"
	configGoTemplateErrorConfigType  configType = "file"
	dockerfileErrorConfigType        configType = "dockerfile"
	dockerignoreErrorConfigType      configType = "dockerignore file"
	chartFileErrorConfigType         configType = "chart file"
	chartDirectoryErrorConfigType    configType = "chart directory"
)

type FileReader struct {
	manager giterminism.Manager
}

func NewFileReader(manager giterminism.Manager) FileReader {
	return FileReader{manager}
}