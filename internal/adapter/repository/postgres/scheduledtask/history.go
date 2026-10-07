package scheduledtask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/scheduledtask/domain"
)

// History contains only accepted outcomes of this task version. It never reads
// the user's ordinary chat history or other scheduled tasks.
func (s *Store) History(ctx context.Context, taskID string, version, limit int) (string, error) {
	if s == nil || s.db == nil || taskID == "" || version < 1 || limit < 1 || limit > 50 {
		return "", fmt.Errorf("invalid scheduled task history request")
	}
	var rows []struct {
		ScheduledAt string
		ResultJSON  []byte
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT scheduled_at::text AS scheduled_at, result_json
		FROM t_scheduled_task_occurrence WHERE task_id = ? AND version = ?
		AND status IN ('reported', 'no_report', 'uncertain') AND result_json IS NOT NULL
		ORDER BY scheduled_at DESC LIMIT ?`, taskID, version, limit).Scan(&rows).Error; err != nil {
		return "", err
	}
	var lines []string
	for i := len(rows) - 1; i >= 0; i-- {
		var outcome domain.Outcome
		if err := json.Unmarshal(rows[i].ResultJSON, &outcome); err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("%s: %s", rows[i].ScheduledAt, string(rows[i].ResultJSON)))
	}
	return strings.Join(lines, "\n"), nil
}
