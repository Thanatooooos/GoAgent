package extraction

import "testing"

func TestEvaluatePreFilterUsesGatedPreferenceTriggerDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		message            string
		wantSkip           bool
		wantSkipReason     string
		wantHasTrigger     bool
		wantMatchedTrigger string
	}{
		{
			name:           "algorithm question without trigger is skipped",
			message:        "这道算法题怎么做？给定一个数组，返回两数之和。",
			wantSkip:       true,
			wantSkipReason: SkipReasonOneOffAlgorithm,
		},
		{
			name:               "algorithm question with future trigger is not skipped",
			message:            "以后遇到这种算法题请先给思路，再给代码。给定一个数组，返回两数之和。",
			wantHasTrigger:     true,
			wantMatchedTrigger: "以后",
		},
		{
			name:               "algorithm question with after trigger is not skipped",
			message:            "之后这种算法题都先讲解思路。给定一个链表，怎么反转？",
			wantHasTrigger:     true,
			wantMatchedTrigger: "之后",
		},
		{
			name:               "algorithm question with preference trigger is not skipped",
			message:            "我希望你以后做算法题先讲思路，再给代码。给定一个数组，返回最大子序和。",
			wantHasTrigger:     true,
			wantMatchedTrigger: "我希望",
		},
		{
			name:               "algorithm question with avoid trigger is not skipped",
			message:            "不要一上来就给算法题答案，先讲思路。给定一个数组，返回最大值。",
			wantHasTrigger:     true,
			wantMatchedTrigger: "不要",
		},
		{
			name:               "algorithm question with troubleshooting trigger is not skipped",
			message:            "遇到问题先给排查步骤，再回答这道算法题。给定一个字符串，如何反转？",
			wantHasTrigger:     true,
			wantMatchedTrigger: "遇到问题先",
		},
		{
			name:           "transient error without trigger is skipped",
			message:        "这个报错怎么处理？panic: nil pointer dereference",
			wantSkip:       true,
			wantSkipReason: SkipReasonTransientError,
		},
		{
			name:               "transient error with trigger is not skipped",
			message:            "以后我贴报错时请先帮我定位根因。这个报错怎么处理？panic: nil pointer dereference",
			wantHasTrigger:     true,
			wantMatchedTrigger: "以后",
		},
		{
			name:           "temporary command without trigger is skipped",
			message:        "git log",
			wantSkip:       true,
			wantSkipReason: SkipReasonTemporaryCommand,
		},
		{
			name:               "temporary command with future trigger is not skipped",
			message:            "以后 git log 都加 --oneline",
			wantHasTrigger:     true,
			wantMatchedTrigger: "以后",
		},
		{
			name:           "greeting without trigger is skipped",
			message:        "你好呀",
			wantSkip:       true,
			wantSkipReason: SkipReasonGreeting,
		},
		{
			name:           "translation without trigger is skipped",
			message:        "把这句话翻译成英文：今天先修这个 bug",
			wantSkip:       true,
			wantSkipReason: SkipReasonTranslation,
		},
		{
			name:           "calculation without trigger is skipped",
			message:        "帮我计算 17*23 等于多少",
			wantSkip:       true,
			wantSkipReason: SkipReasonCalculation,
		},
		{
			name:           "short follow-up without trigger is skipped",
			message:        "那然后呢？",
			wantSkip:       true,
			wantSkipReason: SkipReasonShortFollowUp,
		},
		{
			name:           "empty message is skipped as short follow-up",
			message:        "   ",
			wantSkip:       true,
			wantSkipReason: SkipReasonShortFollowUp,
		},
		{
			name:               "plain long-term preference is not skipped",
			message:            "以后默认用中文回答我。",
			wantHasTrigger:     true,
			wantMatchedTrigger: "以后",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := EvaluatePreFilter(PreFilterInput{
				Message: tc.message,
			})

			if got.Skip != tc.wantSkip {
				t.Fatalf("Skip = %v, want %v, result=%+v", got.Skip, tc.wantSkip, got)
			}
			if got.SkipReason != tc.wantSkipReason {
				t.Fatalf("SkipReason = %q, want %q, result=%+v", got.SkipReason, tc.wantSkipReason, got)
			}
			if got.HasPreferenceTrigger != tc.wantHasTrigger {
				t.Fatalf("HasPreferenceTrigger = %v, want %v, result=%+v", got.HasPreferenceTrigger, tc.wantHasTrigger, got)
			}
			if tc.wantMatchedTrigger != "" && !containsString(got.MatchedTriggers, tc.wantMatchedTrigger) {
				t.Fatalf("MatchedTriggers = %+v, want to contain %q", got.MatchedTriggers, tc.wantMatchedTrigger)
			}
		})
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
