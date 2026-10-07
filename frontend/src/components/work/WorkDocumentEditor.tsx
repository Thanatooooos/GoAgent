import { useEffect } from "react";
import { Extension, type JSONContent } from "@tiptap/core";
import { EditorContent, useEditor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { TableKit, TableCell, TableHeader } from "@tiptap/extension-table";
import { Plugin } from "@tiptap/pm/state";
import { type WorkDocument, type WorkNode, workRequestId } from "@/services/workService";

const blocks = [
  "paragraph",
  "heading",
  "bulletList",
  "orderedList",
  "listItem",
  "table",
  "tableRow",
  "tableCell",
  "tableHeader"
];
const StableIDs = Extension.create({
  name: "workBlockIds",
  addGlobalAttributes: () => [
    {
      types: blocks,
      attributes: {
        workId: {
          default: null,
          parseHTML: (element: HTMLElement) => element.getAttribute("data-work-id"),
          renderHTML: (attrs: Record<string, unknown>) =>
            attrs.workId ? { "data-work-id": attrs.workId } : {}
        }
      }
    }
  ],
  addProseMirrorPlugins() {
    return [
      new Plugin({
        appendTransaction(transactions, _old, state) {
          if (!transactions.some((t) => t.docChanged)) return null;
          const seen = new Set<string>();
          const tr = state.tr;
          state.doc.descendants((node, pos) => {
            if (!blocks.includes(node.type.name)) return;
            const id = node.attrs.workId as string | null;
            if (!id || seen.has(id)) {
              const next = workRequestId();
              tr.setNodeMarkup(pos, undefined, { ...node.attrs, workId: next });
              seen.add(next);
            } else seen.add(id);
          });
          return tr.docChanged ? tr : null;
        }
      })
    ];
  }
});
export function toEditorDocument(body: WorkDocument): JSONContent {
  const map = (n: WorkNode): JSONContent => ({
    type: n.type,
    ...(n.type === "text"
      ? {
          text: n.text || "",
          ...(n.marks?.length ? { marks: n.marks.map((type) => ({ type })) } : {})
        }
      : {
          attrs: { workId: n.id, ...(n.type === "heading" ? { level: n.level } : {}) },
          ...(n.content?.length ? { content: n.content.map(map) } : {})
        })
  });
  return map(body.root);
}
export function fromEditorDocument(doc: JSONContent, rootId = "root"): WorkDocument {
  const map = (n: JSONContent): WorkNode =>
    n.type === "text"
      ? {
          type: "text",
          text: n.text || "",
          ...(n.marks?.length ? { marks: n.marks.map((m) => m.type) } : {})
        }
      : {
          type: n.type || "paragraph",
          id: n.type === "doc" ? rootId : n.attrs?.workId || workRequestId(),
          ...(n.type === "heading" ? { level: n.attrs?.level || 1 } : {}),
          ...(n.content?.length ? { content: n.content.map(map) } : {})
        };
  return { schemaVersion: 1, root: map(doc) };
}

export function WorkDocumentEditor({
  body,
  editable,
  onChange,
  editorKey,
  showToolbar = true
}: {
  body: WorkDocument;
  editable: boolean;
  onChange: (doc: WorkDocument) => void;
  editorKey: string;
  showToolbar?: boolean;
}) {
  const editor = useEditor(
    {
      extensions: [
        StarterKit.configure({
          heading: { levels: [1, 2, 3] },
          blockquote: false,
          codeBlock: false,
          horizontalRule: false,
          hardBreak: false,
          link: false,
          underline: false,
          trailingNode: false
        }),
        TableKit.configure({ table: { resizable: false }, tableCell: false, tableHeader: false }),
        TableCell.extend({ content: "paragraph+" }),
        TableHeader.extend({ content: "paragraph+" }),
        StableIDs
      ],
      content: toEditorDocument(body),
      editable,
      onUpdate: ({ editor: current }) =>
        onChange(fromEditorDocument(current.getJSON(), body.root.id))
    },
    [editorKey]
  );
  useEffect(() => {
    editor?.setEditable(editable, false);
  }, [editor, editable]);
  if (!editor) return null;
  const button = (label: string, action: () => void, active = false) => (
    <button
      type="button"
      disabled={!editable}
      className={active ? "is-active" : ""}
      onMouseDown={(e) => e.preventDefault()}
      onClick={action}
    >
      {label}
    </button>
  );
  return (
    <div className="work-document-editor">
      {showToolbar && (
        <div className="work-editor-toolbar" aria-label="文档格式">
          {button(
            "正文",
            () => editor.chain().focus().setParagraph().run(),
            editor.isActive("paragraph")
          )}
          {([1, 2, 3] as const).map((level) => (
            <span key={level}>
              {button(
                `标题 ${level}`,
                () => editor.chain().focus().toggleHeading({ level }).run(),
                editor.isActive("heading", { level })
              )}
            </span>
          ))}
          {button("粗体", () => editor.chain().focus().toggleBold().run(), editor.isActive("bold"))}
          {button(
            "斜体",
            () => editor.chain().focus().toggleItalic().run(),
            editor.isActive("italic")
          )}
          {button(
            "列表",
            () => editor.chain().focus().toggleBulletList().run(),
            editor.isActive("bulletList")
          )}
          {button(
            "编号",
            () => editor.chain().focus().toggleOrderedList().run(),
            editor.isActive("orderedList")
          )}
          {button("表格", () =>
            editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()
          )}
          {editor.isActive("table") && (
            <>
              {button("加行", () => editor.chain().focus().addRowAfter().run())}
              {button("加列", () => editor.chain().focus().addColumnAfter().run())}
              {button("删行", () => editor.chain().focus().deleteRow().run())}
              {button("删列", () => editor.chain().focus().deleteColumn().run())}
              {button("删除表格", () => editor.chain().focus().deleteTable().run())}
            </>
          )}
          {button("撤销", () => editor.chain().focus().undo().run())}
          {button("重做", () => editor.chain().focus().redo().run())}
        </div>
      )}
      <EditorContent editor={editor} aria-label="文档正文" />
    </div>
  );
}
