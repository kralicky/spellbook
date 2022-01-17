package deps

import (
	"context"

	"github.com/magefile/mage/mg"
)

type targetDeps struct {
	deps          []interface{}
	serialDeps    []interface{}
	ctxDeps       []ctxDepsEntry
	serialCtxDeps []ctxDepsEntry
}

func New() *targetDeps {
	return &targetDeps{}
}

type ctxDepsEntry struct {
	ctx context.Context
	fns []interface{}
}

func (d *targetDeps) Deps(fns ...interface{}) {
	d.deps = append(d.deps, fns...)
}

func (d *targetDeps) SerialDeps(fns ...interface{}) {
	d.serialDeps = append(d.serialDeps, fns...)
}

func (d *targetDeps) CtxDeps(ctx context.Context, fns ...interface{}) {
	d.ctxDeps = append(d.ctxDeps, ctxDepsEntry{
		ctx: ctx,
		fns: fns,
	})
}

func (d *targetDeps) SerialCtxDeps(ctx context.Context, fns ...interface{}) {
	d.serialCtxDeps = append(d.serialCtxDeps, ctxDepsEntry{
		ctx: ctx,
		fns: fns,
	})
}

func (d *targetDeps) Resolve() {
	if len(d.deps) > 0 {
		mg.Deps(d.deps...)
	}
	if len(d.serialDeps) > 0 {
		mg.SerialDeps(d.serialDeps...)
	}
	for _, entry := range d.ctxDeps {
		mg.CtxDeps(entry.ctx, entry.fns...)
	}
	for _, entry := range d.serialCtxDeps {
		mg.SerialCtxDeps(entry.ctx, entry.fns...)
	}
}
