package paging

import "testing"

func TestNormalize(t *testing.T) {
	page, pageSize := Normalize(0, 999, 10, 100)
	if page != 1 {
		t.Fatalf("Normalize page = %d, want 1", page)
	}
	if pageSize != 100 {
		t.Fatalf("Normalize pageSize = %d, want 100", pageSize)
	}
}
