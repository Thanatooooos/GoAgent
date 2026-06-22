#!/usr/bin/env python3
"""Compare retrieve eval summaries across search modes."""
import json
import sys
from pathlib import Path


def load(path: Path) -> dict:
    return json.load(path.open(encoding="utf-8"))


def label(d: dict) -> dict:
    o = d["overall"]
    return {
        "mrr": o["mrr"],
        "hit1": o["hitRateAtK"]["1"],
        "hit3": o["hitRateAtK"]["3"],
        "hit5": o["hitRateAtK"]["5"],
        "hit10": o["hitRateAtK"]["10"],
        "ndcg3": o["averageNdcgAtK"]["3"],
    }


def channels(d: dict) -> dict[str, float]:
    out = {}
    for c in d.get("channels", []):
        out[c["channelName"]] = c["hitRateAtK"].get("1", 0)
    return out


def main() -> None:
    files = sys.argv[1:]
    if not files:
        files = [
            "testdata/retrieve_eval_corpus_semantic.json",
            "testdata/retrieve_eval_corpus_keyword.json",
            "testdata/retrieve_eval_corpus_hybrid_fresh.json",
            "testdata/retrieve_eval_corpus_hybrid_direct.json",
        ]

    rows = []
    for f in files:
        p = Path(f)
        if not p.exists():
            continue
        d = load(p)
        m = label(d)
        rows.append((p.stem.replace("retrieve_eval_corpus_", ""), m, channels(d)))

    print(f"{'mode':<22} {'MRR':>6} {'Hit@1':>7} {'Hit@3':>7} {'Hit@5':>7} {'Hit@10':>7} {'nDCG@3':>7}")
    print("-" * 70)
    for name, m, _ in rows:
        print(f"{name:<22} {m['mrr']:>6.3f} {m['hit1']:>6.1%} {m['hit3']:>6.1%} {m['hit5']:>6.1%} {m['hit10']:>6.1%} {m['ndcg3']:>6.3f}")

    # pairwise hybrid vs semantic if both present
    by_name = {n: m for n, m, _ in rows}
    if "hybrid_fresh" in by_name and "semantic" in by_name:
        h, s = by_name["hybrid_fresh"], by_name["semantic"]
        print(f"\nhybrid_fresh vs semantic: MRR {h['mrr']-s['mrr']:+.3f}  Hit@1 {h['hit1']-s['hit1']:+.1%}")

    if "hybrid_fresh" in by_name and "keyword" in by_name:
        h, k = by_name["hybrid_fresh"], by_name["keyword"]
        print(f"hybrid_fresh vs keyword:  MRR {h['mrr']-k['mrr']:+.3f}  Hit@1 {h['hit1']-k['hit1']:+.1%}")

    if len(rows) >= 2:
        sem = next((m for n, m, _ in rows if n == "semantic"), None)
        hyb = next((m for n, m, _ in rows if "hybrid" in n), None)
        key = next((m for n, m, _ in rows if n == "keyword"), None)
        if sem and hyb and key:
            print("\n=== Takeaway ===")
            print(f"semantic Hit@1 {sem['hit1']:.1%} | keyword Hit@1 {key['hit1']:.1%} | hybrid Hit@1 {hyb['hit1']:.1%}")
            best_mrr = max(sem["mrr"], key["mrr"], hyb["mrr"])
            best_name = ["semantic", "keyword", "hybrid"][[sem["mrr"], key["mrr"], hyb["mrr"]].index(best_mrr)]
            print(f"best MRR: {best_name} ({best_mrr:.3f})")


if __name__ == "__main__":
    main()
