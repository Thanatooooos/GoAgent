package capability

import (
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/runtime/persistence"
)

func ArchiveConversationEpisode(service *EpisodeService) Def {
	return Def{ID: ArchiveConversationEpisodeID, Description: archiveConversationEpisodeDescription, JSONSchema: json.RawMessage(archiveConversationEpisodeSchema),
		Validate: func(value Value) error { _, err := parseArchiveArgs(value); return err },
		Describe: func(value Value, ctx Context) (Operation, error) {
			if !ctx.AllowEpisodeArchive {
				return Operation{}, Deny("a conversation episode has already been archived for this user message")
			}
			return Operation{ID: ArchiveConversationEpisodeID, Summary: "archive conversation episode", Input: value}, nil
		},
		Execute: func(value Value, ctx Context) (Result, error) {
			args, err := parseArchiveArgs(value)
			if err != nil {
				return Result{}, err
			}
			episode, err := service.Archive(ctx, args.Summary, args.Topics, args.Importance)
			if err != nil {
				return Result{}, err
			}
			body, _ := json.Marshal(struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}{episode.ID, episode.Status})
			return Result{Content: "conversation episode archived", Value: body, Evidence: []persistence.EvidenceRef{{ID: episode.ID, Kind: "conversation_episode"}}}, nil
		},
	}
}

func SearchConversationHistory(service *EpisodeService) Def {
	return Def{ID: SearchConversationHistoryID, Description: searchConversationHistoryDescription, JSONSchema: json.RawMessage(searchConversationHistorySchema),
		Validate: func(value Value) error { _, err := parseSearchArgs(value); return err },
		Describe: func(value Value, _ Context) (Operation, error) {
			return Operation{ID: SearchConversationHistoryID, Summary: "search conversation history", Input: value}, nil
		},
		Execute: func(value Value, ctx Context) (Result, error) {
			args, err := parseSearchArgs(value)
			if err != nil {
				return Result{}, err
			}
			hits, ambiguous, err := service.Search(ctx, args.Query, args.ConversationID, args.TopK)
			if err != nil {
				return Result{}, err
			}
			body, err := json.Marshal(struct {
				Ambiguous bool                     `json:"ambiguous"`
				Episodes  []persistence.EpisodeHit `json:"episodes"`
			}{ambiguous, hits})
			if err != nil {
				return Result{}, err
			}
			return Result{Content: string(body), Value: body}, nil
		},
	}
}

type archiveArgs struct {
	Summary    string   `json:"summary"`
	Topics     []string `json:"topics"`
	Importance string   `json:"importance"`
}

func parseArchiveArgs(value Value) (archiveArgs, error) {
	var args archiveArgs
	if err := json.Unmarshal(value, &args); err != nil {
		return args, fmt.Errorf("invalid arguments: %w", err)
	}
	args.Summary = strings.TrimSpace(args.Summary)
	args.Importance = strings.TrimSpace(args.Importance)
	args.Topics = normalizeTopics(args.Topics)
	if args.Summary == "" || len(args.Topics) == 0 || len(args.Topics) > 5 {
		return args, fmt.Errorf("summary and one to five topics are required")
	}
	if args.Importance != "low" && args.Importance != "normal" && args.Importance != "high" {
		return args, fmt.Errorf("importance must be low, normal, or high")
	}
	return args, nil
}
func normalizeTopics(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if len([]rune(value)) > 32 || value == "" {
			continue
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

type searchArgs struct {
	Query          string `json:"query"`
	TopK           int    `json:"top_k"`
	ConversationID string `json:"conversation_id"`
}

func parseSearchArgs(value Value) (searchArgs, error) {
	var args searchArgs
	if err := json.Unmarshal(value, &args); err != nil {
		return args, fmt.Errorf("invalid arguments: %w", err)
	}
	args.Query = strings.TrimSpace(args.Query)
	args.ConversationID = strings.TrimSpace(args.ConversationID)
	if args.Query == "" {
		return args, fmt.Errorf("query is required")
	}
	if args.TopK == 0 {
		args.TopK = 3
	}
	if args.TopK < 1 || args.TopK > 5 {
		return args, fmt.Errorf("top_k must be between 1 and 5")
	}
	return args, nil
}
