"""Score manually reviewed evidence concepts in retrieve-eval output."""

import argparse
import json
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--samples", type=Path, required=True)
    parser.add_argument("--results", type=Path, required=True)
    parser.add_argument("--top-k", type=int, default=5)
    args = parser.parse_args()

    if args.top_k < 1:
        parser.error("--top-k must be positive")

    samples = json.loads(args.samples.read_text(encoding="utf-8"))["samples"]
    results = json.loads(args.results.read_text(encoding="utf-8"))["samples"]
    by_name = {row["name"]: row for row in results}
    reviewed = [row for row in samples if row.get("expectedEvidenceGroups")]
    if not reviewed:
        parser.error("no samples contain expectedEvidenceGroups")

    complete = 0
    for sample in reviewed:
        result = by_name.get(sample["name"])
        if result is None:
            parser.error(f"missing result for {sample['name']}")
        retrieved = {item["chunkId"] for item in result["retrieved"][: args.top_k]}
        missing = [
            group["name"]
            for group in sample["expectedEvidenceGroups"]
            if not retrieved.intersection(group["chunkIds"])
        ]
        complete += not missing
        print(f"{sample['name']}: {'complete' if not missing else 'missing ' + ', '.join(missing)}")

    print(f"complete evidence groups@{args.top_k}: {complete}/{len(reviewed)}")


if __name__ == "__main__":
    main()
