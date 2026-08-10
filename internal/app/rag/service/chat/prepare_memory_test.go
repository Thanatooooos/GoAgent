package chat

import (
	"testing"
)

func TestNextTaskIDRoundTrips(t *testing.T) {
	id, err := NextTaskID()
	if err != nil {
		t.Fatalf("NextTaskID: %v", err)
	}
	if id == "" {
		t.Fatal("NextTaskID returned empty")
	}
}
