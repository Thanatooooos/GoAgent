package rag

import (
	"context"
	"encoding/json"
	convruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/stream"
)

// RecoverRuntimePublication completes an unfinished hot stream without repeating
// its deltas. Cold streams are rebuilt from the journal by the regular replay path.
func RecoverRuntimePublication(ctx context.Context, manager stream.StreamManager, task string, recover func(context.Context) (convruntime.JournalEntry, error)) error {
	events, _, err := manager.GetEvents(ctx, task, 0)
	if err != nil || len(events) == 0 {
		return err
	}
	finished := false
	for _, event := range events {
		if event.Name == internalStopEventName {
			return nil
		}
		if event.Name == "finish" {
			finished = true
		}
	}
	if finished {
		if events[len(events)-1].Done {
			return nil
		}
		return newRuntimeStreamSink(manager, task).stream.SendDone()
	}
	entry, err := recover(ctx)
	if err != nil || entry.ID == "" {
		return err
	}
	if entry.EventType == convruntime.EventInterrupted || entry.EventType == convruntime.EventCancelled || entry.EventType == convruntime.EventFailed {
		for _, event := range events {
			var marker struct {
				RuntimeEventID string `json:"runtimeEventId"`
			}
			if json.Unmarshal(event.Data, &marker) == nil && marker.RuntimeEventID == entry.ID {
				if !events[len(events)-1].Done {
					return newRuntimeStreamSink(manager, task).stream.SendDone()
				}
				return nil
			}
		}
	}
	return newRuntimeStreamSink(manager, task).Append(ctx, entry)
}

// A transport error can have closed the old stream before publication recovered.
// Reconnect emits the recovered finish rather than stopping at the old done.
func PublicationResumeOffset(events []stream.StreamEvent, offset int) int {
	oldDone := -1
	lastFinish := -1
	for i, event := range events {
		var terminal struct {
			RuntimeEventID string `json:"runtimeEventId"`
		}
		if (event.Name == "error" || event.Name == "cancel") && json.Unmarshal(event.Data, &terminal) == nil && terminal.RuntimeEventID != "" {
			lastFinish = i
			if oldDone >= 0 {
				return i
			}
		}
		if event.Name == "finish" {
			lastFinish = i
		}
		if event.Name == "finish" && oldDone >= 0 {
			return i
		}
		if event.Done {
			oldDone = i
		}
	}
	if offset >= len(events) && lastFinish >= 0 {
		return lastFinish
	}
	return offset
}
