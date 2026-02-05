package config

import (
	"embed"
)

//go:embed init.yaml.template
var templateFS embed.FS

// GetTemplate returns the embedded init.yaml.template content
func GetTemplate() ([]byte, error) {
	return templateFS.ReadFile("init.yaml.template")
}
