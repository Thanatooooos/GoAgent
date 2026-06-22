package extraction

import (
	"strings"
	"testing"
)

func TestApplyPreferencePostFilter(t *testing.T) {
	tests := []struct {
		name                string
		input               StructuredPreferenceCandidate
		wantRejected        bool
		wantRejectionReason string
		wantCandidate       *StructuredPreferenceCandidate
	}{
		{
			name: "normalizes allowlisted canonical key",
			input: StructuredPreferenceCandidate{
				ScopeType:    " global ",
				MemoryType:   " preference ",
				CanonicalKey: " Response.Language ",
				Summary:      "以后默认用中文回答",
				Content:      " 中文 ",
				Confidence:   0.95,
			},
			wantCandidate: &StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "response.language",
				Summary:      "以后默认用中文回答",
				Content:      "中文",
				Confidence:   0.95,
			},
		},
		{
			name: "rejects low confidence candidate",
			input: StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "response.language",
				Summary:      "以后默认用中文回答",
				Content:      "中文",
				Confidence:   0.79,
			},
			wantRejected:        true,
			wantRejectionReason: RejectionReasonLowConfidence,
		},
		{
			name: "rejects confidence above one",
			input: StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "response.language",
				Summary:      "以后默认用中文回答",
				Content:      "中文",
				Confidence:   1.5,
			},
			wantRejected:        true,
			wantRejectionReason: RejectionReasonInvalidConfidence,
		},
		{
			name: "rejects content that is too long",
			input: StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "behavior.avoid",
				Summary:      "以后不要输出太冗长的解释",
				Content:      strings.Repeat("长", 201),
				Confidence:   0.93,
			},
			wantRejected:        true,
			wantRejectionReason: RejectionReasonContentTooLong,
		},
		{
			name: "rejects obvious sensitive content",
			input: StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "behavior.avoid",
				Summary:      "以后不要让我重复贴敏感信息",
				Content:      "password=123456",
				Confidence:   0.92,
			},
			wantRejected:        true,
			wantRejectionReason: RejectionReasonSensitiveContent,
		},
		{
			name: "rejects temporary wording",
			input: StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "response.language",
				Summary:      "今天先用中文",
				Content:      "今天用中文",
				Confidence:   0.91,
			},
			wantRejected:        true,
			wantRejectionReason: RejectionReasonTemporaryWording,
		},
		{
			name: "rejects unsupported memory type",
			input: StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "knowledge",
				CanonicalKey: "response.language",
				Summary:      "项目默认语言是中文",
				Content:      "中文",
				Confidence:   0.95,
			},
			wantRejected:        true,
			wantRejectionReason: RejectionReasonInvalidMemoryType,
		},
		{
			name: "rejects non-allowlist key after normalization",
			input: StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: " workflow.reply.style ",
				Summary:      "以后更口语化一点",
				Content:      "口语化",
				Confidence:   0.9,
			},
			wantRejected:        true,
			wantRejectionReason: RejectionReasonInvalidCanonicalKey,
		},
		{
			name: "rejects generic troubleshooting first step",
			input: StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "workflow.troubleshooting.first_step",
				Summary:      "以后遇到问题先分析一下",
				Content:      "先分析一下",
				Confidence:   0.94,
			},
			wantRejected:        true,
			wantRejectionReason: RejectionReasonInvalidTroubleshootingFirstStep,
		},
		{
			name: "accepts concrete troubleshooting first step",
			input: StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "workflow.troubleshooting.first_step",
				Summary:      "以后遇到报错先判断是不是环境问题",
				Content:      "先判断是不是环境问题",
				Confidence:   0.94,
			},
			wantCandidate: &StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "workflow.troubleshooting.first_step",
				Summary:      "以后遇到报错先判断是不是环境问题",
				Content:      "先判断是不是环境问题",
				Confidence:   0.94,
			},
		},
		{
			name: "accepts logic error troubleshooting action after normalization",
			input: StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "workflow.troubleshooting.first_step",
				Summary:      "以后遇到问题先判断是不是逻辑错误",
				Content:      "先判断是不是逻辑错误",
				Confidence:   0.92,
			},
			wantCandidate: &StructuredPreferenceCandidate{
				ScopeType:    "global",
				MemoryType:   "preference",
				CanonicalKey: "workflow.troubleshooting.first_step",
				Summary:      "以后遇到问题先判断是不是逻辑错误",
				Content:      "先判断是不是逻辑错误",
				Confidence:   0.92,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplyPreferencePostFilter(tc.input)

			if got.Rejected != tc.wantRejected {
				t.Fatalf("Rejected = %v, want %v, result=%+v", got.Rejected, tc.wantRejected, got)
			}
			if got.RejectionReason != tc.wantRejectionReason {
				t.Fatalf("RejectionReason = %q, want %q, result=%+v", got.RejectionReason, tc.wantRejectionReason, got)
			}
			if tc.wantCandidate == nil {
				if got.Candidate != nil {
					t.Fatalf("Candidate = %+v, want nil", got.Candidate)
				}
				return
			}
			if got.Candidate == nil {
				t.Fatalf("Candidate = nil, want %+v", tc.wantCandidate)
			}
			if *got.Candidate != *tc.wantCandidate {
				t.Fatalf("Candidate = %+v, want %+v", *got.Candidate, *tc.wantCandidate)
			}
		})
	}
}

func TestDefaultPreferenceCandidateMinConfidence(t *testing.T) {
	if DefaultPreferenceCandidateMinConfidence != 0.8 {
		t.Fatalf("DefaultPreferenceCandidateMinConfidence = %v, want 0.8", DefaultPreferenceCandidateMinConfidence)
	}
}
