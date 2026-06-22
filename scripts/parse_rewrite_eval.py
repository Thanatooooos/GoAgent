import json
import sys
from collections import Counter
from pathlib import Path

p = Path(sys.argv[1] if len(sys.argv) > 1 else "testdata/evals/rewrite/latest_run_clean.json")
raw = p.read_text(encoding="utf-8")
i = raw.rfind('{"suite"')
if i < 0:
    print("no JSON payload in", p)
    raise SystemExit(1)
data = json.loads(raw[i:])

meta = data.get("run_metadata", {})
agg = data.get("aggregate", {})
metrics = agg.get("metrics", {})
samples = data["samples"]
passed = sum(1 for s in samples if s.get("passed"))

print("=== RUN ===")
print("run_at:", meta.get("run_at"))
print("sample_set:", meta.get("sample_set_id"))
print()
print(f"pass: {passed}/{len(samples)} = {passed/len(samples)*100:.1f}%")
print()
print("=== METRICS ===")
for k in sorted(metrics):
    print(f"  {k}: {metrics[k]}")
print()

fail_reasons = Counter()
critical = Counter()
tags_fail = Counter()
tags_pass = Counter()
for s in samples:
    for r in s.get("failure_reasons", []):
        fail_reasons[r] += 1
    for c in s.get("critical_failures", []):
        critical[c] += 1
    bucket = tags_pass if s.get("passed") else tags_fail
    for t in s.get("tags") or []:
        bucket[t] += 1

print("=== failure_reasons ===")
for k, v in fail_reasons.most_common():
    print(f"  {v}x {k}")
print()
print("=== critical_failures ===")
for k, v in critical.most_common():
    print(f"  {v}x {k}")
print()
print("=== tag pass rates ===")
for t in sorted(set(tags_pass) | set(tags_fail)):
    p_cnt = tags_pass.get(t, 0)
    f_cnt = tags_fail.get(t, 0)
    tot = p_cnt + f_cnt
    if tot:
        print(f"  {t}: {p_cnt}/{tot} ({p_cnt/tot*100:.0f}%)")
print()
print("=== failed samples ===")
for s in samples:
    if s.get("passed"):
        continue
    print(f"  {s['name']}: {s.get('failure_reasons')} | critical={s.get('critical_failures')}")

print()
print("=== coref ===")
execs = data.get("artifacts", {}).get("executions", {})
for s in samples:
    if not s["name"].startswith("coref_"):
        continue
    rc = execs.get(s["name"], {}).get("retrieval_comparison", {})
    print(
        f"  {s['name']}: passed={s.get('passed')} critical_ok={rc.get('critical_ids_ok')} "
        f"candidate_ids={rc.get('candidate_retrieved_ids')}"
    )
