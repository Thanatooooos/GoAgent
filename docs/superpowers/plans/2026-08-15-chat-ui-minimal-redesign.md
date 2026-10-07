# Chat UI Minimal Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refine the existing `/chat` experience into a restrained white AI chat UI while preserving all current chat behavior and backend boundaries.

**Architecture:** Keep the current `MainLayout`, chat store, SSE flow, message model, and component responsibilities. Apply the redesign through surgical JSX/class changes in chat-only components plus a small chat-specific CSS layer for typography, overflow, responsive spacing, and composer behavior.

**Tech Stack:** React 18, TypeScript, Vite, Tailwind CSS, Lucide React, React Markdown, React Virtuoso.

---

### Task 1: Capture baseline and establish a safe verification loop

**Files:**
- Inspect only: `frontend/src/pages/ChatPage.tsx`, `frontend/src/components/layout/MainLayout.tsx`, `frontend/src/components/layout/Header.tsx`, `frontend/src/components/layout/Sidebar.tsx`, `frontend/src/components/chat/*.tsx`, `frontend/src/styles/globals.css`.
- Test: browser page at `http://localhost:5173/chat`.

- [ ] **Step 1: Confirm the existing frontend server and browser page.**

Run from `frontend`:

```powershell
npm run dev -- --host 0.0.0.0
```

Open `http://localhost:5173/chat`, record the current session URL, and verify that existing sessions, messages, Markdown, and the composer are visible.

- [ ] **Step 2: Record the visual baseline.**

Inspect the desktop page and a narrow viewport. Note Sidebar width, Header height, message column width, composer position, page-level horizontal scroll, and whether code blocks scroll internally. Do not change application behavior during this step.

- [ ] **Step 3: Define the post-change checks.**

Use these checks after each visual pass: `document.documentElement.scrollWidth === document.documentElement.clientWidth`, visible composer bounding rectangle, computed Sidebar width, message column max width, and a code block's `scrollWidth > clientWidth` only when code is wider than its container.

### Task 2: Rebuild the neutral chat shell and navigation presentation

**Files:**
- Modify: `frontend/src/components/layout/MainLayout.tsx`
- Modify: `frontend/src/components/layout/Header.tsx`
- Modify: `frontend/src/components/layout/Sidebar.tsx`
- Modify: `frontend/src/pages/ChatPage.tsx`

- [ ] **Step 1: Simplify the shell class contract.**

Keep the existing `Sidebar` open/close state and `Header` callback, but make the outer shell `min-w-0`, use white main content, and use a `264px` desktop sidebar. Keep the mobile overlay/drawer interaction intact.

- [ ] **Step 2: Replace decorative Sidebar blocks with neutral controls.**

Keep every current handler and navigation target. Replace the brand/quick-start gradient card with a compact brand row and a plain new-conversation button. Keep daily brief/admin actions as low-emphasis navigation rows. Restyle search as a single light input row and preserve query filtering.

- [ ] **Step 3: Restyle session groups and user footer.**

Preserve grouping, select, rename, delete, and dropdown menu behavior. Use `#f1f1f1` hover, `#e9e9e9` selected, neutral text, subtle 8px radii, and no visible card shadows. Preserve the bottom user menu and logout action.

- [ ] **Step 4: Make Header compact and content-focused.**

Keep the mobile menu button and current-session lookup. Render current title plus subdued model context without adding a traditional navbar or new application state. Keep the existing `/chat` and `/chat/:sessionId` navigation behavior unchanged.

- [ ] **Step 5: Run the dev page and verify navigation behavior.**

Refresh the browser and verify: new conversation, session selection, search, rename, delete menu, daily brief, admin link, mobile drawer open/close, and user menu still work before moving on.

### Task 3: Refine message reading layout and Markdown rendering

**Files:**
- Modify: `frontend/src/components/chat/MessageList.tsx`
- Modify: `frontend/src/components/chat/MessageItem.tsx`
- Modify: `frontend/src/components/chat/FeedbackButtons.tsx`
- Modify: `frontend/src/components/chat/ThinkingIndicator.tsx`
- Modify: `frontend/src/components/chat/ApprovalPendingCard.tsx`
- Modify: `frontend/src/components/chat/MarkdownRenderer.tsx`
- Modify: `frontend/src/styles/globals.css`

- [ ] **Step 1: Constrain the reading column.**

Keep Virtuoso refs, scroll-to-bottom behavior, streaming follow behavior, and message order unchanged. Set the list to a fluid `minmax(0, 820px)` reading column with responsive horizontal padding and enough bottom space for the composer.

- [ ] **Step 2: Restyle user and assistant messages.**

Keep the user/assistant branching and all assistant subcomponents. Use a right-aligned `#f4f4f4` user bubble capped around 76% width with a 20px radius. Keep assistant content on white with readable paragraph spacing and no enclosing card.

- [ ] **Step 3: Tone down assistant action controls.**

Preserve copy/like/dislike handlers and feedback state. Make controls visually quiet by default, reveal stronger contrast on hover/focus, and avoid saturated blue/green/red except for a selected state where existing semantics require it.

- [ ] **Step 4: Tone down runtime status panels without removing them.**

Preserve thinking, agent reasoning, memory, recall, tool calls, approval, fallback, loading, error, and cancellation content. Replace strong colored cards with neutral bordered sections or restrained semantic accents so the main conversation remains visually dominant.

- [ ] **Step 5: Rework Markdown class coverage.**

Keep `ReactMarkdown`, `remarkGfm`, `rehypeRaw`, citation components, syntax highlighting, image fallback, links, table structure, and code-copy behavior. Add explicit readable styles for h1/h2/h3, paragraphs, lists, blockquotes, links, inline code, tables, and code blocks. Ensure long prose wraps, tables scroll inside a wrapper, and code blocks use their own `overflow-x-auto` container.

- [ ] **Step 6: Verify long-content rendering in the browser.**

Open a conversation containing Markdown or use an existing long response. Confirm headings, lists, tables, links, inline code, code-language labels, code copy, feedback buttons, and loading states do not overflow the page.

### Task 4: Restyle the composer and empty state without changing send behavior

**Files:**
- Modify: `frontend/src/components/chat/ChatInput.tsx`
- Modify: `frontend/src/components/chat/WelcomeScreen.tsx`
- Modify: `frontend/src/pages/ChatPage.tsx`
- Modify: `frontend/src/styles/globals.css`

- [ ] **Step 1: Keep the existing ChatInput behavior intact.**

Do not change `sendMessage`, `cancelGeneration`, `deepThinkingEnabled`, `setDeepThinkingEnabled`, `inputFocusKey`, IME composition handling, Enter/Shift+Enter behavior, auto-grow cap, or streaming stop behavior.

- [ ] **Step 2: Apply the neutral composer visual.**

Use a centered max width close to the reading column, white background, 1px `#e5e5e5` border, 24–28px radius, a very light shadow, a minimum height around 58px, neutral deep-thinking control, and a dark circular send button. Keep the send button disabled when empty and preserve the square stop state while streaming.

- [ ] **Step 3: Reduce composer helper chrome.**

Keep the existing keyboard hint and deep-thinking explanation, but make them subdued and compact. Use neutral text and remove the old blue emphasis except where the enabled state must be recognizable.

- [ ] **Step 4: Simplify WelcomeScreen.**

Keep sample-question loading and preset prompts wired to the existing store. Remove the gradient/grid/floating decoration and large cards. Render a compact “What can I help with?” heading, the same composer interaction, and three or four lightweight suggestion chips/buttons that populate the existing input.

- [ ] **Step 5: Verify empty and populated states.**

Create/open a new conversation in the browser. Check the welcome layout, chip interaction, disabled send state, typing, auto-grow, Enter send, Shift+Enter newline, deep-thinking toggle, and transition into the normal message list.

### Task 5: Add responsive and overflow safeguards

**Files:**
- Modify: `frontend/src/styles/globals.css`
- Modify: `frontend/src/components/layout/MainLayout.tsx`
- Modify: `frontend/src/components/layout/Sidebar.tsx`
- Modify: `frontend/src/components/chat/MessageList.tsx`
- Modify: `frontend/src/components/chat/ChatInput.tsx`

- [ ] **Step 1: Add chat-only responsive rules.**

At small widths, keep Sidebar hidden until the existing Header menu opens it, reduce page padding, set user bubbles to a safe mobile max width, and keep the composer within the viewport using `max-width: 100%` and safe horizontal padding.

- [ ] **Step 2: Add overflow containment.**

Use `min-w-0` on flex/grid parents, `overflow-wrap:anywhere` for long text/links where needed, `overflow-x:auto` only on code/table containers, and ensure the sticky composer does not cover the last message by retaining or adjusting list bottom padding.

- [ ] **Step 3: Check desktop and narrow viewports in the browser.**

At the normal desktop viewport and a narrow mobile-sized viewport, verify no document-level horizontal scrollbar, drawer behavior, readable message padding, visible composer, and internally scrolling code blocks.

### Task 6: Execute visual iteration and final verification

**Files:**
- Modify only the chat UI files above as required by visual findings.

- [ ] **Step 1: Run the first complete browser pass.**

Refresh the live page and inspect layout proportions, Sidebar width, Header height, message spacing, user bubble size, assistant typography, composer size/position, border weight, radius consistency, and whitespace.

- [ ] **Step 2: Make a second surgical visual pass.**

Adjust only the values that remain visually off after the first pass. Recheck the same page instead of relying on static class inspection.

- [ ] **Step 3: Run frontend lint.**

From `frontend`:

```powershell
npm run lint
```

Expected: exit code 0 and no ESLint errors or warnings.

- [ ] **Step 4: Run frontend build.**

From `frontend`:

```powershell
npm run build
```

Expected: exit code 0 and a generated production bundle in `frontend/dist`.

- [ ] **Step 5: Audit the final diff and boundaries.**

Run:

```powershell
git diff --check
git status --short
git diff -- frontend/src/components frontend/src/pages/ChatPage.tsx frontend/src/styles/globals.css
```

Confirm no backend files, API services, stores, message types, or unrelated user changes were modified by this task.
