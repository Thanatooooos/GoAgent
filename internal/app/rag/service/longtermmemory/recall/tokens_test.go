package recall

import (
	"reflect"
	"testing"
)

func TestBuildRecallSearchTokensFiltersNoiseAndBuildsCJKBigrams(t *testing.T) {
	t.Parallel()

	got := buildRecallSearchTokens("请问 这个 main bus 怎么 处理 RocketMQ 移除")

	want := []string{"main", "bus", "处理", "rocketmq", "移除", "请问", "这个", "怎么"}
	if reflect.DeepEqual(got, want) {
		t.Fatalf("expected noise tokens to be filtered, got=%v", got)
	}

	expectedSubset := []string{"main", "bus", "rocketmq", "移除"}
	for _, token := range expectedSubset {
		if !containsString(got, token) {
			t.Fatalf("expected token %q in %v", token, got)
		}
	}
	if containsString(got, "请问") || containsString(got, "这个") || containsString(got, "怎么") {
		t.Fatalf("expected noise tokens to be removed, got=%v", got)
	}
}

func TestBuildRecallSearchTokensCapsResultLength(t *testing.T) {
	t.Parallel()

	got := buildRecallSearchTokens("alpha beta gamma delta epsilon zeta eta theta iota kappa lambda")
	if len(got) != maxRecallSearchTokens {
		t.Fatalf("expected %d tokens, got %d: %v", maxRecallSearchTokens, len(got), got)
	}
}

func TestBuildDistinctCJKBigramsDeduplicatesAdjacentPairs(t *testing.T) {
	t.Parallel()

	got := buildDistinctCJKBigrams("中文中文")
	want := []string{"中文", "文中"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildDistinctCJKBigrams() = %v, want %v", got, want)
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
