"""Display T2Retrieval benchmark results with comparison."""
import json, sys
from pathlib import Path

path = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("testdata/t2retrieval_10k_results.json")
data = json.loads(path.read_text(encoding="utf-8"))

o = data["overall"]
samples = data["samples"]
zero_hit = sum(1 for s in samples if s["firstRelevantRank"] == 0)
hit1 = sum(1 for s in samples if s["hitAtK"].get("1", False))
hit10 = sum(1 for s in samples if s["hitAtK"].get("10", False))

print(f"=== T2Retrieval 基准 ({len(samples)} queries x 10000 passages) ===")
print()
print(f"{'指标':<8} {'@1':<10} {'@3':<10} {'@5':<10} {'@10':<10}")
print(f"{'---':<8} {'---':<10} {'---':<10} {'---':<10} {'---':<10}")
print(f"{'HitRate':<8} {o['hitRateAtK']['1']:<10.4f} {o['hitRateAtK']['3']:<10.4f} {o['hitRateAtK']['5']:<10.4f} {o['hitRateAtK']['10']:<10.4f}")
print(f"{'Recall':<8} {o['averageRecallAtK']['1']:<10.4f} {o['averageRecallAtK']['3']:<10.4f} {o['averageRecallAtK']['5']:<10.4f} {o['averageRecallAtK']['10']:<10.4f}")
print(f"{'NDCG':<8} {o['averageNdcgAtK']['1']:<10.4f} {o['averageNdcgAtK']['3']:<10.4f} {o['averageNdcgAtK']['5']:<10.4f} {o['averageNdcgAtK']['10']:<10.4f}")
print(f"{'MRR':<8} {o['mrr']:<10.4f}")
print()
print(f"命中: Hit@1={hit1}/{len(samples)} ({hit1/len(samples)*100:.1f}%)  Hit@10={hit10}/{len(samples)}  ZeroHits={zero_hit}")

# Compare with 5K
print()
print("=== 5K vs 10K 对比 ===")
print(f"{'':<10} {'5K(200q)':<12} {'10K(222q)':<12} {'delta':<10}")
print(f"{'MRR':<10} {1.0:<12.4f} {o['mrr']:<12.4f} {'-':<10}")
print(f"{'Hit@1':<10} {1.0:<12.4f} {o['hitRateAtK']['1']:<12.4f} {'-':<10}")
rec10_delta = o['averageRecallAtK']['10'] - 0.8913
print(f"{'Rec@10':<10} {0.8913:<12.4f} {o['averageRecallAtK']['10']:<12.4f} {rec10_delta:+10.4f}")

# Worst queries
samples_sorted = sorted(samples, key=lambda s: s["firstRelevantRank"] if s["firstRelevantRank"] > 0 else 999)
print(f"\n最差查询:")
for s in samples_sorted[-5:]:
    rank = s["firstRelevantRank"] if s["firstRelevantRank"] > 0 else "miss"
    print(f"  rank={str(rank):>4}  rec@10={s['recallAtK']['10']:.2f}  {s['query'][:55]}")
