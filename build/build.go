package build

import (
	"os"
	"path/filepath"

	"github.com/kralicky/spellbook/internal/deps"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var buildDeps = deps.New()

var (
	Deps          = buildDeps.Deps
	SerialDeps    = buildDeps.SerialDeps
	CtxDeps       = buildDeps.CtxDeps
	SerialCtxDeps = buildDeps.SerialCtxDeps
)

var Config = struct {
	CgoEnabled bool
	ExtraFlags []string
	ExtraEnv   map[string]string
}{}

func Build() error {
	buildDeps.Resolve()

	cgoEnabled := "0"
	if Config.CgoEnabled {
		cgoEnabled = "1"
	}

	binaries := map[string]string{}

	if _, err := os.Stat("./main.go"); err == nil {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		binaries["."] = filepath.Base(wd)
	}
	entries, err := os.ReadDir("./cmd")
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if _, err := os.Stat(filepath.Join("cmd", name, "main.go")); err == nil {
				binaries["./cmd/"+name] = filepath.Join("bin", name)
			}
		}
	}
	for dir, dest := range binaries {
		env := map[string]string{
			"CGO_ENABLED": cgoEnabled,
		}
		for k, v := range Config.ExtraEnv {
			env[k] = v
		}
		if err := sh.RunWith(env, mg.GoCmd(), "build", "-ldflags", "-w -s", "-o", dest, dir); err != nil {
			return err
		}
	}
	return nil
}
