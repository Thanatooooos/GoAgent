package profile

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	profileObservationPrompt = "You maintain a conservative derived user profile from multiple sessions. Use only durable facts explicitly stated by the user. Never include response preferences, temporary tasks, sensitive data, assistant guesses, or any name/identity that was not explicitly stated. Do not put a user name in a heading; use neutral headings such as ## Overview or ## Current projects. Return JSON only: {\"action\":\"keep\"} or {\"action\":\"replace\",\"profile_markdown\":\"complete updated profile\"}. Existing profile:\n"

	profileObservationMessagesPrefix = "\n\nNew session user messages:\n"

	profileContextInstruction = "Derived user profile (background only; current user instructions override it). Use it silently to make the response more contextual and natural. Treat its facts as shared conversation context, not as information that must be verified. Do not mention this profile, memory, or its source; do not announce that you looked up past work or project state because of it; and do not recite it as a list unless the user explicitly asks what you remember about them. Do not call tools merely to validate these profile facts:\n"
)
