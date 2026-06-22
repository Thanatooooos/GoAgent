package evaluation

import "testing"

func TestParseSuiteName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    SuiteName
		wantErr bool
	}{
		{name: "summary", input: "summary", want: SuiteSummary},
		{name: "rewrite", input: "rewrite", want: SuiteRewrite},
		{name: "all", input: "all", want: SuiteAll},
		{name: "trim and lowercase", input: " Summary ", want: SuiteSummary},
		{name: "invalid", input: "tool", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSuiteName(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseSuiteName(%q) expected error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSuiteName(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ParseSuiteName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSuiteNameKinds(t *testing.T) {
	if !SuiteSummary.IsExecutableSuite() {
		t.Fatal("SuiteSummary should be executable")
	}
	if !SuiteRewrite.IsExecutableSuite() {
		t.Fatal("SuiteRewrite should be executable")
	}
	if SuiteAll.IsExecutableSuite() {
		t.Fatal("SuiteAll should not be executable")
	}
	if !SuiteAll.IsAggregateSuite() {
		t.Fatal("SuiteAll should be aggregate")
	}
	if SuiteSummary.IsAggregateSuite() {
		t.Fatal("SuiteSummary should not be aggregate")
	}
}
