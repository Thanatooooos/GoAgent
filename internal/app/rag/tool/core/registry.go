package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Registry stores tool modules and exposes compatibility helpers for legacy Tool usage.
type Registry struct {
	modules map[string]ToolModule
}

// ModuleRegistry is the module-centric registry type used by the runtime.
type ModuleRegistry = Registry

func NewRegistry() *Registry {
	return &Registry{
		modules: map[string]ToolModule{},
	}
}

func (r *Registry) Register(tool Tool) error {
	return r.RegisterModule(NewLegacyToolAdapter(tool).Module())
}

func (r *Registry) RegisterModule(module ToolModule) error {
	if r == nil {
		return fmt.Errorf("tool registry is required")
	}
	module = module.Normalize()
	if err := module.Validate(); err != nil {
		return err
	}
	name := strings.TrimSpace(module.Name)
	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.modules[name] = module
	return nil
}

func (r *Registry) MustRegister(tool Tool) {
	if err := r.Register(tool); err != nil {
		panic(err)
	}
}

func (r *Registry) MustRegisterModule(module ToolModule) {
	if err := r.RegisterModule(module); err != nil {
		panic(err)
	}
}

func (r *Registry) Get(name string) (Tool, bool) {
	module, ok := r.GetModule(name)
	if !ok {
		return nil, false
	}
	return moduleToolAdapter{module: module}, true
}

func (r *Registry) GetModule(name string) (ToolModule, bool) {
	if r == nil {
		return ToolModule{}, false
	}
	module, ok := r.modules[strings.TrimSpace(name)]
	return module, ok
}

func (r *Registry) GetBehavior(name string) (ToolBehavior, bool) {
	module, ok := r.GetModule(name)
	if !ok {
		return ToolBehavior{}, false
	}
	return module.Behavior, true
}

func (r *Registry) GetSpec(name string) (ToolSpec, bool) {
	module, ok := r.GetModule(name)
	if !ok {
		return ToolSpec{}, false
	}
	return module.Spec, true
}

func (r *Registry) ListDefinitions() []Definition {
	if r == nil || len(r.modules) == 0 {
		return []Definition{}
	}
	items := make([]Definition, 0, len(r.modules))
	for _, module := range r.modules {
		items = append(items, module.Spec.Normalize().Definition)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

type moduleToolAdapter struct {
	module ToolModule
}

func (a moduleToolAdapter) Definition() Definition {
	return a.module.Normalize().Spec.Definition
}

func (a moduleToolAdapter) Invoke(ctx context.Context, call Call) (Result, error) {
	return a.module.Invoker.Invoke(ctx, call)
}
