// Package chat orchestrates RAG chat: prepare stages, tool/agent workflow, streaming answer, and trace emission.
//
// Source files are grouped by responsibility prefix:
//   - prepare_*   — read-path stage orchestration
//   - execute_*   — tool workflow, prompt, streaming, result handling
//   - stage_*     — shared stage types and trace helpers
//   - agent_*     — agent runtime integration
//   - observability_* / runtime_path — logging and trace emission
//   - budget_*    — token budget trimming
//   - service.go / types.go / deps.go — entry, DTOs, construction
package chat
