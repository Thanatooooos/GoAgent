package config

import "testing"

func TestLoadConfig_Defaults(t *testing.T) {
	// tests run with package dir as working directory; load from repository configs
	if err := LoadConfig("../../../configs"); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	cfg := Get()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	// assert a few known values from configs/application.yaml
	if cfg.AI.Chat.DefaultModel != "qwen3-max" {
		t.Fatalf("unexpected chat.default-model: %s", cfg.AI.Chat.DefaultModel)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("unexpected server.port: %d", cfg.Server.Port)
	}
}
