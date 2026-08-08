package dailybrief

import (
	"context"
	"strings"
	"testing"

	"local/rag-project/internal/app/dailybrief/port"
)

func TestPublishTransactionRequiresDB(t *testing.T) {
	t.Parallel()

	tx := NewPublishTransaction(nil)
	err := tx(context.Background(), func(ctx context.Context, issueRepo port.IssueRepository, itemRepo port.ItemRepository) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected nil db transaction to fail")
	}
	if !strings.Contains(err.Error(), "gorm db is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
