package resolve

import (
	agentcapability "local/rag-project/internal/app/agent/capability"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
)

type MatchedCapability struct {
	Name      string
	Spec      agentcapability.Spec
	Selection selectcapability.CapabilitySelection
}

type ResolvedCapability struct {
	Name      string
	Spec      agentcapability.Spec
	Handle    agentcapability.Handle
	Input     any
	Selection selectcapability.CapabilitySelection
}
