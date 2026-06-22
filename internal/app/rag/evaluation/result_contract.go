package evaluation

type RunMetadata struct {
	RunAt            string         `json:"run_at"`
	Suite            string         `json:"suite"`
	EvaluatorVersion string         `json:"evaluator_version,omitempty"`
	SampleSetID      string         `json:"sample_set_id,omitempty"`
	ModelConfig      map[string]any `json:"model_config,omitempty"`
}

type SharedSampleResult struct {
	Name             string         `json:"name"`
	Tags             []string       `json:"tags,omitempty"`
	Passed           bool           `json:"passed"`
	CriticalFailures []string       `json:"critical_failures,omitempty"`
	RuleChecks       map[string]any `json:"rule_checks,omitempty"`
	JudgeChecks      map[string]any `json:"judge_checks,omitempty"`
	Scores           map[string]any `json:"scores,omitempty"`
	FailureReasons   []string       `json:"failure_reasons,omitempty"`
}

type TagAggregate struct {
	Tag     string         `json:"tag"`
	Metrics map[string]any `json:"metrics,omitempty"`
}

type SharedAggregateResult struct {
	PassRate            float64        `json:"pass_rate"`
	CriticalFailureRate float64        `json:"critical_failure_rate"`
	ByTag               []TagAggregate `json:"by_tag,omitempty"`
	Metrics             map[string]any `json:"metrics,omitempty"`
}

type SuiteResult struct {
	Suite       string                `json:"suite"`
	RunMetadata RunMetadata           `json:"run_metadata"`
	Samples     []SharedSampleResult  `json:"samples"`
	Aggregate   SharedAggregateResult `json:"aggregate"`
	Artifacts   map[string]any        `json:"artifacts,omitempty"`
}
