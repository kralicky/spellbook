package docker

import (
	"github.com/kralicky/spellbook/internal/deps"
	"github.com/magefile/mage/sh"
)

var dockerDeps = deps.New()

var (
	Deps          = dockerDeps.Deps
	SerialDeps    = dockerDeps.SerialDeps
	CtxDeps       = dockerDeps.CtxDeps
	SerialCtxDeps = dockerDeps.SerialCtxDeps
)

var Config = struct {
	Tag        string
	Dockerfile string
	Dir        string
	ExtraArgs  []string
	ExtraEnv   map[string]string
}{
	Dir: ".",
}

func Docker() error {
	args := []string{"build"}
	if Config.Tag != "" {
		args = append(args, "-t", Config.Tag)
	}
	if Config.Dockerfile != "" {
		args = append(args, "-f", Config.Dockerfile)
	}
	args = append(args, Config.ExtraArgs...)
	args = append(args, Config.Dir)
	env := map[string]string{
		"DOCKER_BUILDKIT": "1",
	}
	for k, v := range Config.ExtraEnv {
		env[k] = v
	}
	return sh.RunWithV(env, "docker", args...)
}
