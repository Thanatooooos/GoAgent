#!/usr/bin/env python3
"""Move flat knowledge/service files into responsibility subpackages."""

from __future__ import annotations

import os
import shutil

ROOT = os.path.join("internal", "app", "knowledge", "service")

MOVES: dict[str, list[str]] = {
    "base": [
        "knowledge_base_service.go",
        "knowledge_base_service_test.go",
    ],
    "chunk": [
        "knowledge_chunk_service.go",
        "knowledge_chunk_command_service.go",
        "knowledge_chunk_query_service.go",
        "knowledge_chunk_support.go",
        "knowledge_chunk_vector_sync.go",
        "knowledge_chunk_service_test.go",
    ],
    "document": [
        "knowledge_document_service.go",
        "knowledge_document_command_service.go",
        "knowledge_document_query_service.go",
        "knowledge_document_input.go",
        "knowledge_document_builders.go",
        "knowledge_document_delete_transaction.go",
        "knowledge_document_ingestion_completion.go",
        "knowledge_document_ingestion_reconcile.go",
        "knowledge_document_schedule_service.go",
        "knowledge_document_service_test.go",
    ],
    "process": [
        "document_process_service.go",
        "document_process_pipeline.go",
        "document_process_persistence.go",
        "document_process_runtime_state.go",
    ],
}


def main() -> None:
    repo = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    os.chdir(repo)

    for subpkg, files in MOVES.items():
        target_dir = os.path.join(ROOT, subpkg)
        os.makedirs(target_dir, exist_ok=True)
        for name in files:
            src = os.path.join(ROOT, name)
            dst = os.path.join(target_dir, name)
            if not os.path.exists(src):
                raise SystemExit(f"missing source file: {src}")
            shutil.move(src, dst)
            with open(dst, encoding="utf-8") as f:
                content = f.read()
            content = content.replace("package service\n", f"package {subpkg}\n", 1)
            with open(dst, "w", encoding="utf-8", newline="\n") as f:
                f.write(content)
            print(f"moved {name} -> {subpkg}/")

    print("knowledge service split complete")


if __name__ == "__main__":
    main()
