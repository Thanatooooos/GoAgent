package capability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/domain"
)

const (
	MemoryAddID    = "memory_add"
	MemoryUpdateID = "memory_update"
	MemoryDeleteID = "memory_delete"
)

type CoreMemoryService interface {
	AddCoreMemory(ctx context.Context, userID, sourceMessageID, content string) (domain.MemoryItem, error)
	UpdateCoreMemory(ctx context.Context, userID, memoryID, sourceMessageID, content string) (domain.MemoryItem, error)
	DeleteCoreMemory(ctx context.Context, userID, memoryID string) (domain.MemoryItem, error)
}

var memoryAddSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["content"],"properties":{"content":{"type":"string","minLength":1,"maxLength":400}}}`)
var memoryUpdateSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["memory_id","content"],"properties":{"memory_id":{"type":"string","minLength":1},"content":{"type":"string","minLength":1,"maxLength":400}}}`)
var memoryDeleteSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["memory_id"],"properties":{"memory_id":{"type":"string","minLength":1}}}`)

func CoreMemoryAdd(service CoreMemoryService) Def {
	return Def{ID: MemoryAddID, Description: coreMemoryAddDescription, JSONSchema: memoryAddSchema,
		Validate: func(value Value) error { _, err := parseMemoryAddArgs(value); return err },
		Describe: func(value Value, ctx Context) (Operation, error) {
			args, err := parseMemoryAddArgs(value)
			if err != nil {
				return Operation{}, err
			}
			if !ctx.AllowMemoryMutation {
				return Operation{}, Deny("core memory changes require an explicit user request")
			}
			return Operation{ID: MemoryAddID, Summary: "save user core memory", Input: mustJSON(args)}, nil
		},
		Execute: func(value Value, ctx Context) (Result, error) {
			args, err := parseMemoryAddArgs(value)
			if err != nil {
				return Result{}, err
			}
			if !ctx.AllowMemoryMutation {
				return Result{}, fmt.Errorf("core memory changes require an explicit user request")
			}
			item, err := service.AddCoreMemory(ctx.Context, ctx.UserID, ctx.UserMessageID, args.Content)
			if err != nil {
				return Result{}, err
			}
			return coreMemoryResult("saved", item)
		},
	}
}

func CoreMemoryUpdate(service CoreMemoryService) Def {
	return Def{ID: MemoryUpdateID, Description: coreMemoryUpdateDescription, JSONSchema: memoryUpdateSchema,
		Validate: func(value Value) error { _, err := parseMemoryUpdateArgs(value); return err },
		Describe: func(value Value, ctx Context) (Operation, error) {
			args, err := parseMemoryUpdateArgs(value)
			if err != nil {
				return Operation{}, err
			}
			if !ctx.AllowMemoryMutation {
				return Operation{}, Deny("core memory changes require an explicit user request")
			}
			return Operation{ID: MemoryUpdateID, Summary: "update user core memory", Input: mustJSON(args)}, nil
		},
		Execute: func(value Value, ctx Context) (Result, error) {
			args, err := parseMemoryUpdateArgs(value)
			if err != nil {
				return Result{}, err
			}
			if !ctx.AllowMemoryMutation {
				return Result{}, fmt.Errorf("core memory changes require an explicit user request")
			}
			item, err := service.UpdateCoreMemory(ctx.Context, ctx.UserID, args.MemoryID, ctx.UserMessageID, args.Content)
			if err != nil {
				return Result{}, err
			}
			return coreMemoryResult("updated", item)
		},
	}
}

func CoreMemoryDelete(service CoreMemoryService) Def {
	return Def{ID: MemoryDeleteID, Description: coreMemoryDeleteDescription, JSONSchema: memoryDeleteSchema,
		Validate: func(value Value) error { _, err := parseMemoryDeleteArgs(value); return err },
		Describe: func(value Value, ctx Context) (Operation, error) {
			args, err := parseMemoryDeleteArgs(value)
			if err != nil {
				return Operation{}, err
			}
			if !ctx.AllowMemoryMutation {
				return Operation{}, Deny("core memory changes require an explicit user request")
			}
			return Operation{ID: MemoryDeleteID, Summary: "forget user core memory", Input: mustJSON(args)}, nil
		},
		Execute: func(value Value, ctx Context) (Result, error) {
			args, err := parseMemoryDeleteArgs(value)
			if err != nil {
				return Result{}, err
			}
			if !ctx.AllowMemoryMutation {
				return Result{}, fmt.Errorf("core memory changes require an explicit user request")
			}
			item, err := service.DeleteCoreMemory(ctx.Context, ctx.UserID, args.MemoryID)
			if err != nil {
				return Result{}, err
			}
			return coreMemoryResult("deleted", item)
		},
	}
}

type memoryAddArgs struct {
	Content string `json:"content"`
}
type memoryUpdateArgs struct {
	MemoryID string `json:"memory_id"`
	Content  string `json:"content"`
}
type memoryDeleteArgs struct {
	MemoryID string `json:"memory_id"`
}

func parseMemoryAddArgs(value Value) (memoryAddArgs, error) {
	var args memoryAddArgs
	err := decodeCoreMemoryArgs(value, &args, func() bool { args.Content = strings.TrimSpace(args.Content); return args.Content != "" })
	return args, err
}
func parseMemoryUpdateArgs(value Value) (memoryUpdateArgs, error) {
	var args memoryUpdateArgs
	err := decodeCoreMemoryArgs(value, &args, func() bool {
		args.MemoryID = strings.TrimSpace(args.MemoryID)
		args.Content = strings.TrimSpace(args.Content)
		return args.MemoryID != "" && args.Content != ""
	})
	return args, err
}
func parseMemoryDeleteArgs(value Value) (memoryDeleteArgs, error) {
	var args memoryDeleteArgs
	err := decodeCoreMemoryArgs(value, &args, func() bool { args.MemoryID = strings.TrimSpace(args.MemoryID); return args.MemoryID != "" })
	return args, err
}

func decodeCoreMemoryArgs(value Value, target any, valid func() bool) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if !valid() {
		return fmt.Errorf("required core memory arguments are missing")
	}
	return nil
}

func coreMemoryResult(action string, item domain.MemoryItem) (Result, error) {
	body, err := json.Marshal(struct {
		Action   string `json:"action"`
		MemoryID string `json:"memory_id"`
		Content  string `json:"content"`
		Status   string `json:"status"`
	}{action, item.ID, item.Content, item.Status})
	if err != nil {
		return Result{}, err
	}
	return Result{Content: string(body), Value: body}, nil
}

func mustJSON(value any) Value { encoded, _ := json.Marshal(value); return encoded }
