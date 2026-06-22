// Package service is the stable application-facing entry for ingestion services.
//
// Subpackages:
//   - pipeline/: pipeline CRUD and validation
//   - task/: task creation and query
//   - executor/: workflow execution orchestration
//   - workflow/: execution state, graph builder, node contracts
//   - runner/: concrete pipeline node runners
//   - observer/: task/node observation and metrics
package service
