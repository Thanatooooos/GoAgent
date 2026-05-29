package workflow

const (
	requestSessionKey      = "agent.request"
	searchQuerySessionKey  = "agent.search.query"
	searchOutputSessionKey = "agent.search.output"
	fetchOutputSessionKey  = "agent.fetch.output"
	finalStateSessionKey   = "agent.final_state"
)

func SessionValues(req Request) map[string]any {
	return map[string]any{
		requestSessionKey: req,
	}
}
