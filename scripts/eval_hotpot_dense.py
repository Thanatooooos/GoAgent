"""Evaluate the HotpotQA probe with the same local embeddings used by LightRAG."""

import argparse
import hashlib
import json
from pathlib import Path

import numpy as np
import requests


INPUT = Path("testdata/hotpot_graph_probe.json")
LOCAL_MODEL = "qwen3-embedding:0.6b"
SILICONFLOW_MODEL = "Qwen/Qwen3-Embedding-8B"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--provider", choices=("ollama", "siliconflow"), default="ollama")
    parser.add_argument("--url")
    args = parser.parse_args()
    model = LOCAL_MODEL if args.provider == "ollama" else SILICONFLOW_MODEL
    cache_path = Path(
        "testdata/hotpot_dense_embeddings.json"
        if args.provider == "ollama"
        else "testdata/hotpot_dense_embeddings_siliconflow.json"
    )
    url = args.url or (
        "http://127.0.0.1:11435" if args.provider == "ollama" else "https://api.siliconflow.cn/v1"
    )

    source = INPUT.read_bytes()
    probe = json.loads(source)
    documents = probe["documents"]
    questions = probe["questions"]
    items = [("doc:" + doc["id"], doc["text"]) for doc in documents]
    items += [("question:" + question["id"], question["question"]) for question in questions]
    fingerprint = hashlib.sha256(source).hexdigest()
    cache = {"model": model, "source_sha256": fingerprint, "vectors": {}}
    if cache_path.exists():
        old = json.loads(cache_path.read_text(encoding="utf-8"))
        if old["model"] == model and old["source_sha256"] == fingerprint:
            cache = old

    missing = [(key, content) for key, content in items if key not in cache["vectors"]]
    session = requests.Session()
    if args.provider == "siliconflow":
        pairs = [
            line.split("=", 1)
            for line in Path(".env").read_text(encoding="utf-8").splitlines()
            if "=" in line and not line.lstrip().startswith("#")
        ]
        api_key = dict(pairs)["AI_PROVIDERS_SILICONFLOW_API_KEY"].strip().strip('"')
        session.headers["Authorization"] = "Bearer " + api_key
    for start in range(0, len(missing), 8):
        batch = missing[start : start + 8]
        inputs = [content for _, content in batch]
        if args.provider == "ollama":
            response = session.post(url + "/api/embed", json={"model": model, "input": inputs}, timeout=120)
        else:
            response = session.post(
                url + "/embeddings",
                json={"model": model, "input": inputs, "dimensions": 1024},
                timeout=120,
            )
        response.raise_for_status()
        data = response.json()
        vectors = (
            data["embeddings"]
            if args.provider == "ollama"
            else [item["embedding"] for item in sorted(data["data"], key=lambda item: item["index"])]
        )
        if len(vectors) != len(batch):
            raise RuntimeError("Ollama returned an unexpected embedding count")
        for (key, _), vector in zip(batch, vectors, strict=True):
            cache["vectors"][key] = vector
        cache_path.write_text(json.dumps(cache), encoding="utf-8")
        print(f"embedded {min(start + 8, len(missing))}/{len(missing)} missing texts", flush=True)

    document_vectors = np.asarray(
        [cache["vectors"]["doc:" + doc["id"]] for doc in documents], dtype=np.float32
    )
    question_vectors = np.asarray(
        [cache["vectors"]["question:" + question["id"]] for question in questions],
        dtype=np.float32,
    )
    document_vectors /= np.linalg.norm(document_vectors, axis=1, keepdims=True)
    question_vectors /= np.linalg.norm(question_vectors, axis=1, keepdims=True)
    scores = question_vectors @ document_vectors.T
    ids = [doc["id"] for doc in documents]

    for top_k in (3, 5, 10):
        for kind in ("all", "bridge", "comparison"):
            selected = [
                (question, row)
                for question, row in zip(questions, scores, strict=True)
                if kind == "all" or question["type"] == kind
            ]
            full = any_hit = 0
            recall = 0.0
            for question, row in selected:
                ranked = sorted(range(len(ids)), key=lambda index: (-row[index], ids[index]))
                found = {ids[index] for index in ranked[:top_k]}
                gold = set(question["expectedDocumentIds"])
                hits = len(found & gold)
                full += hits == len(gold)
                any_hit += hits > 0
                recall += hits / len(gold)
            print(
                f"dense {args.provider} {kind} top{top_k}: full={full}/{len(selected)} "
                f"any={any_hit}/{len(selected)} mean_recall={recall / len(selected):.3f}"
            )


if __name__ == "__main__":
    main()
