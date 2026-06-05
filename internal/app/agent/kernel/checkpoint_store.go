package kernel

import "github.com/cloudwego/eino/compose"

// CheckpointStore is the kernel-facing alias for Eino's checkpoint store
// contract.
//
// Responsibility boundary:
// - it stores execution recovery state for graph resume
// - it does not replace the service-facing approval session store
//
// Approval lifecycle lookup and outward resume semantics are intentionally
// handled outside this contract by agent runtime/session storage.
type CheckpointStore = compose.CheckPointStore
