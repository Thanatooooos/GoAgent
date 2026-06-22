import json
from pathlib import Path

raw = Path("testdata/evals/rewrite/latest_run_clean.json").read_text(encoding="utf-8")
data = json.loads(raw[raw.rfind('{"suite"'):])
execs = data["artifacts"]["executions"]
names = [
    "coref_redis_persistence_followup",
    "alias_listen_ports_shorthand",
    "intent_guard_cancel_downstream",
    "regress_guard_slice_118",
    "regress_guard_multi_gmp",
]
for n in names:
    s = next(x for x in data["samples"] if x["name"] == n)
    art = execs[n]
    rc = art.get("retrieval_comparison", {})
    print("=" * 60)
    print(n, "passed=", s["passed"])
    print("  failures:", s.get("failure_reasons"))
    print("  rewritten:", art.get("rewritten_query"))
    print("  subs:", art.get("sub_questions"), "count=", len(art.get("sub_questions") or []))
    print("  rewrite checks:", {k: v for k, v in (s.get("rule_checks") or {}).items() if not v})
    if rc:
        b, c = rc.get("baseline", {}), rc.get("candidate", {})
        print("  baseline MRR", b.get("reciprocalRank"), "ids", rc.get("baseline_retrieved_ids"))
        print("  candidate MRR", c.get("reciprocalRank"), "ids", rc.get("candidate_retrieved_ids"))
        print("  expected", rc.get("expected_ids"), "critical", rc.get("critical_expected_ids"))
        print("  delta MRR", rc.get("delta", {}).get("mrr"))
