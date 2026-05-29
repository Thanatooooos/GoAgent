package recall

import (
	"testing"

	"local/rag-project/internal/app/rag/port"
)

func TestBuildRuleRequestCacheKeyIsStableAcrossKBOrder(t *testing.T) {
	t.Parallel()

	versions := port.ScopeVersions{
		GlobalVersion: 3,
		KBVersions: map[string]int64{
			"kb-b": 2,
			"kb-a": 1,
		},
	}

	left := buildRuleRequestCacheKey("u-1", []string{"kb-b", "kb-a"}, versions)
	right := buildRuleRequestCacheKey("u-1", []string{"kb-a", "kb-b"}, versions)
	if left != right {
		t.Fatalf("expected stable rule cache key, left=%q right=%q", left, right)
	}
}

func TestBuildFactRequestCacheKeyNormalizesQueryAndSortsInputs(t *testing.T) {
	t.Parallel()

	versions := port.ScopeVersions{
		GlobalVersion: 5,
		KBVersions: map[string]int64{
			"kb-2": 7,
			"kb-1": 6,
		},
	}

	left := buildFactRequestCacheKey("u-1", "  Main BUS  ", []string{"kb-2", "kb-1"}, 20, "text-embedding", "v1", versions)
	right := buildFactRequestCacheKey("u-1", "main bus", []string{"kb-1", "kb-2"}, 20, "text-embedding", "v1", versions)
	if left != right {
		t.Fatalf("expected stable fact cache key, left=%q right=%q", left, right)
	}
}

func TestHashScopeVersionsReturnsDeterministicString(t *testing.T) {
	t.Parallel()

	got := hashScopeVersions(map[string]int64{
		"kb-b": 2,
		"kb-a": 1,
	})
	if got != "kb-a=1,kb-b=2" {
		t.Fatalf("hashScopeVersions() = %q", got)
	}
}
