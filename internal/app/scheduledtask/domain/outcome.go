package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Signal string

const (
	SignalReport    Signal = "report"
	SignalNoReport  Signal = "no_report"
	SignalUncertain Signal = "uncertain"
)

type Outcome struct {
	Signal   Signal          `json:"signal"`
	Body     string          `json:"body,omitempty"`
	Reason   string          `json:"reason,omitempty"`
	Sources  []string        `json:"sources,omitempty"`
	Artifact json.RawMessage `json:"artifact,omitempty"`
}

func ParseOutcome(raw string) (Outcome, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result Outcome
	if err := decoder.Decode(&result); err != nil {
		return Outcome{}, fmt.Errorf("decode task outcome: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Outcome{}, fmt.Errorf("task outcome contains trailing data")
	}
	result.Body = strings.TrimSpace(result.Body)
	result.Reason = strings.TrimSpace(result.Reason)
	for i := range result.Sources {
		result.Sources[i] = strings.TrimSpace(result.Sources[i])
	}
	switch result.Signal {
	case SignalReport:
		if result.Body == "" {
			return Outcome{}, fmt.Errorf("report requires body")
		}
	case SignalNoReport:
	case SignalUncertain:
		if result.Reason == "" {
			return Outcome{}, fmt.Errorf("uncertain requires reason")
		}
	default:
		return Outcome{}, fmt.Errorf("invalid task signal %q", result.Signal)
	}
	return result, nil
}

func EncodeOutcome(outcome Outcome) (string, error) {
	raw, err := json.Marshal(outcome)
	if err != nil {
		return "", err
	}
	if _, err := ParseOutcome(string(raw)); err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(raw)), nil
}
