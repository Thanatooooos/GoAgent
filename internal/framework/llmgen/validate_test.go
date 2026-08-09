package llmgen

import "testing"

func TestRefValidatorValidateHitAndMiss(t *testing.T) {
	v := NewRefValidator([]string{"chunk-a", " chunk-b "})
	if id, reason := v.Validate("chunk-a"); id != "chunk-a" || reason != ReasonOK {
		t.Fatalf("validate chunk-a = (%q,%q)", id, reason)
	}
	if id, reason := v.Validate("chunk-b"); id != "chunk-b" || reason != ReasonOK {
		t.Fatalf("validate chunk-b (trimmed candidate) = (%q,%q)", id, reason)
	}
	if _, reason := v.Validate("chunk-zzz"); reason != ReasonNotInCandidateSet {
		t.Fatalf("validate unknown = reason %q, want not_in_candidate_set", reason)
	}
	if _, reason := v.Validate("   "); reason != ReasonEmpty {
		t.Fatalf("validate blank = reason %q, want empty", reason)
	}
}

func TestRefValidatorValidateMany(t *testing.T) {
	v := NewRefValidator([]string{"chunk-a"})
	results := v.ValidateMany([]string{"chunk-a", "nope", ""})
	if len(results) != 3 {
		t.Fatalf("ValidateMany length = %d", len(results))
	}
	if results[0].Reason != ReasonOK || results[0].ID != "chunk-a" {
		t.Fatalf("results[0] = %+v", results[0])
	}
	if results[1].Reason != ReasonNotInCandidateSet || results[1].ID != "" {
		t.Fatalf("results[1] = %+v", results[1])
	}
	if results[2].Reason != ReasonEmpty {
		t.Fatalf("results[2] = %+v", results[2])
	}
}

func TestRefValidatorValidateNilReceiver(t *testing.T) {
	var v *RefValidator
	if _, reason := v.Validate("chunk-a"); reason != ReasonMalformed {
		t.Fatalf("nil receiver validate = reason %q, want malformed", reason)
	}
}

func TestRefValidatorTrimsRefAndPreservesOriginal(t *testing.T) {
	v := NewRefValidator([]string{"chunk-a"})
	results := v.ValidateMany([]string{" chunk-a "})
	if len(results) != 1 {
		t.Fatalf("ValidateMany length = %d", len(results))
	}
	if results[0].Ref != " chunk-a " {
		t.Fatalf("RefResult.Ref should preserve original input, got %q", results[0].Ref)
	}
	if results[0].Reason != ReasonOK || results[0].ID != "chunk-a" {
		t.Fatalf("results[0] = %+v", results[0])
	}
}
