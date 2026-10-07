# Modern AI Chat UI Minimal Redesign

## Scope

Refine only `/chat` and `/chat/:sessionId` into a white, low-distraction AI chat experience. Keep `/admin`, `/brief`, backend APIs, chat data structures, stores, SSE handling, and message-send behavior unchanged.

## Design

`MainLayout` remains the shell for Sidebar, Header, responsive drawer state, and the main content region. `Sidebar` keeps all existing session actions (new, search, group, select, rename, delete), the daily brief link, admin entry, and user menu, but removes the decorative gradient/card treatment in favor of a neutral navigation surface. `Header` stays compact and contains only the mobile menu affordance, current conversation title, and subdued model context.

`ChatPage` continues to choose between the empty welcome state and the message state from the existing store data. `MessageList`, `MessageItem`, approval UI, citations, feedback, copy, and regeneration keep their current behavior and handlers. Only their presentation changes: user messages become restrained right-aligned gray bubbles; assistant messages remain in the white reading column. `MarkdownRenderer` keeps the current Markdown/GFM/highlighting pipeline and gains readable typography and overflow constraints.

`ChatInput` keeps its existing send, upload/attachment, deep-thinking, auto-grow, and keyboard behavior. It is restyled as a centered, large rounded composer with a disabled neutral send button when empty. Empty state suggestions are lightweight chips that feed the existing input/send flow without adding a new chat state.

## Visual System

- Main background: `#ffffff`; sidebar background: `#f8f8f8`.
- Primary text: `#2f2f2f`; secondary text: `#6b6b6b`; border: `#e5e5e5`.
- Sidebar width: approximately `264px`; session items use an `8px` radius.
- Sidebar hover: `#f1f1f1`; selected session: `#e9e9e9`.
- User message: `#f4f4f4`, right aligned, approximately `20px` radius, max width around 76%.
- Assistant reading column: approximately `820px` maximum width, `15px`–`16px` text, `1.72` line-height.
- Composer: centered, approximately `58px` minimum height, `26px` radius, 1px border, very light shadow.
- Neutral Lucide icons only; no new UI framework or dependency.
- Motion is limited to 150–200ms hover, focus, and drawer transitions.

## Responsive Behavior

Desktop keeps a fixed-width sidebar and flexible chat column. At small widths the sidebar is hidden by default, the Header menu opens the existing drawer, and the chat column/composer use the full available width. The page must not horizontally scroll. Long prose wraps; code blocks scroll within themselves; the composer remains visible and usable.

## Verification

1. Run `npm run lint` and `npm run build` from `frontend`.
2. Use the running Vite page in the browser to inspect the current baseline, then after implementation refresh the real chat page and perform at least two visual adjustment passes.
3. Check desktop proportions: sidebar width, header height, message column width, message spacing, user bubble size, assistant typography, and composer placement.
4. Check empty state, session hover/selection, search, rename/delete, feedback/copy controls, Markdown headings/lists/tables/links/inline code/code blocks, and streaming/loading presentation.
5. Check a narrow viewport for drawer behavior, no page-level horizontal scroll, composer visibility, and code-block-only horizontal scrolling.

## Non-goals

No backend or API changes, no chat data-model changes, no changes to send flow, no removal of existing session/chat functionality, and no redesign of admin or daily brief pages.
