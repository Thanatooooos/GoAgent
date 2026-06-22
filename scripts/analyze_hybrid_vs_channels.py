#!/usr/bin/env python3
"""Compare merged hybrid ranking vs single-channel rankings from same eval run."""
import json
import sys
from pathlib import Path


def rr(rank: int | None) -> float:
    return 0.0 if not rank else 1.0 / rank


def eval_ranking(ranked_ids: list[str], expected: set[str]) -> tuple[int, float, dict[int, bool]]:
    first = 0
    for i, cid in enumerate(ranked_ids):
        if cid in expected:
            first = i + 1
            break
    hits = {k: any(rid in expected for rid in ranked_ids[:k]) for k in (1, 3, 5, 10)}
    return first, rr(first), hits


def main() -> None:
    path = Path(sys.argv[1] if len(sys.argv) > 1 else "testdata/retrieve_eval_corpus_hybrid_direct.json")
    data = json.load(path.open(encoding="utf-8"))

    modes = {"merged": [], "vector_global": [], "keyword": [], "metadata_title": []}
    uplift_vs_vector = []

    for s in data["samples"]:
        merged_rr = s["reciprocalRank"]
        merged_hit1 = s["hitAtK"]["1"]
        modes["merged"].append((merged_rr, merged_hit1))

        channel_by_name = {c["channelName"]: c for c in s.get("channels", [])}
        for ch in ("vector_global", "keyword", "metadata_title"):
            c = channel_by_name.get(ch)
            if not c:
                continue
            rank = c.get("firstRelevantRank") or 0
            modes[ch].append((rr(rank if rank else None), c.get("hitAtK", {}).get("1", False)))

        v = channel_by_name.get("vector_global")
        if v:
            v_rank = v.get("firstRelevantRank") or 0
            m_rank = s.get("firstRelevantRank") or 0
            uplift_vs_vector.append((merged_rr - rr(v_rank if v_rank else None), s["name"], v_rank, m_rank))

    n = len(modes["merged"])
    print(f"=== From {path.name} (n={n}) ===\n")
    print(f"{'mode':<16} {'MRR':>6} {'Hit@1':>7} {'Hit@10(est)':>12}")
    for mode, rows in modes.items():
        if not rows:
            continue
        mrr = sum(r[0] for r in rows) / len(rows)
        hit1 = sum(1 for r in rows if r[1]) / len(rows)
        print(f"{mode:<16} {mrr:>6.3f} {hit1:>6.1%} {'':>12}")

    better = [x for x in uplift_vs_vector if x[0] > 1e-9]
    worse = [x for x in uplift_vs_vector if x[0] < -1e-9]
    same = [x for x in uplift_vs_vector if abs(x[0]) <= 1e-9]
    print(f"\nmerged vs vector_global per-sample: better={len(better)} worse={len(worse)} same={len(same)}")
    print("hybrid wins (vector rank -> merged rank):")
    for d, name, vr, mr in sorted(better, reverse=True)[:6]:
        print(f"  {name}: {vr or '-'} -> {mr or '-'} (dRR={d:+.3f})")
    print("hybrid loses:")
    for d, name, vr, mr in sorted(worse)[:6]:
        print(f"  {name}: {vr or '-'} -> {mr or '-'} (dRR={d:+.3f})")


if __name__ == "__main__":
    main()
