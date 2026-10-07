package dailybrief

import (
	"context"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"local/rag-project/internal/app/dailybrief/domain"
)

// GORM applies model defaults before building INSERT ... ON CONFLICT too.
// A zero enabled flag must remain zero for both initial saves and updates.
func TestSubscriptionUpsertPreservesExplicitDisabledFlag(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	repo := NewSubscriptionRepository(db)
	for _, enabled := range []bool{false, true, false} {
		sub := domain.NewSubscription("test-user", "UTC", "08:00", []string{domain.TopicKeyTechDev}, nil)
		sub.Enabled = enabled
		saved, err := repo.Upsert(context.Background(), sub)
		if err != nil {
			t.Fatal(err)
		}
		if saved.Enabled != enabled {
			t.Fatalf("explicit enabled=%t became %t during upsert", enabled, saved.Enabled)
		}
	}
}
