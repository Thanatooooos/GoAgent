import json
from pathlib import Path

raw = Path("testdata/evals/rewrite/latest_run_clean.json").read_text(encoding="utf-8")
data = json.loads(raw[raw.rfind('{"suite"'):])
print("top keys:", list(data.keys()))
agg = data.get("aggregate") or data.get("aggregates") or data.get("metrics")
print("aggregate:", json.dumps(agg, ensure_ascii=False, indent=2)[:3000] if agg else None)

# search nested
def find_keys(obj, prefix=""):
    if isinstance(obj, dict):
        for k, v in obj.items():
            if any(x in k.lower() for x in ("mrr", "uplift", "regression", "pass_rate", "retrieval")):
                print(f"{prefix}{k}: {v}")
            find_keys(v, prefix + k + ".")
    elif isinstance(obj, list) and obj and isinstance(obj[0], dict):
        find_keys(obj[0], prefix + "[0].")

find_keys(data)
