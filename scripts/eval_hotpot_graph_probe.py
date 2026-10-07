"""Evaluate paragraph-level evidence coverage on the local HotpotQA probe."""

import json
from pathlib import Path

from sklearn.feature_extraction.text import TfidfVectorizer


INPUT = Path("testdata/hotpot_graph_probe.json")


def main() -> None:
    probe = json.loads(INPUT.read_text(encoding="utf-8"))
    documents = probe["documents"]
    questions = probe["questions"]
    ids = [document["id"] for document in documents]
    vectorizer = TfidfVectorizer(lowercase=True, stop_words="english", ngram_range=(1, 2))
    document_vectors = vectorizer.fit_transform(document["text"] for document in documents)
    question_vectors = vectorizer.transform(question["question"] for question in questions)
    scores = (question_vectors @ document_vectors.T).toarray()

    for top_k in (3, 5, 10):
        full = 0
        any_hit = 0
        recall = 0.0
        for question, row in zip(questions, scores, strict=True):
            ranked = sorted(range(len(ids)), key=lambda index: (-row[index], ids[index]))
            found = set(ids[index] for index in ranked[:top_k])
            gold = set(question["expectedDocumentIds"])
            hits = len(found & gold)
            full += hits == len(gold)
            any_hit += hits > 0
            recall += hits / len(gold)
        print(
            f"TF-IDF global top{top_k}: full={full}/{len(questions)} "
            f"any={any_hit}/{len(questions)} mean_recall={recall / len(questions):.3f}"
        )


if __name__ == "__main__":
    main()
