package rag

import "testing"

func TestCompareDistributedIDsUsesNumericOrdering(t *testing.T) {
	if got := compareDistributedIDs("9", "10"); got >= 0 {
		t.Fatalf("compareDistributedIDs(9, 10) = %d, want < 0", got)
	}
	if got := compareDistributedIDs("100", "20"); got <= 0 {
		t.Fatalf("compareDistributedIDs(100, 20) = %d, want > 0", got)
	}
	if got := compareDistributedIDs("20", "20"); got != 0 {
		t.Fatalf("compareDistributedIDs(20, 20) = %d, want 0", got)
	}
}
