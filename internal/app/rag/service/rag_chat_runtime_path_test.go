package service

import "testing"

func TestResolveChatPath(t *testing.T) {
	if path := resolveChatPath(RagChatInput{UseAgentRuntime: true}); path != chatPathAgentRuntimeTopLevel {
		t.Fatalf("expected %q, got %q", chatPathAgentRuntimeTopLevel, path)
	}
	if path := resolveChatPath(RagChatInput{}); path != chatPathRagChatLegacyMain {
		t.Fatalf("expected %q, got %q", chatPathRagChatLegacyMain, path)
	}
}

func TestResolveToolBackend(t *testing.T) {
	if backend := resolveToolBackend(ragChatToolStageResult{backend: toolBackendAgentRuntime}); backend != toolBackendAgentRuntime {
		t.Fatalf("unexpected backend: %q", backend)
	}
	if backend := resolveToolBackend(ragChatToolStageResult{fallbackFrom: "agent_runtime"}); backend != toolBackendToolWorkflowFallback {
		t.Fatalf("expected fallback backend, got %q", backend)
	}
}

func TestBuildRuntimePathTraceExtra(t *testing.T) {
	extra := buildRuntimePathTraceExtra(chatPathRagChatLegacyMain, toolBackendToolWorkflow, RagChatInput{}, ragChatAgentModeDiagnostic)
	runtimePath, ok := extra["runtimePath"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimePath map, got %#v", extra["runtimePath"])
	}
	if runtimePath["chatPath"] != chatPathRagChatLegacyMain {
		t.Fatalf("unexpected chatPath: %#v", runtimePath["chatPath"])
	}
	if runtimePath["toolBackend"] != toolBackendToolWorkflow {
		t.Fatalf("unexpected toolBackend: %#v", runtimePath["toolBackend"])
	}
}
