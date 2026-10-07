package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/runtime/persistence"
)

// ToolCall is a single model-requested capability invocation.
type ToolCall struct {
	ID           string
	CapabilityID string
	Arguments    capability.Value
}

func (c ToolCall) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("tool call id is required")
	}
	if strings.TrimSpace(c.CapabilityID) == "" {
		return fmt.Errorf("capability id is required")
	}
	return nil
}

// ToolRunner is the only runtime component allowed to invoke a capability.
// It makes the lifecycle durable without coupling capability definitions to the
// journal, PostgreSQL, or legacy chat implementation.
type ToolRunner struct {
	registry  *capability.Registry
	lifecycle *Lifecycle
}

func NewToolRunner(registry *capability.Registry, lifecycle *Lifecycle) *ToolRunner {
	return &ToolRunner{registry: registry, lifecycle: lifecycle}
}

func (r *ToolRunner) Run(ctx context.Context, session persistence.Session, call ToolCall, toolContext capability.Context) (capability.Operation, capability.Result, error) {
	if r == nil || r.registry == nil || r.lifecycle == nil {
		return capability.Operation{}, capability.Result{}, fmt.Errorf("runtime tool runner is not configured")
	}
	if err := call.Validate(); err != nil {
		return capability.Operation{}, capability.Result{}, err
	}
	toolContext.ToolCallID = call.ID
	if _, err := r.lifecycle.BeginTool(ctx, session, call.ID, call.CapabilityID, string(call.Arguments)); err != nil {
		return capability.Operation{}, capability.Result{}, fmt.Errorf("record pending tool call: %w", err)
	}

	def, operation, err := r.registry.Prepare(call.CapabilityID, call.Arguments, bindToolContext(ctx, session, toolContext))
	if err != nil {
		return capability.Operation{}, capability.Result{}, r.settlePreparationError(ctx, session, call, err)
	}
	if _, err := r.lifecycle.StartTool(ctx, session, call.ID, def.ID); err != nil {
		return capability.Operation{}, capability.Result{}, fmt.Errorf("record executing tool call: %w", err)
	}

	result, executeErr := def.Execute(call.Arguments, bindToolContext(ctx, session, toolContext))
	if executeErr != nil {
		if _, settleErr := r.lifecycle.SettleTool(ctx, session, call.ID, def.ID, ToolStateFailed, executeErr.Error(), nil); settleErr != nil {
			return operation, capability.Result{}, fmt.Errorf("execute capability %q: %w; record failed tool call: %v", def.ID, executeErr, settleErr)
		}
		return operation, capability.Result{}, fmt.Errorf("execute capability %q: %w", def.ID, executeErr)
	}
	detail, err := marshalToolResult(result)
	if err != nil {
		if _, settleErr := r.lifecycle.SettleTool(ctx, session, call.ID, def.ID, ToolStateFailed, fmt.Sprintf("encode result: %v", err), nil); settleErr != nil {
			return operation, capability.Result{}, fmt.Errorf("encode capability %q result: %w; record failed tool call: %v", def.ID, err, settleErr)
		}
		return operation, capability.Result{}, fmt.Errorf("encode capability %q result: %w", def.ID, err)
	}
	if _, err := r.lifecycle.SettleTool(ctx, session, call.ID, def.ID, ToolStateCompleted, detail, result.Evidence); err != nil {
		return operation, capability.Result{}, fmt.Errorf("record completed tool call: %w", err)
	}
	return operation, result, nil
}

func (r *ToolRunner) settlePreparationError(ctx context.Context, session persistence.Session, call ToolCall, cause error) error {
	state := ToolStateFailed
	if capability.IsDenied(cause) {
		state = ToolStateDenied
	}
	if _, err := r.lifecycle.SettleTool(ctx, session, call.ID, call.CapabilityID, state, cause.Error(), nil); err != nil {
		return fmt.Errorf("prepare capability %q: %w; record %s tool call: %v", call.CapabilityID, cause, state, err)
	}
	return fmt.Errorf("prepare capability %q: %w", call.CapabilityID, cause)
}

func bindToolContext(ctx context.Context, session persistence.Session, toolContext capability.Context) capability.Context {
	toolContext.Context = ctx
	toolContext.RuntimeSessionID = session.ID
	toolContext.ConversationID = session.ConversationID
	toolContext.UserMessageID = session.UserMessageID
	toolContext.UserID = session.UserID
	return toolContext
}

func marshalToolResult(result capability.Result) (string, error) {
	payload := struct {
		Content string           `json:"content"`
		Value   capability.Value `json:"value"`
	}{Content: result.Content, Value: result.Value}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
