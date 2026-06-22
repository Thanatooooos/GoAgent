package extraction

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	SkipReasonNone             = ""
	SkipReasonOneOffAlgorithm  = "one_off_algorithm"
	SkipReasonTransientError   = "transient_error"
	SkipReasonTemporaryCommand = "temporary_command"
	SkipReasonGreeting         = "greeting"
	SkipReasonTranslation      = "translation"
	SkipReasonCalculation      = "calculation"
	SkipReasonShortFollowUp    = "short_follow_up"
	maxShortMessageRuneLength  = 12
)

var (
	calculationPattern = regexp.MustCompile(`\d+\s*[\+\-\*/]\s*\d+`)
	commandPrefixes    = []string{
		"git ",
		"kubectl ",
		"docker ",
		"go test ",
		"npm ",
		"pnpm ",
		"python ",
		"curl ",
	}
	preferenceTriggerSignals = []string{
		"遇到问题先",
		"请一直用",
		"我更喜欢",
		"我希望",
		"以后",
		"之后",
		"不要",
	}
)

type PreFilterInput struct {
	Message string
}

type PreFilterResult struct {
	HasPreferenceTrigger bool
	MatchedTriggers      []string
	Skip                 bool
	SkipReason           string
}

func EvaluatePreFilter(input PreFilterInput) PreFilterResult {
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return PreFilterResult{Skip: true, SkipReason: SkipReasonShortFollowUp}
	}

	matchedTriggers := detectPreferenceTriggers(message)
	if len(matchedTriggers) > 0 {
		return PreFilterResult{
			HasPreferenceTrigger: true,
			MatchedTriggers:      matchedTriggers,
			Skip:                 false,
			SkipReason:           SkipReasonNone,
		}
	}

	if reason := detectSkipReason(message); reason != SkipReasonNone {
		return PreFilterResult{
			Skip:       true,
			SkipReason: reason,
		}
	}

	return PreFilterResult{
		Skip:       false,
		SkipReason: SkipReasonNone,
	}
}

func detectPreferenceTriggers(message string) []string {
	matched := make([]string, 0, len(preferenceTriggerSignals))
	for _, signal := range preferenceTriggerSignals {
		if strings.Contains(message, signal) {
			matched = append(matched, signal)
		}
	}
	return matched
}

func detectSkipReason(message string) string {
	switch {
	case looksLikeAlgorithmQuestion(message):
		return SkipReasonOneOffAlgorithm
	case looksLikeTransientError(message):
		return SkipReasonTransientError
	case looksLikeTemporaryCommand(message):
		return SkipReasonTemporaryCommand
	case looksLikeGreeting(message):
		return SkipReasonGreeting
	case looksLikeTranslation(message):
		return SkipReasonTranslation
	case looksLikeCalculation(message):
		return SkipReasonCalculation
	case looksLikeShortFollowUp(message):
		return SkipReasonShortFollowUp
	default:
		return SkipReasonNone
	}
}

func looksLikeAlgorithmQuestion(message string) bool {
	markers := []string{
		"算法题",
		"leetcode",
		"给定一个",
		"两数之和",
		"最大子序和",
		"反转链表",
	}
	for _, marker := range markers {
		if strings.Contains(strings.ToLower(message), strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func looksLikeTransientError(message string) bool {
	lower := strings.ToLower(message)
	markers := []string{
		"报错",
		"panic:",
		"exception",
		"traceback",
		"nil pointer",
		"error:",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeTemporaryCommand(message string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(message))
	for _, prefix := range commandPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return strings.Contains(message, "临时命令")
}

func looksLikeGreeting(message string) bool {
	trimmed := strings.TrimSpace(message)
	if utf8.RuneCountInString(trimmed) > maxShortMessageRuneLength {
		return false
	}
	greetings := []string{"你好", "您好", "hello", "hi", "嗨", "早上好", "晚上好"}
	for _, greeting := range greetings {
		if strings.Contains(strings.ToLower(trimmed), strings.ToLower(greeting)) {
			return true
		}
	}
	return false
}

func looksLikeTranslation(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(message, "翻译") || strings.Contains(lower, "translate")
}

func looksLikeCalculation(message string) bool {
	return strings.Contains(message, "计算") || strings.Contains(message, "等于多少") || calculationPattern.MatchString(message)
}

func looksLikeShortFollowUp(message string) bool {
	trimmed := strings.TrimSpace(message)
	if utf8.RuneCountInString(trimmed) > maxShortMessageRuneLength {
		return false
	}
	followUps := []string{
		"然后呢",
		"还有呢",
		"为什么",
		"继续",
		"展开说说",
	}
	for _, followUp := range followUps {
		if strings.Contains(trimmed, followUp) {
			return true
		}
	}
	return false
}
