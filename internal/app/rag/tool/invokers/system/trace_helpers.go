package builtin

import (
	"strings"

	ragdomain "local/rag-project/internal/app/rag/domain"
)

func findTraceNode(nodes []ragdomain.RagTraceNode, nodeID string) *ragdomain.RagTraceNode {
	for i := range nodes {
		if strings.TrimSpace(nodes[i].NodeID) == strings.TrimSpace(nodeID) {
			return &nodes[i]
		}
	}
	return nil
}
