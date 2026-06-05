package runtime

import (
	"strconv"
	"strings"
	"time"

	agentstate "local/rag-project/internal/app/agent/state"
)

// ReplayView is the minimal inspection view for one runtime session.
// It is intentionally summary-oriented rather than a full event-sourced rebuild.
type ReplayView struct {
	SessionID                   string                 `json:"session_id,omitempty"`
	Question                    string                 `json:"question,omitempty"`
	RuntimeName                 string                 `json:"runtime_name,omitempty"`
	RuntimeVersion              string                 `json:"runtime_version,omitempty"`
	ResumedFrom                 string                 `json:"resumed_from,omitempty"`
	ResumeCount                 int                    `json:"resume_count,omitempty"`
	LastSequence                int                    `json:"last_sequence,omitempty"`
	EventCount                  int                    `json:"event_count,omitempty"`
	LastUpdatedAt               time.Time              `json:"last_updated_at"`
	Checkpoint                  *CheckpointRef         `json:"checkpoint,omitempty"`
	CheckpointState             ReplayCheckpointView   `json:"checkpoint_state"`
	EventTypeCounts             map[string]int         `json:"event_type_counts,omitempty"`
	Timeline                    []ReplayEventView      `json:"timeline,omitempty"`
	Nodes                       []ReplayNodeView       `json:"nodes,omitempty"`
	Capabilities                []ReplayCapabilityView `json:"capabilities,omitempty"`
	Branches                    []ReplayBranchView     `json:"branches,omitempty"`
	Decisions                   []ReplayDecisionView   `json:"decisions,omitempty"`
	LastDecision                *ReplayDecisionView    `json:"last_decision,omitempty"`
	FinalAnswer                 string                 `json:"final_answer,omitempty"`
	DegradeReason               string                 `json:"degrade_reason,omitempty"`
	EvidenceSufficient          bool                   `json:"evidence_sufficient,omitempty"`
	EvidenceReason              string                 `json:"evidence_reason,omitempty"`
	ContinueCount               int                    `json:"continue_count,omitempty"`
	LastBranchTarget            string                 `json:"last_branch_target,omitempty"`
	LastBranchReason            string                 `json:"last_branch_reason,omitempty"`
	LastProgressKind            string                 `json:"last_progress_kind,omitempty"`
	LastNewURLCount             int                    `json:"last_new_url_count,omitempty"`
	LastNewEvidenceCount        int                    `json:"last_new_evidence_count,omitempty"`
	ConsecutiveNoProgressRounds int                    `json:"consecutive_no_progress_rounds,omitempty"`
}

// ReplayCheckpointView summarizes checkpoint lifecycle state for inspection.
type ReplayCheckpointView struct {
	ID              string    `json:"id,omitempty"`
	Node            string    `json:"node,omitempty"`
	EventOffset     int       `json:"event_offset,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	Status          string    `json:"status,omitempty"`
	Active          bool      `json:"active,omitempty"`
	LastInterruptAt time.Time `json:"last_interrupt_at"`
	LastResumeAt    time.Time `json:"last_resume_at"`
	ResumedFrom     string    `json:"resumed_from,omitempty"`
	ResumeCount     int       `json:"resume_count,omitempty"`
}

// ReplayNodeView summarizes one observed node visit.
type ReplayNodeView struct {
	Node       string    `json:"node,omitempty"`
	Status     string    `json:"status,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Detail     string    `json:"detail,omitempty"`
}

// ReplayDecisionView summarizes one decision emitted during execution.
type ReplayDecisionView struct {
	Node       string    `json:"node,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	Target     string    `json:"target,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`
	Reasoning  string    `json:"reasoning,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// ReplayCapabilityView summarizes one capability attempt observed in the event log.
type ReplayCapabilityView struct {
	Node          string    `json:"node,omitempty"`
	Status        string    `json:"status,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	InputSummary  string    `json:"input_summary,omitempty"`
	OutputSummary string    `json:"output_summary,omitempty"`
}

// ReplayBranchView summarizes one branch selection emitted by the runtime.
type ReplayBranchView struct {
	Node      string    `json:"node,omitempty"`
	Target    string    `json:"target,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// ReplayEventView is the event-by-event inspection record for a session journal.
type ReplayEventView struct {
	Sequence     int       `json:"sequence,omitempty"`
	Node         string    `json:"node,omitempty"`
	EventType    string    `json:"event_type,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	Summary      string    `json:"summary,omitempty"`
	CheckpointID string    `json:"checkpoint_id,omitempty"`
}

// BuildReplayView projects a runtime session into a compact inspection view.
func BuildReplayView(session *RuntimeSession) ReplayView {
	if session == nil {
		return ReplayView{}
	}

	view := ReplayView{
		SessionID:                   session.SessionID,
		Question:                    firstNonEmpty(session.Request.Question, session.Snapshot.Request.Question),
		RuntimeName:                 session.Metadata.RuntimeName,
		RuntimeVersion:              session.Metadata.RuntimeVersion,
		ResumedFrom:                 session.Metadata.ResumedFrom,
		ResumeCount:                 session.Metadata.ResumeCount,
		EventCount:                  len(session.Journal),
		LastUpdatedAt:               session.Metadata.UpdatedAt,
		Checkpoint:                  cloneCheckpointRef(session.Checkpoint),
		CheckpointState:             buildCheckpointView(session),
		EventTypeCounts:             make(map[string]int),
		FinalAnswer:                 session.Snapshot.Answer.Final,
		DegradeReason:               session.Snapshot.Answer.DegradeReason,
		EvidenceSufficient:          session.Snapshot.Evidence.Sufficient,
		EvidenceReason:              session.Snapshot.Evidence.SufficiencyReason,
		ContinueCount:               session.Snapshot.Execution.ContinueCount,
		LastBranchTarget:            session.Snapshot.Execution.LastBranchTarget,
		LastBranchReason:            session.Snapshot.Execution.LastBranchReason,
		LastProgressKind:            session.Snapshot.Execution.LastProgressKind,
		LastNewURLCount:             session.Snapshot.Execution.LastNewURLCount,
		LastNewEvidenceCount:        session.Snapshot.Execution.LastNewEvidenceCount,
		ConsecutiveNoProgressRounds: session.Snapshot.Execution.ConsecutiveNoProgressRounds,
	}

	for _, event := range session.Journal {
		view.LastSequence = event.Sequence
		if eventType := strings.TrimSpace(event.EventType); eventType != "" {
			view.EventTypeCounts[eventType]++
		}
		view.Timeline = append(view.Timeline, ReplayEventView{
			Sequence:     event.Sequence,
			Node:         event.Node,
			EventType:    event.EventType,
			Timestamp:    event.Timestamp,
			Summary:      eventSummary(event),
			CheckpointID: checkpointIDFromEvent(event),
		})
		switch strings.TrimSpace(event.EventType) {
		case agentstate.EventTypeNodeStart:
			view.Nodes = append(view.Nodes, ReplayNodeView{
				Node:      event.Node,
				Status:    "running",
				StartedAt: event.Timestamp,
			})
		case agentstate.EventTypeNodeFinish:
			if idx := findLastRunningNode(view.Nodes, event.Node); idx >= 0 {
				view.Nodes[idx].Status = "completed"
				view.Nodes[idx].FinishedAt = event.Timestamp
			} else {
				view.Nodes = append(view.Nodes, ReplayNodeView{
					Node:       event.Node,
					Status:     "completed",
					FinishedAt: event.Timestamp,
				})
			}
		case agentstate.EventTypeNodeError:
			if idx := findLastRunningNode(view.Nodes, event.Node); idx >= 0 {
				view.Nodes[idx].Status = "error"
				view.Nodes[idx].FinishedAt = event.Timestamp
				view.Nodes[idx].Detail = event.PayloadText
			} else {
				view.Nodes = append(view.Nodes, ReplayNodeView{
					Node:       event.Node,
					Status:     "error",
					FinishedAt: event.Timestamp,
					Detail:     event.PayloadText,
				})
			}
		case agentstate.EventTypeInterrupt:
			if idx := findLastRunningNode(view.Nodes, event.Node); idx >= 0 {
				view.Nodes[idx].Status = "interrupted"
				view.Nodes[idx].FinishedAt = event.Timestamp
				view.Nodes[idx].Detail = event.PayloadText
			} else {
				view.Nodes = append(view.Nodes, ReplayNodeView{
					Node:       event.Node,
					Status:     "interrupted",
					StartedAt:  event.Timestamp,
					FinishedAt: event.Timestamp,
					Detail:     event.PayloadText,
				})
			}
		case agentstate.EventTypeDecisionEmitted:
			decision := parseReplayDecision(event)
			view.Decisions = append(view.Decisions, decision)
			view.LastDecision = cloneReplayDecision(decision)
		case agentstate.EventTypeCapabilityStart:
			view.Capabilities = append(view.Capabilities, ReplayCapabilityView{
				Node:         event.Node,
				Status:       "running",
				StartedAt:    event.Timestamp,
				InputSummary: strings.TrimSpace(event.PayloadText),
			})
		case agentstate.EventTypeCapabilityResult:
			if idx := findLastRunningCapability(view.Capabilities, event.Node); idx >= 0 {
				view.Capabilities[idx].Status = "completed"
				view.Capabilities[idx].FinishedAt = event.Timestamp
				view.Capabilities[idx].OutputSummary = strings.TrimSpace(event.PayloadText)
			} else {
				view.Capabilities = append(view.Capabilities, ReplayCapabilityView{
					Node:          event.Node,
					Status:        "completed",
					FinishedAt:    event.Timestamp,
					OutputSummary: strings.TrimSpace(event.PayloadText),
				})
			}
		case agentstate.EventTypeCapabilitySkipped:
			view.Capabilities = append(view.Capabilities, ReplayCapabilityView{
				Node:          event.Node,
				Status:        "skipped",
				StartedAt:     event.Timestamp,
				FinishedAt:    event.Timestamp,
				OutputSummary: strings.TrimSpace(event.PayloadText),
			})
		case agentstate.EventTypeBranchSelected:
			view.Branches = append(view.Branches, ReplayBranchView{
				Node:      event.Node,
				Target:    strings.TrimSpace(event.PayloadText),
				Summary:   branchSummary(event),
				Timestamp: event.Timestamp,
			})
		}
	}

	if len(view.EventTypeCounts) == 0 {
		view.EventTypeCounts = nil
	}

	return view
}

func findLastRunningNode(nodes []ReplayNodeView, node string) int {
	for i := len(nodes) - 1; i >= 0; i-- {
		if nodes[i].Node == node && nodes[i].Status == "running" {
			return i
		}
	}
	return -1
}

func findLastRunningCapability(capabilities []ReplayCapabilityView, node string) int {
	for i := len(capabilities) - 1; i >= 0; i-- {
		if capabilities[i].Node == node && capabilities[i].Status == "running" {
			return i
		}
	}
	return -1
}

func parseReplayDecision(event agentstate.RuntimeEvent) ReplayDecisionView {
	if event.Decision != nil {
		return ReplayDecisionView{
			Node:       event.Node,
			Kind:       event.Decision.Kind,
			Target:     event.Decision.Target,
			Confidence: event.Decision.Confidence,
			Reasoning:  event.Decision.Reasoning,
			Summary:    event.PayloadText,
			Timestamp:  event.Timestamp,
		}
	}

	payload := strings.TrimSpace(event.PayloadText)
	decision := ReplayDecisionView{
		Node:      event.Node,
		Summary:   payload,
		Timestamp: event.Timestamp,
	}
	if payload == "" {
		return decision
	}

	head := payload
	reasoning := ""
	if before, after, found := strings.Cut(payload, " reasoning="); found {
		head = before
		reasoning = strings.TrimSpace(after)
	}

	for _, token := range strings.Fields(head) {
		key, value, found := strings.Cut(token, "=")
		if !found {
			continue
		}
		switch key {
		case "kind":
			decision.Kind = value
		case "target":
			decision.Target = value
		case "confidence":
			if confidence, err := strconv.ParseFloat(value, 64); err == nil {
				decision.Confidence = confidence
			}
		}
	}
	decision.Reasoning = reasoning
	return decision
}

func eventSummary(event agentstate.RuntimeEvent) string {
	if event.Decision != nil {
		return firstNonEmpty(event.PayloadText, event.Decision.Kind+"->"+event.Decision.Target)
	}
	if event.Checkpoint != nil && event.Checkpoint.ID != "" {
		return firstNonEmpty(event.PayloadText, "checkpoint_id="+event.Checkpoint.ID)
	}
	if event.EventType == agentstate.EventTypeStateApplied && event.Delta != nil {
		return summarizeDelta(*event.Delta)
	}
	if event.EventType == agentstate.EventTypeBranchSelected {
		return branchSummary(event)
	}
	return strings.TrimSpace(event.PayloadText)
}

func checkpointIDFromEvent(event agentstate.RuntimeEvent) string {
	if event.Checkpoint == nil {
		return ""
	}
	return strings.TrimSpace(event.Checkpoint.ID)
}

func cloneCheckpointRef(ref *CheckpointRef) *CheckpointRef {
	if ref == nil {
		return nil
	}
	cloned := *ref
	return &cloned
}

func cloneReplayDecision(decision ReplayDecisionView) *ReplayDecisionView {
	cloned := decision
	return &cloned
}

func buildCheckpointView(session *RuntimeSession) ReplayCheckpointView {
	view := ReplayCheckpointView{
		ResumedFrom: session.Metadata.ResumedFrom,
		ResumeCount: session.Metadata.ResumeCount,
	}
	if session.Checkpoint != nil {
		view.ID = session.Checkpoint.ID
		view.Node = session.Checkpoint.Node
		view.EventOffset = session.Checkpoint.EventOffset
		view.CreatedAt = session.Checkpoint.CreatedAt
		view.Status = "recorded"
	}

	for _, event := range session.Journal {
		switch event.EventType {
		case agentstate.EventTypeInterrupt:
			view.Active = true
			view.Status = "interrupted"
			view.LastInterruptAt = event.Timestamp
			if view.ID == "" {
				view.ID = checkpointIDFromEvent(event)
			}
			if view.Node == "" {
				view.Node = strings.TrimSpace(event.Node)
			}
			if view.EventOffset == 0 {
				view.EventOffset = event.Sequence
			}
		case agentstate.EventTypeResumeCompleted:
			view.Active = false
			view.Status = "resumed"
			view.LastResumeAt = event.Timestamp
			if view.ID == "" {
				view.ID = checkpointIDFromEvent(event)
			}
			if view.Node == "" {
				view.Node = strings.TrimSpace(event.Node)
			}
		}
	}
	return view
}

func summarizeDelta(delta agentstate.StateDelta) string {
	parts := make([]string, 0, 8)
	if delta.Request != nil {
		if delta.Request.ConversationID != nil {
			parts = append(parts, "request.conversation_id")
		}
		if len(delta.Request.KnowledgeBaseIDs) > 0 {
			parts = append(parts, "request.knowledge_bases")
		}
		if delta.Request.RuntimeOptions != nil {
			parts = append(parts, "request.runtime_options")
		}
	}
	if delta.Context != nil {
		if delta.Context.RewrittenQuery != nil {
			parts = append(parts, "context.rewritten_query")
		}
		if delta.Context.SearchQuery != nil {
			parts = append(parts, "context.search_query")
		}
		if delta.Context.SearchProvider != nil || delta.Context.SearchProviderActual != nil {
			parts = append(parts, "context.search_provider")
		}
		if delta.Context.SearchErrorClass != nil {
			parts = append(parts, "context.search_error_class")
		}
		if delta.Context.FetchErrorClass != nil {
			parts = append(parts, "context.fetch_error_class")
		}
		if len(delta.Context.SearchResults) > 0 {
			parts = append(parts, "context.search_results")
		}
		if len(delta.Context.FetchResults) > 0 {
			parts = append(parts, "context.fetch_results")
		}
		if delta.Context.PreferredURLs != nil {
			parts = append(parts, "context.preferred_urls")
		}
		if delta.Context.AvoidURLs != nil {
			parts = append(parts, "context.avoid_urls")
		}
		if len(delta.Context.SeenURLs) > 0 {
			parts = append(parts, "context.seen_urls")
		}
		if len(delta.Context.MemoryRefs) > 0 {
			parts = append(parts, "context.memory_refs")
		}
		if len(delta.Context.Notes) > 0 {
			parts = append(parts, "context.notes")
		}
	}
	if delta.Evidence != nil {
		if len(delta.Evidence.AddItems) > 0 {
			parts = append(parts, "evidence.items")
		}
		if delta.Evidence.Sufficient != nil || delta.Evidence.SufficiencyReason != nil {
			parts = append(parts, "evidence.sufficiency")
		}
		if delta.Evidence.NewItemsThisRound != nil {
			parts = append(parts, "evidence.new_items_this_round")
		}
		if len(delta.Evidence.OpenQuestions) > 0 {
			parts = append(parts, "evidence.open_questions")
		}
	}
	if delta.Approval != nil {
		if delta.Approval.Status != nil || delta.Approval.Reason != nil || delta.Approval.Node != nil ||
			delta.Approval.Capability != nil || delta.Approval.CheckpointID != nil || delta.Approval.RerunNode != nil ||
			delta.Approval.RequestedAt != nil || delta.Approval.ReviewedAt != nil || delta.Approval.DecisionNote != nil {
			parts = append(parts, "approval.state")
		}
	}
	if delta.Execution != nil {
		if delta.Execution.CurrentNode != nil {
			parts = append(parts, "execution.current_node")
		}
		if delta.Execution.IterationIncrement != 0 {
			parts = append(parts, "execution.iteration")
		}
		if delta.Execution.ContinueCountIncrement != 0 {
			parts = append(parts, "execution.continue_count")
		}
		if delta.Execution.LastBranchTarget != nil || delta.Execution.LastBranchReason != nil {
			parts = append(parts, "execution.branch")
		}
		if delta.Execution.LastProgressKind != nil {
			parts = append(parts, "execution.progress_kind")
		}
		if delta.Execution.LastNewURLCount != nil || delta.Execution.LastNewEvidenceCount != nil {
			parts = append(parts, "execution.progress")
		}
		if delta.Execution.ConsecutiveNoProgressRounds != nil {
			parts = append(parts, "execution.no_progress_rounds")
		}
		if len(delta.Execution.ScheduledActions) > 0 {
			parts = append(parts, "execution.scheduled_actions")
		}
		if len(delta.Execution.CompletedActions) > 0 {
			parts = append(parts, "execution.completed_actions")
		}
		if len(delta.Execution.FailedActions) > 0 {
			parts = append(parts, "execution.failed_actions")
		}
		if delta.Execution.Interrupted != nil || delta.Execution.InterruptReason != nil {
			parts = append(parts, "execution.interrupt")
		}
	}
	if delta.Answer != nil {
		if delta.Answer.Draft != nil {
			parts = append(parts, "answer.draft")
		}
		if delta.Answer.DegradeReason != nil {
			parts = append(parts, "answer.degrade_reason")
		}
		if delta.Answer.Final != nil {
			parts = append(parts, "answer.final")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "applied " + strings.Join(parts, ", ")
}

func branchSummary(event agentstate.RuntimeEvent) string {
	target := strings.TrimSpace(event.PayloadText)
	if target == "" {
		return ""
	}
	return "selected " + target
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
