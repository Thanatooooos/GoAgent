#!/usr/bin/env python3
"""Per-sample delta between two retrieve eval summaries."""
import json
import sys
from pathlib import Path

a = json.load(Path(sys.argv[1]).open(encoding="utf-8"))
b = json.load(Path(sys.argv[2]).open(encoding="utf-8"))
label_a, label_b = sys.argv[3], sys.argv[4]

sa = {s["name"]: s for s in a["samples"]}
sb = {s["name"]: s for s in b["samples"]}

better, worse, same = [], [], []
for name in sa:
    if name not in sb:
        continue
    d = sb[name]["reciprocalRank"] - sa[name]["reciprocalRank"]
    ra = sa[name].get("firstRelevantRank") or 0
    rb = sb[name].get("firstRelevantRank") or 0
    row = (d, name, ra, rb, sa[name]["reciprocalRank"], sb[name]["reciprocalRank"])
    if d > 1e-9:
        better.append(row)
    elif d < -1e-9:
        worse.append(row)
    else:
        same.append(row)

print(f"{label_b} vs {label_a}: better={len(better)} worse={len(worse)} same={len(same)}")
print(f"\n{label_b} wins:")
for row in sorted(better, reverse=True)[:8]:
    print(f"  {row[1]}: rank {row[2] or '-'}->{row[3] or '-'} (dRR={row[0]:+.3f})")
print(f"\n{label_b} loses:")
for row in sorted(worse)[:8]:
    print(f"  {row[1]}: rank {row[2] or '-'}->{row[3] or '-'} (dRR={row[0]:+.3f})")
