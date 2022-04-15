package build

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"emperror.dev/errors"
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
	CgoEnabled   bool
	ExtraFlags   []string
	LDFlags      []string
	ExtraEnv     map[string]string
	ExtraTargets map[string]string
}{
	LDFlags: []string{"-w", "-s"},
}

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
	for dir, dest := range Config.ExtraTargets {
		binaries[dir] = dest
	}

	wg := sync.WaitGroup{}
	errs := []error{}
	mu := sync.Mutex{}
	for dir, dest := range binaries {
		wg.Add(1)
		go func(dir, dest string) {
			defer wg.Done()
			env := map[string]string{
				"CGO_ENABLED": cgoEnabled,
			}
			for k, v := range Config.ExtraEnv {
				env[k] = v
			}
			args := []string{"build"}
			if len(Config.LDFlags) > 0 {
				args = append(args, "-ldflags", strings.Join(Config.LDFlags, " "))
			}
			args = append(args, Config.ExtraFlags...)
			args = append(args, "-o", dest, dir)
			if err := sh.RunWith(env, mg.GoCmd(), args...); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(dir, dest)
	}
	wg.Wait()
	return errors.Combine(errs...)
}
