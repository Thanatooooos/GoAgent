# Wiki Focused Graph Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the unreadable full-circle Wiki graph with a focused, searchable, interactive graph that exposes one node's neighborhood and details first.

**Architecture:** Keep the existing Wiki API and SVG renderer, but derive a focused graph view from the existing `nodes` and `edges` data. Add local UI state for selected node, search text, and focused/all mode; render only related edges prominently in focused mode, while keeping the existing full graph available as an explicit option. Add a right-side selected-node panel using the existing node counts and edge relationships.

**Tech Stack:** React, TypeScript, React Router, existing Tailwind utility classes, inline SVG, browser-based local integration verification.

---

### Task 1: Define the focused graph behavior before UI changes

**Files:**
- Modify: `frontend/src/pages/admin/knowledge/WikiBrowserPage.tsx`

- [ ] **Step 1: Add the focused-mode state and pure derived relationships**

Define selected node, search text, and `graphMode` state. Derive the selected node, related node IDs, and related edges from the existing graph response. Focus mode must include the selected node and every node connected by an incoming or outgoing edge; all mode must include every graph node and edge.

- [ ] **Step 2: Verify the existing page remains unchanged before rendering changes**

Run `npx tsc --noEmit` from `frontend` and confirm the baseline source still type-checks before changing the SVG markup.

### Task 2: Implement the focused graph canvas

**Files:**
- Modify: `frontend/src/pages/admin/knowledge/WikiBrowserPage.tsx`

- [ ] **Step 1: Write the intended interaction check**

The browser check will assert that the initial graph exposes a focused-node label, clicking a visible node changes the selected node, search filters node visibility, and switching to all mode restores all edges. This is the regression contract for the UI behavior because the repository has no frontend test runner.

- [ ] **Step 2: Render focus mode by default**

Replace the current always-visible circular canvas with a bounded SVG that renders the selected node and its one-hop neighborhood prominently. Dim unrelated nodes and edges only when all-mode is not active; do not delete them from the data so switching modes is instantaneous.

- [ ] **Step 3: Add search and mode controls**

Add a labeled node search input and two buttons: `聚焦关系` and `全部关系`. Search should select the first matching node and keep the graph in focus mode; empty search restores the current selection without changing mode.

- [ ] **Step 4: Add the selected-node detail panel**

Render the selected node title, summary, in-link count, out-link count, and a compact list of related page titles. Keep navigation to the existing `?slug=` route for the selected node.

### Task 3: Verify the real page and package output

**Files:**
- Test: `frontend/src/pages/admin/knowledge/WikiBrowserPage.tsx` through browser integration

- [ ] **Step 1: Run type-check and production build**

Run `npx tsc --noEmit` and `npm run build` from `frontend`; both must exit 0.

- [ ] **Step 2: Run the browser interaction check against `http://localhost:5173/admin/knowledge/kb_demo_01/wiki`**

Verify the page shows the focused view without label collisions, click a node and confirm the detail panel changes, enter a node name and confirm search selection, switch to `全部关系`, and confirm the graph exposes the full edge set. Capture a screenshot of the focused state.

- [ ] **Step 3: Inspect browser console output**

Record application errors separately from known browser-extension warnings and React Router future-flag warnings.

- [ ] **Step 4: Review the diff and report remaining limitations**

Use `git diff --check` and report any unrelated worktree changes without modifying them.

---

## Self-review

- The plan covers the approved focused graph, search, mode switch, detail panel, click navigation, build verification, and browser verification.
- No backend or API changes are required for this UI redesign.
- The existing full graph remains available through an explicit user action, so dense data is not discarded.
