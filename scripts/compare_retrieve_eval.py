#!/usr/bin/env python3
"""Compare direct vs rewrite retrieve eval summaries."""
import json
import sys
from pathlib import Path


def load(path: Path) -> dict:
    return json.load(path.open(encoding="utf-8"))


def main() -> None:
    if len(sys.argv) != 3:
        print("usage: compare_retrieve_eval.py direct.json rewrite.json")
        sys.exit(1)

    direct = load(Path(sys.argv[1]))
    rewrite = load(Path(sys.argv[2]))

    def overall_label(d: dict) -> str:
        o = d["overall"]
        hit1 = o["hitRateAtK"].get("1", 0)
        return f"MRR={o['mrr']:.4f} Hit@1={hit1:.1%}"

    print("=== Overall ===")
    print(f"direct:  {overall_label(direct)}")
    print(f"rewrite: {overall_label(rewrite)}")

    d_samples = {s["name"]: s for s in direct["samples"]}
    r_samples = {s["name"]: s for s in rewrite["samples"]}

    better, worse, same = [], [], []
    for name in d_samples:
        if name not in r_samples:
            continue
        d_rr = d_samples[name]["reciprocalRank"]
        r_rr = r_samples[name]["reciprocalRank"]
        delta = r_rr - d_rr
        row = (delta, name, d_rr, r_rr, d_samples[name].get("firstRelevantRank"), r_samples[name].get("firstRelevantRank"))
        if delta > 1e-9:
            better.append(row)
        elif delta < -1e-9:
            worse.append(row)
        else:
            same.append(row)

    print(f"\n=== Per-sample delta (rewrite - direct) ===")
    print(f"better: {len(better)}  worse: {len(worse)}  same: {len(same)}")

    print("\nTop improvements:")
    for row in sorted(better, reverse=True)[:8]:
        print(f"  +{row[0]:.3f} {row[1]} rank {row[4]}->{row[5]} rr {row[2]:.3f}->{row[3]:.3f}")

    print("\nTop regressions:")
    for row in sorted(worse)[:8]:
        print(f"  {row[0]:.3f} {row[1]} rank {row[4]}->{row[5]} rr {row[2]:.3f}->{row[3]:.3f}")

    # channels
    def ch_hit(d: dict, name: str) -> float | None:
        for c in d.get("channels", []):
            if c["channelName"] == name:
                return c["hitRateAtK"].get("1")
        return None

    print("\n=== Channel Hit@1 ===")
    for ch in ["keyword", "vector_global", "metadata_title"]:
        d_h, r_h = ch_hit(direct, ch), ch_hit(rewrite, ch)
        if d_h is not None:
            print(f"  {ch}: direct={d_h:.1%} rewrite={r_h:.1%}")


if __name__ == "__main__":
    main()
