package pattern

import (
	agentcapability "local/rag-project/internal/app/agent/capability"
	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentplanner "local/rag-project/internal/app/agent/planner"
	agentruntime "local/rag-project/internal/app/agent/runtime"
)

// AssemblyContext contains the generic capability catalog and bindings a
// pattern needs during assembly.
type AssemblyContext struct {
	Registry *agentcapability.Registry
	Bindings agentcapability.RoleBindings
}

// RuntimeConfig contains cross-pattern runtime concerns injected at assembly
// time.
type RuntimeConfig struct {
	Planner                        agentplanner.Planner
	CapabilityCatalogBuilder       agentcatalog.Builder
	CapabilitySelector             selectcapability.Selector
	CapabilityResolver             agentresolve.Resolver
	OutputMode                     string
	PreferExternalEvidenceWorkflow bool
	ApprovalSessionStore           agentruntime.SessionStore
	Kernel                         agentkernel.BuilderConfig
}
