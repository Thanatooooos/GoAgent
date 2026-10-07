"""Summarize complete LightRAG source coverage at comparable document budgets."""

import argparse
import json
import statistics
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, default=Path("testdata/hotpot_siliconflow_results.json"))
    args = parser.parse_args()
    data = json.loads(args.input.read_text(encoding="utf-8"))
    counts = data["documentStatus"]
    if counts.get("processed") != 199 or counts.get("failed") != 0:
        raise RuntimeError(f"incomplete document index: {counts}")
    if "inProgress" in data:
        raise RuntimeError("query evaluation is still running")

    for mode, result in data["results"].items():
        rows = result["rows"]
        if len(rows) != 20:
            raise RuntimeError(f"incomplete {mode} result: {len(rows)}/20")
        counts_by_budget = {}
        for budget in (3, 5, 10):
            full = any_hit = 0
            recall = 0.0
            for row in rows:
                unique_ids = list(dict.fromkeys(row["retrievedDocumentIds"]))
                gold = set(row["expectedDocumentIds"])
                hits = len(set(unique_ids[:budget]) & gold)
                full += hits == len(gold)
                any_hit += hits > 0
                recall += hits / len(gold)
            counts_by_budget[budget] = (full, any_hit, recall / len(rows))
        mean_sources = statistics.mean(row["retrievedDocumentCount"] for row in rows)
        print(
            f"{mode}: refs_mean={mean_sources:.1f} all_full={result['fullEvidence']}/20 "
            + " ".join(
                f"top{budget}_full={full}/20 top{budget}_any={any_hit}/20 "
                f"top{budget}_recall={recall:.3f}"
                for budget, (full, any_hit, recall) in counts_by_budget.items()
            )
        )


if __name__ == "__main__":
    main()
