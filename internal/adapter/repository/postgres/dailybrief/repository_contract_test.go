package dailybrief

import (
	"testing"

	"local/rag-project/internal/app/dailybrief/port"
)

func TestRepositoriesImplementDailyBriefPorts(t *testing.T) {
	t.Parallel()

	var _ port.SubscriptionRepository = NewSubscriptionRepository(nil)
	var _ port.IssueRepository = NewIssueRepository(nil)
	var _ port.ItemRepository = NewItemRepository(nil)
	var _ port.GenerationRunRepository = NewGenerationRunRepository(nil)
}
