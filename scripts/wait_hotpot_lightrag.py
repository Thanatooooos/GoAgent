"""Run the full LightRAG probe once every submitted document is indexed."""

import argparse
import json
import subprocess
import sys
import time
from pathlib import Path

import requests


INPUT = Path("testdata/hotpot_graph_probe.json")
EVAL_LOG = Path("testdata/hotpot_lightrag_eval.log")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default="http://127.0.0.1:9621")
    parser.add_argument("--timeout-hours", type=float, default=8)
    parser.add_argument("--max-recoveries", type=int, default=2)
    parser.add_argument("--output", type=Path, default=Path("testdata/hotpot_lightrag_results.json"))
    parser.add_argument("--eval-log", type=Path, default=EVAL_LOG)
    args = parser.parse_args()

    expected = len(json.loads(INPUT.read_text(encoding="utf-8"))["documents"])
    deadline = time.monotonic() + args.timeout_hours * 3600
    session = requests.Session()
    recovery_requests = 0
    while time.monotonic() < deadline:
        try:
            response = session.get(args.url + "/documents/status_counts", timeout=15)
            response.raise_for_status()
            counts = response.json()["status_counts"]
        except requests.RequestException as exc:
            print(f"status unavailable: {exc}", flush=True)
            time.sleep(30)
            continue
        print(f"status {counts}", flush=True)
        if counts.get("all") == expected and counts.get("processed") == expected:
            break
        if counts.get("all") == expected:
            pipeline = session.get(args.url + "/documents/pipeline_status", timeout=15)
            pipeline.raise_for_status()
            if not pipeline.json().get("busy", False) and recovery_requests < args.max_recoveries:
                retry = session.post(args.url + "/documents/reprocess_failed", timeout=15)
                retry.raise_for_status()
                recovery_requests += 1
                print(
                    f"resumed unfinished documents (request {recovery_requests}/{args.max_recoveries})",
                    flush=True,
                )
            elif not pipeline.json().get("busy", False) and recovery_requests >= args.max_recoveries:
                raise RuntimeError(f"LightRAG did not finish after recovery attempts: {counts}")
        time.sleep(30)
    else:
        raise TimeoutError(f"LightRAG did not index {expected} documents in time")

    print("all documents indexed; evaluating", flush=True)
    with args.eval_log.open("w", encoding="utf-8") as log:
        subprocess.run(
            [
                sys.executable, "-X", "utf8", "scripts/eval_lightrag_hotpot.py",
                "--url", args.url, "--output", str(args.output),
            ],
            stdout=log,
            stderr=subprocess.STDOUT,
            check=True,
        )
    print(f"evaluation finished: {args.eval_log}", flush=True)


if __name__ == "__main__":
    main()
