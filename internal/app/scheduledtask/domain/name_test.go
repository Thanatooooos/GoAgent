package domain

import "testing"

func TestDisplayNameDoesNotExposeLegacyPrompt(t *testing.T) {
	v := Version{Prompt: "Use web_fetch with private execution details", ReportMode: ReportOnCondition, ConditionKind: ConditionEvent}
	if v.DisplayName() != "事件监测" {
		t.Fatal(v.DisplayName())
	}
	v.Name = " 官方赛事追踪 "
	if v.DisplayName() != "官方赛事追踪" {
		t.Fatal(v.DisplayName())
	}
}
