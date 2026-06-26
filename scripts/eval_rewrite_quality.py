"""Evaluate rewrite quality against reference rewrites from dialogue dataset."""

import json
import os
import sys
import time
from openai import OpenAI

# --- Config ---
INPUT = "testdata/evals/rewrite/rewrite_500.json"
OUTPUT = "testdata/evals/rewrite/rewrite_quality_result.json"
MAX_SAMPLES = 500
BATCH_DELAY = 0.3  # seconds between API calls

# --- Load API key from .env ---
def load_env(path=".env"):
    env = {}
    if os.path.exists(path):
        with open(path) as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    k, v = line.split("=", 1)
                    env[k] = v
    return env

env = load_env()
api_key = env.get("AI_PROVIDERS_BAILIAN_API_KEY", "")

if not api_key:
    print("ERROR: AI_PROVIDERS_BAILIAN_API_KEY not found in .env")
    sys.exit(1)

client = OpenAI(
    api_key=api_key,
    base_url="https://dashscope.aliyuncs.com/compatible-mode/v1",
)

# --- Prompt ---
SYSTEM_PROMPT = """你是一个查询改写助手。将对话中最后一句改写为可独立检索的查询。

规则:
1. 将指代词替换为具体实体，让问题脱离上下文也能理解。
2. 去掉口语赘述，保留核心语义。
3. 保留原文中的技术术语、英文缩写、专有名词。
4. 只输出改写后的问题文本，不要 JSON，不要解释。"""

def build_user_prompt(dialogue):
    """Build the user prompt from dialogue history."""
    # The last turn is the query to rewrite, previous turns are context
    context = "\n".join(d.strip() for d in dialogue[:-1] if d.strip())
    query = dialogue[-1].strip() if dialogue else ""
    return f"对话历史:\n{context}\n\n需要改写的句子: {query}\n\n改写结果:"

def call_rewrite(dialogue):
    """Call LLM to rewrite the last utterance."""
    user_prompt = build_user_prompt(dialogue)
    try:
        resp = client.chat.completions.create(
            model="qwen-max",
            messages=[
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": user_prompt},
            ],
            temperature=0.1,
            max_tokens=200,
        )
        return resp.choices[0].message.content.strip()
    except Exception as e:
        return f"ERROR: {e}"

# --- Simple text metrics (no external libs needed) ---
def char_ngrams(text, n):
    return set(text[i:i+n] for i in range(len(text)-n+1))

def bleu_like(ref, hyp, n=2):
    """Character n-gram precision (BLEU-like without external libs)."""
    if not hyp or not ref:
        return 0.0
    scores = []
    for i in range(1, n+1):
        ref_ngrams = char_ngrams(ref, i)
        hyp_ngrams = char_ngrams(hyp, i)
        if not hyp_ngrams:
            scores.append(0.0)
            continue
        overlap = len(ref_ngrams & hyp_ngrams)
        scores.append(overlap / len(hyp_ngrams) if hyp_ngrams else 0.0)
    return sum(scores) / len(scores) if scores else 0.0

def rouge_l(ref, hyp):
    """Longest common subsequence based ROUGE-L (character-level)."""
    if not hyp or not ref:
        return 0.0, 0.0, 0.0
    m, n = len(ref), len(hyp)
    dp = [[0]*(n+1) for _ in range(m+1)]
    for i in range(1, m+1):
        for j in range(1, n+1):
            if ref[i-1] == hyp[j-1]:
                dp[i][j] = dp[i-1][j-1] + 1
            else:
                dp[i][j] = max(dp[i-1][j], dp[i][j-1])
    lcs = dp[m][n]
    prec = lcs / n if n > 0 else 0
    rec = lcs / m if m > 0 else 0
    f1 = 2 * prec * rec / (prec + rec) if (prec + rec) > 0 else 0
    return prec, rec, f1

def edit_distance(s1, s2):
    """Levenshtein distance."""
    if len(s1) < len(s2):
        return edit_distance(s2, s1)
    if len(s2) == 0:
        return len(s1)
    prev = list(range(len(s2)+1))
    for i, c1 in enumerate(s1):
        curr = [i+1]
        for j, c2 in enumerate(s2):
            curr.append(min(
                prev[j+1]+1,      # delete
                curr[j]+1,         # insert
                prev[j]+(c1!=c2)   # replace
            ))
        prev = curr
    return prev[-1]

# --- Main ---
def main():
    with open(INPUT, encoding="utf-8") as f:
        data = json.load(f)

    samples = data[:MAX_SAMPLES]
    print(f"Evaluating {len(samples)} samples...")

    results = []
    bleu_scores = []
    rouge_f1_scores = []
    char_ratios = []
    edit_dists = []

    for i, s in enumerate(samples):
        dialogue = s["dialogue"]
        ref = s["rewrite"]
        # Remove speaker prefix from last utterance if present
        last = dialogue[-1].strip() if dialogue else ""

        our_rewrite = call_rewrite(dialogue)

        # Compute metrics
        bleu = bleu_like(ref, our_rewrite)
        _, _, rouge_f1 = rouge_l(ref, our_rewrite)
        ed = edit_distance(ref, our_rewrite)
        ratio = len(our_rewrite) / max(len(ref), 1)

        bleu_scores.append(bleu)
        rouge_f1_scores.append(rouge_f1)
        char_ratios.append(ratio)
        edit_dists.append(ed)

        results.append({
            "dialogue": dialogue,
            "reference": ref,
            "our_rewrite": our_rewrite,
            "bleu_char2": round(bleu, 4),
            "rouge_l_f1": round(rouge_f1, 4),
            "edit_dist": ed,
            "len_ratio": round(ratio, 3),
        })

        if (i+1) % 50 == 0:
            avg_bleu = sum(bleu_scores)/len(bleu_scores)
            avg_rouge = sum(rouge_f1_scores)/len(rouge_f1_scores)
            print(f"  {i+1}/{len(samples)}  BLEU={avg_bleu:.4f}  ROUGE-L={avg_rouge:.4f}")

        time.sleep(BATCH_DELAY)

    # --- Summary ---
    n = len(bleu_scores)
    avg_bleu = sum(bleu_scores)/n
    avg_rouge = sum(rouge_f1_scores)/n
    avg_ratio = sum(char_ratios)/n
    avg_edit = sum(edit_dists)/n

    print(f"\n=== Rewrite Quality Summary ({n} samples) ===")
    print(f"  BLEU-char2 avg:  {avg_bleu:.4f}")
    print(f"  ROUGE-L F1 avg:  {avg_rouge:.4f}")
    print(f"  Length ratio avg: {avg_ratio:.3f} (our/ref)")
    print(f"  Avg edit distance: {avg_edit:.1f}")

    # Save
    with open(OUTPUT, "w", encoding="utf-8") as f:
        json.dump({
            "summary": {
                "samples": n,
                "bleu_char2_avg": round(avg_bleu, 4),
                "rouge_l_f1_avg": round(avg_rouge, 4),
                "length_ratio_avg": round(avg_ratio, 3),
                "avg_edit_distance": round(avg_edit, 1),
            },
            "results": results,
        }, f, ensure_ascii=False, indent=2)
    print(f"\nSaved to {OUTPUT}")

if __name__ == "__main__":
    main()
