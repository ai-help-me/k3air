package install

import (
	_ "embed"
)

//go:embed k3s-uninstall.sh.tmpl
var uninstallTmplContent string
