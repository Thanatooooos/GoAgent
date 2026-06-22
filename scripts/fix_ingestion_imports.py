#!/usr/bin/env python3
"""Fix cross-package type references after ingestion service split."""

from __future__ import annotations

import os
import re

ROOT = os.path.join("internal", "app", "ingestion", "service")

WORKFLOW_IMPORT = '\tingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"\n'
RUNNER_IMPORT = '\tingestionrunner "local/rag-project/internal/app/ingestion/service/runner"\n'
OBSERVER_IMPORT = '\tingestionobserver "local/rag-project/internal/app/ingestion/service/observer"\n'


def read(path: str) -> str:
    with open(path, encoding="utf-8") as f:
        return f.read()


def write(path: str, content: str) -> None:
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write(content)


def ensure_import(content: str, import_line: str) -> str:
    if import_line.strip() in content:
        return content
    m = re.search(r"import \(\n", content)
    if not m:
        return content
    insert_at = m.end()
    return content[:insert_at] + import_line + content[insert_at:]


def fix_runner_dir() -> None:
    for name in os.listdir(os.path.join(ROOT, "runner")):
        if not name.endswith(".go"):
            continue
        path = os.path.join(ROOT, "runner", name)
        content = read(path)
        if name == "workflow_node_runner_registry.go":
            content = content.replace("ingestionworkflow", "ingestionworkflow")
        else:
            content = ensure_import(content, WORKFLOW_IMPORT)
            content = re.sub(r"\bExecutionState\b", "ingestionworkflow.ExecutionState", content)
            content = re.sub(r"\bChunkPayload\b", "ingestionworkflow.ChunkPayload", content)
            content = re.sub(r"\bParsedDocument\b", "ingestionworkflow.ParsedDocument", content)
            content = re.sub(r"\bSourcePayload\b", "ingestionworkflow.SourcePayload", content)
            content = re.sub(r"\bIndexResult\b", "ingestionworkflow.IndexResult", content)
        write(path, content)


def fix_observer_dir() -> None:
    for name in os.listdir(os.path.join(ROOT, "observer")):
        if not name.endswith(".go"):
            continue
        path = os.path.join(ROOT, "observer", name)
        content = read(path)
        content = ensure_import(content, WORKFLOW_IMPORT)
        content = re.sub(r"\bExecutionState\b", "ingestionworkflow.ExecutionState", content)
        content = re.sub(r"\bWorkflowNodeSpec\b", "ingestionworkflow.WorkflowNodeSpec", content)
        write(path, content)


def fix_executor_dir() -> None:
    for name in os.listdir(os.path.join(ROOT, "executor")):
        if not name.endswith(".go"):
            continue
        path = os.path.join(ROOT, "executor", name)
        content = read(path)
        content = ensure_import(content, WORKFLOW_IMPORT)
        content = ensure_import(content, RUNNER_IMPORT)
        content = ensure_import(content, OBSERVER_IMPORT)
        content = re.sub(r"\bExecutionState\b", "ingestionworkflow.ExecutionState", content)
        content = re.sub(r"\bWorkflowSpec\b", "ingestionworkflow.WorkflowSpec", content)
        content = re.sub(r"\bWorkflowNodeSpec\b", "ingestionworkflow.WorkflowNodeSpec", content)
        content = re.sub(r"\bWorkflowEdgeSpec\b", "ingestionworkflow.WorkflowEdgeSpec", content)
        content = re.sub(r"\bWorkflowBuilder\b", "ingestionworkflow.WorkflowBuilder", content)
        content = re.sub(r"\bNewEinoGraphWorkflowBuilder\b", "ingestionworkflow.NewEinoGraphWorkflowBuilder", content)
        content = re.sub(r"\bNodeRunnerRegistry\b", "ingestionrunner.NodeRunnerRegistry", content)
        content = re.sub(r"\bNodeRunner\b", "ingestionrunner.NodeRunner", content)
        content = re.sub(r"\bTaskObserver\b", "ingestionobserver.TaskObserver", content)
        content = re.sub(r"\bMetricsService\b", "ingestionobserver.MetricsService", content)
        content = re.sub(r"\bevaluateWorkflowCondition\b", "ingestionworkflow.EvaluateWorkflowCondition", content)
        write(path, content)


def fix_pipeline() -> None:
    path = os.path.join(ROOT, "pipeline", "service_pipeline.go")
    content = read(path)
    content = ensure_import(content, WORKFLOW_IMPORT)
    content = ensure_import(content, RUNNER_IMPORT)
    content = re.sub(r"\bNodeRunnerRegistry\b", "ingestionrunner.NodeRunnerRegistry", content)
    content = re.sub(r"\bNodeIOContract\b", "ingestionworkflow.NodeIOContract", content)
    content = re.sub(r"\bNodeInputRequirement\b", "ingestionworkflow.NodeInputRequirement", content)
    content = re.sub(r"\bListNodeIOContracts\b", "ingestionworkflow.ListNodeIOContracts", content)
    content = content.replace("getNodeIOContract", "ingestionworkflow.GetNodeIOContract")
    content = content.replace("artifactSetFromNames", "ingestionworkflow.ArtifactSetFromNames")
    content = content.replace("mergeArtifactSets", "ingestionworkflow.MergeArtifactSets")
    content = content.replace("artifactSetContainsAny", "ingestionworkflow.ArtifactSetContainsAny")
    content = content.replace("artifactSetNames", "ingestionworkflow.ArtifactSetNames")
    write(path, content)


def fix_task() -> None:
    path = os.path.join(ROOT, "task", "service_task.go")
    content = read(path)
    # task uses readStringSetting from old runner helpers - use workflow via duplicate or export
    write(path, content)


def fix_workflow_tests() -> None:
    path = os.path.join(ROOT, "workflow", "graph_workflow_test.go")
    if not os.path.exists(path):
        return
    content = read(path)
    content = content.replace("evaluateWorkflowCondition", "EvaluateWorkflowCondition")
    write(path, content)


def main() -> None:
    repo = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    os.chdir(repo)
    fix_runner_dir()
    fix_observer_dir()
    fix_executor_dir()
    fix_pipeline()
    fix_workflow_tests()
    print("fixed cross-package references")


if __name__ == "__main__":
    main()
