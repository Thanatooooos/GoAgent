package kernel

import (
	"fmt"
	"strings"
	"time"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func appendSessionEvent(session *agentruntime.RuntimeSession, event agentstate.RuntimeEvent) {
	if session == nil {
		return
	}
	if strings.TrimSpace(event.SessionID) == "" {
		event.SessionID = session.SessionID
	}
	event.Node = strings.TrimSpace(event.Node)
	event.EventType = strings.TrimSpace(event.EventType)
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.Sequence = len(session.Journal) + 1
	if strings.TrimSpace(event.ID) == "" {
		event.ID = buildSessionEventID(event.SessionID, event.Sequence)
	}
	session.Journal = append(session.Journal, event)
}

func buildSessionEventID(sessionID string, sequence int) string {
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID == "" {
		return fmt.Sprintf("event-%06d", sequence)
	}
	return fmt.Sprintf("%s:%06d", trimmedSessionID, sequence)
}
