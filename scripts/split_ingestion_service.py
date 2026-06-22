#!/usr/bin/env python3
"""Move ingestion/service files into responsibility subpackages."""

from __future__ import annotations

import os
import shutil

ROOT = os.path.join("internal", "app", "ingestion", "service")

MOVES: dict[str, tuple[str, str]] = {
    # (target_subdir, new_package)
    "workflow_execution_state.go": ("workflow", "workflow"),
    "workflow_builder_eino.go": ("workflow", "workflow"),
    "workflow_condition.go": ("workflow", "workflow"),
    "node_contracts.go": ("workflow", "workflow"),
    "workflow_node_runner_registry.go": ("runner", "runner"),
    "runner_helpers_shared.go": ("runner", "runner"),
    "runner_enrichment_helpers.go": ("runner", "runner"),
    "runner_fetcher.go": ("runner", "runner"),
    "runner_parser.go": ("runner", "runner"),
    "runner_chunker.go": ("runner", "runner"),
    "runner_indexer.go": ("runner", "runner"),
    "runner_enhancer.go": ("runner", "runner"),
    "runner_enricher.go": ("runner", "runner"),
    "observer_task_repository.go": ("observer", "observer"),
    "observer_task_multi.go": ("observer", "observer"),
    "observer_metrics.go": ("observer", "observer"),
    "executor_workflow.go": ("executor", "executor"),
    "executor_eino_graph.go": ("executor", "executor"),
    "service_pipeline.go": ("pipeline", "pipeline"),
    "service_task.go": ("task", "task"),
}

TEST_MOVES: dict[str, str] = {
    "graph_workflow_test.go": "workflow",
    "executor_workflow_test.go": "executor",
    "runner_node_test.go": "runner",
    "observer_metrics_test.go": "observer",
    "observer_task_repository_test.go": "observer",
    "observer_task_multi_test.go": "observer",
    "service_task_test.go": "task",
}


def move_file(name: str, subdir: str, package: str) -> None:
    src = os.path.join(ROOT, name)
    if not os.path.exists(src):
        print("skip missing", name)
        return
    dst_dir = os.path.join(ROOT, subdir)
    os.makedirs(dst_dir, exist_ok=True)
    dst = os.path.join(dst_dir, name)
    with open(src, encoding="utf-8") as f:
        content = f.read()
    if content.startswith("package service"):
        content = f"package {package}" + content[len("package service") :]
    with open(dst, "w", encoding="utf-8", newline="\n") as f:
        f.write(content)
    os.remove(src)
    print("moved", name, "->", subdir + "/")


def main() -> None:
    repo = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    os.chdir(repo)
    for name, (subdir, package) in MOVES.items():
        move_file(name, subdir, package)
    for name, subdir in TEST_MOVES.items():
        move_file(name, subdir, subdir)
    print("done")


if __name__ == "__main__":
    main()
