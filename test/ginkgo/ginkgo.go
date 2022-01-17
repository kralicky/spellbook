package test

import (
	"github.com/kralicky/spellbook/internal/deps"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var testDeps = deps.New()

var (
	Deps          = testDeps.Deps
	SerialDeps    = testDeps.SerialDeps
	CtxDeps       = testDeps.CtxDeps
	SerialCtxDeps = testDeps.SerialCtxDeps
)

var Config = struct {
	GinkgoArgs []string
}{
	GinkgoArgs: []string{
		"-r",
		"--randomize-suites",
		"--fail-on-pending",
		"--keep-going",
		"--cover",
		"--coverprofile=cover.out",
		"--race",
		"--trace",
		"--timeout=10m",
	},
}

func Test() error {
	testDeps.Resolve()
	return sh.RunV(mg.GoCmd(),
		append([]string{"run", "github.com/onsi/ginkgo/v2/ginkgo"}, Config.GinkgoArgs...)...)
}
