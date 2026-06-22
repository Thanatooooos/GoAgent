package extraction

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	DefaultPreferenceCandidateMinConfidence = 0.8
	maxPreferenceCandidateContentLength     = 200

	RejectionReasonContentTooLong                  = "content_too_long"
	RejectionReasonSensitiveContent                = "sensitive_content"
	RejectionReasonLowConfidence                   = "low_confidence"
	RejectionReasonTemporaryWording                = "temporary_wording"
	RejectionReasonInvalidTroubleshootingFirstStep = "invalid_troubleshooting_first_step"
)

var obviousSensitiveContentPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|passwd|api[_-]?key|access[_-]?key|secret|token)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)sk-[a-z0-9]{10,}`),
	regexp.MustCompile(`密码\s*[:：=]\s*\S+`),
}

var temporaryWordingTokens = []string{
	"今天",
	"刚刚",
	"现在",
	"这次",
	"明天",
}

var genericTroubleshootingFirstStepPhrases = map[string]struct{}{
	"先分析一下":    {},
	"分析一下":     {},
	"先看看情况":    {},
	"看看情况":     {},
	"看情况":      {},
	"先按最佳实践处理": {},
	"按最佳实践处理":  {},
	"先处理一下":    {},
	"处理一下":     {},
	"先排查一下":    {},
	"排查一下":     {},
}

var troubleshootingActionVerbs = []string{
	"看",
	"查看",
	"检查",
	"确认",
	"判断",
	"比较",
	"对比",
	"给",
	"提供",
	"收集",
	"复现",
	"定位",
	"验证",
}

var troubleshootingActionObjects = []string{
	"错误日志",
	"日志",
	"最小复现",
	"复现",
	"配置差异",
	"配置",
	"环境变量",
	"环境问题",
	"环境",
	"逻辑",
	"逻辑错误",
	"代码逻辑",
	"数据库",
	"连接",
	"网络连接",
	"超时",
	"内存",
	"CPU",
	"cpu",
	"磁盘",
	"依赖版本",
	"版本",
	"报错信息",
	"错误信息",
	"堆栈",
	"调用栈",
	"输入输出",
	"网络",
	"权限",
}

type PostFilterResult struct {
	Candidate       *StructuredPreferenceCandidate
	Rejected        bool
	RejectionReason string
}

func ApplyPreferencePostFilter(candidate StructuredPreferenceCandidate) PostFilterResult {
	normalized := StructuredPreferenceCandidate{
		ScopeType:    normalizeLower(candidate.ScopeType),
		MemoryType:   normalizeLower(candidate.MemoryType),
		CanonicalKey: normalizeLower(candidate.CanonicalKey),
		Summary:      strings.TrimSpace(candidate.Summary),
		Content:      strings.TrimSpace(candidate.Content),
		Confidence:   candidate.Confidence,
	}

	if normalized.ScopeType == "" || normalized.MemoryType == "" || normalized.CanonicalKey == "" || normalized.Summary == "" || normalized.Content == "" {
		return rejectPostFilter(RejectionReasonMissingField)
	}
	if normalized.ScopeType != "global" {
		return rejectPostFilter(RejectionReasonInvalidScopeType)
	}
	if normalized.MemoryType != "preference" {
		return rejectPostFilter(RejectionReasonInvalidMemoryType)
	}
	if normalized.CanonicalKey == "workflow.first_step" {
		return rejectPostFilter(RejectionReasonDeprecatedWorkflowKey)
	}
	if _, ok := phase1ExtractionCanonicalKeys[normalized.CanonicalKey]; !ok {
		return rejectPostFilter(RejectionReasonInvalidCanonicalKey)
	}
	if normalized.Confidence < DefaultPreferenceCandidateMinConfidence {
		return rejectPostFilter(RejectionReasonLowConfidence)
	}
	if normalized.Confidence > 1 {
		return rejectPostFilter(RejectionReasonInvalidConfidence)
	}
	if utf8.RuneCountInString(normalized.Content) > maxPreferenceCandidateContentLength {
		return rejectPostFilter(RejectionReasonContentTooLong)
	}
	if containsObviousSensitiveContent(normalized.Summary) || containsObviousSensitiveContent(normalized.Content) {
		return rejectPostFilter(RejectionReasonSensitiveContent)
	}
	if containsTemporaryWording(normalized.Summary) || containsTemporaryWording(normalized.Content) {
		return rejectPostFilter(RejectionReasonTemporaryWording)
	}
	if normalized.CanonicalKey == "workflow.troubleshooting.first_step" && !isConcreteTroubleshootingFirstStep(normalized.Content) {
		return rejectPostFilter(RejectionReasonInvalidTroubleshootingFirstStep)
	}

	return PostFilterResult{
		Candidate: &normalized,
	}
}

func rejectPostFilter(reason string) PostFilterResult {
	return PostFilterResult{
		Rejected:        true,
		RejectionReason: reason,
	}
}

func containsObviousSensitiveContent(content string) bool {
	content = strings.TrimSpace(content)
	for _, pattern := range obviousSensitiveContentPatterns {
		if pattern.MatchString(content) {
			return true
		}
	}
	return false
}

func containsTemporaryWording(content string) bool {
	for _, token := range temporaryWordingTokens {
		if strings.Contains(content, token) {
			return true
		}
	}
	return false
}

func isConcreteTroubleshootingFirstStep(content string) bool {
	content = strings.TrimSpace(content)
	if _, ok := genericTroubleshootingFirstStepPhrases[content]; ok {
		return false
	}

	hasActionVerb := false
	for _, verb := range troubleshootingActionVerbs {
		if strings.Contains(content, verb) {
			hasActionVerb = true
			break
		}
	}
	if !hasActionVerb {
		return false
	}

	for _, object := range troubleshootingActionObjects {
		if strings.Contains(content, object) {
			return true
		}
	}
	return false
}
