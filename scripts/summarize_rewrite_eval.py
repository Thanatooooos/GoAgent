import json
from collections import Counter
from pathlib import Path

p = Path("testdata/evals/rewrite/latest_run_clean.json")
raw = p.read_text(encoding="utf-8")
i = raw.rfind('{"suite"')
data = json.loads(raw[i:])

summary = data.get("summary", {})
print("=== RUN ===")
print("run_at:", data.get("run_metadata", {}).get("run_at"))
print("sample_set:", data.get("run_metadata", {}).get("sample_set_id"))
print()

print("=== SUMMARY ===")
for k, v in sorted(summary.items()):
    print(f"  {k}: {v}")
print()

samples = data.get("samples", [])
passed = sum(1 for s in samples if s.get("passed"))
print(f"pass: {passed}/{len(samples)} = {passed/len(samples)*100:.1f}%")
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
    tag_set = s.get("tags") or []
    bucket = tags_pass if s.get("passed") else tags_fail
    for t in tag_set:
        bucket[t] += 1

print("=== failure_reasons ===")
for k, v in fail_reasons.most_common():
    print(f"  {v}x {k}")
print()

print("=== critical_failures ===")
for k, v in critical.most_common():
    print(f"  {v}x {k}")
print()

print("=== tag pass rates (samples may have multiple tags) ===")
all_tags = set(tags_pass) | set(tags_fail)
for t in sorted(all_tags):
    p_cnt = tags_pass.get(t, 0)
    f_cnt = tags_fail.get(t, 0)
    tot = p_cnt + f_cnt
    if tot:
        print(f"  {t}: {p_cnt}/{tot} pass ({p_cnt/tot*100:.0f}%)")
