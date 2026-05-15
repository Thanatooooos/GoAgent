package tool

import ragcore "local/rag-project/internal/app/rag/tool/core"

// Registry is the module-centric registry type used by the runtime.
type Registry = ragcore.Registry

// ModuleRegistry is an alias kept for backward compatibility.
type ModuleRegistry = Registry

func NewRegistry() *Registry {
	return ragcore.NewRegistry()
}
