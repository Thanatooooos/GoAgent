package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"local/rag-project/internal/framework/config"
	aimodel "local/rag-project/internal/infra-ai/model"
)

func TestModelHealthStoreOpensAfterFailures(t *testing.T) {
	loadModelTestConfig(t)

	store := aimodel.NewModelHealthStore()
	store.MarkFailure("m1")

	if store.AllowCall("m1") {
		t.Fatal("expected call to be blocked while circuit is open")
	}
	if !store.IsUnavailable("m1") {
		t.Fatal("expected model to be unavailable")
	}
}

func TestModelHealthStoreHalfOpenAllowsSingleProbe(t *testing.T) {
	loadModelTestConfig(t)

	store := aimodel.NewModelHealthStore()
	store.MarkFailure("m1")

	time.Sleep(20 * time.Millisecond)

	if !store.AllowCall("m1") {
		t.Fatal("expected first call after open window to be allowed")
	}
	if store.AllowCall("m1") {
		t.Fatal("expected second half-open probe to be blocked")
	}
}

func TestModelHealthStoreSuccessClosesCircuit(t *testing.T) {
	loadModelTestConfig(t)

	store := aimodel.NewModelHealthStore()
	store.MarkFailure("m1")
	time.Sleep(20 * time.Millisecond)

	if !store.AllowCall("m1") {
		t.Fatal("expected half-open probe call to be allowed")
	}

	store.MarkSuccess("m1")

	if !store.AllowCall("m1") {
		t.Fatal("expected calls to be allowed after success")
	}
	if store.IsUnavailable("m1") {
		t.Fatal("expected model to be available after success")
	}
}

func loadModelTestConfig(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	content := []byte(`
ai:
  providers:
    p:
      url: http://example.com
    noop:
      url: http://example.com
  selection:
    failure-threshold: 1
    open-duration-ms: 10
  chat:
    default-model: first
    deep-thinking-model: deep
    candidates:
      - id: first
        provider: p
        model: m1
        priority: 10
      - id: deep
        provider: p
        model: m2
        priority: 20
        supports-thinking: true
  embedding:
    default-model: emb
    candidates:
      - id: emb
        provider: p
        model: em1
        dimension: 1024
  rerank:
    default-model: rr
    candidates:
      - id: rr
        provider: noop
        model: noop
`)

	path := filepath.Join(dir, "application.yaml")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	if err := config.LoadConfig(dir); err != nil {
		t.Fatalf("load config failed: %v", err)
	}
}
