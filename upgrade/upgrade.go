package upgrade

import (
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

func Upgrade() error {
	deps, err := sh.Output(mg.GoCmd(), "list", "-f", "{{if not (or .Main .Indirect)}}{{.Path}}{{end}}", "-m", "all")
	if err != nil {
		return err
	}
	err = sh.RunV(mg.GoCmd(), append([]string{"get"}, strings.Split(deps, "\n")...)...)
	if err != nil {
		return err
	}
	err = sh.RunV(mg.GoCmd(), "mod", "tidy")
	if err != nil {
		return err
	}
	return nil
}
