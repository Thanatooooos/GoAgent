import json
from collections import defaultdict
from pathlib import Path

raw = Path("testdata/evals/rewrite/v2_24_run.json").read_bytes()
idx = raw.rfind(b'{"suite"')
data = json.loads(raw[idx:])
samples = data["samples"]
agg = data["aggregate"]

print("RUN", data["run_metadata"]["run_at"])
passed = sum(1 for s in samples if s["passed"])
print(f"PASS {passed}/{len(samples)} ({agg['pass_rate']*100:.1f}%)")
print(
    f"MRR {agg.get('baseline_mrr', 0):.3f} -> {agg.get('candidate_mrr', 0):.3f} "
    f"(uplift {agg.get('mrr_uplift', 0):+.3f})"
)
print(f"Hit@1 uplift {agg.get('hit_at_1_uplift', 0):+.3f}")
print(f"Hit@5 uplift {agg.get('hit_at_5_uplift', 0):+.3f}")

by_tag = defaultdict(lambda: {"t": 0, "p": 0, "fail": []})
for s in samples:
    for tag in s.get("tags") or []:
        by_tag[tag]["t"] += 1
        if s["passed"]:
            by_tag[tag]["p"] += 1
        else:
            by_tag[tag]["fail"].append(s["name"])

print("\nBY_TAG")
for tag, v in sorted(by_tag.items(), key=lambda x: x[1]["p"] / max(x[1]["t"], 1)):
    print(f"  {tag}: {v['p']}/{v['t']}", v["fail"] or "")

print("\nFAILURES")
for s in samples:
    if not s["passed"]:
        print(f"  {s['name']}: {s.get('critical_failures') or s.get('failure_reasons')}")
