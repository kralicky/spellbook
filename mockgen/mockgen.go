package mockgen

import (
	"strings"
	"sync"

	"emperror.dev/errors"
	"github.com/kralicky/spellbook/internal/deps"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var mockgenDeps = deps.New()

var (
	Deps          = mockgenDeps.Deps
	SerialDeps    = mockgenDeps.SerialDeps
	CtxDeps       = mockgenDeps.CtxDeps
	SerialCtxDeps = mockgenDeps.SerialCtxDeps
)

type Mock struct {
	Source string
	Dest   string
	Types  []string
}

var Config = struct {
	Mocks []Mock
}{}

func Mockgen() error {
	wg := sync.WaitGroup{}
	errs := []error{}
	mu := sync.Mutex{}
	for _, mock := range Config.Mocks {
		wg.Add(1)
		go func(mock Mock) {
			defer wg.Done()
			err := sh.RunV(mg.GoCmd(), "run", "github.com/golang/mock/mockgen",
				"-source="+mock.Source,
				"-destination="+mock.Dest,
				strings.Join(mock.Types, ","))
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(mock)
	}
	wg.Wait()
	return errors.Combine(errs...)
}
