package history

import "testing"

func TestParseStructuredSummaryRejectsUnknownFields(t *testing.T) {
	_, err := ParseStructuredSummary(`{"schema_version":1,"goal":"x","unknown":"y"}`)
	if err == nil {
		t.Fatal("expected unknown fields to be rejected")
	}
}
