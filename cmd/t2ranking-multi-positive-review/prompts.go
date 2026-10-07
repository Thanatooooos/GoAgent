package main

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	relevanceReviewSystemPrompt = "You are a careful Chinese retrieval relevance annotator. Follow the user format exactly."

	singlePassageReviewInstruction = "Label this single Chinese passage for the question. strong_positive: independently and correctly answers it. weak_positive: useful but not safe or sufficient alone. negative: insufficient, wrong, misleading, or unrelated. Return ONLY JSON {\\\"label\\\":\\\"strong_positive|weak_positive|negative\\\",\\\"answerPoints\\\":[...],\\\"rationale\\\":\\\"...\\\"}.\\nQuestion: "

	singlePassageContextHeader = "\\nPassage:\\n"

	candidateReviewInstruction = "Task: label every candidate passage for the Chinese question below. strong_positive: independently and correctly answers the question. weak_positive: provides a necessary/meaningful fact but cannot safely answer alone. negative: related wording alone is insufficient, wrong, misleading, or unrelated. Treat originalRole as provenance only, not truth. Return ONLY JSON: {\\\"labels\\\":[{\\\"passageId\\\":\\\"...\\\",\\\"label\\\":\\\"strong_positive|weak_positive|negative\\\",\\\"answerPoints\\\":[...],\\\"rationale\\\":...}]}. You MUST return exactly 10 labels, one for every listed passage ID, including irrelevant passages.\nQuestion: "

	candidatePassageTemplate = "\n[passageId=%s]\n%s\n"

	requiredPassageIDsHeader = "\nREQUIRED passageId list (return each exactly once): "
)
