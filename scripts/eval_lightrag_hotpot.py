"""Measure source-document coverage from LightRAG query references."""

import argparse
import json
import time
from pathlib import Path

import requests


INPUT = Path("testdata/hotpot_graph_probe.json")
OUTPUT = Path("testdata/hotpot_lightrag_results.json")
MODES = ("naive", "local", "global", "hybrid", "mix")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default="http://127.0.0.1:9621")
    parser.add_argument("--output", type=Path, default=OUTPUT)
    parser.add_argument("--limit", type=int, default=0, help="0 evaluates all 20 questions")
    parser.add_argument("--allow-partial", action="store_true", help="permit fewer indexed documents than the full probe")
    parser.add_argument("--modes", nargs="+", choices=MODES, default=list(MODES))
    args = parser.parse_args()

    probe = json.loads(INPUT.read_text(encoding="utf-8"))
    session = requests.Session()
    counts = session.get(args.url + "/documents/status_counts", timeout=15).json()["status_counts"]
    if not args.allow_partial and counts.get("processed", 0) < len(probe["documents"]):
        raise RuntimeError(f"full evaluation requires {len(probe['documents'])} indexed documents; status={counts}")

    questions = probe["questions"][: args.limit or None]
    results = {}
    for mode in args.modes:
        rows = []
        for question in questions:
            started = time.monotonic()
            response = session.post(
                args.url + "/query",
                json={
                    "query": question["question"],
                    "mode": mode,
                    "only_need_context": True,
                    "include_references": True,
                    "top_k": 5,
                },
                timeout=180,
            )
            response.raise_for_status()
            found = [Path(ref.get("file_path", "")).stem for ref in response.json().get("references", [])]
            gold = set(question["expectedDocumentIds"])
            hits = len(set(found) & gold)
            rows.append(
                {
                    "questionId": question["id"],
                    "expectedDocumentIds": question["expectedDocumentIds"],
                    "retrievedDocumentIds": found,
                    "retrievedDocumentCount": len(set(found)),
                    "goldHits": hits,
                    "seconds": round(time.monotonic() - started, 3),
                }
            )
            print(f"{mode} {question['id']} hits={hits}/{len(gold)}", flush=True)
            args.output.write_text(
                json.dumps({"documentStatus": counts, "results": results, "inProgress": {mode: rows}}, indent=2)
                + "\n",
                encoding="utf-8",
            )
        results[mode] = {
            "fullEvidence": sum(row["goldHits"] == 2 for row in rows),
            "anyEvidence": sum(row["goldHits"] > 0 for row in rows),
            "meanRecall": sum(row["goldHits"] / 2 for row in rows) / len(rows),
            "rows": rows,
        }

    args.output.write_text(json.dumps({"documentStatus": counts, "results": results}, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {args.output}")


if __name__ == "__main__":
    main()
