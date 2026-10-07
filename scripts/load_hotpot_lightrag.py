"""Insert HotpotQA probe paragraphs into the isolated local LightRAG service."""

import argparse
import json
import time
from pathlib import Path

import requests


INPUT = Path("testdata/hotpot_graph_probe.json")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default="http://127.0.0.1:9621")
    parser.add_argument("--all", action="store_true", help="insert all 199 documents")
    parser.add_argument("--wait-seconds", type=int, default=600)
    args = parser.parse_args()

    probe = json.loads(INPUT.read_text(encoding="utf-8"))
    documents = {document["id"]: document for document in probe["documents"]}
    if args.all:
        selected_ids = sorted(documents)
    else:
        first_question = probe["questions"][0]
        gold = first_question["expectedDocumentIds"]
        distractor = next(doc_id for doc_id in first_question["contextDocumentIds"] if doc_id not in gold)
        selected_ids = gold + [distractor]

    session = requests.Session()
    track_ids = []
    for doc_id in selected_ids:
        response = session.post(
            args.url + "/documents/text",
            json={"text": documents[doc_id]["text"], "file_source": f"hotpot/{doc_id}.txt"},
            timeout=30,
        )
        if response.status_code == 409:
            print(f"already present {doc_id}: {response.text}")
            continue
        response.raise_for_status()
        result = response.json()
        print(f"accepted {doc_id} {result}")
        if result.get("track_id"):
            track_ids.append(result["track_id"])

    if args.wait_seconds <= 0:
        return

    deadline = time.monotonic() + args.wait_seconds
    while time.monotonic() < deadline:
        results = []
        for track_id in track_ids:
            response = session.get(args.url + f"/documents/track_status/{track_id}", timeout=15)
            response.raise_for_status()
            results.extend(response.json().get("documents", []))
        statuses = [str(item.get("status", "")).lower() for item in results]
        print(f"status {statuses}")
        if statuses and all(status in {"processed", "failed"} for status in statuses):
            if "failed" in statuses:
                raise RuntimeError("one or more LightRAG documents failed to index")
            return
        time.sleep(10)
    raise TimeoutError(f"indexing did not finish within {args.wait_seconds}s")


if __name__ == "__main__":
    main()
