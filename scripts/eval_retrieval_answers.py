"""Generate cited answers from saved retrieval results for manual review."""

import argparse
import json
import os
import time
from pathlib import Path

import psycopg2
import requests


MODEL = "Qwen/Qwen3-32B"
API_URL = "https://api.siliconflow.cn/v1/chat/completions"
ANSWER_SYSTEM_PROMPT = "请仅依据给出的证据回答问题。每个主要结论后引用支持它的 chunk ID，格式为 [chunk ID]。证据不足时明确说明，不要凭常识补充。"
ANSWER_USER_PROMPT_TEMPLATE = "问题：{query}\n\n证据：\n{context}"
CONDITIONS = {
    "keyword5": ".cache/retrieval_multi_keyword5_v2_executed.json",
    "hybrid5": ".cache/retrieval_multi_hybrid5_v2_executed.json",
    "hybrid10": ".cache/retrieval_multi_hybrid10_v2_executed.json",
}


def load_env() -> dict[str, str]:
    values = {}
    for line in Path(".env").read_text(encoding="utf-8").splitlines():
        if line.strip() and not line.lstrip().startswith("#") and "=" in line:
            key, value = line.split("=", 1)
            values[key] = value.strip().strip('"')
    return values


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path(".cache/retrieval_answer_probe.json"))
    parser.add_argument("--samples", type=Path, default=Path("docs/retrieval_multi_evidence_samples.json"))
    parser.add_argument("--condition", action="append", default=[], metavar="NAME=PATH")
    parser.add_argument("--model", default=MODEL)
    parser.add_argument("--no-thinking", action="store_true", help="request non-reasoning mode")
    parser.add_argument("--max-new", type=int, default=0, help="stop after this many new answers; 0 means all")
    args = parser.parse_args()

    conditions = CONDITIONS.copy()
    if args.condition:
        conditions = {}
        for spec in args.condition:
            name, sep, path = spec.partition("=")
            if not sep or not name or not path:
                parser.error("--condition must be NAME=PATH")
            conditions[name] = path

    samples = {
        row["name"]: row
        for row in json.loads(args.samples.read_text(encoding="utf-8"))["samples"]
        if row.get("expectedEvidenceGroups")
    }
    results = {
        condition: {
            row["name"]: row
            for row in json.loads(Path(path).read_text(encoding="utf-8"))["samples"]
        }
        for condition, path in conditions.items()
    }
    chunk_ids = {
        item["chunkId"]
        for rows in results.values()
        for name in samples
        for item in rows[name]["retrieved"]
    }

    env = load_env()
    api_key = os.environ.get("AI_PROVIDERS_SILICONFLOW_API_KEY") or env["AI_PROVIDERS_SILICONFLOW_API_KEY"]
    if not api_key:
        parser.error("SiliconFlow API key is missing")
    with psycopg2.connect(
        host="127.0.0.1",
        port=5432,
        dbname="ragent",
        user=env["SPRING_DATASOURCE_USERNAME"],
        password=env["SPRING_DATASOURCE_PASSWORD"],
    ) as db:
        db.set_session(readonly=True)
        with db.cursor() as cursor:
            cursor.execute(
                "SELECT chunk_id, content FROM t_knowledge_chunk_vector WHERE chunk_id = ANY(%s)",
                (list(chunk_ids),),
            )
            content = dict(cursor.fetchall())
    missing = chunk_ids - content.keys()
    if missing:
        parser.error(f"{len(missing)} retrieved chunks are missing from the database")

    output = {"model": args.model, "noThinking": args.no_thinking, "conditions": conditions, "answers": []}
    if args.output.exists():
        output = json.loads(args.output.read_text(encoding="utf-8"))
        if (output["model"] != args.model or output.get("noThinking", False) != args.no_thinking
                or output["conditions"] != conditions):
            parser.error("existing output uses different model, thinking mode, or conditions")
    done = {(row["condition"], row["name"]) for row in output["answers"]}
    session = requests.Session()
    session.headers["Authorization"] = f"Bearer {api_key}"
    new_count = 0

    for name, sample in samples.items():
        for condition, rows in results.items():
            if (condition, name) in done:
                continue
            hits = rows[name]["retrieved"]
            evidence = [
                {"chunkId": item["chunkId"], "content": content[item["chunkId"]]}
                for item in hits
            ]
            context = "\n\n".join(f"[{row['chunkId']}]\n{row['content']}" for row in evidence)
            payload = {
                "model": args.model,
                "temperature": 0,
                "max_tokens": 1000,
                "messages": [
                    {"role": "system", "content": ANSWER_SYSTEM_PROMPT},
                    {"role": "user", "content": ANSWER_USER_PROMPT_TEMPLATE.format(query=sample['query'], context=context)},
                ],
            }
            if args.no_thinking:
                payload["enable_thinking"] = False
            for attempt in range(5):
                response = session.post(API_URL, json=payload, timeout=180)
                if response.status_code != 429:
                    break
                if attempt == 0:
                    print(f"SiliconFlow 429: {response.text[:500]}", flush=True)
                if attempt == 4:
                    break
                delay = min(15 * 2**attempt, 60)
                print(f"rate limited; retrying in {delay}s", flush=True)
                time.sleep(delay)
            if not response.ok:
                print(f"SiliconFlow {response.status_code}: {response.text[:500]}", flush=True)
            response.raise_for_status()
            data = response.json()
            output["answers"].append({
                "name": name,
                "condition": condition,
                "question": sample["query"],
                "evidence": evidence,
                "answer": data["choices"][0]["message"]["content"],
                "usage": data.get("usage"),
            })
            args.output.parent.mkdir(parents=True, exist_ok=True)
            args.output.write_text(json.dumps(output, ensure_ascii=False, indent=2), encoding="utf-8")
            print(f"{len(output['answers'])}/{len(samples) * len(conditions)} {name} {condition}", flush=True)
            new_count += 1
            if args.max_new and new_count >= args.max_new:
                return


if __name__ == "__main__":
    main()
