package runtime

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	taskJSONToolLoopInstruction = "The JSON result contract applies to your final answer only. First use the available tools as needed to execute the task; tool calls are not final answers. Do not claim a tool succeeded or failed without an actual tool result."

	taskJSONFinalInstruction = "Return the final result as the exact JSON object required by the original task. Use only the information and tool results already available above. Preserve uncertainty when evidence is insufficient; do not add facts or claim additional checks."

	taskToolUnavailablePrefix = "Tool unavailable: "

	taskToolFailedPrefix = "Tool failed: "

	currentDateContextPrefix = "Today's date: "

	localTimeContextTemplate = "Current date and time: %s (%s). Resolve every relative time such as \"tomorrow\" or \"in an hour\" against this clock."

	structuredSummaryRequest = "Return only the complete JSON summary."

	structuredSummarySystemPrompt = `You maintain a durable conversation summary. Output exactly one JSON object with these fields:
{"schema_version":1,"goal":"","active_priorities":[],"user_preferences":[],"constraints":[],"established_facts":[],"recent_progress":[],"open_questions":[],"background_issues":[]}

The previous summary and the new older conversation records are input. Return a new, complete, self-contained summary that replaces the previous summary. Newer records override conflicting older facts. Remove stale conclusions. Keep only information that can affect later conversation, decisions, or work; do not describe the merging process. Do not include tool calls or raw tool results.

Previous summary JSON:
`

	structuredSummaryRecordsPrefix = `

Conversation records to absorb:
`
)
