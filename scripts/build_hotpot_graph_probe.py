"""Build a small, reproducible cross-document retrieval probe from HotpotQA."""

import hashlib
import json
from pathlib import Path

import requests


API_URL = "https://datasets-server.huggingface.co/rows"
DATASET = "hotpotqa/hotpot_qa"
OUTPUT = Path("testdata/hotpot_graph_probe.json")
DOCUMENT_DIR = Path("testdata/hotpot_graph_probe_docs")
PER_TYPE = 10


def main() -> None:
    response = requests.get(
        API_URL,
        params={
            "dataset": DATASET,
            "config": "distractor",
            "split": "validation",
            "offset": 0,
            "length": 100,
        },
        timeout=30,
    )
    response.raise_for_status()
    rows = [entry["row"] for entry in response.json()["rows"]]

    questions = []
    documents = {}
    counts = {"bridge": 0, "comparison": 0}
    for row in rows:
        kind = row["type"]
        if kind not in counts or counts[kind] >= PER_TYPE:
            continue
        gold_titles = sorted(set(row["supporting_facts"]["title"]))
        titles = row["context"]["title"]
        sentences = row["context"]["sentences"]
        if len(gold_titles) != 2 or len(titles) != 10 or not set(gold_titles) <= set(titles):
            continue

        context_ids = []
        title_to_id = {}
        for title, parts in zip(titles, sentences, strict=True):
            body = " ".join(parts).strip()
            doc_id = hashlib.sha256((title + "\n" + body).encode()).hexdigest()[:20]
            context_ids.append(doc_id)
            title_to_id[title] = doc_id
            documents[doc_id] = {"id": doc_id, "title": title, "text": f"{title}\n\n{body}"}

        questions.append(
            {
                "id": row["id"],
                "question": row["question"],
                "answer": row["answer"],
                "type": kind,
                "level": row["level"],
                "expectedDocumentIds": [title_to_id[title] for title in gold_titles],
                "contextDocumentIds": context_ids,
                "supportingFacts": row["supporting_facts"],
            }
        )
        counts[kind] += 1
        if all(count == PER_TYPE for count in counts.values()):
            break

    if counts != {"bridge": PER_TYPE, "comparison": PER_TYPE}:
        raise RuntimeError(f"insufficient valid HotpotQA examples: {counts}")

    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    DOCUMENT_DIR.mkdir(parents=True, exist_ok=True)
    payload = {
        "source": "https://huggingface.co/datasets/hotpotqa/hotpot_qa",
        "license": "CC BY-SA 4.0",
        "selection": "First 10 bridge and 10 comparison examples among distractor validation rows 0-99 with two distinct supporting titles and 10 context paragraphs",
        "questions": questions,
        "documents": [documents[key] for key in sorted(documents)],
    }
    OUTPUT.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    for doc_id, doc in documents.items():
        (DOCUMENT_DIR / f"{doc_id}.txt").write_text(doc["text"] + "\n", encoding="utf-8")
    print(f"questions={len(questions)} documents={len(documents)} output={OUTPUT}")


if __name__ == "__main__":
    main()
