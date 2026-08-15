# Nested Chunk List Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display parent and child knowledge chunks as correctly grouped nested rows in the chunk management page.

**Architecture:** The chunk page service will load the document's filtered chunk records, group children under their parent records, and paginate by top-level groups. The HTTP response will expose nested `children` records plus parent/child metadata; the existing frontend table will render parent rows followed by indented child rows while preserving row operations and selection.

**Tech Stack:** Go, Gin, PostgreSQL repository abstraction, React, TypeScript, Tailwind UI components.

---

### Task 1: Add server-side parent/child grouping

**Files:**
- Modify: `internal/app/knowledge/service/chunk/knowledge_chunk_service.go`
- Modify: `internal/app/knowledge/service/chunk/knowledge_chunk_query_service.go`
- Test: `internal/app/knowledge/service/chunk/knowledge_chunk_service_test.go`

- [x] Add a `KnowledgeChunkGroup` result type and a `Groups` field to the page result while retaining record totals separately from group totals.
- [x] Add a failing test proving a parent with two children is returned as one paginated group and that pagination counts groups, not raw records.
- [x] Load all filtered records, group by `ParentChunkID`, preserve parent/child ordering, and treat orphan/flat records as top-level groups.
- [x] Run the focused Go service test and confirm the new test fails before implementation, then passes after implementation.

### Task 2: Expose nested chunk data from the HTTP API

**Files:**
- Modify: `internal/adapter/http/knowledge/knowledge_chunk_handler.go`
- Test: `internal/adapter/http/knowledge/test/knowledge_chunk_handler_test.go`

- [x] Add `recordType`, `parentChunkId`, and recursive `children` fields to the chunk view model.
- [x] Return nested top-level records and a separate raw record count for the UI summary.
- [x] Add a failing handler test that checks the response contains a parent record with nested child records.
- [x] Run the focused handler tests and confirm the nested response passes.

### Task 3: Render nested rows in the frontend

**Files:**
- Modify: `frontend/src/services/knowledgeService.ts`
- Modify: `frontend/src/pages/admin/knowledge/KnowledgeChunksPage.tsx`

- [x] Extend the TypeScript chunk model with parent/child metadata and nested children.
- [x] Flatten only for selection and mutations; render the API groups as parent rows with indented child rows.
- [x] Keep edit, enable/disable, delete, batch operations, and pagination working for both levels.
- [x] Show group and raw-record totals so the count distinction is visible.

### Task 4: Verify and integrate

**Files:**
- No additional production files.

- [x] Run Go focused tests and frontend build; lint is blocked by the repository's existing ESLint/plugin configuration mismatch.
- [x] Inspect the diff and verify no unrelated production changes are included.
- [ ] Commit the feature branch, merge it into `main`, rerun verification on the merged result, and remove the temporary worktree and branch.
