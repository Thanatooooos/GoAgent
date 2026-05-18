"""Analyze T2Retrieval benchmark results by positive-count bucket and tuning categories.

Usage:
    python scripts/analyze_t2retrieval_results.py
    python scripts/analyze_t2retrieval_results.py testdata/t2retrieval_10k_results.json --worst 15
"""

from __future__ import annotations

import argparse
import json
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Analyze T2Retrieval retrieve-eval JSON output")
    parser.add_argument(
        "results",
        nargs="?",
        default="testdata/t2retrieval_10k_results.json",
        help="path to retrieve-eval JSON output",
    )
    parser.add_argument("--worst", type=int, default=12, help="number of worst samples to print")
    parser.add_argument(
        "--category-limit",
        type=int,
        default=8,
        help="number of samples to print per tuning category",
    )
    parser.add_argument(
        "--export-json",
        default="",
        help="optional path to write structured category analysis JSON",
    )
    parser.add_argument(
        "--export-md",
        default="",
        help="optional path to write a markdown tuning checklist",
    )
    return parser.parse_args()


def load_results(path: Path) -> dict:
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def bucket_name(expected_count: int) -> str:
    if expected_count <= 1:
        return "1"
    if expected_count <= 3:
        return "2-3"
    if expected_count <= 7:
        return "4-7"
    return "8+"


def bucket_order(name: str) -> int:
    order = {"1": 0, "2-3": 1, "4-7": 2, "8+": 3}
    return order.get(name, 99)


def rank_group(rank: int) -> str:
    if rank <= 0:
        return "miss"
    if rank == 1:
        return "rank1"
    if rank <= 3:
        return "rank2-3"
    if rank <= 10:
        return "rank4-10"
    return "rank10+"


def safe_get(metric_map: dict, key: int) -> float:
    return float(metric_map.get(str(key), metric_map.get(key, 0.0)))


def summarize_bucket(samples: list[dict]) -> dict:
    count = len(samples)
    if count == 0:
        return {
            "count": 0,
            "hit1": 0.0,
            "hit10": 0.0,
            "mrr": 0.0,
            "recall10": 0.0,
            "ndcg10": 0.0,
            "avg_expected": 0.0,
        }

    return {
        "count": count,
        "hit1": sum(1 for s in samples if safe_get(s["hitAtK"], 1)) / count,
        "hit10": sum(1 for s in samples if safe_get(s["hitAtK"], 10)) / count,
        "mrr": sum(float(s["reciprocalRank"]) for s in samples) / count,
        "recall10": sum(safe_get(s["recallAtK"], 10) for s in samples) / count,
        "ndcg10": sum(safe_get(s["ndcgAtK"], 10) for s in samples) / count,
        "avg_expected": sum(len(s["expectedIds"]) for s in samples) / count,
    }


def format_pct(value: float) -> str:
    return f"{value:.4f}"


def sample_rank(sample: dict) -> int:
    return int(sample.get("firstRelevantRank", 0) or 0)


def recall_at(sample: dict, k: int) -> float:
    return safe_get(sample["recallAtK"], k)


def ndcg_at(sample: dict, k: int) -> float:
    return safe_get(sample["ndcgAtK"], k)


def category_matches(sample: dict, category: str) -> bool:
    expected = len(sample["expectedIds"])
    rank = sample_rank(sample)
    recall10 = recall_at(sample, 10)

    if category == "single_positive_miss":
        return expected == 1 and rank <= 0
    if category == "single_positive_low_rank":
        return expected == 1 and rank > 1
    if category == "few_positive_low_rank":
        return 2 <= expected <= 3 and rank > 1
    if category == "multi_positive_low_recall":
        return expected >= 4 and recall10 < 0.8
    raise ValueError(f"unknown category: {category}")


def category_sort_key(category: str, sample: dict) -> tuple[Any, ...]:
    rank = sample_rank(sample)
    normalized_rank = 999 if rank <= 0 else rank
    recall10 = recall_at(sample, 10)
    expected = len(sample["expectedIds"])
    query = sample["query"]

    if category == "single_positive_miss":
        return (normalized_rank, -recall10, query)
    if category in {"single_positive_low_rank", "few_positive_low_rank"}:
        return (normalized_rank, -recall10, query)
    if category == "multi_positive_low_recall":
        return (recall10, normalized_rank, -expected, query)
    raise ValueError(f"unknown category: {category}")


def build_categories(samples: list[dict], limit: int) -> dict[str, list[dict]]:
    category_names = [
        "single_positive_miss",
        "single_positive_low_rank",
        "few_positive_low_rank",
        "multi_positive_low_recall",
    ]
    categories: dict[str, list[dict]] = {}
    for name in category_names:
        matched = [sample for sample in samples if category_matches(sample, name)]
        matched.sort(key=lambda sample: category_sort_key(name, sample))
        categories[name] = matched[:limit] if limit > 0 else matched
    return categories


def summarize_category(samples: list[dict]) -> dict[str, float]:
    if not samples:
        return {"count": 0, "hit1": 0.0, "mrr": 0.0, "recall10": 0.0, "ndcg10": 0.0}

    count = len(samples)
    return {
        "count": count,
        "hit1": sum(1 for sample in samples if safe_get(sample["hitAtK"], 1)) / count,
        "mrr": sum(float(sample["reciprocalRank"]) for sample in samples) / count,
        "recall10": sum(recall_at(sample, 10) for sample in samples) / count,
        "ndcg10": sum(ndcg_at(sample, 10) for sample in samples) / count,
    }


def category_title(name: str) -> str:
    mapping = {
        "single_positive_miss": "single_positive_miss",
        "single_positive_low_rank": "single_positive_low_rank",
        "few_positive_low_rank": "few_positive_low_rank",
        "multi_positive_low_recall": "multi_positive_low_recall",
    }
    return mapping[name]


def sample_brief(sample: dict) -> dict[str, Any]:
    return {
        "name": sample["name"],
        "query": sample["query"],
        "positiveCount": len(sample["expectedIds"]),
        "firstRelevantRank": sample_rank(sample),
        "hitAt1": bool(safe_get(sample["hitAtK"], 1)),
        "recallAt10": recall_at(sample, 10),
        "ndcgAt10": ndcg_at(sample, 10),
    }


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)
        f.write("\n")


def write_markdown(path: Path, source_name: str, categories: dict[str, list[dict]]) -> None:
    lines = [f"# T2Retrieval Tuning Checklist: {source_name}", ""]
    for name, samples in categories.items():
        summary = summarize_category(samples)
        lines.append(f"## {category_title(name)}")
        lines.append(
            f"- count: {summary['count']}"
            f", hit@1: {summary['hit1']:.4f}"
            f", mrr: {summary['mrr']:.4f}"
            f", recall@10: {summary['recall10']:.4f}"
            f", ndcg@10: {summary['ndcg10']:.4f}"
        )
        if not samples:
            lines.append("- none")
            lines.append("")
            continue
        for sample in samples:
            rank = sample_rank(sample)
            rank_text = "miss" if rank <= 0 else str(rank)
            lines.append(
                f"- rank={rank_text}, pos={len(sample['expectedIds'])}, "
                f"rec@10={recall_at(sample, 10):.2f}, ndcg@10={ndcg_at(sample, 10):.2f}, "
                f"query={sample['query']}"
            )
        lines.append("")

    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines).rstrip() + "\n")


def main() -> None:
    args = parse_args()
    path = Path(args.results)
    data = load_results(path)
    samples = data["samples"]
    overall = data["overall"]

    grouped: dict[str, list[dict]] = defaultdict(list)
    rank_groups = Counter()
    for sample in samples:
        bucket = bucket_name(len(sample["expectedIds"]))
        grouped[bucket].append(sample)
        rank_groups[rank_group(int(sample.get("firstRelevantRank", 0) or 0))] += 1

    print(f"=== T2Retrieval Result Analysis: {path.name} ===")
    print()
    print(
        "overall:"
        f" samples={overall['sampleCount']}"
        f" hit@1={format_pct(safe_get(overall['hitRateAtK'], 1))}"
        f" hit@10={format_pct(safe_get(overall['hitRateAtK'], 10))}"
        f" mrr={format_pct(float(overall['mrr']))}"
        f" recall@10={format_pct(safe_get(overall['averageRecallAtK'], 10))}"
        f" ndcg@10={format_pct(safe_get(overall['averageNdcgAtK'], 10))}"
    )
    print()

    print("by_positive_bucket:")
    print(f"{'bucket':<8} {'count':<6} {'avg_pos':<8} {'hit@1':<8} {'hit@10':<8} {'mrr':<8} {'rec@10':<8} {'ndcg@10':<8}")
    for bucket in sorted(grouped.keys(), key=bucket_order):
        metrics = summarize_bucket(grouped[bucket])
        print(
            f"{bucket:<8} {metrics['count']:<6d} {metrics['avg_expected']:<8.2f} "
            f"{metrics['hit1']:<8.4f} {metrics['hit10']:<8.4f} {metrics['mrr']:<8.4f} "
            f"{metrics['recall10']:<8.4f} {metrics['ndcg10']:<8.4f}"
        )
    print()

    print("rank_distribution:")
    total = len(samples) or 1
    for key in ("miss", "rank1", "rank2-3", "rank4-10"):
        count = rank_groups.get(key, 0)
        print(f"  {key:<8} {count:>3d}  ({count / total:.1%})")
    print()

    def worst_key(sample: dict) -> tuple[int, float, int, str]:
        rank = int(sample.get("firstRelevantRank", 0) or 0)
        normalized_rank = 999 if rank <= 0 else rank
        return (
            normalized_rank,
            safe_get(sample["recallAtK"], 10),
            len(sample["expectedIds"]),
            sample["query"],
        )

    worst_samples = sorted(samples, key=worst_key, reverse=True)[: args.worst]
    print(f"worst_samples_top_{len(worst_samples)}:")
    for sample in worst_samples:
        rank = int(sample.get("firstRelevantRank", 0) or 0)
        bucket = bucket_name(len(sample["expectedIds"]))
        rank_text = "miss" if rank <= 0 else str(rank)
        print(
            f"  rank={rank_text:>4} bucket={bucket:<4} pos={len(sample['expectedIds']):>2d} "
            f"rec@10={safe_get(sample['recallAtK'], 10):.2f} ndcg@10={safe_get(sample['ndcgAtK'], 10):.2f} "
            f"{sample['query'][:80]}"
        )

    print()
    categories = build_categories(samples, args.category_limit)
    print("tuning_categories:")
    for name, category_samples in categories.items():
        metrics = summarize_category(category_samples)
        print(
            f"  {category_title(name)} count={int(metrics['count'])}"
            f" hit@1={metrics['hit1']:.4f}"
            f" mrr={metrics['mrr']:.4f}"
            f" rec@10={metrics['recall10']:.4f}"
            f" ndcg@10={metrics['ndcg10']:.4f}"
        )
        for sample in category_samples:
            rank = sample_rank(sample)
            rank_text = "miss" if rank <= 0 else str(rank)
            print(
                f"    - rank={rank_text:>4} pos={len(sample['expectedIds']):>2d} "
                f"rec@10={recall_at(sample, 10):.2f} ndcg@10={ndcg_at(sample, 10):.2f} "
                f"{sample['query'][:80]}"
            )

    if args.export_json:
        export_json_path = Path(args.export_json)
        export_payload = {
            "source": str(path),
            "overall": overall,
            "categories": {
                name: {
                    "summary": summarize_category(category_samples),
                    "samples": [sample_brief(sample) for sample in category_samples],
                }
                for name, category_samples in categories.items()
            },
        }
        write_json(export_json_path, export_payload)

    if args.export_md:
        write_markdown(Path(args.export_md), path.name, categories)


if __name__ == "__main__":
    main()
