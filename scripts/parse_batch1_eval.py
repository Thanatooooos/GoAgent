import json
from collections import Counter
from pathlib import Path

p = Path("testdata/evals/rewrite/batch1_after_fix.json")
raw = p.read_text(encoding="utf-8")
i = raw.rfind('{"suite"')
if i < 0:
    print("no json found")
    raise SystemExit(1)
data = json.loads(raw[i:])
samples = data["samples"]
passed = sum(1 for s in samples if s.get("passed"))
print(f"pass: {passed}/{len(samples)} = {passed/len(samples)*100:.1f}%")
print("metrics:", data.get("aggregate", {}).get("metrics"))
print()
for s in samples:
    if not s["name"].startswith("coref_"):
        continue
    rc = data["artifacts"]["executions"][s["name"]].get("retrieval_comparison", {})
    print(
        s["name"],
        "passed=",
        s.get("passed"),
        "baseline_ids=",
        rc.get("baseline_retrieved_ids"),
        "candidate_ids=",
        rc.get("candidate_retrieved_ids"),
        "critical_ok=",
        rc.get("critical_ids_ok"),
    )
