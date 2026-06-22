import json
from pathlib import Path

p = Path("testdata/evals/rewrite/batch3_collapse_test.json")
raw = p.read_text(encoding="utf-8")
i = raw.rfind('{"suite"')
if i < 0:
    print("NO JSON - stderr tail:")
    print(Path("testdata/evals/rewrite/batch3_collapse_test.err").read_text(encoding="utf-8")[-2000:])
    raise SystemExit(1)
data = json.loads(raw[i:])
samples = data["samples"]
passed = sum(1 for s in samples if s.get("passed"))
print(f"pass: {passed}/{len(samples)}")
execs = data.get("artifacts", {}).get("executions", {})
for s in samples:
    art = execs.get(s["name"], {})
    subs = art.get("sub_questions") or []
    rc = s.get("rule_checks") or {}
    print(f"  {s['name']}: passed={s['passed']} sub_ok={rc.get('sub_question_count_ok')} count={len(subs)}")
