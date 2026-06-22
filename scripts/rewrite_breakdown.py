import json
from pathlib import Path

raw = Path("testdata/evals/rewrite/latest_run_clean.json").read_text(encoding="utf-8")
data = json.loads(raw[raw.rfind('{"suite"'):])
samples = data["samples"]

rewrite_only_pass = 0
retrieval_only_fail = 0
both_fail = 0
for s in samples:
    rc = s.get("rule_checks") or {}
    rewrite_ok = all(rc.values()) if rc else False
    retrieval_fail = "critical_expected_ids_missing" in (s.get("critical_failures") or [])
    if rewrite_ok and s.get("passed"):
        rewrite_only_pass += 1
    elif rewrite_ok and not s.get("passed"):
        retrieval_only_fail += 1
    elif not rewrite_ok:
        both_fail += 1

print("rewrite rules all ok & passed:", rewrite_only_pass)
print("rewrite rules ok but failed (likely retrieval):", retrieval_only_fail)
print("rewrite rules failed:", both_fail)

# list rewrite-ok retrieval-fail
print("\nrewrite OK, retrieval fail:")
for s in samples:
    rc = s.get("rule_checks") or {}
    if all(rc.values()) and not s.get("passed"):
        print(" ", s["name"], s.get("tags"), s.get("failure_reasons"))

print("\nrewrite fail:")
for s in samples:
    rc = s.get("rule_checks") or {}
    if rc and not all(rc.values()):
        bad = [k for k,v in rc.items() if not v]
        print(" ", s["name"], bad)

# split_guard / single_intent
for tag in ("single_intent", "split_guard", "split_required", "multi_intent"):
    tagged = [s for s in samples if tag in (s.get("tags") or [])]
    if tagged:
        p = sum(1 for s in tagged if s.get("passed"))
        print(f"\n{tag}: {p}/{len(tagged)}")
