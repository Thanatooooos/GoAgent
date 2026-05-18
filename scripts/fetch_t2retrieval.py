"""
Fetch a T2Retrieval subset from HuggingFace C-MTEB and convert it to goagent
evaluation inputs.

Usage:
    pip install datasets pandas
    python scripts/fetch_t2retrieval.py --output-prefix t2retrieval_10k

Outputs:
    testdata/<prefix>_passages.json   - passages for goagent knowledge base import
    testdata/<prefix>_samples.json    - eval samples in goagent retrieve-eval format
    testdata/<prefix>_summary.json    - dataset statistics and sampling metadata
"""

from __future__ import annotations

import argparse
import json
import os
import random
import sys
from collections import Counter, defaultdict
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
TESTDATA = REPO_ROOT / "testdata"

DEFAULT_PASSAGE_LIMIT = 10000
DEFAULT_SAMPLE_LIMIT = 300
DEFAULT_MIN_PASSAGE_LEN = 80
DEFAULT_MAX_PASSAGE_LEN = 2000
DEFAULT_SAMPLE_STRATEGY = "stratified"
DEFAULT_OUTPUT_PREFIX = "t2retrieval"
DEFAULT_TOP_K = 10
DEFAULT_SEED = 20260516

SAMPLE_STRATEGIES = ("coverage_desc", "random", "stratified")
BUCKET_ORDER = ("1", "2-3", "4-7", "8+")


def log(msg: str) -> None:
    print(f"[t2retrieval] {msg}", file=sys.stderr)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Build T2Retrieval subset files for goagent")
    parser.add_argument("--passage-limit", type=int, default=DEFAULT_PASSAGE_LIMIT, help="max passages to export")
    parser.add_argument("--sample-limit", type=int, default=DEFAULT_SAMPLE_LIMIT, help="max eval samples to export")
    parser.add_argument("--min-passage-len", type=int, default=DEFAULT_MIN_PASSAGE_LEN, help="skip shorter passages")
    parser.add_argument("--max-passage-len", type=int, default=DEFAULT_MAX_PASSAGE_LEN, help="skip longer passages")
    parser.add_argument(
        "--sample-strategy",
        choices=SAMPLE_STRATEGIES,
        default=DEFAULT_SAMPLE_STRATEGY,
        help="query sampling strategy",
    )
    parser.add_argument("--seed", type=int, default=DEFAULT_SEED, help="random seed used by random/stratified strategies")
    parser.add_argument("--output-prefix", default=DEFAULT_OUTPUT_PREFIX, help="output filename prefix under testdata/")
    parser.add_argument("--top-k", type=int, default=DEFAULT_TOP_K, help="topK written into eval samples")
    return parser.parse_args()


def load_datasets():
    log("loading C-MTEB/T2Retrieval from HuggingFace...")
    try:
        from datasets import load_dataset
    except ImportError:
        log("pip install datasets first")
        sys.exit(1)

    corpus_ds = load_dataset("C-MTEB/T2Retrieval", split="corpus", streaming=True)
    queries_ds = load_dataset("C-MTEB/T2Retrieval", split="queries", streaming=True)
    qrels_ds = load_dataset("C-MTEB/T2Retrieval-qrels", split="dev", streaming=True)
    return corpus_ds, queries_ds, qrels_ds


def build_passages(corpus_ds, limit: int, min_len: int, max_len: int) -> tuple[dict[str, str], dict[str, int]]:
    """Extract up to `limit` passages of reasonable length."""
    passages: dict[str, str] = {}
    stats = {"skipped_short": 0, "skipped_long": 0}
    for row in corpus_ds:
        pid = str(row["id"]).strip()
        text = str(row["text"]).strip()
        if len(text) < min_len:
            stats["skipped_short"] += 1
            continue
        if len(text) > max_len:
            stats["skipped_long"] += 1
            continue
        if pid in passages:
            continue
        passages[pid] = text
        if len(passages) >= limit:
            break
    log(
        "passages: selected=%d skipped_short=%d skipped_long=%d"
        % (len(passages), stats["skipped_short"], stats["skipped_long"])
    )
    return passages, stats


def build_queries_map(queries_ds) -> dict[str, str]:
    """Build qid -> query text map."""
    queries: dict[str, str] = {}
    for row in queries_ds:
        qid = str(row["id"]).strip()
        text = str(row["text"]).strip()
        if qid and text:
            queries[qid] = text
    log(f"queries loaded: {len(queries)}")
    return queries


def relevance_bucket(relevant_count: int) -> str:
    if relevant_count <= 1:
        return "1"
    if relevant_count <= 3:
        return "2-3"
    if relevant_count <= 7:
        return "4-7"
    return "8+"


def build_sample_candidates(qrels_ds, passages: dict[str, str], queries: dict[str, str]) -> list[dict]:
    """Build all candidate queries that still have at least one mapped relevant passage."""
    qid_to_pids: dict[str, set[str]] = {}
    qid_to_score: dict[str, dict[str, int]] = {}
    score_values: set[int] = set()
    for row in qrels_ds:
        qid = str(row["qid"]).strip()
        pid = str(row["pid"]).strip()
        score = int(row.get("score", 1))
        score_values.add(score)
        if pid not in passages:
            continue
        if qid not in qid_to_pids:
            qid_to_pids[qid] = set()
            qid_to_score[qid] = {}
        qid_to_pids[qid].add(pid)
        qid_to_score[qid][pid] = max(qid_to_score[qid].get(pid, 0), score)

    candidates: list[dict] = []
    for qid, pid_set in qid_to_pids.items():
        query_text = queries.get(qid, "")
        if not query_text:
            continue
        pids = sorted(pid_set)
        scores = {pid: qid_to_score[qid].get(pid, 1) for pid in pids}
        relevant_count = len(pids)
        total_relevance = sum(scores.values())
        candidates.append(
            {
                "qid": qid,
                "name": f"t2_{qid}",
                "query": query_text,
                "tags": ["t2retrieval", "c-mteb"],
                "target": "chunk",
                "expectedIds": pids,
                "expectedRelevance": scores,
                "relevantCount": relevant_count,
                "totalRelevance": total_relevance,
                "bucket": relevance_bucket(relevant_count),
            }
        )

    log(
        "candidate queries: %d with score_values=%s"
        % (len(candidates), ",".join(str(v) for v in sorted(score_values)))
    )
    return candidates


def select_samples(candidates: list[dict], limit: int, strategy: str, seed: int) -> list[dict]:
    if limit <= 0 or not candidates:
        return []

    if strategy == "coverage_desc":
        ordered = sorted(
            candidates,
            key=lambda item: (-item["totalRelevance"], -item["relevantCount"], item["qid"]),
        )
        return ordered[:limit]

    rng = random.Random(seed)

    if strategy == "random":
        ordered = list(candidates)
        rng.shuffle(ordered)
        return ordered[:limit]

    grouped: dict[str, list[dict]] = defaultdict(list)
    for item in candidates:
        grouped[item["bucket"]].append(item)
    for bucket in BUCKET_ORDER:
        rng.shuffle(grouped[bucket])

    selected: list[dict] = []
    while len(selected) < limit:
        progressed = False
        for bucket in BUCKET_ORDER:
            bucket_items = grouped[bucket]
            if not bucket_items:
                continue
            selected.append(bucket_items.pop())
            progressed = True
            if len(selected) >= limit:
                break
        if not progressed:
            break
    return selected


def build_samples(candidates: list[dict], top_k: int) -> list[dict]:
    samples = []
    for item in candidates:
        samples.append(
            {
                "name": item["name"],
                "query": item["query"],
                "tags": item["tags"],
                "target": item["target"],
                "expectedIds": item["expectedIds"],
                "expectedRelevance": item["expectedRelevance"],
                "searchMode": "hybrid",
                "topK": top_k,
            }
        )
    avg_relevant = sum(len(s["expectedIds"]) for s in samples) / max(len(samples), 1)
    log(f"samples built: {len(samples)} queries with avg {avg_relevant:.1f} relevant passages")
    return samples


def bucket_counts(items: list[dict]) -> dict[str, int]:
    counter = Counter(item["bucket"] for item in items)
    return {bucket: counter.get(bucket, 0) for bucket in BUCKET_ORDER}


def summarize(passages: dict[str, str], passage_stats: dict[str, int], candidates: list[dict], selected: list[dict], args: argparse.Namespace) -> dict:
    total_chars = sum(len(v) for v in passages.values())
    avg_len = total_chars / max(len(passages), 1)
    avg_relevant = sum(item["relevantCount"] for item in selected) / max(len(selected), 1)
    return {
        "dataset": "C-MTEB/T2Retrieval",
        "output_prefix": args.output_prefix,
        "passage_count": len(passages),
        "passage_avg_chars": round(avg_len, 1),
        "sample_count": len(selected),
        "avg_relevant_per_sample": round(avg_relevant, 1),
        "candidate_query_count": len(candidates),
        "sample_strategy": args.sample_strategy,
        "seed": args.seed,
        "top_k": args.top_k,
        "passage_filters": {
            "min_len": args.min_passage_len,
            "max_len": args.max_passage_len,
            "skipped_short": passage_stats["skipped_short"],
            "skipped_long": passage_stats["skipped_long"],
        },
        "candidate_bucket_distribution": bucket_counts(candidates),
        "selected_bucket_distribution": bucket_counts(selected),
        "relevance_grades": sorted(
            {
                grade
                for item in selected
                for grade in item["expectedRelevance"].values()
            }
        ),
    }


def export_json(data, path: Path, label: str) -> None:
    TESTDATA.mkdir(parents=True, exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
    log(f"wrote {label}: {path} ({os.path.getsize(path)} bytes)")


def main() -> None:
    args = parse_args()
    corpus_ds, queries_ds, qrels_ds = load_datasets()

    passages, passage_stats = build_passages(
        corpus_ds,
        limit=args.passage_limit,
        min_len=args.min_passage_len,
        max_len=args.max_passage_len,
    )
    queries = build_queries_map(queries_ds)
    candidates = build_sample_candidates(qrels_ds, passages, queries)
    selected = select_samples(candidates, limit=args.sample_limit, strategy=args.sample_strategy, seed=args.seed)
    samples = build_samples(selected, top_k=args.top_k)
    summary = summarize(passages, passage_stats, candidates, selected, args)

    prefix = args.output_prefix.strip() or DEFAULT_OUTPUT_PREFIX
    export_json(
        {"description": "T2Retrieval passages for goagent knowledge base import", "meta": summary, "passages": passages},
        TESTDATA / f"{prefix}_passages.json",
        "passages",
    )
    export_json(
        {"meta": summary, "samples": samples},
        TESTDATA / f"{prefix}_samples.json",
        "eval samples",
    )
    export_json(summary, TESTDATA / f"{prefix}_summary.json", "summary")

    log(
        "done. passage_limit=%d sample_limit=%d strategy=%s seed=%d"
        % (args.passage_limit, args.sample_limit, args.sample_strategy, args.seed)
    )
    log(f"summary: {json.dumps(summary, ensure_ascii=False)}")


if __name__ == "__main__":
    main()
