package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"local/rag-project/internal/app/scheduledtask/domain"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type Proposal struct {
	Name              string               `json:"name,omitempty"`
	Status            string               `json:"status"` // none, clarify, draft, or manage
	Question          string               `json:"question,omitempty"`
	Action            string               `json:"action,omitempty"`
	TaskID            string               `json:"taskId,omitempty"`
	Prompt            string               `json:"prompt,omitempty"`
	Schedule          domain.Schedule      `json:"schedule,omitempty"`
	ReportMode        domain.ReportMode    `json:"reportMode,omitempty"`
	ConditionKind     domain.ConditionKind `json:"conditionKind,omitempty"`
	AllowedWebDomains []string             `json:"allowedWebDomains,omitempty"`
	AllowedToolIDs    []string             `json:"allowedToolIds,omitempty"`
	KnowledgeBaseIDs  []string             `json:"knowledgeBaseIds,omitempty"`
}

type Proposer struct{ Model aichat.LLMService }

type TaskSummary struct {
	ID       string            `json:"id"`
	Status   domain.TaskStatus `json:"status"`
	Prompt   string            `json:"prompt"`
	Schedule domain.Schedule   `json:"schedule"`
	Config   domain.Version    `json:"config"`
}

// ReviewConfig analyzes a user-edited configuration without activating it.
// The complete reviewed configuration must still be previewed and confirmed.
func (p Proposer) ReviewConfig(ctx context.Context, config domain.Version, now time.Time) (Proposal, error) {
	config.TaskID, config.Number, config.ConfirmedAt = "preview", 1, now
	if err := config.Validate(); err != nil {
		return Proposal{}, err
	}
	proposal, err := p.propose(ctx,
		scheduledTaskConfigReviewRequest,
		config.Schedule.Timezone, now, []TaskSummary{{ID: "preview", Status: domain.TaskActive, Prompt: config.Prompt, Schedule: config.Schedule, Config: config}})
	if err != nil {
		return Proposal{}, err
	}
	if proposal.Status == "clarify" {
		return Proposal{}, fmt.Errorf("请完善配置：%s", proposal.Question)
	}
	if proposal.Status != "draft" || proposal.TaskID != "preview" {
		return Proposal{}, fmt.Errorf("configuration review did not return a complete draft")
	}
	if strings.TrimSpace(config.Name) != "" {
		proposal.Name = strings.TrimSpace(config.Name)
	}
	return proposal, nil
}

func (p Proposer) propose(ctx context.Context, utterance, timezone string, now time.Time, tasks []TaskSummary) (Proposal, error) {
	if p.Model == nil || strings.TrimSpace(utterance) == "" || now.IsZero() {
		return Proposal{}, fmt.Errorf("proposal requires model, message, and time")
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return Proposal{}, fmt.Errorf("invalid browser timezone: %w", err)
	}
	jsonMode := true
	tools := false
	temperature := 0.0
	maxTokens := 2048
	taskList, err := json.Marshal(tasks)
	if err != nil {
		return Proposal{}, err
	}
	request := convention.ChatRequest{JSONMode: &jsonMode, EnableTools: &tools, Temperature: &temperature, Thinking: &tools, MaxTokens: &maxTokens,
		Messages: []convention.ChatMessage{
			convention.SystemMessage(scheduledTaskProposalSystemPrompt),
			convention.UserMessage(fmt.Sprintf(scheduledTaskProposalInputTemplate, now.In(loc).Format(time.RFC3339), timezone, string(taskList), utterance)),
		}}
	// The review contract is appended to every invocation because reviewing an
	// explicitly submitted configuration is now the only caller. It overrides
	// the future-intent detection rules the base prompt still carries.
	request.Messages[0].Content += scheduledTaskConfigReviewSystemPrompt
	var raw string
	if aware, ok := p.Model.(aichat.ContextAwareLLMService); ok {
		raw, err = aware.ChatWithRequestContext(ctx, request)
	} else {
		raw, err = p.Model.ChatWithRequest(request)
	}
	if err != nil {
		return Proposal{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var proposal Proposal
	if err := decoder.Decode(&proposal); err != nil {
		return Proposal{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Proposal{}, fmt.Errorf("proposal has trailing data")
	}
	switch proposal.Status {
	case "none":
		return Proposal{Status: "none"}, nil
	case "manage":
		if proposal.Action == "open_manager" && proposal.TaskID == "" {
			return Proposal{Status: "manage", Action: "open_manager"}, nil
		}
		if proposal.Action != "pause" && proposal.Action != "resume" && proposal.Action != "open_manager" {
			return Proposal{}, fmt.Errorf("unsupported task management action")
		}
		for _, task := range tasks {
			if task.ID == proposal.TaskID {
				return Proposal{Status: "manage", Action: proposal.Action, TaskID: proposal.TaskID}, nil
			}
		}
		return Proposal{}, fmt.Errorf("managed task does not belong to the user")
	case "clarify":
		if strings.TrimSpace(proposal.Question) == "" {
			return Proposal{}, fmt.Errorf("clarification question is empty")
		}
		return Proposal{Status: "clarify", Question: strings.TrimSpace(proposal.Question)}, nil
	case "draft":
		if proposal.TaskID == "" {
			proposal.Schedule.Timezone = timezone
			if len(proposal.KnowledgeBaseIDs) > 0 {
				return Proposal{}, fmt.Errorf("new chat task cannot add knowledge base scope")
			}
		} else {
			var existing *TaskSummary
			for i := range tasks {
				if tasks[i].ID == proposal.TaskID {
					existing = &tasks[i]
					break
				}
			}
			if existing == nil {
				return Proposal{}, fmt.Errorf("edited task does not belong to the user")
			}
			if strings.TrimSpace(proposal.Name) == "" {
				proposal.Name = existing.Config.Name
			}
			for _, id := range proposal.KnowledgeBaseIDs {
				allowed := false
				for _, approved := range existing.Config.KnowledgeBaseIDs {
					if id == approved {
						allowed = true
					}
				}
				if !allowed {
					return Proposal{}, fmt.Errorf("chat edit cannot expand knowledge base scope")
				}
			}
		}
		version := domain.Version{TaskID: "preview", Number: 1, Name: proposal.Name, Prompt: proposal.Prompt, Schedule: proposal.Schedule,
			ReportMode: proposal.ReportMode, ConditionKind: proposal.ConditionKind,
			AllowedWebDomains: proposal.AllowedWebDomains, AllowedToolIDs: proposal.AllowedToolIDs, KnowledgeBaseIDs: proposal.KnowledgeBaseIDs, ConfirmedAt: now}
		if err := version.Validate(); err != nil {
			return Proposal{}, err
		}
		next, err := proposal.Schedule.Next(now)
		if err != nil || next.IsZero() {
			return Proposal{}, fmt.Errorf("proposed schedule has no future occurrence")
		}
		return proposal, nil
	default:
		return Proposal{}, fmt.Errorf("invalid proposal status %q", proposal.Status)
	}
}
