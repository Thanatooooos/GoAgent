package planexecute

import agentpattern "local/rag-project/internal/app/agent/pattern"

// Config assembles the first explicit plan-execute pattern.
type Config struct {
	Assembly    agentpattern.AssemblyContext
	Runtime     agentpattern.RuntimeConfig
	Synthesizer PlanSynthesizer
}
