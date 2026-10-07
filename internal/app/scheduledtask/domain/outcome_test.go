package domain

import "testing"

func TestParseOutcome(t *testing.T) {
	cases := []struct {
		input string
		ok    bool
	}{
		{`{"signal":"report","body":"比赛延期","sources":["https://example.com"]}`, true},
		{`{"signal":"no_report"}`, true},
		{`{"signal":"uncertain","reason":"官网不可用"}`, true},
		{`{"signal":"report"}`, false},
		{`{"signal":"uncertain"}`, false},
		{`{"signal":"other","body":"x"}`, false},
		{`{"signal":"no_report"} {"signal":"report"}`, false},
		{`{"signal":"no_report"} garbage`, false},
		{`{"signal":"no_report","unexpected":true}`, false},
	}
	for _, tc := range cases {
		_, err := ParseOutcome(tc.input)
		if (err == nil) != tc.ok {
			t.Errorf("ParseOutcome(%q) error = %v; want ok %v", tc.input, err, tc.ok)
		}
	}
}
