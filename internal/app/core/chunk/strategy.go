package chunk

type Strategy string

const (
	StrategyFixedSize Strategy = "fixed_size"
	StrategyMarkdown  Strategy = "markdown"
)
