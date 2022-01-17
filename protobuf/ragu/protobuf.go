package protobuf

import (
	"os"
	"path/filepath"
	"sync"

	"emperror.dev/errors"
	"github.com/kralicky/ragu/pkg/ragu"
	"github.com/kralicky/spellbook/internal/deps"
)

var protobufDeps = deps.New()

var (
	Deps          = protobufDeps.Deps
	SerialDeps    = protobufDeps.SerialDeps
	CtxDeps       = protobufDeps.CtxDeps
	SerialCtxDeps = protobufDeps.SerialCtxDeps
)

type Proto struct {
	Source  string
	DestDir string
}

var Config = struct {
	Protos []Proto
}{}

func Protobuf() error {
	protobufDeps.Resolve()
	wg := sync.WaitGroup{}
	errs := []error{}
	mu := sync.Mutex{}
	for _, proto := range Config.Protos {
		wg.Add(1)
		go func(proto Proto) {
			defer wg.Done()
			protos, err := ragu.GenerateCode(proto.Source, true)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}
			for _, f := range protos {
				path := filepath.Join(proto.DestDir, f.GetName())
				if info, err := os.Stat(path); err == nil {
					if info.Mode()&0200 == 0 {
						if err := os.Chmod(path, 0644); err != nil {
							mu.Lock()
							errs = append(errs, err)
							mu.Unlock()
							return
						}
					}
				}
				if err := os.WriteFile(path, []byte(f.GetContent()), 0444); err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
					return
				}
			}
		}(proto)
	}
	wg.Wait()
	return errors.Combine(errs...)
}
