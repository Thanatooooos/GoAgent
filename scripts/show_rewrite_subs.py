import json
import sys
from pathlib import Path

p = Path(sys.argv[1] if len(sys.argv) > 1 else "testdata/evals/rewrite/latest_run_clean.json")
raw = p.read_text(encoding="utf-8")
i = raw.rfind('{"suite"')
data = json.loads(raw[i:])
execs = data.get("artifacts", {}).get("executions", {})
samples = data["samples"]

# filter: failed on sub_question_count or tag split_guard/single_intent
focus = []
for s in samples:
    rc = s.get("rule_checks") or {}
    subs_fail = rc.get("sub_question_count_ok") is False
    tags = set(s.get("tags") or [])
    if subs_fail or tags & {"split_guard", "single_intent", "split_required", "multi_intent"}:
        focus.append(s)

print(f"=== sub_questions 明细（共 {len(focus)} 条相关样本）===\n")
for s in sorted(focus, key=lambda x: x["name"]):
    art = execs.get(s["name"], {})
    subs = art.get("sub_questions") or []
    rewritten = art.get("rewritten_query") or ""
    expect = art.get("retrieval_expectation") or {}
    # rewrite expectation may be in sample only via rule - get max from failures
    passed = s.get("passed")
    sub_ok = (s.get("rule_checks") or {}).get("sub_question_count_ok")
    tags = ",".join(s.get("tags") or [])
    print(f"{s['name']}")
    print(f"  tags: {tags}")
    print(f"  passed={passed}  sub_question_count_ok={sub_ok}  count={len(subs)}")
    print(f"  rewritten: {rewritten[:120]}{'...' if len(rewritten)>120 else ''}")
    for j, q in enumerate(subs, 1):
        print(f"    sub[{j}]: {q}")
    if not subs:
        print("    (no sub_questions in artifact)")
    print()

print("=== 仅 sub_question_count 失败 ===")
for s in samples:
    rc = s.get("rule_checks") or {}
    if rc.get("sub_question_count_ok") is False:
        art = execs.get(s["name"], {})
        subs = art.get("sub_questions") or []
        print(f"  {s['name']}: count={len(subs)} subs={subs}")

print()
print("=== 全量 sub_questions 计数分布 ===")
from collections import Counter
dist = Counter()
for name, art in execs.items():
    subs = art.get("sub_questions") or []
    dist[len(subs)] += 1
for n in sorted(dist):
    print(f"  {n} subs: {dist[n]} samples")
