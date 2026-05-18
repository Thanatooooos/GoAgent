"""
Resolve T2Retrieval eval samples by mapping passage IDs to goagent chunk IDs.

Usage:
    python scripts/resolve_eval_samples.py

Inputs:
    testdata/t2retrieval_samples.json      — eval samples with passage IDs
    testdata/t2retrieval_passages_mapping.json  — passage_id → chunk_id mapping (from corpus-loader)

Output:
    testdata/t2retrieval_eval_resolved.json — eval samples with goagent chunk IDs
"""

import json
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
TESTDATA = REPO_ROOT / "testdata"


def log(msg: str) -> None:
    print(f"[resolve] {msg}", file=sys.stderr)


def load_json(path: Path) -> dict:
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def main() -> None:
    import argparse
    parser = argparse.ArgumentParser(description="Resolve eval sample passage IDs → chunk IDs")
    parser.add_argument("--samples", default=str(TESTDATA / "t2retrieval_samples.json"), help="eval samples JSON")
    parser.add_argument("--mapping", required=True, help="passage→chunk mapping JSON from corpus-loader")
    parser.add_argument("--output", default=str(TESTDATA / "t2retrieval_eval_resolved.json"), help="output path")
    args = parser.parse_args()

    samples_path = Path(args.samples)
    mapping_path = Path(args.mapping)
    output_path = Path(args.output)

    if not mapping_path.exists():
        log(f"mapping file not found: {mapping_path}")
        log("run corpus-loader first to generate the mapping")
        sys.exit(1)

    samples_data = load_json(samples_path)
    mappings = load_json(mapping_path)

    # Build pid → chunkID lookup
    pid_to_chunk: dict[str, str] = {}
    for m in mappings:
        pid_to_chunk[m["passageId"]] = m["chunkId"]

    log(f"loaded {len(samples_data['samples'])} samples, {len(pid_to_chunk)} mappings")

    resolved = []
    skipped = 0
    total_expected = 0
    total_resolved = 0

    for sample in samples_data["samples"]:
        expected_ids = sample.get("expectedIds", [])
        expected_rel = sample.get("expectedRelevance", {})

        # Resolve passage IDs → chunk IDs
        resolved_ids = []
        resolved_rel = {}
        for pid in expected_ids:
            chunk_id = pid_to_chunk.get(pid)
            if chunk_id:
                resolved_ids.append(chunk_id)
                if pid in expected_rel:
                    resolved_rel[chunk_id] = expected_rel[pid]
            else:
                skipped += 1

        total_expected += len(expected_ids)
        total_resolved += len(resolved_ids)

        # Keep only samples that still have at least 1 relevant chunk
        if not resolved_ids:
            continue

        sample["expectedIds"] = resolved_ids
        sample["expectedRelevance"] = resolved_rel
        sample["name"] = f"t2_{sample['name']}" if not sample['name'].startswith("t2_") else sample["name"]
        resolved.append(sample)

    log(f"resolved: {len(resolved)} samples ({len(samples_data['samples']) - len(resolved)} dropped for lacking mappings)")
    log(f"passage→chunk: {total_resolved}/{total_expected} resolved ({skipped} missing from mapping)")

    source_meta = samples_data.get("meta", {}) if isinstance(samples_data, dict) else {}
    resolved_meta = {
        "source_samples": str(samples_path),
        "source_mapping": str(mapping_path),
        "input_sample_count": len(samples_data["samples"]),
        "output_sample_count": len(resolved),
        "dropped_sample_count": len(samples_data["samples"]) - len(resolved),
        "total_expected_ids": total_expected,
        "resolved_expected_ids": total_resolved,
        "missing_expected_ids": skipped,
        "resolve_rate": round((total_resolved / total_expected), 4) if total_expected else 0,
        "source_meta": source_meta,
    }

    output = {"meta": resolved_meta, "samples": resolved}
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(output, f, ensure_ascii=False, indent=2)
    log(f"wrote {output_path} ({output_path.stat().st_size} bytes)")


if __name__ == "__main__":
    main()
