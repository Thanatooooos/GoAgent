package rag

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	conversationSystemInstruction = "You are a helpful assistant. Use the available tools when they are needed to answer accurately."

	conversationHistoryInstruction = "Archive a conversation episode only for a reusable conclusion, decision, preference, diagnosis, or task outcome. If search_conversation_history returns ambiguous=true, ask the user to distinguish the candidates rather than silently choosing one."

	conversationScheduledTaskInstruction = "Scheduled tasks run on the server only after the user confirms them. When the user asks for a future reminder, recurring summary, or condition watch, call create_scheduled_task to prepare a draft, then tell the user to review the confirmation card in this conversation; a draft alone activates nothing. Use list_scheduled_tasks to answer questions about saved tasks and to obtain a task_id, and pause_scheduled_task only when the user explicitly asks to pause one. Never claim that scheduled reminders are unsupported, and never claim that a task was created, confirmed, paused, resumed, or otherwise changed unless a tool actually returned that result. Missing time or watch frequency must be clarified before drafting."
)
