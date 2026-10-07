package evaluation

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	judgePayloadHeader = "Judge payload:\n"

	judgeSystemInstruction = "You are an offline evaluation judge.\n\nPrompt template:\n"

	judgeRubricHeader = "\n\nRubric:\n"

	judgeResultInstruction = "\n\nReturn strict JSON only with keys: passed, score, missed_items, incorrect_claims, reason, details.\n- passed must be boolean\n- score must be a number between 0 and 1\n- details may contain prompt-specific structured data\nDo not wrap the response in prose."

	summaryEquivalenceAnswerGuidance = "Answer based only on the provided session context. If the context is insufficient, say so clearly. Respond in Chinese."
)
