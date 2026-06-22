import json
from pathlib import Path

p = Path("testdata/evals/rewrite/latest_run_clean.json")
raw = p.read_text(encoding="utf-8")
i = raw.rfind('{"suite"')
data = json.loads(raw[i:])
for s in data.get("samples", []):
    if not s.get("name", "").startswith("coref_"):
        continue
    rc = s.get("artifacts", {}).get("retrieval_comparison", {})
    rw = s.get("artifacts", {}).get("rewrite", {})
    b = rc.get("baseline", {})
    c = rc.get("candidate", {})
    print(s["name"], "passed=", s.get("passed"))
    print("  rewritten:", rw.get("rewrittenQuery", ""))
    print("  subs:", rw.get("subQuestions"))
    print("  baseline  MRR=", b.get("reciprocalRank"), "hit@1=", b.get("hitAtK", {}).get("1"), "retrievedCount=", b.get("retrievedCount"))
    print("  candidate MRR=", c.get("reciprocalRank"), "hit@1=", c.get("hitAtK", {}).get("1"), "retrievedCount=", c.get("retrievedCount"))
    print("  critical_ids_ok=", rc.get("critical_ids_ok"), "reasons=", rc.get("failure_reasons"))
    print()
