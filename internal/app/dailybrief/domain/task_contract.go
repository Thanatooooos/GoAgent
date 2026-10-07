package domain

// TaskContract is saved in each immutable scheduled-task configuration.
// Sources are research preferences, not a pre-collected evidence list.
type TaskContract struct {
	Topics           []string `json:"topics"`
	Sources          []string `json:"sources"`
	MaxItems         int      `json:"maxItems"`
	MaxItemsPerTopic int      `json:"maxItemsPerTopic"`
}
