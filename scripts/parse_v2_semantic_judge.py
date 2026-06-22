import json
from collections import defaultdict
from pathlib import Path

path = Path("testdata/evals/rewrite/v2_24_semantic_judge.json")
raw = path.read_bytes()
idx = raw.rfind(b'{"suite"')
if idx < 0:
    raise SystemExit("no suite json in output")
text = raw[idx:].decode("utf-8", errors="replace").lstrip("\ufeff")
data = json.loads(text)
samples = data["samples"]
agg = data["aggregate"]

print("RUN", data["run_metadata"]["run_at"])
passed = sum(1 for s in samples if s["passed"])
print(f"RULE_PASS {passed}/{len(samples)} ({agg['pass_rate']*100:.1f}%)")
metrics = agg.get("metrics") or {}
print(f"MRR uplift {metrics.get('mrr_uplift', 0):+.3f}")
print(f"avg_semantic {metrics.get('avg_semantic_score')}")
print(f"avg_judge {metrics.get('avg_judge_score')}")
print(f"semantic_judge_overrides {metrics.get('semantic_judge_override_count', 0)}")

by_path = defaultdict(int)
rule_only_pass = 0
for s in samples:
    path = (s.get("scores") or {}).get("pass_path")
    if path:
        by_path[path] += 1
    if s["passed"] and (s.get("rule_checks") or {}).get("rule_passed"):
        rule_only_pass += 1
print("\nPASS_PATH", dict(by_path))
print(f"passed_with_rule_green {rule_only_pass}/{passed}")

rule_fail_judge_pass = []
rule_fail_semantic_pass = []
rule_fail_both_pass = []
for s in samples:
    rule_pass = s["passed"]
    scores = s.get("scores") or {}
    judge = scores.get("judge_score")
    semantic = scores.get("semantic_score")
    judge_pass = judge is not None and judge >= 0.65
    semantic_pass = semantic is not None and semantic >= 0.65
    if not rule_pass and judge_pass:
        rule_fail_judge_pass.append((s["name"], judge, semantic))
    if not rule_pass and semantic_pass:
        rule_fail_semantic_pass.append(s["name"])
    if not rule_pass and judge_pass and semantic_pass:
        rule_fail_both_pass.append(s["name"])

print(f"\nRULE_FAIL but JUDGE>=0.65: {len(rule_fail_judge_pass)}")
for name, j, sem in rule_fail_judge_pass:
    print(f"  {name}: judge={j}, semantic={sem}")

print(f"\nRULE_FAIL but SEMANTIC>=0.65: {len(rule_fail_semantic_pass)}")
for name in rule_fail_semantic_pass:
    print(f"  {name}")

print(f"\nRULE_FAIL but BOTH>=0.65: {len(rule_fail_both_pass)}")
for name in rule_fail_both_pass:
    print(f"  {name}")

print("\nRULE_FAILURES")
for s in samples:
    if not s["passed"]:
        print(f"  {s['name']}: {s.get('failure_reasons')}")
