# AGENT.md

## 1. Project Overview
- `goagent` is a Go-first AI application with a React/Vite frontend, centered on knowledge management, ingestion pipelines, RAG chat, and an evolving Agent runtime.
- Backend stack: Go 1.25, Gin, GORM/PostgreSQL, Viper config, Redis/MinIO integrations, MCP support, and CloudWeGo Eino-based AI orchestration.
- Frontend stack: React 18 + TypeScript + Vite + Zustand + Tailwind/Radix UI.
- Current project phase: the main paths already exist. Prefer stability, boundary clarity, evaluation quality, and approval/resume UX over broad new feature expansion.

## 2. Commands
- Download Go dependencies: `go mod download`
- Start backend server: `go run ./cmd/server`
- Run core Go tests: `go test ./cmd/... ./internal/... -count=1`
- Start local integration dependencies: `docker compose up -d postgres object-storage object-storage-init`
- Run integration tests: `make test-integration`
- Install frontend dependencies: `cd frontend && npm install`
- Start frontend dev server: `cd frontend && npm run dev`
- Build frontend: `cd frontend && npm run build`
- Lint frontend: `cd frontend && npm run lint`

## 3. Architecture
- Backend code lives under `internal/`; the main domains are `knowledge`, `ingestion`, `rag`, `agent`, and `user`.
- `internal/app/rag/tool` is the stable production tool path. `internal/app/agent` is the newer runtime path for capability-based orchestration, planner/handoff flow, and approval/resume support.
- `knowledge` owns knowledge bases, documents, chunks, and document processing. `ingestion` owns `pipeline -> task -> task_node` execution and reconciliation. `rag` owns chat, retrieve, rewrite, prompt assembly, memory, trace, and evaluation.
- Frontend code lives in `frontend/src`; chat state, approval recovery, and SSE-driven runtime status are already wired and should be extended through existing stores/services instead of ad hoc state paths.
- Runtime config is centered in `configs/application.yaml`. Additional project context lives in `docs/project_progress_context.md` and `docs/agent_capability_onboarding.md`.

## 4. Conventions
- Follow existing domain boundaries. New behavior should usually extend the owning module rather than introducing parallel abstractions.
- Treat `internal/app/rag/tool` and `internal/app/agent` as distinct paths with different responsibilities. Preserve the current split unless the task explicitly requires convergence work.
- For new non-diagnostic agent capabilities, follow `docs/agent_capability_onboarding.md`: typed input/output, full `capability.Spec`, `NormalizeInput(...)`, `Invoke(...)`, registration in the correct assembly group, and contract/integration coverage.
- Use existing non-diagnostic capabilities such as `search`, `fetch`, `external_evidence`, `think`, `knowledge_discovery`, `memory_recall`, and `content_summarize` as templates before copying diagnosis-oriented flows.
- Reuse the existing approval, resume, SSE, trace, and evaluation plumbing instead of creating side channels.
- Keep tests close to the affected behavior. Prefer extending existing package-level and service-level suites over adding isolated one-off harnesses.

## 5. Hard Constraints
- Do not treat this repository as a greenfield build. The main flows already exist; avoid speculative rewrites or broad architecture replacement unless explicitly requested.
- Do not collapse `rag/tool` work into `agent` work, or `agent` work back into `rag/tool`, without a clear task that requires changing that boundary.
- Do not modify committed database migrations casually. Add new migrations when needed; avoid rewriting historical schema steps.
- Do not commit secrets, tokens, `.env` values, or machine-local credentials.
- Do not bypass approval/resume behavior when touching agent execution. Pending approval lookup, SSE events, and frontend recovery are part of the supported product path.
- Do not add new capabilities by cloning diagnosis-heavy implementations as the default template when the feature is not diagnosis-oriented.

## 6. Gotchas
- This project is in a quality-closing phase, not a capability-bootstrap phase. Before adding new surface area, check whether the real need is to improve stability, consistency, or observability in an existing path.
- `summary` quality is an active workstream. Be careful with changes around structured summary schema, rendering, validation, token budgeting, and offline evaluation because they can improve one metric while causing drift elsewhere.
- `internal/app/agent` and `internal/app/rag/tool` currently coexist on purpose. If a task feels duplicated, confirm whether it is deliberate before "cleaning it up".
- Frontend chat behavior depends on existing SSE event handling and state reconciliation for `approval_pending`, `agent_outcome`, and related runtime events. Preserve those flows when updating UI behavior.
- Frontend dev expects the backend API to be reachable from the Vite app; if the UI looks broken, verify the backend is running before assuming the frontend implementation is wrong.
- `scripts/` contains multiple standalone Go programs, so `go test ./...` is not a reliable repository-wide command. Prefer `go test ./cmd/... ./internal/... -count=1` unless you are intentionally working inside `scripts/`.
